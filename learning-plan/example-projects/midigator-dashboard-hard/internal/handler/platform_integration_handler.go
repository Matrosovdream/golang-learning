package handler

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PlatformIntegrationHandler reports integration health for the platform plane.
// It probes the Postgres connection pool; Redis is not checked here. The router
// guards it with platform-admin middleware.
type PlatformIntegrationHandler struct {
	pool *pgxpool.Pool
}

func NewPlatformIntegrationHandler(pool *pgxpool.Pool) *PlatformIntegrationHandler {
	return &PlatformIntegrationHandler{pool: pool}
}

// Health pings the database pool and reports its status.
func (h *PlatformIntegrationHandler) Health(w http.ResponseWriter, r *http.Request) {
	database := "ok"
	status := http.StatusOK
	if err := h.pool.Ping(r.Context()); err != nil {
		database = "down"
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, map[string]any{
		"database": database,
		"status":   status,
	})
}
