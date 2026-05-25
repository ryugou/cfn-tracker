package sql

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/williamsjokvist/cfn-tracker/pkg/model"
)

const playStatsInsertColumns = `
	user_id, character, match_replay_id,
	battle_hub_match_play_count, casual_match_play_count, corner_time, cornered_time,
	custom_room_match_play_count, drive_impact, drive_impact_to_drive_impact,
	drive_parry, drive_reversal,
	gauge_rate_ca, gauge_rate_drive_arts, gauge_rate_drive_guard, gauge_rate_drive_impact,
	gauge_rate_drive_other, gauge_rate_drive_reversal,
	gauge_rate_drive_rush_from_cancel, gauge_rate_drive_rush_from_parry,
	gauge_rate_sa_lv1, gauge_rate_sa_lv2, gauge_rate_sa_lv3,
	just_parry, punish_counter, rank_match_play_count,
	received_drive_impact, received_drive_impact_to_drive_impact,
	received_punish_counter, received_stun, received_throw_count, received_throw_drive_parry,
	rival_ai_achieved_challenge_count, rival_ai_highest_league_rank, rival_ai_highest_league_rank_txt,
	stun, target_clear_count,
	throw_count, throw_drive_parry, throw_tech,
	total_all_character_play_point,
	enjoy_fight_point, enjoy_total_point, enjoy_user_point,
	world_tour_seconds, ranked_match_seconds, casual_match_seconds, custom_room_seconds,
	battle_hub_seconds, offline_match_seconds, arcade_seconds, practice_seconds, extreme_seconds
`

const playStatsInsertValues = `
	:user_id, :character, :match_replay_id,
	:battle_hub_match_play_count, :casual_match_play_count, :corner_time, :cornered_time,
	:custom_room_match_play_count, :drive_impact, :drive_impact_to_drive_impact,
	:drive_parry, :drive_reversal,
	:gauge_rate_ca, :gauge_rate_drive_arts, :gauge_rate_drive_guard, :gauge_rate_drive_impact,
	:gauge_rate_drive_other, :gauge_rate_drive_reversal,
	:gauge_rate_drive_rush_from_cancel, :gauge_rate_drive_rush_from_parry,
	:gauge_rate_sa_lv1, :gauge_rate_sa_lv2, :gauge_rate_sa_lv3,
	:just_parry, :punish_counter, :rank_match_play_count,
	:received_drive_impact, :received_drive_impact_to_drive_impact,
	:received_punish_counter, :received_stun, :received_throw_count, :received_throw_drive_parry,
	:rival_ai_achieved_challenge_count, :rival_ai_highest_league_rank, :rival_ai_highest_league_rank_txt,
	:stun, :target_clear_count,
	:throw_count, :throw_drive_parry, :throw_tech,
	:total_all_character_play_point,
	:enjoy_fight_point, :enjoy_total_point, :enjoy_user_point,
	:world_tour_seconds, :ranked_match_seconds, :casual_match_seconds, :custom_room_seconds,
	:battle_hub_seconds, :offline_match_seconds, :arcade_seconds, :practice_seconds, :extreme_seconds
`

func (s *Storage) SavePlayStats(ctx context.Context, snap model.PlayStatsSnapshot) error {
	query := fmt.Sprintf(
		`INSERT INTO play_stats_snapshots (%s) VALUES (%s)`,
		strings.TrimSpace(playStatsInsertColumns),
		strings.TrimSpace(playStatsInsertValues),
	)
	if _, err := s.db.NamedExecContext(ctx, query, snap); err != nil {
		return fmt.Errorf("insert play stats snapshot: %w", err)
	}
	return nil
}

// GetPlayStatsHistory returns user-wide play-stats snapshots. The Capcom
// /play battle_stats payload is a user-level aggregate (not per-character),
// so the `character` argument is accepted for backwards compatibility only
// and is ignored — filtering by it would silently drop snapshots whose
// stored character tag (the user's favorite at capture time) differs from
// the caller-supplied value. Per-character views must be derived at a
// higher layer by attributing snapshot deltas to each match's character.
func (s *Storage) GetPlayStatsHistory(
	ctx context.Context,
	userId, _ /* character (ignored) */, from, to string,
	limit uint16,
) ([]*model.PlayStatsSnapshot, error) {
	wheres := []string{"user_id = ?"}
	args := []interface{}{userId}
	if from != "" {
		wheres = append(wheres, "DATE(snapshot_at) >= ?")
		args = append(args, from)
	}
	if to != "" {
		wheres = append(wheres, "DATE(snapshot_at) <= ?")
		args = append(args, to)
	}
	limitClause := ""
	if limit > 0 {
		limitClause = fmt.Sprintf("LIMIT %d", limit)
	}
	query := fmt.Sprintf(`
		SELECT * FROM play_stats_snapshots
		WHERE %s
		ORDER BY snapshot_at ASC
		%s
	`, strings.Join(wheres, " AND "), limitClause)
	var rows []*model.PlayStatsSnapshot
	if err := s.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, fmt.Errorf("execute play stats query: %w", err)
	}
	return rows, nil
}

// GetLatestPlayStatsSnapshot returns the most recent play_stats_snapshots
// row for the given user, or (nil, nil) when none exists. Used by the
// baseline dedup check during StartTracking — strictly better than the
// previous full-day GetPlayStatsHistory scan, since `LIMIT 1` lets SQLite
// stop emitting rows after the first.
//
// The existing index is idx_play_stats_user_char_at(user_id, character,
// snapshot_at). With a user-only WHERE clause SQLite still uses that index
// to find the user's rows but cannot avoid a sort because the leading
// `character` column splits snapshot_at across character buckets. That is
// acceptable here: this query runs once per StartTracking, snapshot volume
// per user is small (handful per session), and any sort cost is dwarfed by
// the Capcom /play HTTP call it gates. If per-user snapshot counts ever
// grow past ~10k (e.g. months of continuous tracking with high turnover),
// add a dedicated idx_play_stats_user_at(user_id, snapshot_at, id) so this
// becomes a pure index seek.
//
// Snapshots are user-wide aggregates (see GetPlayStatsHistory's docstring);
// scoping this lookup by character would let a favorite-character switch
// silently bypass the 30-minute dedup window — the first restart on the new
// character would always insert a duplicate baseline even if a recent
// snapshot already covers the user. Filter by user only.
func (s *Storage) GetLatestPlayStatsSnapshot(
	ctx context.Context,
	userId string,
) (*model.PlayStatsSnapshot, error) {
	query := `
		SELECT * FROM play_stats_snapshots
		WHERE user_id = ?
		ORDER BY snapshot_at DESC, id DESC
		LIMIT 1
	`
	var rows []*model.PlayStatsSnapshot
	if err := s.db.SelectContext(ctx, &rows, query, userId); err != nil {
		return nil, fmt.Errorf("get latest play stats snapshot: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

func (s *Storage) GetMatchesWithPlayStats(
	ctx context.Context,
	userId, character string,
	limit uint8,
	offset uint16,
) ([]*model.MatchWithStats, error) {
	// Filter character at the SQL layer so pagination (LIMIT/OFFSET) is
	// applied to character-scoped rows. Filtering after a full-character
	// fetch caused the requested page to be drained by rows for other
	// characters, returning an empty slice for the selected one.
	pagination := ""
	if limit != 0 || offset != 0 {
		pagination = fmt.Sprintf("LIMIT %d OFFSET %d", limit, offset)
	}
	// Empty character = "all characters" (per-character views are now derived
	// from snapshot deltas; the selector may target every match the user has
	// played). Non-empty character keeps the per-character pagination scope.
	whereParts := []string{"user_id = ?"}
	args := []interface{}{userId}
	if character != "" {
		whereParts = append(whereParts, "character = ?")
		args = append(args, character)
	}
	query := fmt.Sprintf(`
		SELECT * FROM matches
		WHERE %s
		ORDER BY date DESC, time DESC
		%s
	`, strings.Join(whereParts, " AND "), pagination)
	var matches []*model.Match
	if err := s.db.SelectContext(ctx, &matches, query, args...); err != nil {
		return nil, fmt.Errorf("get matches for stats join: %w", err)
	}
	if len(matches) == 0 {
		return []*model.MatchWithStats{}, nil
	}

	replayIds := make([]string, 0, len(matches))
	for _, m := range matches {
		if m.ReplayID != "" {
			replayIds = append(replayIds, m.ReplayID)
		}
	}
	statsByReplay := map[string]*model.PlayStatsSnapshot{}
	if len(replayIds) > 0 {
		// Snapshots are user-wide (one per match_replay_id regardless of
		// favorite-character tag), so we look them up by (user_id,
		// match_replay_id) only — joining on character would drop
		// snapshots whose stored character tag doesn't match the match's
		// character (e.g. when the user's favorite changed mid-session).
		//
		// ORDER BY snapshot_at ASC, id ASC pins the iteration order so the
		// later (newest) row wins when the same (user_id, match_replay_id)
		// has multiple snapshots — without it, map assignment is at the
		// mercy of SQLite's row order and a duplicate could leave an older
		// snapshot stuck in the result.
		q, args, err := sqlx.In(`
			SELECT * FROM play_stats_snapshots
			WHERE user_id = ? AND match_replay_id IN (?)
			ORDER BY snapshot_at ASC, id ASC
		`, userId, replayIds)
		if err != nil {
			return nil, fmt.Errorf("prepare stats lookup: %w", err)
		}
		var rows []*model.PlayStatsSnapshot
		if err := s.db.SelectContext(ctx, &rows, q, args...); err != nil {
			return nil, fmt.Errorf("fetch stats for matches: %w", err)
		}
		for _, r := range rows {
			if r.MatchReplayId.Valid {
				statsByReplay[r.MatchReplayId.String] = r
			}
		}
	}

	out := make([]*model.MatchWithStats, 0, len(matches))
	for _, m := range matches {
		entry := &model.MatchWithStats{Match: *m}
		if snap, ok := statsByReplay[m.ReplayID]; ok {
			entry.Stats = snap
		}
		out = append(out, entry)
	}
	return out, nil
}
