// Command worker drains the asynq queues (currently webhook:process): it ingests
// inbound webhooks into the right case type and publishes CaseReceived events so
// notifications are created.
package main

import (
	"context"
	"log"

	"midigator/internal/config"
	"midigator/internal/db"
	"midigator/internal/events"
	pg "midigator/internal/repository/postgres"
	"midigator/internal/service"
	"midigator/internal/worker"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	pool, err := db.Connect(ctx, cfg.DatabaseURL, 30)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	// Repositories the processor + subscribers need.
	chargebackRepo := pg.NewChargebackRepository(pool)
	preventionRepo := pg.NewPreventionRepository(pool)
	orderValidationRepo := pg.NewOrderValidationRepository(pool)
	rdrRepo := pg.NewRdrRepository(pool)
	webhookRepo := pg.NewWebhookRepository(pool)
	userRepo := pg.NewUserRepository(pool)
	notificationRepo := pg.NewNotificationRepository(pool)

	// Event bus + subscribers (CaseReceived -> notify tenant users).
	bus := events.NewBus()
	service.RegisterEventSubscribers(bus, notificationRepo, userRepo)

	proc := worker.NewProcessor(chargebackRepo, preventionRepo, orderValidationRepo, rdrRepo, webhookRepo, bus)
	srv, mux := worker.NewServer(cfg.RedisAddr, proc)

	log.Printf("worker draining queues (redis=%s)", cfg.RedisAddr)
	if err := srv.Run(mux); err != nil {
		log.Fatalf("worker: %v", err)
	}
}
