package router

import (
	"log"
	"net/http"
	"time"

	"coinledger/internal/handler"
)

// New builds the HTTP handler: the routes wrapped in middleware.
func New(h *handler.NodeHandler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /txs", h.SubmitTx)
	mux.HandleFunc("GET /txs/{hash}", h.GetTx)
	mux.HandleFunc("GET /accounts/{address}", h.GetAccount)
	mux.HandleFunc("GET /mempool", h.Mempool)
	mux.HandleFunc("POST /mine", h.Mine)
	mux.HandleFunc("GET /blocks", h.ListBlocks)
	mux.HandleFunc("GET /blocks/latest", h.LatestBlock)
	mux.HandleFunc("GET /blocks/{height}", h.GetBlock)
	mux.HandleFunc("GET /blocks/{height}/txs/{hash}/proof", h.Proof)
	mux.HandleFunc("GET /chain/validate", h.Validate)
	mux.HandleFunc("GET /stats", h.Stats)
	mux.HandleFunc("GET /healthz", h.Health)

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

// recoverMiddleware turns a panic into a 500 instead of crashing the node.
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic recovered: %v", rec)
				http.Error(w, `{"error":"internal server error","code":"internal"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
