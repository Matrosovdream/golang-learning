// Command seed loads demo data (rights, tenant, roles, users, synthetic cases).
package main

import (
	"context"
	"log"

	"midigator/internal/config"
	"midigator/internal/db"
	"midigator/internal/seed"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	pool, err := db.Connect(ctx, cfg.DatabaseURL, 30)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	if err := seed.Run(ctx, pool); err != nil {
		log.Fatalf("seed: %v", err)
	}
	log.Println("seed complete: demo tenant, roles, users, and synthetic cases loaded")
}
