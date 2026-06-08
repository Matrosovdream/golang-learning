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

	"github.com/jmoiron/sqlx"

	// Register the pgx stdlib driver under the name "pgx" for database/sql.
	_ "github.com/jackc/pgx/v5/stdlib"

	"authservice/internal/auth"
	"authservice/internal/config"
	"authservice/internal/handler"
	pgrepo "authservice/internal/repository/postgres"
	"authservice/internal/router"
	"authservice/internal/service"
)

func main() {
	cfg := config.Load()

	db, err := connectWithRetry(cfg.DatabaseURL, 10)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	// Wire the layers. The JWT manager is used both by the service (to issue
	// tokens) and by the router's Auth middleware (to verify them).
	tokens := auth.NewJWTManager(cfg.JWTSecret, cfg.TokenTTL)
	repo := pgrepo.NewUserRepository(db)
	svc := service.NewAuthService(repo, tokens)
	h := handler.NewAuthHandler(svc)
	r := router.New(h, tokens.Parse)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	log.Println("bye")
}

// connectWithRetry opens the sqlx DB (pgx driver), retrying until Postgres is ready.
func connectWithRetry(dsn string, attempts int) (*sqlx.DB, error) {
	var lastErr error
	for i := 0; i < attempts; i++ {
		db, err := sqlx.Open("pgx", dsn)
		if err != nil {
			lastErr = err
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			err = db.PingContext(ctx)
			cancel()
			if err == nil {
				return db, nil
			}
			_ = db.Close()
			lastErr = err
		}
		log.Printf("waiting for database (attempt %d/%d): %v", i+1, attempts, lastErr)
		time.Sleep(2 * time.Second)
	}
	return nil, lastErr
}
