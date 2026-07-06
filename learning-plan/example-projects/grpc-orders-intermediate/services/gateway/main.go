// gateway: the public REST entrypoint. It dials catalog + orders over gRPC and
// exposes a REST API. main wires the clients, the handler, and graceful shutdown.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"grpcorders/pkg/obs"
	"grpcorders/services/gateway/internal/client"
	"grpcorders/services/gateway/internal/handler"
)

func main() {
	logger := obs.NewLogger("gateway")

	clients, err := client.Dial(
		env("CATALOG_ADDR", "localhost:9002"),
		env("ORDERS_ADDR", "localhost:9003"),
	)
	if err != nil {
		log.Fatalf("dial backends: %v", err)
	}
	defer clients.Close()

	h := handler.New(clients, logger)
	srv := &http.Server{Addr: ":" + env("HTTP_PORT", "8080"), Handler: h.Routes()}

	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
		<-stop
		logger.Info("shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	logger.Info("gateway listening", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve: %v", err)
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
