package sql

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/williamsjokvist/cfn-tracker/pkg/model"
)

const benchmarkSamplePlayersPerRank = 5

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
				rank_offset, league_rank, lp, mr, mr_ranking, wins, losses, win_diff, last_play_at,
				fetched_at, stats_json, last_error
			) VALUES (
				:source_user_id, :target_user_id, :fighter_id, :character, :character_tool_name,
				:rank_offset, :league_rank, :lp, :mr, :mr_ranking, :wins, :losses, :win_diff, :last_play_at,
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
		WITH ranked AS (
			SELECT
				id,
				ROW_NUMBER() OVER (
					PARTITION BY rank_offset
					ORDER BY win_diff DESC, wins DESC, mr DESC, lp DESC, fighter_id ASC
				) AS sample_rank
			FROM benchmark_players
			WHERE source_user_id = ?
				AND character = ?
				AND last_error = ''
				AND stats_json IS NOT NULL
				AND stats_json != ''
				AND wins > losses
		)
		SELECT b.*
		FROM benchmark_players b
		INNER JOIN ranked r ON r.id = b.id
		WHERE r.sample_rank <= ?
		ORDER BY b.rank_offset ASC, r.sample_rank ASC
	`, sourceUserId, character, benchmarkSamplePlayersPerRank); err != nil {
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

func (s *Storage) GetBenchmarkRefreshTargets(ctx context.Context) ([]model.BenchmarkRefreshTarget, error) {
	rows := []model.BenchmarkRefreshTarget{}
	if err := s.db.SelectContext(ctx, &rows, `
		WITH user_chars AS (
			SELECT DISTINCT user_id, character
			FROM matches
			WHERE user_id != '' AND character != ''
		)
		SELECT
			uc.user_id,
			uc.character,
			COALESCE(MAX(bp.fetched_at), '') AS fetched_at
		FROM user_chars uc
		LEFT JOIN benchmark_players bp
			ON bp.source_user_id = uc.user_id
			AND bp.character = uc.character
		GROUP BY uc.user_id, uc.character
		ORDER BY uc.user_id ASC, uc.character ASC
	`); err != nil {
		return nil, fmt.Errorf("select benchmark refresh targets: %w", err)
	}
	return rows, nil
}
