// Package events defines the shared event types. A package comment documents
// the package as a whole; many files can share one package name.
package events

// A const block groups related constants. With no explicit type these are
// untyped string constants — they take a type only when used.
const (
	OrderPlaced    = "order.placed"
	StockReserved  = "stock.reserved"
	StockRejected  = "stock.rejected"
	PaymentSettled = "payment.settled"
)

// A struct is a typed group of fields. The back-tick text after a field is a
// struct tag — metadata read via reflection (here by encoding/json) to map an
// exported Go field to a JSON key.
type RequestedItem struct {
	ProductID int64 `json:"product_id"` // int64: 64-bit signed integer
	Quantity  int   `json:"quantity"`   // int: implementation-sized integer
}

// Field names must be uppercase (exported) to be visible outside the package —
// and encoding/json only marshals exported fields.
type OrderPlacedEvent struct {
	OrderID int64           `json:"order_id"`
	UserID  int64           `json:"user_id"`
	Items   []RequestedItem `json:"items"` // []T is a slice: a growable sequence
}

type ReservedItem struct {
	ProductID      int64  `json:"product_id"`
	ProductName    string `json:"product_name"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
}

type StockReservedEvent struct {
	OrderID    int64          `json:"order_id"`
	Items      []ReservedItem `json:"items"`
	TotalCents int64          `json:"total_cents"`
}

type StockRejectedEvent struct {
	OrderID int64  `json:"order_id"`
	Reason  string `json:"reason"`
}

type PaymentSettledEvent struct {
	OrderID     int64 `json:"order_id"`
	AmountCents int64 `json:"amount_cents"`
}
