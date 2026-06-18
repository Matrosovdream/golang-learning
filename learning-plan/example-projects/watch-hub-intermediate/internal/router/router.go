package router

import (
	"log"
	"net/http"
	"time"

	"watchhub/internal/handler"
)

// New wires the routes and middleware. The SSE stream is mounted outside the
// logging middleware's timing (it is a long-lived connection, not a request).
func New(m *handler.MonitorHandler, e *handler.EventsHandler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /monitors", m.Create)
	mux.HandleFunc("GET /monitors", m.List)
	mux.HandleFunc("GET /monitors/{id}", m.Get)
	mux.HandleFunc("DELETE /monitors/{id}", m.Delete)
	mux.HandleFunc("POST /monitors/{id}/check", m.CheckNow)

	mux.HandleFunc("POST /checks", m.Batch)
	mux.HandleFunc("GET /stats", m.Stats)
	mux.HandleFunc("GET /healthz", m.Health)

	mux.HandleFunc("GET /events", e.Stream)

	return recoverMiddleware(loggingMiddleware(mux))
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

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
