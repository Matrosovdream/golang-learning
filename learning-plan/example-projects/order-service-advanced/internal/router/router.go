package router

import (
	"net/http"

	"orderservice/internal/handler"
	"orderservice/internal/middleware"
)

// New registers the product and order routes and wraps them in the middleware
// chain. More specific patterns (".../pay", ".../restock") win over "/{id}".
func New(ph *handler.ProductHandler, oh *handler.OrderHandler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /products", ph.Create)
	mux.HandleFunc("GET /products", ph.List)
	mux.HandleFunc("GET /products/{id}", ph.Get)
	mux.HandleFunc("POST /products/{id}/restock", ph.Restock)

	mux.HandleFunc("POST /orders", oh.Checkout)
	mux.HandleFunc("GET /orders", oh.List)
	mux.HandleFunc("GET /orders/{id}", oh.Get)
	mux.HandleFunc("POST /orders/{id}/pay", oh.Pay)
	mux.HandleFunc("POST /orders/{id}/ship", oh.Ship)
	mux.HandleFunc("POST /orders/{id}/cancel", oh.Cancel)

	return middleware.Chain(mux, middleware.RequestID, middleware.Logger, middleware.Recover)
}
