package main

import (
	"context"
	"errors"
	"math"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errNotFound = errors.New("poll not found")

type PollResults struct {
	PollID     int64          `json:"poll_id"`
	Question   string         `json:"question"`
	Status     string         `json:"status"`
	TotalVotes int64          `json:"total_votes"`
	Results    []OptionResult `json:"results"`
}

type OptionResult struct {
	OptionID int64   `json:"option_id"`
	Label    string  `json:"label"`
	Votes    int64   `json:"votes"`
	Percent  float64 `json:"percent"`
}

type TopPoll struct {
	PollID   int64  `json:"poll_id"`
	Question string `json:"question"`
	Votes    int64  `json:"votes"`
}

type Store struct{ db *pgxpool.Pool }

// PollResults counts votes per option in SQL — the database does the
// aggregation, Go only computes the percentages on top.
func (s *Store) PollResults(ctx context.Context, pollID int64) (*PollResults, error) {
	res := &PollResults{PollID: pollID, Results: []OptionResult{}}
	err := s.db.QueryRow(ctx,
		`SELECT question, status FROM polls WHERE id = $1`, pollID,
	).Scan(&res.Question, &res.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, err
	}

	// LEFT JOIN so options with zero votes still show up.
	rows, err := s.db.Query(ctx, `
		SELECT o.id, o.label, COUNT(v.id) AS votes
		FROM options o
		LEFT JOIN votes v ON v.option_id = o.id
		WHERE o.poll_id = $1
		GROUP BY o.id, o.label
		ORDER BY votes DESC, o.id`, pollID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var or OptionResult
		if err := rows.Scan(&or.OptionID, &or.Label, &or.Votes); err != nil {
			return nil, err
		}
		res.TotalVotes += or.Votes
		res.Results = append(res.Results, or)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if res.TotalVotes > 0 {
		for i := range res.Results {
			share := float64(res.Results[i].Votes) / float64(res.TotalVotes)
			res.Results[i].Percent = math.Round(share*1000) / 10 // one decimal
		}
	}
	return res, nil
}

// TopPolls returns the most-voted polls.
func (s *Store) TopPolls(ctx context.Context, limit int) ([]TopPoll, error) {
	rows, err := s.db.Query(ctx, `
		SELECT p.id, p.question, COUNT(v.id) AS votes
		FROM polls p
		LEFT JOIN votes v ON v.poll_id = p.id
		GROUP BY p.id, p.question
		ORDER BY votes DESC, p.id
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	top := []TopPoll{}
	for rows.Next() {
		var t TopPoll
		if err := rows.Scan(&t.PollID, &t.Question, &t.Votes); err != nil {
			return nil, err
		}
		top = append(top, t)
	}
	return top, rows.Err()
}
