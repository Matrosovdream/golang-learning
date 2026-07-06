// catalog: a gRPC service owning products + stock. main only wires things:
// build deps, install the three interceptors, expose /metrics, serve.
package main

import (
	"log"
	"os"

	"grpcorders/pkg/grpcserve"
	"grpcorders/pkg/obs"
	catalogv1 "grpcorders/proto/catalog/v1"
	"grpcorders/services/catalog/internal/repository"
	"grpcorders/services/catalog/internal/server"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	logger := obs.NewLogger("catalog")
	metrics := obs.NewMetrics("catalog")

	repo := repository.NewMemoryProductRepo()

	// The interceptor chain: request-id (so downstream logs correlate) → logging
	// → metrics. Every RPC gets all three, for free.
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
		obs.RequestIDUnaryServer,
		obs.LoggingUnaryServer(logger),
		metrics.UnaryServerInterceptor(),
	))
	catalogv1.RegisterCatalogServiceServer(srv, server.New(repo))
	reflection.Register(srv)

	obs.ServeMetrics(":"+env("METRICS_PORT", "2112"), metrics, logger)

	if err := grpcserve.Serve(logger, "catalog", env("GRPC_PORT", "9002"), srv); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
