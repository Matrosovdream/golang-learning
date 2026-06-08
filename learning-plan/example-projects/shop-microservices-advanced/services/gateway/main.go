package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"shop/pkg/db"
	"shop/services/gateway/internal/client"
	"shop/services/gateway/internal/handler"
)

func main() {
	httpPort := db.Getenv("HTTP_PORT", "8080")
	usersAddr := db.Getenv("USERS_ADDR", "localhost:9001")
	catalogAddr := db.Getenv("CATALOG_ADDR", "localhost:9002")
	ordersAddr := db.Getenv("ORDERS_ADDR", "localhost:9003")

	clients, err := client.Dial(usersAddr, catalogAddr, ordersAddr)
	if err != nil {
		log.Fatalf("dial services: %v", err)
	}
	defer clients.Close()

	srv := &http.Server{
		Addr:         ":" + httpPort,
		Handler:      handler.New(clients).Routes(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("gateway listening on :%s (HTTP)", httpPort)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("gateway shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
}
