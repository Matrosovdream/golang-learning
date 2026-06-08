package router

import (
	"net/http"

	"blogapi/internal/handler"
	"blogapi/internal/middleware"
)

// New registers the routes and wraps them in the middleware chain.
// More specific patterns (".../comments") win over "/posts/{id}".
func New(h *handler.PostHandler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /posts", h.Create)
	mux.HandleFunc("GET /posts", h.List)
	mux.HandleFunc("GET /posts/{id}", h.Get)
	mux.HandleFunc("PUT /posts/{id}", h.Update)
	mux.HandleFunc("DELETE /posts/{id}", h.Delete)
	mux.HandleFunc("POST /posts/{id}/comments", h.AddComment)
	mux.HandleFunc("GET /posts/{id}/comments", h.ListComments)

	// First listed is the outermost wrapper: RequestID -> Logger -> Recover.
	return middleware.Chain(mux, middleware.RequestID, middleware.Logger, middleware.Recover)
}
