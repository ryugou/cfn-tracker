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
	go ch.runPlayStatsRefreshLoop(ctx)
	go ch.runBenchmarkRefreshLoop(ctx)
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
