package router

import (
	"net/http"

	"authservice/internal/handler"
	"authservice/internal/middleware"
)

// New registers the routes. /register and /login are public; /me is wrapped in
// the Auth middleware so only requests with a valid Bearer token reach it.
// The whole mux is then wrapped in the request-id/logger/recover chain.
func New(h *handler.AuthHandler, parseToken func(string) (int64, error)) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /register", h.Register)
	mux.HandleFunc("POST /login", h.Login)
	mux.Handle("GET /me", middleware.Auth(parseToken)(http.HandlerFunc(h.Me)))

	return middleware.Chain(mux, middleware.RequestID, middleware.Logger, middleware.Recover)
}
