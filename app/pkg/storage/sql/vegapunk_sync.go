package sql

import (
	"context"
	"fmt"
	"time"

	"github.com/williamsjokvist/cfn-tracker/pkg/model"
)

func (s *Storage) EnqueueVegapunkSyncJob(ctx context.Context, kind, dedupeKey string, payload []byte) error {
	if kind == "" {
		return fmt.Errorf("vegapunk sync kind is empty")
	}
	if dedupeKey == "" {
		return fmt.Errorf("vegapunk sync dedupe key is empty")
	}
	if len(payload) == 0 {
		return fmt.Errorf("vegapunk sync payload is empty")
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO vegapunk_sync_queue (
			kind, dedupe_key, payload_json, attempts, last_error, next_attempt_at, processed_at
		) VALUES (?, ?, ?, 0, '', '', '')
		ON CONFLICT(dedupe_key) DO UPDATE SET
			kind = excluded.kind,
			payload_json = excluded.payload_json,
			attempts = 0,
			last_error = '',
			next_attempt_at = '',
			processed_at = '',
			updated_at = DATETIME('NOW')
	`, kind, dedupeKey, string(payload)); err != nil {
		return fmt.Errorf("enqueue vegapunk sync job: %w", err)
	}
	return nil
}

func (s *Storage) GetDueVegapunkSyncJobs(ctx context.Context, limit int) ([]*model.VegapunkSyncJob, error) {
	if limit <= 0 {
		limit = 20
	}
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	rows := []*model.VegapunkSyncJob{}
	if err := s.db.SelectContext(ctx, &rows, `
		SELECT * FROM vegapunk_sync_queue
		WHERE processed_at = ''
		  AND (next_attempt_at = '' OR next_attempt_at <= ?)
		ORDER BY created_at ASC, id ASC
		LIMIT ?
	`, now, limit); err != nil {
		return nil, fmt.Errorf("select due vegapunk sync jobs: %w", err)
	}
	return rows, nil
}

func (s *Storage) MarkVegapunkSyncJobDone(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx, `
		UPDATE vegapunk_sync_queue
		SET processed_at = DATETIME('NOW'),
		    last_error = '',
		    updated_at = DATETIME('NOW')
		WHERE id = ?
	`, id); err != nil {
		return fmt.Errorf("mark vegapunk sync job done: %w", err)
	}
	return nil
}

func (s *Storage) MarkVegapunkSyncJobFailed(ctx context.Context, id int64, attempts int, lastError string, nextAttemptAt time.Time) error {
	if _, err := s.db.ExecContext(ctx, `
		UPDATE vegapunk_sync_queue
		SET attempts = ?,
		    last_error = ?,
		    next_attempt_at = ?,
		    updated_at = DATETIME('NOW')
		WHERE id = ?
	`, attempts, lastError, nextAttemptAt.UTC().Format("2006-01-02 15:04:05"), id); err != nil {
		return fmt.Errorf("mark vegapunk sync job failed: %w", err)
	}
	return nil
}
