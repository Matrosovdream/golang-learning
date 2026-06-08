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
	"eventshop/services/orders/internal/httpapi"
	"eventshop/services/orders/internal/repository"
	"eventshop/services/orders/internal/service"
)

const exchange = "shop.events"

func main() {
	ctx := context.Background()
	dsn := db.Getenv("DATABASE_URL", "postgres://orders:orders@localhost:5432/orders?sslmode=disable")
	amqpURL := db.Getenv("AMQP_URL", "amqp://guest:guest@localhost:5672/")
	port := db.Getenv("HTTP_PORT", "8080")

	pool, err := db.Connect(ctx, dsn, 15)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	repo := repository.NewOrderRepository(pool)
	if err := repo.Migrate(ctx); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	bus, err := broker.Connect(amqpURL, exchange, 30)
	if err != nil {
		log.Fatalf("broker: %v", err)
	}
	defer bus.Close()

	svc := service.New(repo, bus)

	// React to the downstream events that update an order's status.
	err = bus.Consume("orders.events",
		[]string{events.StockReserved, events.StockRejected, events.PaymentSettled},
		func(routingKey string, body []byte) error {
			c := context.Background()
			switch routingKey {
			case events.StockReserved:
				var e events.StockReservedEvent
				if err := json.Unmarshal(body, &e); err != nil {
					return err
				}
				return svc.OnStockReserved(c, e)
			case events.StockRejected:
				var e events.StockRejectedEvent
				if err := json.Unmarshal(body, &e); err != nil {
					return err
				}
				return svc.OnStockRejected(c, e)
			case events.PaymentSettled:
				var e events.PaymentSettledEvent
				if err := json.Unmarshal(body, &e); err != nil {
					return err
				}
				return svc.OnPaymentSettled(c, e)
			}
			return nil
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
		log.Printf("orders listening on :%s (HTTP) + consuming events", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("orders shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
