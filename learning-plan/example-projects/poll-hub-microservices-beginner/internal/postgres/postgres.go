// Package postgres opens the connection pool all services share.
package postgres

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a pgx pool and pings it, retrying for ~30 seconds so a
// service that starts before the database inside Compose just waits for
// it instead of crashing.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	for attempt := 1; ; attempt++ {
		if err = pool.Ping(ctx); err == nil {
			return pool, nil
		}
		if attempt == 10 {
			pool.Close()
			return nil, fmt.Errorf("ping database: %w", err)
		}
		log.Printf("database not ready (attempt %d/10): %v", attempt, err)
		time.Sleep(3 * time.Second)
	}
}
