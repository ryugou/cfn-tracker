package sql

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/williamsjokvist/cfn-tracker/pkg/model"
)

func (s *Storage) SaveSF6CharacterMoves(ctx context.Context, moves []model.SF6CharacterMove) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sf6 character moves transaction: %w", err)
	}
	defer tx.Rollback()

	for _, move := range moves {
		if move.Character == "" || move.Locale == "" || move.Source == "" || move.Name == "" {
			continue
		}
		if _, err := tx.NamedExecContext(ctx, `
			INSERT INTO sf6_character_moves (
				character, locale, source, category, name, command, description,
				startup, active, recovery, hit_advantage, block_advantage, cancel,
				damage, combo_scaling, drive_gauge_gain_hit, drive_gauge_loss_block,
				drive_gauge_loss_punish, sa_gauge_gain, attribute, remarks, raw_text,
				source_url, fetched_at, updated_at
			) VALUES (
				:character, :locale, :source, :category, :name, :command, :description,
				:startup, :active, :recovery, :hit_advantage, :block_advantage, :cancel,
				:damage, :combo_scaling, :drive_gauge_gain_hit, :drive_gauge_loss_block,
				:drive_gauge_loss_punish, :sa_gauge_gain, :attribute, :remarks, :raw_text,
				:source_url, :fetched_at, DATETIME('NOW')
			)
			ON CONFLICT(character, locale, source, category, name, command, startup, active)
			DO UPDATE SET
				description = excluded.description,
				recovery = excluded.recovery,
				hit_advantage = excluded.hit_advantage,
				block_advantage = excluded.block_advantage,
				cancel = excluded.cancel,
				damage = excluded.damage,
				combo_scaling = excluded.combo_scaling,
				drive_gauge_gain_hit = excluded.drive_gauge_gain_hit,
				drive_gauge_loss_block = excluded.drive_gauge_loss_block,
				drive_gauge_loss_punish = excluded.drive_gauge_loss_punish,
				sa_gauge_gain = excluded.sa_gauge_gain,
				attribute = excluded.attribute,
				remarks = excluded.remarks,
				raw_text = excluded.raw_text,
				source_url = excluded.source_url,
				fetched_at = excluded.fetched_at,
				updated_at = DATETIME('NOW')
		`, move); err != nil {
			return fmt.Errorf("upsert sf6 character move %s/%s: %w", move.Source, move.Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sf6 character moves transaction: %w", err)
	}
	return nil
}

func (s *Storage) ReplaceSF6CharacterMoves(ctx context.Context, character, locale string, moves []model.SF6CharacterMove) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replace sf6 character moves transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM sf6_character_moves
		WHERE character = ? AND locale = ?
	`, character, locale); err != nil {
		return fmt.Errorf("delete sf6 character moves: %w", err)
	}
	for _, move := range moves {
		if move.Character == "" || move.Locale == "" || move.Source == "" || move.Name == "" {
			continue
		}
		if _, err := tx.NamedExecContext(ctx, `
			INSERT INTO sf6_character_moves (
				character, locale, source, category, name, command, description,
				startup, active, recovery, hit_advantage, block_advantage, cancel,
				damage, combo_scaling, drive_gauge_gain_hit, drive_gauge_loss_block,
				drive_gauge_loss_punish, sa_gauge_gain, attribute, remarks, raw_text,
				source_url, fetched_at, updated_at
			) VALUES (
				:character, :locale, :source, :category, :name, :command, :description,
				:startup, :active, :recovery, :hit_advantage, :block_advantage, :cancel,
				:damage, :combo_scaling, :drive_gauge_gain_hit, :drive_gauge_loss_block,
				:drive_gauge_loss_punish, :sa_gauge_gain, :attribute, :remarks, :raw_text,
				:source_url, :fetched_at, DATETIME('NOW')
			)
			ON CONFLICT(character, locale, source, category, name, command, startup, active)
			DO UPDATE SET
				description = excluded.description,
				recovery = excluded.recovery,
				hit_advantage = excluded.hit_advantage,
				block_advantage = excluded.block_advantage,
				cancel = excluded.cancel,
				damage = excluded.damage,
				combo_scaling = excluded.combo_scaling,
				drive_gauge_gain_hit = excluded.drive_gauge_gain_hit,
				drive_gauge_loss_block = excluded.drive_gauge_loss_block,
				drive_gauge_loss_punish = excluded.drive_gauge_loss_punish,
				sa_gauge_gain = excluded.sa_gauge_gain,
				attribute = excluded.attribute,
				remarks = CASE
					WHEN sf6_character_moves.remarks = '' THEN excluded.remarks
					WHEN excluded.remarks = '' THEN sf6_character_moves.remarks
					WHEN sf6_character_moves.remarks = excluded.remarks THEN sf6_character_moves.remarks
					ELSE sf6_character_moves.remarks || ' / ' || excluded.remarks
				END,
				raw_text = excluded.raw_text,
				source_url = excluded.source_url,
				fetched_at = excluded.fetched_at,
				updated_at = DATETIME('NOW')
		`, move); err != nil {
			return fmt.Errorf("insert sf6 character move %s/%s: %w", move.Source, move.Name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace sf6 character moves transaction: %w", err)
	}
	return nil
}

func (s *Storage) GetSF6CharacterMoves(ctx context.Context, character, locale string, limit int) ([]model.SF6CharacterMove, error) {
	if limit <= 0 {
		limit = 500
	}
	rows := []model.SF6CharacterMove{}
	if err := s.db.SelectContext(ctx, &rows, `
		SELECT * FROM sf6_character_moves
		WHERE character = ? AND locale = ?
		ORDER BY
			CASE source WHEN 'frame' THEN 0 WHEN 'movelist' THEN 1 ELSE 2 END,
			category ASC,
			id ASC
		LIMIT ?
	`, character, locale, limit); err != nil {
		return nil, fmt.Errorf("select sf6 character moves: %w", err)
	}
	return rows, nil
}

func (s *Storage) FindSF6CharacterMoves(ctx context.Context, character, locale string, terms []string, limit int) ([]model.SF6CharacterMove, error) {
	if limit <= 0 {
		limit = 40
	}
	args := []any{character, locale}
	termClauses := []string{}
	for _, term := range terms {
		if term == "" {
			continue
		}
		clause := `(name LIKE ? OR category LIKE ? OR description LIKE ? OR remarks LIKE ? OR cancel LIKE ? OR attribute LIKE ?)`
		if strings.Contains(term, "キャンセル") {
			clause = `(cancel != '' OR name LIKE ? OR category LIKE ? OR description LIKE ? OR remarks LIKE ? OR cancel LIKE ? OR attribute LIKE ?)`
		}
		termClauses = append(termClauses, clause)
		pattern := "%" + term + "%"
		args = append(args, pattern, pattern, pattern, pattern, pattern, pattern)
	}
	where := ""
	if len(termClauses) > 0 {
		where = ` AND (` + strings.Join(termClauses, ` OR `) + `)`
	}
	args = append(args, limit)

	rows := []model.SF6CharacterMove{}
	if err := s.db.SelectContext(ctx, &rows, `
		SELECT * FROM sf6_character_moves
		WHERE character = ? AND locale = ?
	`+where+`
		ORDER BY
			CASE
				WHEN source = 'frame' AND cancel != '' THEN 0
				WHEN source = 'frame' THEN 1
				WHEN source = 'movelist' THEN 2
				ELSE 3
			END,
			category ASC,
			id ASC
		LIMIT ?
	`, args...); err != nil {
		return nil, fmt.Errorf("find sf6 character moves: %w", err)
	}
	return rows, nil
}

func (s *Storage) SF6CharacterDataFresh(ctx context.Context, locale string, expectedCharacters int, maxAge time.Duration) (bool, error) {
	if locale == "" {
		locale = "ja-jp"
	}
	var row struct {
		CharacterCount int    `db:"character_count"`
		OldestFetched  string `db:"oldest_fetched"`
	}
	if err := s.db.GetContext(ctx, &row, `
		SELECT
			COUNT(DISTINCT character) AS character_count,
			COALESCE(MIN(fetched_at), '') AS oldest_fetched
		FROM sf6_character_moves
		WHERE locale = ?
	`, locale); err != nil {
		return false, fmt.Errorf("select sf6 character data freshness: %w", err)
	}
	if row.CharacterCount < expectedCharacters || row.OldestFetched == "" {
		return false, nil
	}
	oldest, err := parseSQLiteTime(row.OldestFetched)
	if err != nil {
		return false, nil
	}
	return time.Since(oldest) < maxAge, nil
}

func parseSQLiteTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
	} {
		t, err := time.ParseInLocation(layout, value, time.Local)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time format %q", value)
}
