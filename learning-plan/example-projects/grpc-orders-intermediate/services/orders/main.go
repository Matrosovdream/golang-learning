// orders: a gRPC service that places orders by calling catalog over gRPC. main
// wires the catalog client (with the request-id-forwarding interceptor), the
// interceptor chain, /metrics, and serves.
package main

import (
	"log"
	"os"

	"grpcorders/pkg/grpcserve"
	"grpcorders/pkg/obs"
	catalogv1 "grpcorders/proto/catalog/v1"
	ordersv1 "grpcorders/proto/orders/v1"
	"grpcorders/services/orders/internal/repository"
	"grpcorders/services/orders/internal/server"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

func main() {
	logger := obs.NewLogger("orders")
	metrics := obs.NewMetrics("orders")

	// Client connection to catalog. The client interceptor re-attaches this
	// request's id to the outgoing call, so catalog's logs share our request_id.
	catalogConn, err := grpc.NewClient(env("CATALOG_ADDR", "localhost:9002"),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(obs.RequestIDUnaryClient),
	)
	if err != nil {
		log.Fatalf("catalog client: %v", err)
	}
	defer catalogConn.Close()
	catalogClient := catalogv1.NewCatalogServiceClient(catalogConn)

	repo := repository.NewMemoryOrderRepo()

	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
		obs.RequestIDUnaryServer,
		obs.LoggingUnaryServer(logger),
		metrics.UnaryServerInterceptor(),
	))
	ordersv1.RegisterOrderServiceServer(srv, server.New(repo, catalogClient))
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
