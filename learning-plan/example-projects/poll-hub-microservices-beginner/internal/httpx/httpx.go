// Package httpx is the tiny HTTP toolkit every service shares:
// JSON in/out with a uniform error shape, one-line request logging,
// and a server runner with graceful shutdown.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

// JSON writes v as a JSON response with the given status code.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

// Error writes {"error": "..."} with the given status code, so every
// service fails with the same body shape.
func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, map[string]string{"error": msg})
}

// Decode reads the request body into v, rejecting fields v doesn't have
// — a typo like "qestion" becomes a 400 instead of a silently empty poll.
func Decode(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// Log prints one line per request: method, path, duration.
func Log(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s (%s)", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

// Serve runs srv until SIGINT/SIGTERM, then drains in-flight requests
// for up to 10 seconds. It blocks until shutdown is complete.
func Serve(name string, srv *http.Server) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("%s listening on %s", name, srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("%s: %v", name, err)
		}
	}()

	<-ctx.Done()
	log.Printf("%s shutting down", name)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("%s shutdown: %v", name, err)
	}
}
