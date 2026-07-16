// orders: gRPC service placing orders in its own Postgres, calling catalog over gRPC.
package main

import (
	"context"
	"log"
	"os"

	"grpcobs/pkg/db"
	"grpcobs/pkg/grpcserve"
	"grpcobs/pkg/obs"
	catalogv1 "grpcobs/proto/catalog/v1"
	ordersv1 "grpcobs/proto/orders/v1"
	"grpcobs/services/orders/internal/repository"
	"grpcobs/services/orders/internal/server"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

func main() {
	logger := obs.NewLogger("orders")
	metrics := obs.NewMetrics("orders")
	ctx := context.Background()

	pool, err := db.Connect(ctx, env("DATABASE_URL", "postgres://orders:orders@localhost:5432/orders?sslmode=disable"))
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool, repository.Schema); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	// catalog client with request-id forwarding
	catalogConn, err := grpc.NewClient(env("CATALOG_ADDR", "localhost:9002"),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(obs.RequestIDUnaryClient),
	)
	if err != nil {
		log.Fatalf("catalog client: %v", err)
	}
	defer catalogConn.Close()
	catalogClient := catalogv1.NewCatalogServiceClient(catalogConn)

	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
		obs.RequestIDUnaryServer,
		obs.LoggingUnaryServer(logger),
		metrics.UnaryServerInterceptor(),
	))
	ordersv1.RegisterOrderServiceServer(srv, server.New(repository.NewOrderRepo(pool), catalogClient))

	hs := health.NewServer()
	healthpb.RegisterHealthServer(srv, hs)
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	hs.SetServingStatus("orders.v1.OrderService", healthpb.HealthCheckResponse_SERVING)

	reflection.Register(srv)
	obs.ServeMetrics(":"+env("METRICS_PORT", "2112"), metrics, logger)

	if err := grpcserve.Serve(logger, "orders", env("GRPC_PORT", "9003"), srv); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
