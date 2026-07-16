// Package db is the shared Postgres helper: connect-with-retry (so a service can
// start before its database is ready) and a tiny migrate step.
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a pgx pool, retrying once a second until the DB answers a Ping or
// ctx is cancelled. Compose starts DB + service together, so the service must wait.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	var lastErr error
	for i := 0; i < 30; i++ {
		pool, err := pgxpool.New(ctx, url)
		if err == nil {
			if err = pool.Ping(ctx); err == nil {
				return pool, nil
			}
			pool.Close() // ping failed; drop this pool and retry
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return nil, fmt.Errorf("database unreachable after retries: %w", lastErr)
}

// Migrate runs idempotent schema DDL on startup (CREATE TABLE IF NOT EXISTS …).
func Migrate(ctx context.Context, pool *pgxpool.Pool, ddl string) error {
	_, err := pool.Exec(ctx, ddl)
	return err
}
