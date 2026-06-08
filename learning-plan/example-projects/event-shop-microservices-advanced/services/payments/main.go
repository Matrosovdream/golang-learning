package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"

	"eventshop/pkg/broker"
	"eventshop/pkg/db"
	"eventshop/pkg/events"
	"eventshop/services/payments/internal/repository"
	"eventshop/services/payments/internal/service"
)

const exchange = "shop.events"

func main() {
	ctx := context.Background()
	dsn := db.Getenv("DATABASE_URL", "postgres://payments:payments@localhost:5432/payments?sslmode=disable")
	amqpURL := db.Getenv("AMQP_URL", "amqp://guest:guest@localhost:5672/")

	pool, err := db.Connect(ctx, dsn, 15)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	repo := repository.NewPaymentRepository(pool)
	if err := repo.Migrate(ctx); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	bus, err := broker.Connect(amqpURL, exchange, 30)
	if err != nil {
		log.Fatalf("broker: %v", err)
	}
	defer bus.Close()

	svc := service.New(repo, bus)

	err = bus.Consume("payments.events", []string{events.StockReserved},
		func(_ string, body []byte) error {
			var e events.StockReservedEvent
			if err := json.Unmarshal(body, &e); err != nil {
				return err
			}
			return svc.OnStockReserved(context.Background(), e)
		})
	if err != nil {
		log.Fatalf("consume: %v", err)
	}

	log.Println("payments consuming stock.reserved")
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("payments shutting down...")
}
