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
	sf6DataRefreshInterval       = 3 * time.Minute
	sf6DataRefreshInitialDelay   = 0
	sf6DataRefreshUserDelay      = 3 * time.Minute
	autoRefreshUserTimeout       = 2 * time.Minute
	playStatsRefreshAge          = time.Hour
	benchmarkRefreshAge          = 24 * time.Hour
	benchmarkRefreshInterval     = time.Hour
	benchmarkRefreshInitialDelay = 2 * time.Minute
	benchmarkRefreshJobDelay     = 2 * time.Minute
)

func StartAutoDataRefresh(ctx context.Context, ch *CommandHandler) {
	if ch == nil || ch.sqlDb == nil {
		return
	}
	go ch.runSF6DataRefreshLoop(ctx)
	go ch.runBenchmarkRefreshLoop(ctx)
}

func (ch *CommandHandler) runSF6DataRefreshLoop(ctx context.Context) {
	if !sleepWithContext(ctx, sf6DataRefreshInitialDelay) {
		return
	}
	ch.refreshSF6Data(ctx)
	ticker := time.NewTicker(sf6DataRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			ch.refreshSF6Data(ctx)
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

func (ch *CommandHandler) refreshSF6Data(ctx context.Context) {
	result, err := ch.refreshSF6DataForAllUsers(ctx)
	if err != nil {
		slog.Warn("auto sf6 data refresh failed", slog.Any("error", err))
	}
	if result.matches > 0 || result.playStats > 0 {
		slog.Info("auto sf6 data refreshed", slog.Int("matches", result.matches), slog.Int("play_stats", result.playStats))
	}
}

type sf6DataRefreshResult struct {
	matches   int
	playStats int
}

func (ch *CommandHandler) RefreshMissingMatchesNow() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	result, err := ch.refreshSF6DataForAllUsers(ctx)
	if err != nil {
		return result.matches, model.WrapError(model.ErrGetMatches, err)
	}
	return result.matches, nil
}

func (ch *CommandHandler) refreshSF6DataForAllUsers(ctx context.Context) (sf6DataRefreshResult, error) {
	if ch.cfnClient == nil {
		return sf6DataRefreshResult{}, nil
	}
	users, err := ch.sqlDb.GetUsers(ctx)
	if err != nil {
		return sf6DataRefreshResult{}, fmt.Errorf("auto sf6 data user lookup: %w", err)
	}
	sf6Tracker := sf6.NewSF6Tracker(ch.cfnClient)
	var result sf6DataRefreshResult
	var firstErr error
	for i, user := range users {
		if user == nil || user.Code == "" || ctx.Err() != nil {
			continue
		}
		userCtx, cancel := context.WithTimeout(ctx, autoRefreshUserTimeout)
		userResult, err := ch.refreshSF6DataForUser(userCtx, sf6Tracker, user.Code)
		cancel()
		if err != nil {
			slog.Warn("auto sf6 data refresh failed", slog.String("user_id", user.Code), slog.Any("error", err))
			if firstErr == nil {
				firstErr = fmt.Errorf("user %s: %w", user.Code, err)
			}
		} else {
			result.matches += userResult.matches
			result.playStats += userResult.playStats
		}
		if i < len(users)-1 && !sleepWithContext(ctx, sf6DataRefreshUserDelay) {
			return result, ctx.Err()
		}
	}
	return result, firstErr
}

func (ch *CommandHandler) refreshSF6DataForUser(ctx context.Context, sf6Tracker *sf6.SF6Tracker, userId string) (sf6DataRefreshResult, error) {
	imported, err := ch.refreshMissingMatchesForUser(ctx, sf6Tracker, userId)
	if err != nil {
		return sf6DataRefreshResult{}, err
	}

	refreshedStats, err := ch.refreshStalePlayStatsForUser(ctx, userId)
	if err != nil {
		return sf6DataRefreshResult{matches: imported}, err
	}
	result := sf6DataRefreshResult{matches: imported}
	if refreshedStats {
		result.playStats = 1
	}
	return result, nil
}

func (ch *CommandHandler) refreshStalePlayStatsForUser(ctx context.Context, userId string) (bool, error) {
	latest, err := ch.sqlDb.GetLatestPlayStatsSnapshot(ctx, userId)
	if err != nil {
		return false, fmt.Errorf("latest play stats lookup: %w", err)
	}
	if latestPlayStatsFresh(latest, time.Now(), playStatsRefreshAge) {
		return false, nil
	}
	pp, err := ch.cfnClient.GetPlayStats(ctx, userId)
	if err != nil {
		return false, fmt.Errorf("get play stats: %w", err)
	}
	snap := buildSnapshot(userId, &sf6.PlayStatsResult{
		Character: pp.FighterBannerInfo.FavoriteCharacterName,
		Stats:     &pp.Play.BattleStats,
		BaseInfo:  &pp.Play.BaseInfo,
	}, "")
	if err := ch.sqlDb.SavePlayStats(ctx, snap); err != nil {
		return false, fmt.Errorf("save play stats: %w", err)
	}
	go syncLatestPlayStatsToVegapunkDB(context.Background(), ch.sqlDb, userId)
	return true, nil
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
	statsCtx, cancel := context.WithTimeout(ctx, autoRefreshUserTimeout)
	pp, err := ch.cfnClient.GetPlayStats(statsCtx, userId)
	cancel()
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
