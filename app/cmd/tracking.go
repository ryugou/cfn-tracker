package cmd

import (
	"context"
	dbsql "database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/williamsjokvist/cfn-tracker/pkg/config"
	"github.com/williamsjokvist/cfn-tracker/pkg/model"
	cfgDb "github.com/williamsjokvist/cfn-tracker/pkg/storage/config"
	"github.com/williamsjokvist/cfn-tracker/pkg/storage/sql"
	"github.com/williamsjokvist/cfn-tracker/pkg/storage/txt"
	"github.com/williamsjokvist/cfn-tracker/pkg/tracker"
	"github.com/williamsjokvist/cfn-tracker/pkg/tracker/sf6"
	"github.com/williamsjokvist/cfn-tracker/pkg/tracker/sf6/cfn"
	"github.com/williamsjokvist/cfn-tracker/pkg/tracker/t8"
	"github.com/williamsjokvist/cfn-tracker/pkg/tracker/t8/wavu"
)

type EventEmitFn func(eventName string, optionalData ...interface{})

type TrackingHandler struct {
	sqlDb   *sql.Storage
	nosqlDb *cfgDb.Storage
	txtDb   *txt.Storage

	wavuClient wavu.WavuClient
	cfnClient  cfn.CFNClient

	cfg        *config.BuildConfig
	matchChans []chan model.Match

	cancelPolling context.CancelFunc
	forcePollChan chan struct{}
	gameTracker   tracker.GameTracker
	eventEmitter  EventEmitFn
}

func NewTrackingHandler(
	wavuClient wavu.WavuClient,
	cfnClient cfn.CFNClient,
	sqlDb *sql.Storage,
	nosqlDb *cfgDb.Storage,
	txtDb *txt.Storage,
	cfg *config.BuildConfig,
	matchChans ...chan model.Match,
) *TrackingHandler {
	return &TrackingHandler{
		wavuClient: wavuClient,
		cfnClient:  cfnClient,
		sqlDb:      sqlDb,
		nosqlDb:    nosqlDb,
		txtDb:      txtDb,
		cfg:        cfg,
		matchChans: matchChans,
	}
}

func (ch *TrackingHandler) SetEventEmitter(eventEmitter EventEmitFn) {
	ch.eventEmitter = eventEmitter
}

func (ch *TrackingHandler) StartTracking(userCodeInput string, restore bool) error {
	slog.Info("started tracking", slog.String("user_code", userCodeInput), slog.Bool("restoring", restore))

	ctx, cancel := context.WithCancel(context.Background())
	ch.cancelPolling = cancel

	user, err := ch.gameTracker.GetUser(ctx, userCodeInput)
	if err != nil {
		return model.WrapError(model.ErrGetUser, err)
	}
	if err := ch.sqlDb.SaveUser(ctx, *user); err != nil {
		return model.WrapError(model.ErrSaveUser, err)
	}

	var session *model.Session
	if restore {
		sesh, err := ch.sqlDb.GetLatestSession(ctx, user.Code)
		if err != nil {
			return model.WrapError(model.ErrGetLatestSession, err)
		}
		session = sesh
	} else {
		sesh, err := ch.sqlDb.CreateSession(ctx, user.Code)
		if err != nil {
			return model.WrapError(model.ErrCreateSession, err)
		}
		session = sesh
	}
	if session == nil {
		return model.ErrCreateSession
	}

	session.LP = user.LP
	session.MR = user.MR
	session.UserName = user.DisplayName

	// Baseline play stats snapshot (SF6 only). Skip if a recent snapshot
	// already exists for the same (user, character) within 30 minutes —
	// repeated stop/restart should not pile up empty match_replay_id NULL rows.
	if sf6Tracker, ok := ch.gameTracker.(*sf6.SF6Tracker); ok {
		if res, err := sf6Tracker.PollPlayStats(ctx, user.Code); err == nil {
			recent, _ := ch.sqlDb.GetPlayStatsHistory(
				ctx, user.Code, res.Character,
				time.Now().Add(-30*time.Minute).Format("2006-01-02"),
				"", 0,
			)
			shouldSave := true
			// Results are ORDER BY snapshot_at ASC, so the last element is the newest.
			// Iterate from newest backwards in case any rows have an unparseable timestamp.
			for i := len(recent) - 1; i >= 0; i-- {
				parsed, parseErr := time.Parse("2006-01-02 15:04:05", recent[i].SnapshotAt)
				if parseErr != nil {
					continue
				}
				if time.Since(parsed) < 30*time.Minute {
					shouldSave = false
				}
				// Once we've inspected the newest parseable row we have the answer; older
				// rows can only be older, so they can't improve the dedup decision.
				break
			}
			if shouldSave {
				snap := buildSnapshot(user.Code, res, "")
				if err := ch.sqlDb.SavePlayStats(ctx, snap); err != nil {
					slog.Warn("save baseline play stats failed", slog.Any("error", err))
				}
			}
		} else {
			slog.Warn("baseline play stats fetch failed", slog.Any("error", err))
		}
	}

	// Placeholder first so the UI has username + LP rendered immediately.
	// Backfill follows; its per-match emits override the placeholder, and
	// the last backfilled match leaves the UI in the correct cumulative state.
	ch.eventEmitter("match", model.Match{
		UserName:  session.UserName,
		LP:        session.LP,
		MR:        session.MR,
		SessionId: session.Id,
		UserId:    session.UserId,
	})

	backfilled := ch.backfillSf6(ctx, session)

	ticker := time.NewTicker(30 * time.Second)
	ch.forcePollChan = make(chan struct{})
	defer func() {
		ch.eventEmitter("stopped-tracking")
		ticker.Stop()
		cancel()
		close(ch.forcePollChan)
		ch.forcePollChan = nil
	}()

	matchChan := make(chan model.Match)

	onNewMatch := func(match *model.Match) {
		if match == nil {
			return
		}
		matchChan <- *match
		for _, mc := range ch.matchChans {
			if mc != nil {
				mc <- *match
			}
		}
	}

	// Re-emit the latest known match so the UI repaints session state. Skip when
	// backfill just emitted matches: session.Matches[0] is the same match that
	// was already pushed to the GUI via the backfill loop.
	if !backfilled && len(session.Matches) > 0 {
		match := *session.Matches[0]
		ch.eventEmitter("match", match)
		for _, mc := range ch.matchChans {
			if mc != nil {
				mc <- match
			}
		}
	}

	// If this is a brand-new session AND backfill imported nothing
	// (because everything was already saved in a previous session),
	// re-emit the user's most recent match so the tracking-page summary
	// reflects historical wins/losses/streak/LP instead of all zeros.
	if len(session.Matches) == 0 {
		if last, err := ch.sqlDb.GetLatestMatchForUser(ctx, user.Code); err != nil {
			slog.Warn("fetch latest historical match failed", slog.Any("error", err))
		} else if last != nil {
			session.LP = last.LP
			session.MR = last.MR
			ch.eventEmitter("match", *last)
			for _, mc := range ch.matchChans {
				if mc != nil {
					mc <- *last
				}
			}
		}
	}

	go func() {
		slog.Debug("polling")
		match, err := ch.gameTracker.Poll(ctx, session)
		if err != nil {
			cancel()
			return
		}
		onNewMatch(match)
		for {
			select {
			case <-ch.forcePollChan:
				slog.Info("forced poll")
				match, err := ch.gameTracker.Poll(ctx, session)
				if err != nil {
					cancel()
					return
				}
				onNewMatch(match)
			case <-ticker.C:
				slog.Debug("polling")
				match, err := ch.gameTracker.Poll(ctx, session)
				if err != nil {
					cancel()
					return
				}
				onNewMatch(match)
			case <-ctx.Done():
				close(matchChan)
				return
			}
		}
	}()

	for match := range matchChan {
		ch.eventEmitter("match", match)

		session.LP = match.LP
		session.MR = match.MR
		session.Matches = append([]*model.Match{&match}, session.Matches...)

		if err := ch.sqlDb.UpdateSession(ctx, session); err != nil {
			slog.Error("update session:", slog.Any("error", err))
			break
		}
		if err := ch.sqlDb.SaveMatch(ctx, match); err != nil {
			slog.Error("save match to database", slog.Any("error", err))
			break
		}
		if err := ch.txtDb.SaveMatch(match); err != nil {
			slog.Error("save to text files:", slog.Any("error", err))
			break
		}
		// Per-match play stats snapshot (SF6 only, best-effort)
		ch.playStatsSf6(ctx, session.UserId, match.ReplayID)
	}
	return nil
}

func (ch *TrackingHandler) StopTracking() {
	ch.cancelPolling()
}

func (ch *TrackingHandler) SelectGame(game model.GameType) error {
	var username, password string
	switch game {
	case model.GameTypeT8:
		ch.gameTracker = t8.NewT8Tracker(ch.wavuClient)
	case model.GameTypeSF6:
		ch.gameTracker = sf6.NewSF6Tracker(ch.cfnClient)
		username = ch.cfg.CapIDEmail
		password = ch.cfg.CapIDPassword
	default:
		return model.WrapError(model.ErrSelectGame, fmt.Errorf("game does not exist"))
	}

	authChan := make(chan tracker.AuthStatus)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	go ch.gameTracker.Authenticate(ctx, username, password, authChan)
	for status := range authChan {
		if status.Err != nil {
			return model.WrapError(model.ErrAuth, status.Err)
		}

		ch.eventEmitter("auth-progress", status.Progress)

		if status.Progress >= 100 {
			close(authChan)
			break
		}
	}
	return nil
}

func (ch *TrackingHandler) ForcePoll() {
	if ch.forcePollChan != nil {
		ch.forcePollChan <- struct{}{}
	}
}

// backfillSf6 imports past ranked matches from Capcom that aren't already in
// our matches table, in chronological order. Best-effort: warns and continues
// on any failure to keep the rest of tracking unaffected. Returns true when at
// least one match was emitted so the caller can skip a redundant latest-match
// re-emit.
func (ch *TrackingHandler) backfillSf6(ctx context.Context, session *model.Session) bool {
	sf6Tracker, ok := ch.gameTracker.(*sf6.SF6Tracker)
	if !ok {
		return false
	}
	known, err := ch.sqlDb.GetMatchReplayIDsForUser(ctx, session.UserId)
	if err != nil {
		slog.Warn("backfill: known replays lookup failed", slog.Any("error", err))
		return false
	}
	imported, err := sf6Tracker.BackfillMatches(ctx, session, known)
	if err != nil {
		slog.Warn("backfill: fetch failed", slog.Any("error", err))
		return false
	}
	if len(imported) == 0 {
		return false
	}
	slog.Info("backfill: importing past matches", slog.Int("count", len(imported)))
	emitted := false

	// Walk chronologically (oldest → newest); piggyback on the same fan-out logic
	// the live loop uses so wins/losses/streak are computed per character and the
	// UI gets a match event for each.
	for i := range imported {
		match := imported[i]
		prev := getPreviousMatchForCharacterInSession(session, match.Character)
		match = ApplyRunningCounters(match, prev)

		// Persist the match before mutating any in-memory or downstream state
		// so memory, DB, and UI never diverge. Inspect SaveMatchIfNew because
		// the matches PK is (session_id, date, time) and our timestamps round to
		// HH:MM — two replays in the same minute within one session would be
		// silently ignored by INSERT OR IGNORE, leaving the in-memory session
		// ahead of the DB.
		inserted, err := ch.sqlDb.SaveMatchIfNew(ctx, match)
		if err != nil {
			slog.Warn("backfill: save match failed", slog.Any("error", err))
			break
		}
		if !inserted {
			slog.Warn(
				"backfill: skipping match — primary key collision (session_id, date, time)",
				slog.String("replay_id", match.ReplayID),
				slog.String("date", match.Date),
				slog.String("time", match.Time),
			)
			continue
		}
		// Try to persist the session next. If this fails, the match row is in
		// the DB but the session aggregate is stale — preserve the DB ordering
		// by mutating the in-memory session only after UpdateSession succeeds.
		nextSession := *session
		nextSession.LP = match.LP
		nextSession.MR = match.MR
		nextSession.Matches = append([]*model.Match{&match}, session.Matches...)
		if err := ch.sqlDb.UpdateSession(ctx, &nextSession); err != nil {
			slog.Warn("backfill: update session failed", slog.Any("error", err))
			break
		}
		*session = nextSession

		if err := ch.txtDb.SaveMatch(match); err != nil {
			slog.Warn("backfill: save text files failed", slog.Any("error", err))
			// keep going; text-file failure isn't fatal
		}
		// Note: we intentionally do NOT call playStatsSf6 here. During backfill
		// every imported match would attach an identical "current /play" snapshot
		// (Capcom only returns current data, not historical), creating dozens of
		// duplicate-value rows. The baseline snapshot already captured the state
		// at tracking start; per-match snapshots only add information during live
		// polling where the 100-match window actually shifts.
		// notify the GUI in real time
		ch.eventEmitter("match", match)
		emitted = true
	}
	return emitted
}

// ApplyRunningCounters fills in Wins/Losses/WinStreak/WinRate/LPGain/MRGain
// on match by carrying forward prev's counters and incrementing based on
// match.Victory. Pure function — no DB or state side-effects.
//
// Example progression illustrating the carry-forward requirement:
//
//	win  -> wins=1, losses=0, streak=1
//	lose -> wins=1, losses=1, streak=0  (wins MUST carry forward, not reset)
//	lose -> wins=1, losses=2, streak=0
//	win  -> wins=2, losses=2, streak=1  (losses MUST carry forward, not reset)
//
// Earlier the backfill loop set only the incremented side, leaving the
// other counter at the Match zero value and silently resetting the
// running total every alternate result. Mirror the live SF6Tracker.Poll
// pattern: copy both prev counters first, then increment only the
// relevant one.
func ApplyRunningCounters(match, prev model.Match) model.Match {
	match.Wins = prev.Wins
	match.Losses = prev.Losses
	match.WinStreak = prev.WinStreak
	if match.Victory {
		match.Wins++
		match.WinStreak++
	} else {
		match.Losses++
		match.WinStreak = 0
	}
	total := match.Wins + match.Losses
	if total > 0 {
		match.WinRate = int((float64(match.Wins) / float64(total)) * 100)
	}
	// LP/MR gain: relative to the prior match for same character, or 0 if first
	if prev.ReplayID != "" {
		match.LPGain = prev.LPGain + (match.LP - prev.LP)
		match.MRGain = prev.MRGain + (match.MR - prev.MR)
	}
	return match
}

// getPreviousMatchForCharacterInSession finds the most-recent saved match in
// the running session for the given character. Returns zero-value Match if
// none exists yet.
func getPreviousMatchForCharacterInSession(session *model.Session, character string) model.Match {
	for _, m := range session.Matches {
		if m != nil && m.Character == character {
			return *m
		}
	}
	return model.Match{}
}

// playStatsSf6 captures a /play snapshot and saves it to SQLite. Returns
// nil error on success; warnings are emitted via slog on failure but never
// propagate so the surrounding match flow stays alive.
func (ch *TrackingHandler) playStatsSf6(
	ctx context.Context,
	userCode string,
	matchReplayId string,
) {
	sf6Tracker, ok := ch.gameTracker.(*sf6.SF6Tracker)
	if !ok {
		return
	}
	res, err := sf6Tracker.PollPlayStats(ctx, userCode)
	if err != nil {
		slog.Warn("get play stats failed, skipping snapshot", slog.Any("error", err))
		return
	}
	snap := buildSnapshot(userCode, res, matchReplayId)
	if err := ch.sqlDb.SavePlayStats(ctx, snap); err != nil {
		slog.Warn("save play stats failed", slog.Any("error", err))
	}
}

func buildSnapshot(userCode string, res *sf6.PlayStatsResult, matchReplayId string) model.PlayStatsSnapshot {
	snap := model.PlayStatsSnapshot{
		UserId:    userCode,
		Character: res.Character,
	}
	if matchReplayId != "" {
		snap.MatchReplayId = dbsql.NullString{String: matchReplayId, Valid: true}
	}
	if res.Stats != nil {
		bs := res.Stats
		snap.BattleHubMatchPlayCount = bs.BattleHubMatchPlayCount
		snap.CasualMatchPlayCount = bs.CasualMatchPlayCount
		snap.CornerTime = bs.CornerTime
		snap.CorneredTime = bs.CorneredTime
		snap.CustomRoomMatchPlayCount = bs.CustomRoomMatchPlayCount
		snap.DriveImpact = bs.DriveImpact
		snap.DriveImpactToDriveImpact = bs.DriveImpactToDriveImpact
		snap.DriveParry = bs.DriveParry
		snap.DriveReversal = bs.DriveReversal
		snap.GaugeRateCA = bs.GaugeRateCA
		snap.GaugeRateDriveArts = bs.GaugeRateDriveArts
		snap.GaugeRateDriveGuard = bs.GaugeRateDriveGuard
		snap.GaugeRateDriveImpact = bs.GaugeRateDriveImpact
		snap.GaugeRateDriveOther = bs.GaugeRateDriveOther
		snap.GaugeRateDriveReversal = bs.GaugeRateDriveReversal
		snap.GaugeRateDriveRushFromCancel = bs.GaugeRateDriveRushFromCancel
		snap.GaugeRateDriveRushFromParry = bs.GaugeRateDriveRushFromParry
		snap.GaugeRateSALv1 = bs.GaugeRateSALv1
		snap.GaugeRateSALv2 = bs.GaugeRateSALv2
		snap.GaugeRateSALv3 = bs.GaugeRateSALv3
		snap.JustParry = bs.JustParry
		snap.PunishCounter = bs.PunishCounter
		snap.RankMatchPlayCount = bs.RankMatchPlayCount
		snap.ReceivedDriveImpact = bs.ReceivedDriveImpact
		snap.ReceivedDriveImpactToDriveImpact = bs.ReceivedDriveImpactToDriveImpact
		snap.ReceivedPunishCounter = bs.ReceivedPunishCounter
		snap.ReceivedStun = bs.ReceivedStun
		snap.ReceivedThrowCount = bs.ReceivedThrowCount
		snap.ReceivedThrowDriveParry = bs.ReceivedThrowDriveParry
		snap.RivalAIAchievedChallengeCount = bs.RivalAIAchievedChallengeCount
		snap.RivalAIHighestLeagueRank = bs.RivalAIHighestLeagueRank
		snap.RivalAIHighestLeagueRankTxt = bs.RivalAIHighestLeagueRankTxt
		snap.Stun = bs.Stun
		snap.TargetClearCount = bs.TargetClearCount
		snap.ThrowCount = bs.ThrowCount
		snap.ThrowDriveParry = bs.ThrowDriveParry
		snap.ThrowTech = bs.ThrowTech
		snap.TotalAllCharacterPlayPoint = bs.TotalAllCharacterPlayPoint
	}
	if res.BaseInfo != nil {
		bi := res.BaseInfo
		snap.EnjoyFightPoint = bi.EnjoyFightPoint
		snap.EnjoyTotalPoint = bi.EnjoyTotalPoint
		snap.EnjoyUserPoint = bi.EnjoyUserPoint
		for _, cpt := range bi.ContentPlayTimeList {
			switch cpt.ContentType {
			case cfn.ContentTypeWorldTour:
				snap.WorldTourSeconds = cpt.PlayTime
			case cfn.ContentTypeRankedMatch:
				snap.RankedMatchSeconds = cpt.PlayTime
			case cfn.ContentTypeCasualMatch:
				snap.CasualMatchSeconds = cpt.PlayTime
			case cfn.ContentTypeCustomRoom:
				snap.CustomRoomSeconds = cpt.PlayTime
			case cfn.ContentTypeBattleHub:
				snap.BattleHubSeconds = cpt.PlayTime
			case cfn.ContentTypeOfflineMatch:
				snap.OfflineMatchSeconds = cpt.PlayTime
			case cfn.ContentTypeArcade:
				snap.ArcadeSeconds = cpt.PlayTime
			case cfn.ContentTypePractice:
				snap.PracticeSeconds = cpt.PlayTime
			case cfn.ContentTypeExtreme:
				snap.ExtremeSeconds = cpt.PlayTime
			}
		}
	}
	return snap
}
