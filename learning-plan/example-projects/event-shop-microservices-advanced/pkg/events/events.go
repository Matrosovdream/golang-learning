// Package events defines the message contracts shared by every service: the
// routing keys (event names) and the JSON payload structs. Services only depend
// on these definitions, never on each other.
package events

// Routing keys published on the "shop.events" topic exchange.
const (
	OrderPlaced    = "order.placed"
	StockReserved  = "stock.reserved"
	StockRejected  = "stock.rejected"
	PaymentSettled = "payment.settled"
)

// RequestedItem is a (product, quantity) as requested at checkout.
type RequestedItem struct {
	ProductID int64 `json:"product_id"`
	Quantity  int   `json:"quantity"`
}

// OrderPlaced — published by orders when a new order is created (status pending).
type OrderPlacedEvent struct {
	OrderID int64           `json:"order_id"`
	UserID  int64           `json:"user_id"`
	Items   []RequestedItem `json:"items"`
}

// ReservedItem is a priced line after stock has been reserved.
type ReservedItem struct {
	ProductID      int64  `json:"product_id"`
	ProductName    string `json:"product_name"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
}

// StockReserved — published by inventory after it decrements stock.
type StockReservedEvent struct {
	OrderID    int64          `json:"order_id"`
	Items      []ReservedItem `json:"items"`
	TotalCents int64          `json:"total_cents"`
}

// StockRejected — published by inventory when an order can't be fulfilled.
type StockRejectedEvent struct {
	OrderID int64  `json:"order_id"`
	Reason  string `json:"reason"`
}

// PaymentSettled — published by payments after a (simulated) successful charge.
type PaymentSettledEvent struct {
	OrderID     int64 `json:"order_id"`
	AmountCents int64 `json:"amount_cents"`
}
