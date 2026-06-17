package router

import (
	"net/http"

	"authnetservice/internal/handler"
	"authnetservice/internal/middleware"
)

func New(h *handler.AuthHandler, parseToken func(string) (int64, error)) http.Handler {

	mux := http.NewServeMux()

	mux.HandleFunc("POST /register", h.Register)
	mux.HandleFunc("POST /login", h.Login)
	mux.Handle("GET /me", middleware.Auth(parseToken)(http.HandlerFunc(h.Me)))

	return middleware.Chain(mux, middleware.RequestID, middleware.Logger, middleware.Recover)

}
