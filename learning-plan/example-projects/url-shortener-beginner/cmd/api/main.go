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

	"github.com/jackc/pgx/v5/pgxpool"

	"urlshortener/internal/config"
	"urlshortener/internal/handler"
	pgrepo "urlshortener/internal/repository/postgres"
	"urlshortener/internal/router"
	"urlshortener/internal/service"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	// Postgres may still be booting when the app starts under compose.
	pool, err := connectWithRetry(ctx, cfg.DatabaseURL, 10)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	// Wire the layers: repository -> service -> handler -> router.
	repo := pgrepo.NewLinkRepository(pool)
	svc := service.NewLinkService(repo)
	h := handler.NewLinkHandler(svc, cfg)
	r := router.New(h)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Serve in the background so main can wait for a shutdown signal.
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

// connectWithRetry waits for Postgres to accept connections, retrying a few
// times so the app survives the brief window before the DB is ready.
func connectWithRetry(ctx context.Context, dsn string, attempts int) (*pgxpool.Pool, error) {
	var lastErr error
	for i := 0; i < attempts; i++ {
		pool, err := pgxpool.New(ctx, dsn)
		if err == nil {
			pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err = pool.Ping(pingCtx)
			cancel()
			if err == nil {
				return pool, nil
			}
			pool.Close()
		}
		lastErr = err
		log.Printf("waiting for database (attempt %d/%d): %v", i+1, attempts, err)
		time.Sleep(2 * time.Second)
	}
	return nil, lastErr
}
