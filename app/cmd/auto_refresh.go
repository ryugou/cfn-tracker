package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/williamsjokvist/cfn-tracker/pkg/model"
	"github.com/williamsjokvist/cfn-tracker/pkg/tracker/sf6"
)

const (
	matchRefreshInterval         = 3 * time.Minute
	matchRefreshInitialDelay     = 3 * time.Minute
	matchRefreshUserDelay        = 3 * time.Minute
	playStatsRefreshAge          = time.Hour
	playStatsRefreshInterval     = time.Hour
	playStatsRefreshInitialDelay = time.Minute
	playStatsRefreshUserDelay    = 30 * time.Second
	benchmarkRefreshAge          = 24 * time.Hour
	benchmarkRefreshInterval     = time.Hour
	benchmarkRefreshInitialDelay = 2 * time.Minute
	benchmarkRefreshJobDelay     = 2 * time.Minute
)

func StartAutoDataRefresh(ctx context.Context, ch *CommandHandler) {
	if ch == nil || ch.sqlDb == nil {
		return
	}
	go ch.runMatchRefreshLoop(ctx)
	go ch.runPlayStatsRefreshLoop(ctx)
	go ch.runBenchmarkRefreshLoop(ctx)
}

func (ch *CommandHandler) runMatchRefreshLoop(ctx context.Context) {
	if !sleepWithContext(ctx, matchRefreshInitialDelay) {
		return
	}
	ch.refreshMissingMatches(ctx)
	ticker := time.NewTicker(matchRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			ch.refreshMissingMatches(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (ch *CommandHandler) runBenchmarkRefreshLoop(ctx context.Context) {
	if !sleepWithContext(ctx, benchmarkRefreshInitialDelay) {
		return
	}
	ch.refreshStaleBenchmarks(ctx)
	ticker := time.NewTicker(benchmarkRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			ch.refreshStaleBenchmarks(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (ch *CommandHandler) runPlayStatsRefreshLoop(ctx context.Context) {
	if !sleepWithContext(ctx, playStatsRefreshInitialDelay) {
		return
	}
	ch.refreshStalePlayStats(ctx)
	ticker := time.NewTicker(playStatsRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			ch.refreshStalePlayStats(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (ch *CommandHandler) refreshStalePlayStats(ctx context.Context) {
	if ch.cfnClient == nil {
		return
	}
	users, err := ch.sqlDb.GetUsers(ctx)
	if err != nil {
		slog.Warn("auto play stats user lookup failed", slog.Any("error", err))
		return
	}
	for i, user := range users {
		if user == nil || user.Code == "" || ctx.Err() != nil {
			continue
		}
		latest, err := ch.sqlDb.GetLatestPlayStatsSnapshot(ctx, user.Code)
		if err != nil {
			slog.Warn("auto play stats latest lookup failed", slog.String("user_id", user.Code), slog.Any("error", err))
			continue
		}
		if latestPlayStatsFresh(latest, time.Now(), playStatsRefreshAge) {
			continue
		}
		pp, err := ch.cfnClient.GetPlayStats(ctx, user.Code)
		if err != nil {
			slog.Warn("auto play stats refresh failed", slog.String("user_id", user.Code), slog.Any("error", err))
			continue
		}
		snap := buildSnapshot(user.Code, &sf6.PlayStatsResult{
			Character: pp.FighterBannerInfo.FavoriteCharacterName,
			Stats:     &pp.Play.BattleStats,
			BaseInfo:  &pp.Play.BaseInfo,
		}, "")
		if err := ch.sqlDb.SavePlayStats(ctx, snap); err != nil {
			slog.Warn("auto play stats save failed", slog.String("user_id", user.Code), slog.Any("error", err))
			continue
		}
		go syncLatestPlayStatsToVegapunkDB(context.Background(), ch.sqlDb, user.Code)
		slog.Info("auto play stats refreshed", slog.String("user_id", user.Code), slog.String("character", snap.Character))
		if i < len(users)-1 && !sleepWithContext(ctx, playStatsRefreshUserDelay) {
			return
		}
	}
}

func (ch *CommandHandler) refreshMissingMatches(ctx context.Context) {
	if ch.cfnClient == nil {
		return
	}
	users, err := ch.sqlDb.GetUsers(ctx)
	if err != nil {
		slog.Warn("auto match user lookup failed", slog.Any("error", err))
		return
	}
	sf6Tracker := sf6.NewSF6Tracker(ch.cfnClient)
	for i, user := range users {
		if user == nil || user.Code == "" || ctx.Err() != nil {
			continue
		}
		imported, err := ch.refreshMissingMatchesForUser(ctx, sf6Tracker, user.Code)
		if err != nil {
			slog.Warn("auto match refresh failed", slog.String("user_id", user.Code), slog.Any("error", err))
		} else if imported > 0 {
			slog.Info("auto matches refreshed", slog.String("user_id", user.Code), slog.Int("count", imported))
		}
		if i < len(users)-1 && !sleepWithContext(ctx, matchRefreshUserDelay) {
			return
		}
	}
}

func (ch *CommandHandler) refreshMissingMatchesForUser(ctx context.Context, sf6Tracker *sf6.SF6Tracker, userId string) (int, error) {
	session, err := ch.sqlDb.GetLatestSession(ctx, userId)
	if err != nil {
		return 0, fmt.Errorf("get latest session: %w", err)
	}
	if session == nil {
		session, err = ch.sqlDb.CreateSession(ctx, userId)
		if err != nil {
			return 0, fmt.Errorf("create session: %w", err)
		}
	}
	known, err := ch.sqlDb.GetMatchReplayIDsForUser(ctx, userId)
	if err != nil {
		return 0, fmt.Errorf("known replays lookup: %w", err)
	}
	imported, err := sf6Tracker.BackfillMatches(ctx, session, known)
	if err != nil {
		return 0, fmt.Errorf("fetch battlelog: %w", err)
	}
	if len(imported) == 0 {
		return 0, nil
	}

	inserted := 0
	latestInsertedReplayID := ""
	for i := range imported {
		match := imported[i]
		prev := getPreviousMatchForCharacterInSession(session, match.Character)
		match = applyRunningCounters(match, prev)

		ok, err := ch.sqlDb.SaveMatchIfNew(ctx, match)
		if err != nil {
			return inserted, fmt.Errorf("save match: %w", err)
		}
		if !ok {
			slog.Warn(
				"auto match refresh: skipping match — primary key collision (session_id, date, time)",
				slog.String("replay_id", match.ReplayID),
				slog.String("date", match.Date),
				slog.String("time", match.Time),
			)
			continue
		}
		syncMatchToVegapunkDB(context.Background(), ch.sqlDb, match)

		nextSession := *session
		nextSession.LP = match.LP
		nextSession.MR = match.MR
		nextSession.Matches = append([]*model.Match{&match}, session.Matches...)
		if err := ch.sqlDb.UpdateSession(ctx, &nextSession); err != nil {
			return inserted, fmt.Errorf("update session: %w", err)
		}
		*session = nextSession

		if ch.txtDb != nil {
			if err := ch.txtDb.SaveMatch(match); err != nil {
				slog.Warn("auto match refresh: save text files failed", slog.Any("error", err))
			}
		}
		if ch.EventEmitter != nil {
			ch.EventEmitter("match", match)
		}
		latestInsertedReplayID = match.ReplayID
		inserted++
	}
	if latestInsertedReplayID != "" {
		if err := ch.refreshPlayStatsForReplay(ctx, userId, latestInsertedReplayID); err != nil {
			slog.Warn("auto match refresh: play stats snapshot failed", slog.String("user_id", userId), slog.String("replay_id", latestInsertedReplayID), slog.Any("error", err))
		}
	}
	return inserted, nil
}

func (ch *CommandHandler) refreshPlayStatsForReplay(ctx context.Context, userId, replayId string) error {
	pp, err := ch.cfnClient.GetPlayStats(ctx, userId)
	if err != nil {
		return fmt.Errorf("get play stats: %w", err)
	}
	snap := buildSnapshot(userId, &sf6.PlayStatsResult{
		Character: pp.FighterBannerInfo.FavoriteCharacterName,
		Stats:     &pp.Play.BattleStats,
		BaseInfo:  &pp.Play.BaseInfo,
	}, replayId)
	if err := ch.sqlDb.SavePlayStats(ctx, snap); err != nil {
		return fmt.Errorf("save play stats: %w", err)
	}
	go syncLatestPlayStatsToVegapunkDB(context.Background(), ch.sqlDb, userId)
	return nil
}

func latestPlayStatsFresh(latest *model.PlayStatsSnapshot, now time.Time, maxAge time.Duration) bool {
	if latest == nil || latest.SnapshotAt == "" {
		return false
	}
	snapshotAt, err := parseBenchmarkRefreshTime(latest.SnapshotAt)
	if err != nil {
		return false
	}
	return now.Sub(snapshotAt) < maxAge
}

func (ch *CommandHandler) refreshStaleBenchmarks(ctx context.Context) {
	if ch.cfnClient == nil {
		return
	}
	targets, err := ch.sqlDb.GetBenchmarkRefreshTargets(ctx)
	if err != nil {
		slog.Warn("benchmark refresh target lookup failed", slog.Any("error", err))
		return
	}
	stale := staleBenchmarkTargets(targets, time.Now(), benchmarkRefreshAge)
	for i, target := range stale {
		if ctx.Err() != nil {
			return
		}
		slog.Info("auto refreshing benchmark players", slog.String("user_id", target.UserId), slog.String("character", target.Character))
		if _, err := ch.RefreshBenchmarkPlayers(target.UserId, target.Character); err != nil {
			slog.Warn("auto benchmark refresh failed", slog.String("user_id", target.UserId), slog.String("character", target.Character), slog.Any("error", err))
		}
		if i < len(stale)-1 && !sleepWithContext(ctx, benchmarkRefreshJobDelay) {
			return
		}
	}
}

func staleBenchmarkTargets(targets []model.BenchmarkRefreshTarget, now time.Time, maxAge time.Duration) []model.BenchmarkRefreshTarget {
	out := make([]model.BenchmarkRefreshTarget, 0, len(targets))
	for _, target := range targets {
		if target.UserId == "" || target.Character == "" {
			continue
		}
		if target.FetchedAt == "" {
			out = append(out, target)
			continue
		}
		fetchedAt, err := parseBenchmarkRefreshTime(target.FetchedAt)
		if err != nil || now.Sub(fetchedAt) >= maxAge {
			out = append(out, target)
		}
	}
	return out
}

func parseBenchmarkRefreshTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
	} {
		t, err := time.ParseInLocation(layout, value, time.UTC)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time format %q", value)
}
