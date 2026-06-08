package main

import (
	"context"
	"log"

	"google.golang.org/grpc"

	"shop/pkg/db"
	"shop/pkg/grpcserve"
	usersv1 "shop/proto/users/v1"
	"shop/services/users/internal/repository"
	"shop/services/users/internal/server"
)

func main() {
	ctx := context.Background()
	dsn := db.Getenv("DATABASE_URL", "postgres://users:users@localhost:5432/users?sslmode=disable")
	port := db.Getenv("GRPC_PORT", "9001")

	pool, err := db.Connect(ctx, dsn, 15)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	repo := repository.NewUserRepository(pool)
	if err := repo.Migrate(ctx); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	err = grpcserve.Run("users", port, func(s *grpc.Server) {
		usersv1.RegisterUsersServiceServer(s, server.New(repo))
	})
	if err != nil {
		log.Fatalf("serve: %v", err)
	}
}
