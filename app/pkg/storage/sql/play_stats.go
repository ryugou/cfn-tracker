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

func (s *Storage) GetPlayStatsHistory(
	ctx context.Context,
	userId, character, from, to string,
	limit uint16,
) ([]*model.PlayStatsSnapshot, error) {
	wheres := []string{"user_id = ?", "character = ?"}
	args := []interface{}{userId, character}
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

func (s *Storage) GetPlayStatsCharacters(ctx context.Context, userId string) ([]string, error) {
	query := `
		SELECT DISTINCT character
		FROM play_stats_snapshots
		WHERE user_id = ?
		ORDER BY character ASC
	`
	rows, err := s.db.QueryContext(ctx, query, userId)
	if err != nil {
		return nil, fmt.Errorf("execute distinct characters query: %w", err)
	}
	defer rows.Close()
	characters := make([]string, 0, 4)
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, fmt.Errorf("scan character row: %w", err)
		}
		characters = append(characters, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate character rows: %w", err)
	}
	return characters, nil
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
	query := fmt.Sprintf(`
		SELECT * FROM matches
		WHERE user_id = ? AND character = ?
		ORDER BY date DESC, time DESC
		%s
	`, pagination)
	var matches []*model.Match
	if err := s.db.SelectContext(ctx, &matches, query, userId, character); err != nil {
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
		q, args, err := sqlx.In(`
			SELECT * FROM play_stats_snapshots
			WHERE user_id = ? AND character = ? AND match_replay_id IN (?)
		`, userId, character, replayIds)
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
