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

// Package-level const, visible to the whole package.
const exchange = "shop.events"

// main is the program entry point: no parameters, no return value.
func main() {
	ctx := context.Background() // a root context that is never cancelled

	dsn := db.Getenv("DATABASE_URL", "postgres://orders:orders@localhost:5432/orders?sslmode=disable")
	amqpURL := db.Getenv("AMQP_URL", "amqp://guest:guest@localhost:5672/")
	port := db.Getenv("HTTP_PORT", "8080")

	// pool is a *pgxpool.Pool. log.Fatalf logs then calls os.Exit(1).
	pool, err := db.Connect(ctx, dsn, 15)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close() // runs when main returns

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

	// The last argument is a function literal (closure) passed as a value.
	err = bus.Consume("orders.events",
		[]string{events.StockReserved, events.StockRejected, events.PaymentSettled},
		func(routingKey string, body []byte) error {
			c := context.Background()
			// A switch on the routing-key string. Each case declares its own
			// typed `e` and decodes into it via &e.
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

	// &http.Server{...}: the address of a struct literal built with named fields.
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      httpapi.New(svc).Routes(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	// `go` runs ListenAndServe in a goroutine — it blocks, so it needs its own
	// goroutine while main continues to the signal wait below.
	go func() {
		log.Printf("orders listening on :%s (HTTP) + consuming events", port)
		// errors.Is: ErrServerClosed is the expected error after Shutdown.
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	// make(chan os.Signal, 1): a buffered channel (capacity 1) of OS signals.
	stop := make(chan os.Signal, 1)
	// signal.Notify relays the listed signals into the channel instead of killing us.
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	// <-stop receives from the channel; it BLOCKS here until a signal arrives,
	// which is what keeps main (and the whole program) running.
	<-stop
	log.Println("orders shutting down...")
	// cancel is a func value; defer cancel() releases the context's resources.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
