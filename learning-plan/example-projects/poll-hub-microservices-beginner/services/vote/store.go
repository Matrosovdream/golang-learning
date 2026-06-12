package main

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errAlreadyVoted = errors.New("this voter has already voted in this poll")

type Vote struct {
	ID        int64     `json:"id"`
	PollID    int64     `json:"poll_id"`
	OptionID  int64     `json:"option_id"`
	Voter     string    `json:"voter"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct{ db *pgxpool.Pool }

// CastVote inserts a vote. The UNIQUE (poll_id, voter) constraint makes
// double voting impossible even under concurrent requests — we just
// translate the database's unique_violation into a friendly error.
func (s *Store) CastVote(ctx context.Context, pollID, optionID int64, voter string) (*Vote, error) {
	v := &Vote{PollID: pollID, OptionID: optionID, Voter: voter}
	err := s.db.QueryRow(ctx,
		`INSERT INTO votes (poll_id, option_id, voter)
		 VALUES ($1, $2, $3)
		 RETURNING id, created_at`,
		pollID, optionID, voter,
	).Scan(&v.ID, &v.CreatedAt)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
		return nil, errAlreadyVoted
	}
	if err != nil {
		return nil, err
	}
	return v, nil
}

// ListVotes returns every vote cast in one poll, oldest first.
func (s *Store) ListVotes(ctx context.Context, pollID int64) ([]Vote, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, poll_id, option_id, voter, created_at
		 FROM votes WHERE poll_id = $1 ORDER BY id`, pollID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	votes := []Vote{}
	for rows.Next() {
		var v Vote
		if err := rows.Scan(&v.ID, &v.PollID, &v.OptionID, &v.Voter, &v.CreatedAt); err != nil {
			return nil, err
		}
		votes = append(votes, v)
	}
	return votes, rows.Err()
}
