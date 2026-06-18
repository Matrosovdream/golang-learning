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

	"watchhub/internal/config"
	"watchhub/internal/handler"
	"watchhub/internal/repository/memory"
	"watchhub/internal/router"
	"watchhub/internal/service"
)

func main() {
	cfg := config.Load()

	// Build the concurrency stack, inner to outer:
	//   limiter (rate)  ->  checker (bounded pool + retries)  ->  scheduler.
	store := memory.NewMonitorStore(cfg.HistorySize)
	hub := service.NewEventHub()
	limiter := service.NewLimiter(cfg.RatePerSec, cfg.RateBurst)
	checker := service.NewChecker(cfg.MaxConcurrent, limiter, cfg.MaxRetries)
	sched := service.NewScheduler(store, checker, hub)
	sched.Start()

	monitorH := handler.NewMonitorHandler(sched, checker, cfg)
	eventsH := handler.NewEventsHandler(hub)
	r := router.New(monitorH, eventsH)

	srv := &http.Server{
		Addr:        ":" + cfg.Port,
		Handler:     r,
		ReadTimeout: 5 * time.Second,
		// no WriteTimeout: the SSE stream is intentionally long-lived
		IdleTimeout: 120 * time.Second,
	}

	go func() {
		log.Printf("listening on :%s (GOMAXPROCS=%d, maxConcurrent=%d, rate=%d/s)",
			cfg.Port, runtime.GOMAXPROCS(0), cfg.MaxConcurrent, cfg.RatePerSec)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("shutting down...")
	// Order matters: stop accepting requests, then drain background work.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
	sched.Stop()    // cancel every monitor goroutine and wait for them to return
	limiter.Close() // stop the refill ticker
	hub.Close()     // close every SSE subscriber
	log.Println("bye")
}
