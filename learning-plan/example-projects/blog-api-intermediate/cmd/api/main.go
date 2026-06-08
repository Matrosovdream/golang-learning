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

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"blogapi/internal/config"
	"blogapi/internal/handler"
	pgrepo "blogapi/internal/repository/postgres"
	"blogapi/internal/router"
	"blogapi/internal/service"
)

func main() {
	cfg := config.Load()

	db, err := connectWithRetry(cfg.DatabaseURL, 10)
	if err != nil {
		log.Fatalf("database: %v", err)
	}

	if err := pgrepo.AutoMigrate(db); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	// Wire the layers: repository -> service -> handler -> router.
	repo := pgrepo.NewPostRepository(db)
	svc := service.NewPostService(repo)
	h := handler.NewPostHandler(svc)
	r := router.New(h)

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
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
	log.Println("bye")
}

// connectWithRetry opens the GORM DB, retrying until Postgres is ready.
func connectWithRetry(dsn string, attempts int) (*gorm.DB, error) {
	var lastErr error
	for i := 0; i < attempts; i++ {
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: gormlogger.Default.LogMode(gormlogger.Info),
		})
		if err != nil {
			lastErr = err
		} else if sqlDB, e := db.DB(); e != nil {
			lastErr = e
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			e = sqlDB.PingContext(ctx)
			cancel()
			if e == nil {
				return db, nil
			}
			lastErr = e
		}
		log.Printf("waiting for database (attempt %d/%d): %v", i+1, attempts, lastErr)
		time.Sleep(2 * time.Second)
	}
	return nil, lastErr
}
