package sql

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/williamsjokvist/cfn-tracker/pkg/model"
)

func (s *Storage) SaveAdviceRun(ctx context.Context, run *model.AdviceRun) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin advice transaction: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.NamedExecContext(ctx, `
		INSERT INTO advice_runs (user_id, character, input_window, snapshot_at)
		VALUES (:user_id, :character, :input_window, :snapshot_at)
	`, run)
	if err != nil {
		return fmt.Errorf("insert advice run: %w", err)
	}
	runId, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("get advice run id: %w", err)
	}
	run.Id = runId

	for _, candidate := range run.Candidates {
		if candidate == nil {
			continue
		}
		candidate.RunId = runId
		b, err := json.Marshal(candidate.Evidence)
		if err != nil {
			return fmt.Errorf("marshal advice evidence: %w", err)
		}
		candidate.EvidenceJSON = string(b)
		res, err := tx.NamedExecContext(ctx, `
			INSERT INTO advice_candidates (
				run_id, mode, priority, theme, summary, rationale, action, drill,
				success_criteria, watch_metrics, risks, evidence_json
			) VALUES (
				:run_id, :mode, :priority, :theme, :summary, :rationale, :action, :drill,
				:success_criteria, :watch_metrics, :risks, :evidence_json
			)
		`, candidate)
		if err != nil {
			return fmt.Errorf("insert advice candidate: %w", err)
		}
		candidate.Id, _ = res.LastInsertId()
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit advice transaction: %w", err)
	}
	return nil
}

func (s *Storage) GetLatestAdviceRun(ctx context.Context, userId, character string) (*model.AdviceRun, error) {
	rows := []*model.AdviceRun{}
	if err := s.db.SelectContext(ctx, &rows, `
		SELECT * FROM advice_runs
		WHERE user_id = ? AND character = ?
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, userId, character); err != nil {
		return nil, fmt.Errorf("select latest advice run: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	if err := s.loadAdviceCandidates(ctx, rows[0]); err != nil {
		return nil, err
	}
	return rows[0], nil
}

func (s *Storage) SaveAdviceFeedback(ctx context.Context, fb model.AdviceFeedback) error {
	if _, err := s.db.NamedExecContext(ctx, `
		INSERT INTO advice_feedback (run_id, mode, rating, specificity, usefulness, trust, comment)
		VALUES (:run_id, :mode, :rating, :specificity, :usefulness, :trust, :comment)
	`, fb); err != nil {
		return fmt.Errorf("insert advice feedback: %w", err)
	}
	return nil
}

func (s *Storage) loadAdviceCandidates(ctx context.Context, run *model.AdviceRun) error {
	candidates := []*model.AdviceCandidate{}
	if err := s.db.SelectContext(ctx, &candidates, `
		SELECT * FROM advice_candidates
		WHERE run_id = ?
		ORDER BY mode ASC, id ASC
	`, run.Id); err != nil {
		return fmt.Errorf("select advice candidates: %w", err)
	}
	for _, candidate := range candidates {
		if candidate.EvidenceJSON == "" {
			continue
		}
		if err := json.Unmarshal([]byte(candidate.EvidenceJSON), &candidate.Evidence); err != nil {
			return fmt.Errorf("unmarshal advice evidence: %w", err)
		}
	}
	run.Candidates = candidates
	return nil
}
