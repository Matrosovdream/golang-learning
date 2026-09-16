package router

import (
	"log"
	"net/http"
	"time"

	"p2pchain/internal/handler"
)

// New builds the HTTP handler: routes plus middleware.
func New(h *handler.NodeHandler) http.Handler {
	mux := http.NewServeMux()
	// chain & transactions (as in coinledger)
	mux.HandleFunc("POST /txs", h.SubmitTx)
	mux.HandleFunc("GET /txs/{hash}", h.GetTx)
	mux.HandleFunc("GET /accounts/{address}", h.GetAccount)
	mux.HandleFunc("GET /mempool", h.Mempool)
	mux.HandleFunc("POST /mine", h.Mine)
	mux.HandleFunc("GET /blocks", h.ListBlocks)
	mux.HandleFunc("GET /blocks/latest", h.LatestBlock)
	mux.HandleFunc("GET /blocks/{ref}", h.GetBlock) // height or hash (a hash may be off-chain)
	mux.HandleFunc("GET /chain/validate", h.Validate)
	// the network
	mux.HandleFunc("GET /node/status", h.Status)
	mux.HandleFunc("GET /peers", h.Peers)
	mux.HandleFunc("GET /forks", h.Forks)
	mux.HandleFunc("GET /events", h.Events)
	// chaos
	mux.HandleFunc("POST /debug/partition", h.Partition)
	mux.HandleFunc("DELETE /debug/partition", h.Heal)
	mux.HandleFunc("POST /debug/miner", h.SetMiner)
	mux.HandleFunc("GET /healthz", h.Health)

	return recoverMiddleware(loggingMiddleware(mux))
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/events" { // a stream stays open: logging its duration is noise
			next.ServeHTTP(w, r)
			return
		}
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
				http.Error(w, `{"error":"internal server error","code":"internal"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
