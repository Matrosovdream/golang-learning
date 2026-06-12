package main

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errNotFound = errors.New("poll not found")

type Poll struct {
	ID        int64     `json:"id"`
	Question  string    `json:"question"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	Options   []Option  `json:"options,omitempty"`
}

type Option struct {
	ID    int64  `json:"id"`
	Label string `json:"label"`
}

type Store struct{ db *pgxpool.Pool }

// CreatePoll inserts the poll and all its options in one transaction —
// either everything lands or nothing does.
func (s *Store) CreatePoll(ctx context.Context, question string, labels []string) (*Poll, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) // no-op once committed

	p := &Poll{Question: question}
	err = tx.QueryRow(ctx,
		`INSERT INTO polls (question) VALUES ($1) RETURNING id, status, created_at`,
		question,
	).Scan(&p.ID, &p.Status, &p.CreatedAt)
	if err != nil {
		return nil, err
	}

	for _, label := range labels {
		var opt Option
		err = tx.QueryRow(ctx,
			`INSERT INTO options (poll_id, label) VALUES ($1, $2) RETURNING id, label`,
			p.ID, label,
		).Scan(&opt.ID, &opt.Label)
		if err != nil {
			return nil, err
		}
		p.Options = append(p.Options, opt)
	}

	return p, tx.Commit(ctx)
}

// ListPolls returns all polls, newest first, without their options.
func (s *Store) ListPolls(ctx context.Context) ([]Poll, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, question, status, created_at FROM polls ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	polls := []Poll{}
	for rows.Next() {
		var p Poll
		if err := rows.Scan(&p.ID, &p.Question, &p.Status, &p.CreatedAt); err != nil {
			return nil, err
		}
		polls = append(polls, p)
	}
	return polls, rows.Err()
}

// GetPoll returns one poll with its options.
func (s *Store) GetPoll(ctx context.Context, id int64) (*Poll, error) {
	var p Poll
	err := s.db.QueryRow(ctx,
		`SELECT id, question, status, created_at FROM polls WHERE id = $1`, id,
	).Scan(&p.ID, &p.Question, &p.Status, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(ctx,
		`SELECT id, label FROM options WHERE poll_id = $1 ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var opt Option
		if err := rows.Scan(&opt.ID, &opt.Label); err != nil {
			return nil, err
		}
		p.Options = append(p.Options, opt)
	}
	return &p, rows.Err()
}

// ClosePoll flips a poll to closed; closing twice is fine.
func (s *Store) ClosePoll(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE polls SET status = 'closed' WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errNotFound
	}
	return nil
}

// DeletePoll removes a poll; options and votes follow via ON DELETE CASCADE.
func (s *Store) DeletePoll(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM polls WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errNotFound
	}
	return nil
}
