package sql

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/williamsjokvist/cfn-tracker/pkg/model"
)

type MatchStorage interface {
	SaveMatch(ctx context.Context, match model.Match) error
	GetMatches(ctx context.Context, sessionId uint16, userId string, limit uint8, offset uint16) ([]*model.Match, error)
}

func (s *Storage) SaveMatch(ctx context.Context, match model.Match) error {
	_, err := s.saveMatch(ctx, match)
	return err
}

// SaveMatchIfNew inserts the match and reports whether a new row was actually
// created. Returns false when the underlying INSERT OR IGNORE collides on the
// (session_id, date, time) primary key — important for the backfill path which
// otherwise would treat an ignored insert as a successful save and leave
// in-memory state ahead of the DB.
func (s *Storage) SaveMatchIfNew(ctx context.Context, match model.Match) (bool, error) {
	rows, err := s.saveMatch(ctx, match)
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (s *Storage) saveMatch(ctx context.Context, match model.Match) (int64, error) {
	query := `
		INSERT OR IGNORE INTO matches (
			user_id,
			user_name,
			session_id,
			character,
			lp,
			lp_gain,
			mr,
			mr_gain,
			opponent,
			opponent_character,
			opponent_lp,
			opponent_mr,
			opponent_league,
			wins,
			losses,
			win_rate,
			win_streak,
			victory,
			date,
			time,
			replay_id
		)
		VALUES (
			:user_id,
			:user_name,
			:session_id,
			:character,
			:lp,
			:lp_gain,
			:mr,
			:mr_gain,
			:opponent,
			:opponent_character,
			:opponent_lp,
			:opponent_mr,
			:opponent_league,
			:wins,
			:losses,
			:win_rate,
			:win_streak,
			:victory,
			:date,
			:time,
			:replay_id
		)
	`
	res, err := s.db.NamedExecContext(ctx, query, match)
	if err != nil {
		return 0, fmt.Errorf("create match: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return rows, nil
}

func (s *Storage) GetMatchReplayIDsForUser(ctx context.Context, userId string) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT replay_id FROM matches
		WHERE user_id = ? AND replay_id != ''
	`, userId)
	if err != nil {
		return nil, fmt.Errorf("query existing replay ids: %w", err)
	}
	defer rows.Close()
	out := make(map[string]bool, 64)
	for rows.Next() {
		var rid string
		if err := rows.Scan(&rid); err != nil {
			return nil, fmt.Errorf("scan replay id: %w", err)
		}
		out[rid] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate replay ids: %w", err)
	}
	return out, nil
}

func (s *Storage) GetMatches(ctx context.Context, sessionId uint16, userId string, limit uint8, offset uint16) ([]*model.Match, error) {
	whereStmts := []string{}
	var whereArgs []interface{}
	where := ``
	if sessionId != 0 {
		whereStmts = append(whereStmts, `session_id = (?)`)
		whereArgs = append(whereArgs, sessionId)
	}
	if userId != "" {
		whereStmts = append(whereStmts, `user_id = (?)`)
		whereArgs = append(whereArgs, userId)
	}
	if len(whereStmts) > 0 {
		where = fmt.Sprintf(`WHERE %s`, strings.Join(whereStmts, ` AND `))
	}
	pagination := ``
	if limit != 0 || offset != 0 {
		pagination = fmt.Sprintf(`LIMIT %d OFFSET %d`, limit, offset)
	}
	query, args, err := sqlx.In(fmt.Sprintf(`
		SELECT * FROM matches %s
		ORDER BY date DESC, time DESC
		%s
`, where, pagination), whereArgs...)
	if err != nil {
		return nil, fmt.Errorf("prepare matches by session query: %w", err)
	}
	var matches []*model.Match
	err = s.db.SelectContext(ctx, &matches, query, args...)
	if err != nil {
		return nil, fmt.Errorf("execute matches query: %w", err)
	}

	return matches, nil
}
