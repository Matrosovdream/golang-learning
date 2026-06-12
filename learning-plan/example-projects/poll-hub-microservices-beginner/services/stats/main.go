// stats-service is read-only: it aggregates votes into results and
// rankings straight in SQL. It writes nothing and calls no one.
package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"pollhub/internal/env"
	"pollhub/internal/httpx"
	"pollhub/internal/postgres"
)

func main() {
	pool, err := postgres.Connect(context.Background(),
		env.Get("DATABASE_URL", "postgres://pollhub:pollhub@localhost:5433/pollhub"))
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	api := &API{store: &Store{db: pool}}

	httpx.Serve("stats-service", &http.Server{
		Addr:              ":" + env.Get("PORT", "8083"),
		Handler:           api.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	})
}
