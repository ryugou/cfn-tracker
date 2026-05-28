package sql

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/williamsjokvist/cfn-tracker/pkg/model"
)

func (s *Storage) SaveBenchmarkPlayers(
	ctx context.Context,
	sourceUserId, character string,
	players []*model.BenchmarkPlayer,
) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin benchmark transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM benchmark_players
		WHERE source_user_id = ? AND character = ?
	`, sourceUserId, character); err != nil {
		return fmt.Errorf("clear benchmark players: %w", err)
	}

	for _, player := range players {
		if player == nil {
			continue
		}
		if player.Stats != nil {
			b, err := json.Marshal(player.Stats)
			if err != nil {
				return fmt.Errorf("marshal benchmark stats: %w", err)
			}
			player.StatsJSON = string(b)
		}
		_, err := tx.NamedExecContext(ctx, `
			INSERT INTO benchmark_players (
				source_user_id, target_user_id, fighter_id, character, character_tool_name,
				rank_offset, league_rank, lp, mr, mr_ranking, last_play_at,
				fetched_at, stats_json, last_error
			) VALUES (
				:source_user_id, :target_user_id, :fighter_id, :character, :character_tool_name,
				:rank_offset, :league_rank, :lp, :mr, :mr_ranking, :last_play_at,
				DATETIME('NOW'), :stats_json, :last_error
			)
		`, player)
		if err != nil {
			return fmt.Errorf("insert benchmark player: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit benchmark transaction: %w", err)
	}
	return nil
}

func (s *Storage) GetBenchmarkPlayers(
	ctx context.Context,
	sourceUserId, character string,
) ([]*model.BenchmarkPlayer, error) {
	rows := []*model.BenchmarkPlayer{}
	if err := s.db.SelectContext(ctx, &rows, `
		SELECT * FROM benchmark_players
		WHERE source_user_id = ? AND character = ?
		ORDER BY rank_offset ASC, mr DESC, lp DESC, fighter_id ASC
	`, sourceUserId, character); err != nil {
		return nil, fmt.Errorf("select benchmark players: %w", err)
	}
	for _, row := range rows {
		if row.StatsJSON == "" {
			continue
		}
		var stats model.PlayStatsSnapshot
		if err := json.Unmarshal([]byte(row.StatsJSON), &stats); err != nil {
			return nil, fmt.Errorf("unmarshal benchmark stats: %w", err)
		}
		row.Stats = &stats
	}
	return rows, nil
}
