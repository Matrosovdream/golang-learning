package main

import (
	"context"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	catalogv1 "shop/proto/catalog/v1"
	ordersv1 "shop/proto/orders/v1"
	usersv1 "shop/proto/users/v1"
	"shop/pkg/db"
	"shop/pkg/grpcserve"
	"shop/services/orders/internal/repository"
	"shop/services/orders/internal/server"
)

func main() {
	ctx := context.Background()
	dsn := db.Getenv("DATABASE_URL", "postgres://orders:orders@localhost:5432/orders?sslmode=disable")
	port := db.Getenv("GRPC_PORT", "9003")
	usersAddr := db.Getenv("USERS_ADDR", "localhost:9001")
	catalogAddr := db.Getenv("CATALOG_ADDR", "localhost:9002")

	pool, err := db.Connect(ctx, dsn, 15)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	repo := repository.NewOrderRepository(pool)
	if err := repo.Migrate(ctx); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	// gRPC clients for the downstream services. NewClient is lazy: the actual
	// connection is established on the first RPC.
	usersConn, err := grpc.NewClient(usersAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("users client: %v", err)
	}
	defer usersConn.Close()
	catalogConn, err := grpc.NewClient(catalogAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("catalog client: %v", err)
	}
	defer catalogConn.Close()

	srv := server.New(
		repo,
		usersv1.NewUsersServiceClient(usersConn),
		catalogv1.NewCatalogServiceClient(catalogConn),
	)

	err = grpcserve.Run("orders", port, func(s *grpc.Server) {
		ordersv1.RegisterOrdersServiceServer(s, srv)
	})
	if err != nil {
		log.Fatalf("serve: %v", err)
	}
}
