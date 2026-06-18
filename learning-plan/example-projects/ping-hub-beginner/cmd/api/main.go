package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"pinghub/internal/config"
	"pinghub/internal/handler"
	"pinghub/internal/repository/memory"
	"pinghub/internal/router"
	"pinghub/internal/service"
)

func main() {
	cfg := config.Load()

	// In-memory store (no database): the point of this project is the
	// concurrency primitives, not persistence. The store is guarded by a
	// sync.RWMutex so concurrent requests can read it safely.
	repo := memory.NewCheckStore()
	checker := service.NewChecker(repo, cfg.WorkerCount)
	h := handler.NewCheckHandler(checker, cfg)
	r := router.New(h)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 60 * time.Second, // a batch of slow URLs can take a while
		IdleTimeout:  60 * time.Second,
	}

	// ListenAndServe blocks, so run it on its own goroutine; main then waits
	// for an OS signal below. GOMAXPROCS is how many OS threads can run Go
	// code in parallel — the goroutines above are multiplexed onto them.
	go func() {
		log.Printf("listening on :%s (GOMAXPROCS=%d, workers=%d)",
			cfg.Port, runtime.GOMAXPROCS(0), cfg.WorkerCount)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	// Block until SIGINT/SIGTERM arrives on the channel.
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
