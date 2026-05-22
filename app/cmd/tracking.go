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

	ch.eventEmitter("match", model.Match{
		UserName:  session.UserName,
		LP:        session.LP,
		MR:        session.MR,
		SessionId: session.Id,
		UserId:    session.UserId,
	})

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

	if len(session.Matches) > 0 {
		match := *session.Matches[0]
		ch.eventEmitter("match", match)
		for _, mc := range ch.matchChans {
			if mc != nil {
				mc <- match
			}
		}
	}

	go func() {
		slog.Info("polling")
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
				slog.Info("polling")
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
