package router

import (
	"log"
	"net/http"
	"time"

	"taskmanager/internal/handler"
)

// New builds the HTTP handler: the CRUD routes wrapped in middleware.
func New(h *handler.TaskHandler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /tasks", h.Create)
	mux.HandleFunc("GET /tasks", h.List)
	mux.HandleFunc("GET /tasks/{id}", h.Get)
	mux.HandleFunc("PUT /tasks/{id}", h.Update)
	mux.HandleFunc("DELETE /tasks/{id}", h.Delete)

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
