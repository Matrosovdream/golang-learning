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

	"pubsubhub/internal/config"
	"pubsubhub/internal/handler"
	"pubsubhub/internal/router"
	"pubsubhub/internal/service"
)

func main() {
	cfg := config.Load()

	// Wire the layers together (this is the only place that knows every concrete
	// type). The broker holds all state in-memory behind a sync.RWMutex — there
	// is no database on purpose; the point of this project is the concurrency.
	broker := service.NewBroker(cfg.SubBuffer)
	h := handler.NewPubSubHandler(broker, cfg)
	r := router.New(h)

	// &http.Server{...} configures the server.
	srv := &http.Server{
		Addr:        ":" + cfg.Port,
		Handler:     r,
		ReadTimeout: 5 * time.Second,
		// WriteTimeout: 0 = NO write deadline. SSE responses stream for as long as
		// the client stays connected; any positive WriteTimeout would fire mid-stream
		// and kill every live subscription. The subscribe handler instead exits via
		// r.Context().Done() (client disconnect) or broker.Shutdown().
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}

	// ListenAndServe blocks, so run it on its OWN goroutine; main then waits for a
	// signal below. GOMAXPROCS caps how many OS threads run Go code in parallel —
	// every SSE connection is a goroutine multiplexed onto that small set.
	go func() {
		log.Printf("listening on :%s (GOMAXPROCS=%d, sub_buffer=%d)",
			cfg.Port, runtime.GOMAXPROCS(0), cfg.SubBuffer)
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
	// Close every subscriber channel first so all SSE handlers unblock and return;
	// then Shutdown can drain those now-finished requests instead of waiting them out.
	broker.Shutdown()

	// Give in-flight requests up to 10s to finish before forcing the close.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	log.Println("bye")
}
