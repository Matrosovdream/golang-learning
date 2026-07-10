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

	"crawlerhub/internal/config"
	"crawlerhub/internal/handler"
	"crawlerhub/internal/repository/memory"
	"crawlerhub/internal/router"
	"crawlerhub/internal/service"
)

func main() {
	cfg := config.Load()

	// Wire the layers together (this is the only place that knows every concrete
	// type). The in-memory store guarded by a sync.RWMutex stands in for a
	// database — the point of this project is the concurrency primitives.
	repo := memory.NewCrawlStore()
	crawler := service.NewCrawler(repo, cfg.MaxConcurrency)
	h := handler.NewCrawlHandler(crawler, cfg)
	site := handler.NewSiteHandler(cfg.SitePages) // the built-in mini-site to crawl
	r := router.New(h, site)

	// &http.Server{...} configures the server; the timeouts cap how long each
	// phase of a request may take. WriteTimeout is generous: a deep crawl can run
	// a while.
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// ListenAndServe blocks, so run it on its OWN goroutine; main then waits for a
	// signal below. GOMAXPROCS is how many OS threads run Go code in parallel —
	// the crawl goroutines are multiplexed onto that small set of threads, while
	// MaxConcurrency caps how many fetches are in flight at once.
	go func() {
		log.Printf("listening on :%s (GOMAXPROCS=%d, max_concurrency=%d, site_pages=%d)",
			cfg.Port, runtime.GOMAXPROCS(0), cfg.MaxConcurrency, cfg.SitePages)
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
