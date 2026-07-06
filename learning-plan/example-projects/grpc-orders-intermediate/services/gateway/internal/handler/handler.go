// Package handler is the gateway's REST surface. It decodes JSON, calls the gRPC
// backends, and maps gRPC status codes onto HTTP status codes. A middleware mints
// the request-id at the edge so every downstream service logs the same id.
package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"grpcorders/pkg/obs"
	catalogv1 "grpcorders/proto/catalog/v1"
	ordersv1 "grpcorders/proto/orders/v1"
	"grpcorders/services/gateway/internal/client"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Handler struct {
	clients *client.Clients
	logger  *slog.Logger
}

func New(clients *client.Clients, logger *slog.Logger) *Handler {
	return &Handler{clients: clients, logger: logger}
}

// Routes builds the mux and wraps it in the request-id middleware.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /products", h.createProduct)
	mux.HandleFunc("GET /products", h.listProducts)
	mux.HandleFunc("GET /products/{id}", h.getProduct)
	mux.HandleFunc("POST /orders", h.createOrder)
	mux.HandleFunc("GET /orders/{id}", h.getOrder)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	return h.withRequestID(mux)
}

// withRequestID mints (or reuses) a correlation id and stores it in the context.
func (h *Handler) withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-Id")
		if reqID == "" {
			reqID = obs.NewID()
		}
		w.Header().Set("X-Request-Id", reqID)
		h.logger.Info("http_request", "method", r.Method, "path", r.URL.Path, "request_id", reqID)
		next.ServeHTTP(w, r.WithContext(obs.WithRequestID(r.Context(), reqID)))
	})
}

// ---- products ----

func (h *Handler) createProduct(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name       string `json:"name"`
		PriceCents int64  `json:"price_cents"`
		Stock      int32  `json:"stock"`
	}
	if !decode(w, r, &body) {
		return
	}
	ctx, cancel := timeout(r)
	defer cancel()
	p, err := h.clients.Catalog.CreateProduct(ctx, &catalogv1.CreateProductRequest{
		Name: body.Name, PriceCents: body.PriceCents, Stock: body.Stock,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, productJSON(p))
}

func (h *Handler) listProducts(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := timeout(r)
	defer cancel()
	reply, err := h.clients.Catalog.ListProducts(ctx, &catalogv1.ListProductsRequest{})
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]any, 0, len(reply.GetProducts()))
	for _, p := range reply.GetProducts() {
		out = append(out, productJSON(p))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) getProduct(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx, cancel := timeout(r)
	defer cancel()
	p, err := h.clients.Catalog.GetProduct(ctx, &catalogv1.GetProductRequest{Id: id})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, productJSON(p))
}

// ---- orders ----

func (h *Handler) createOrder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items []struct {
			ProductID int64 `json:"product_id"`
			Quantity  int32 `json:"quantity"`
		} `json:"items"`
	}
	if !decode(w, r, &body) {
		return
	}
	lines := make([]*ordersv1.CreateOrderLine, 0, len(body.Items))
	for _, it := range body.Items {
		lines = append(lines, &ordersv1.CreateOrderLine{ProductId: it.ProductID, Quantity: it.Quantity})
	}
	ctx, cancel := timeout(r)
	defer cancel()
	o, err := h.clients.Orders.CreateOrder(ctx, &ordersv1.CreateOrderRequest{Items: lines})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, orderJSON(o))
}

func (h *Handler) getOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx, cancel := timeout(r)
	defer cancel()
	o, err := h.clients.Orders.GetOrder(ctx, &ordersv1.GetOrderRequest{Id: id})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, orderJSON(o))
}

// ---- shaping + helpers ----

func productJSON(p *catalogv1.Product) map[string]any {
	return map[string]any{"id": p.GetId(), "name": p.GetName(), "price_cents": p.GetPriceCents(), "stock": p.GetStock()}
}

func orderJSON(o *ordersv1.Order) map[string]any {
	items := make([]any, 0, len(o.GetItems()))
	for _, it := range o.GetItems() {
		items = append(items, map[string]any{
			"product_id": it.GetProductId(), "quantity": it.GetQuantity(),
			"name": it.GetName(), "price_cents": it.GetPriceCents(),
		})
	}
	return map[string]any{"id": o.GetId(), "status": o.GetStatus(), "total_cents": o.GetTotalCents(), "items": items}
}

func timeout(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), 3*time.Second)
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "id must be an integer"})
		return 0, false
	}
	return id, true
}

// writeError is the gateway's core job: gRPC status code -> HTTP status.
func writeError(w http.ResponseWriter, err error) {
	code := status.Code(err)
	httpStatus := map[codes.Code]int{
		codes.InvalidArgument:    http.StatusBadRequest,
		codes.NotFound:           http.StatusNotFound,
		codes.AlreadyExists:      http.StatusConflict,
		codes.FailedPrecondition: http.StatusConflict, // e.g. insufficient stock
		codes.Unavailable:        http.StatusServiceUnavailable,
		codes.DeadlineExceeded:   http.StatusGatewayTimeout,
	}[code]
	if httpStatus == 0 {
		httpStatus = http.StatusInternalServerError
	}
	writeJSON(w, httpStatus, map[string]any{"error": status.Convert(err).Message(), "code": code.String()})
}

func writeJSON(w http.ResponseWriter, statusCode int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(v)
}
