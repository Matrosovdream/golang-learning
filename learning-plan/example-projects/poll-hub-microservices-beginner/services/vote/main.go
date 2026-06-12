// vote-service casts votes. Before writing anything it asks
// poll-service (over HTTP) whether the poll exists, is still open, and
// really has the chosen option.
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

	api := &API{
		store: &Store{db: pool},
		polls: NewPollClient(env.Get("POLL_SERVICE_URL", "http://localhost:8081")),
	}

	httpx.Serve("vote-service", &http.Server{
		Addr:              ":" + env.Get("PORT", "8082"),
		Handler:           api.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	})
}
