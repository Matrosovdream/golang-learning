package main

import (
	"context"
	"log"

	"google.golang.org/grpc"

	catalogv1 "shop/proto/catalog/v1"
	"shop/pkg/db"
	"shop/pkg/grpcserve"
	"shop/services/catalog/internal/repository"
	"shop/services/catalog/internal/server"
)

func main() {
	ctx := context.Background()
	dsn := db.Getenv("DATABASE_URL", "postgres://catalog:catalog@localhost:5432/catalog?sslmode=disable")
	port := db.Getenv("GRPC_PORT", "9002")

	pool, err := db.Connect(ctx, dsn, 15)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	repo := repository.NewProductRepository(pool)
	if err := repo.Migrate(ctx); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	err = grpcserve.Run("catalog", port, func(s *grpc.Server) {
		catalogv1.RegisterCatalogServiceServer(s, server.New(repo))
	})
	if err != nil {
		log.Fatalf("serve: %v", err)
	}
}
