package router

import (
	"log"
	"net/http"
	"time"

	"urlshortener/internal/handler"
)

// New builds the HTTP handler: routes wrapped in the middleware chain.
func New(h *handler.LinkHandler) http.Handler {
	mux := http.NewServeMux()

	// Go 1.22+ method + pattern routing. More specific patterns win, so
	// "/api/stats/{code}" is matched before the catch-all "/{code}".
	mux.HandleFunc("POST /shorten", h.Shorten)
	mux.HandleFunc("GET /api/stats/{code}", h.Stats)
	mux.HandleFunc("GET /{code}", h.Redirect)

	return recoverMiddleware(loggingMiddleware(mux))
}

// loggingMiddleware logs method, path, and duration of each request.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

// recoverMiddleware turns a panic into a 500 instead of crashing the server.
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic recovered: %v", rec)
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
