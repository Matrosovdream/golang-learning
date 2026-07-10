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

	"pipelinehub/internal/config"
	"pipelinehub/internal/handler"
	"pipelinehub/internal/repository/memory"
	"pipelinehub/internal/router"
	"pipelinehub/internal/service"
)

func main() {
	cfg := config.Load()

	// Wire the layers together (this is the only place that knows every concrete
	// type). The in-memory store guarded by a sync.RWMutex stands in for a
	// database — the point of this project is the concurrency primitives.
	repo := memory.NewAnalysisStore()
	pipe := service.NewPipeline(repo, cfg.WorkerCount)
	h := handler.NewAnalysisHandler(pipe, cfg)
	r := router.New(h)

	// &http.Server{...} configures the server; the timeouts cap how long each
	// phase of a request may take.
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  10 * time.Second, // a large text blob can take a moment to read
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// ListenAndServe blocks, so run it on its OWN goroutine; main then waits for
	// a signal below. GOMAXPROCS is how many OS threads run Go code in parallel —
	// the pipeline's stage goroutines are multiplexed onto that small set.
	go func() {
		log.Printf("listening on :%s (GOMAXPROCS=%d, workers=%d)",
			cfg.Port, runtime.GOMAXPROCS(0), cfg.WorkerCount)
		// ErrServerClosed is the normal error after Shutdown — not a failure.
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	// A buffered channel (capacity 1) so the signal isn't missed if it arrives
	// before we're blocked on the receive.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM) // route these signals into the channel
	<-stop                                               // block here until a signal is received

	log.Println("shutting down...")
	// Give in-flight requests up to 10s to finish before forcing the close.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	log.Println("bye")
}
