// catalog: gRPC service owning products + stock in its own Postgres database.
package main

import (
	"context"
	"log"
	"os"

	"grpcobs/pkg/db"
	"grpcobs/pkg/grpcserve"
	"grpcobs/pkg/obs"
	catalogv1 "grpcobs/proto/catalog/v1"
	"grpcobs/services/catalog/internal/repository"
	"grpcobs/services/catalog/internal/server"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

func main() {
	logger := obs.NewLogger("catalog")
	metrics := obs.NewMetrics("catalog")
	ctx := context.Background()

	pool, err := db.Connect(ctx, env("DATABASE_URL", "postgres://catalog:catalog@localhost:5432/catalog?sslmode=disable"))
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool, repository.Schema); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
		obs.RequestIDUnaryServer,
		obs.LoggingUnaryServer(logger),
		metrics.UnaryServerInterceptor(),
	))
	catalogv1.RegisterCatalogServiceServer(srv, server.New(repository.NewProductRepo(pool)))

	// Standard gRPC health service — Compose's healthcheck probes this.
	hs := health.NewServer()
	healthpb.RegisterHealthServer(srv, hs)
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	hs.SetServingStatus("catalog.v1.CatalogService", healthpb.HealthCheckResponse_SERVING)

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
