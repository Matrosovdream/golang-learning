package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	catalogv1 "shop/proto/catalog/v1"
	ordersv1 "shop/proto/orders/v1"
	usersv1 "shop/proto/users/v1"
	"shop/services/gateway/internal/client"
)

// Handler is the public REST surface. It translates JSON <-> gRPC and maps gRPC
// status codes to HTTP status codes.
type Handler struct {
	c *client.Clients
}

func New(c *client.Clients) *Handler {
	return &Handler{c: c}
}

// Routes registers the REST endpoints and wraps them in a logging middleware.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /users", h.createUser)
	mux.HandleFunc("GET /users/{id}", h.getUser)
	mux.HandleFunc("POST /products", h.createProduct)
	mux.HandleFunc("GET /products", h.listProducts)
	mux.HandleFunc("GET /products/{id}", h.getProduct)
	mux.HandleFunc("POST /orders", h.createOrder)
	mux.HandleFunc("GET /orders/{id}", h.getOrder)
	return logging(mux)
}

// --- users ---

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if !decode(w, r, &req) {
		return
	}
	u, err := h.c.Users.CreateUser(r.Context(), &usersv1.CreateUserRequest{Email: req.Email, Name: req.Name})
	if err != nil {
		writeGRPCError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, userJSON(u))
}

func (h *Handler) getUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	u, err := h.c.Users.GetUser(r.Context(), &usersv1.GetUserRequest{Id: id})
	if err != nil {
		writeGRPCError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, userJSON(u))
}

// --- products ---

func (h *Handler) createProduct(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		PriceCents int64  `json:"price_cents"`
		Stock      int32  `json:"stock"`
	}
	if !decode(w, r, &req) {
		return
	}
	p, err := h.c.Catalog.CreateProduct(r.Context(), &catalogv1.CreateProductRequest{
		Name: req.Name, PriceCents: req.PriceCents, Stock: req.Stock,
	})
	if err != nil {
		writeGRPCError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, productJSON(p))
}

func (h *Handler) listProducts(w http.ResponseWriter, r *http.Request) {
	resp, err := h.c.Catalog.ListProducts(r.Context(), &catalogv1.ListProductsRequest{})
	if err != nil {
		writeGRPCError(w, err)
		return
	}
	out := make([]map[string]any, len(resp.GetProducts()))
	for i, p := range resp.GetProducts() {
		out[i] = productJSON(p)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) getProduct(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p, err := h.c.Catalog.GetProduct(r.Context(), &catalogv1.GetProductRequest{Id: id})
	if err != nil {
		writeGRPCError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, productJSON(p))
}

// --- orders ---

func (h *Handler) createOrder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID int64 `json:"user_id"`
		Items  []struct {
			ProductID int64 `json:"product_id"`
			Quantity  int32 `json:"quantity"`
		} `json:"items"`
	}
	if !decode(w, r, &req) {
		return
	}
	items := make([]*ordersv1.OrderLineInput, len(req.Items))
	for i, it := range req.Items {
		items[i] = &ordersv1.OrderLineInput{ProductId: it.ProductID, Quantity: it.Quantity}
	}
	o, err := h.c.Orders.CreateOrder(r.Context(), &ordersv1.CreateOrderRequest{UserId: req.UserID, Items: items})
	if err != nil {
		writeGRPCError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, orderJSON(o))
}

func (h *Handler) getOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	o, err := h.c.Orders.GetOrder(r.Context(), &ordersv1.GetOrderRequest{Id: id})
	if err != nil {
		writeGRPCError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, orderJSON(o))
}

// --- view models (snake_case JSON, independent of the protobuf field names) ---

func userJSON(u *usersv1.User) map[string]any {
	return map[string]any{"id": u.GetId(), "email": u.GetEmail(), "name": u.GetName(), "created_at": u.GetCreatedAt()}
}

func productJSON(p *catalogv1.Product) map[string]any {
	return map[string]any{
		"id": p.GetId(), "name": p.GetName(), "price_cents": p.GetPriceCents(),
		"stock": p.GetStock(), "created_at": p.GetCreatedAt(),
	}
}

func orderJSON(o *ordersv1.Order) map[string]any {
	items := make([]map[string]any, len(o.GetItems()))
	for i, it := range o.GetItems() {
		items[i] = map[string]any{
			"product_id": it.GetProductId(), "product_name": it.GetProductName(),
			"quantity": it.GetQuantity(), "unit_price_cents": it.GetUnitPriceCents(),
		}
	}
	return map[string]any{
		"id": o.GetId(), "user_id": o.GetUserId(), "status": o.GetStatus(),
		"total_cents": o.GetTotalCents(), "items": items, "created_at": o.GetCreatedAt(),
	}
}

// --- helpers ---

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// writeGRPCError maps a downstream gRPC status code onto an HTTP status code.
func writeGRPCError(w http.ResponseWriter, err error) {
	st := status.Convert(err)
	httpCode, ok := map[codes.Code]int{
		codes.InvalidArgument:    http.StatusBadRequest,
		codes.NotFound:           http.StatusNotFound,
		codes.AlreadyExists:      http.StatusConflict,
		codes.FailedPrecondition: http.StatusConflict,
		codes.Unavailable:        http.StatusServiceUnavailable, // a downstream service is down
	}[st.Code()]
	if !ok {
		httpCode = http.StatusInternalServerError
	}
	writeError(w, httpCode, st.Message())
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
