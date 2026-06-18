// Package db opens the Postgres connection pool used by every repository.
package db

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a pgx pool, retrying until Postgres accepts connections (so the
// service survives `docker compose up` racing the database).
func Connect(ctx context.Context, dsn string, attempts int) (*pgxpool.Pool, error) {
	var lastErr error
	for i := 0; i < attempts; i++ {
		pool, err := pgxpool.New(ctx, dsn)
		if err == nil {
			pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err = pool.Ping(pingCtx)
			cancel()
			if err == nil {
				return pool, nil
			}
			pool.Close()
		}
		lastErr = err
		log.Printf("waiting for database (attempt %d/%d): %v", i+1, attempts, err)
		time.Sleep(2 * time.Second)
	}
	return nil, lastErr
}
