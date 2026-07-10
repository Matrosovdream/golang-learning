package router

import (
	"log"
	"net/http"
	"time"

	"crawlerhub/internal/handler"
)

// New builds the HTTP handler: the routes wrapped in middleware. It returns the
// http.Handler interface, so main doesn't need to know the concrete type.
func New(h *handler.CrawlHandler, site *handler.SiteHandler) http.Handler {
	mux := http.NewServeMux()
	// Go 1.22+ patterns include the METHOD and {wildcards} in the string.
	mux.HandleFunc("POST /crawls", h.Create)
	mux.HandleFunc("GET /crawls", h.List)
	mux.HandleFunc("GET /crawls/{id}", h.Get) // {id} is read via r.PathValue("id")
	mux.HandleFunc("GET /stats", h.Stats)
	mux.HandleFunc("GET /site/{page}", site.Page) // the built-in mini-site to crawl
	mux.HandleFunc("GET /healthz", h.Health)

	// Middleware composes by wrapping: recover wraps logging wraps the mux.
	return recoverMiddleware(loggingMiddleware(mux))
}

// loggingMiddleware logs method, path, and duration of each request. It takes a
// handler and returns a new one — that's the middleware pattern.
func loggingMiddleware(next http.Handler) http.Handler {
	// http.HandlerFunc adapts a plain func into an http.Handler.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r) // call the wrapped handler
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

// recoverMiddleware turns a panic into a 500 instead of crashing the server.
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// defer + recover() catches a panic from any handler below this point.
		defer func() {
			if rec := recover(); rec != nil { // recover() is non-nil only during a panic
				log.Printf("panic recovered: %v", rec)
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
