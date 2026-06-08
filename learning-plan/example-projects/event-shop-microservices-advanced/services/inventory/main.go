package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"eventshop/pkg/broker"
	"eventshop/pkg/db"
	"eventshop/pkg/events"
	"eventshop/services/inventory/internal/httpapi"
	"eventshop/services/inventory/internal/repository"
	"eventshop/services/inventory/internal/service"
)

const exchange = "shop.events"

func main() {
	ctx := context.Background()
	dsn := db.Getenv("DATABASE_URL", "postgres://inventory:inventory@localhost:5432/inventory?sslmode=disable")
	amqpURL := db.Getenv("AMQP_URL", "amqp://guest:guest@localhost:5672/")
	port := db.Getenv("HTTP_PORT", "8081")

	pool, err := db.Connect(ctx, dsn, 15)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	repo := repository.NewProductRepository(pool)
	if err := repo.Migrate(ctx); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	bus, err := broker.Connect(amqpURL, exchange, 30)
	if err != nil {
		log.Fatalf("broker: %v", err)
	}
	defer bus.Close()

	svc := service.New(repo, bus)

	// React to new orders by reserving stock.
	err = bus.Consume("inventory.events", []string{events.OrderPlaced},
		func(_ string, body []byte) error {
			var e events.OrderPlacedEvent
			if err := json.Unmarshal(body, &e); err != nil {
				return err
			}
			return svc.OnOrderPlaced(context.Background(), e)
		})
	if err != nil {
		log.Fatalf("consume: %v", err)
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      httpapi.New(svc).Routes(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	go func() {
		log.Printf("inventory listening on :%s (HTTP) + consuming order.placed", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("inventory shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
