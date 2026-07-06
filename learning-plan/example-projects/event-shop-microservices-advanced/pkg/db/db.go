// Package db holds the Postgres helper.
package db

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Returns two values — a *pgxpool.Pool (pointer) and an error. Pairing a result
// with an error in the return list is the core Go error idiom.
func Connect(ctx context.Context, dsn string, attempts int) (*pgxpool.Pool, error) {
	var lastErr error // zero value of an interface type is nil
	for i := 0; i < attempts; i++ {
		pool, err := pgxpool.New(ctx, dsn)
		if err == nil {
			// WithTimeout returns a derived context and a cancel func; calling
			// cancel releases the timer. `cancel` is itself a value of func type.
			pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err = pool.Ping(pingCtx)
			cancel()
			if err == nil {
				return pool, nil // a nil error signals success
			}
			pool.Close()
		}
		lastErr = err
		log.Printf("waiting for database (attempt %d/%d): %v", i+1, attempts, err)
		time.Sleep(2 * time.Second)
	}
	return nil, lastErr
}

func Getenv(key, fallback string) string {
	// if-with-init: v is scoped to this if/else only.
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
