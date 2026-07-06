package service

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"

	"eventshop/pkg/broker"
	"eventshop/pkg/events"
	"eventshop/services/inventory/internal/domain"
)

type Service struct {
	repo domain.Repository // interface field
	bus  *broker.Broker    // pointer field
}

func New(repo domain.Repository, bus *broker.Broker) *Service {
	return &Service{repo: repo, bus: bus}
}

// --- product admin (HTTP) ---

func (s *Service) CreateProduct(ctx context.Context, name string, priceCents int64, stock int) (*domain.Product, error) {
	name = strings.TrimSpace(name) // strings: stdlib string helpers
	if name == "" {
		return nil, domain.ValidationError{Field: "name", Message: "is required"}
	}
	if priceCents < 0 {
		return nil, domain.ValidationError{Field: "price_cents", Message: "must be >= 0"}
	}
	if stock < 0 {
		return nil, domain.ValidationError{Field: "stock", Message: "must be >= 0"}
	}
	// &domain.Product{...}: pointer to a new struct; Create fills in its ID.
	p := &domain.Product{Name: name, PriceCents: priceCents, Stock: stock}
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *Service) GetProduct(ctx context.Context, id int64) (*domain.Product, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) ListProducts(ctx context.Context) ([]domain.Product, error) {
	return s.repo.List(ctx)
}

// --- event handler ---

func (s *Service) OnOrderPlaced(ctx context.Context, e events.OrderPlacedEvent) error {
	lines := make([]domain.StockLine, len(e.Items)) // preallocated slice
	for i, it := range e.Items {
		lines[i] = domain.StockLine{ProductID: it.ProductID, Quantity: it.Quantity}
	}

	reserved, err := s.repo.Reserve(ctx, lines)
	if err != nil {
		var stockErr domain.InsufficientStockError
		// A tagless switch (no condition): each case is a boolean — a clean
		// if / else-if chain.
		switch {
		case errors.As(err, &stockErr): // matches a specific error type via &stockErr
			return s.publishRejected(ctx, e.OrderID, stockErr.Error())
		case errors.Is(err, domain.ErrProductNotFound): // matches a sentinel value
			return s.publishRejected(ctx, e.OrderID, "a product does not exist")
		default:
			return err
		}
	}

	var total int64 // zero value is 0
	items := make([]events.ReservedItem, len(reserved))
	for i, l := range reserved {
		items[i] = events.ReservedItem{
			ProductID:      l.ProductID,
			ProductName:    l.ProductName,
			Quantity:       l.Quantity,
			UnitPriceCents: l.UnitPriceCents,
		}
		total += l.UnitPriceCents * int64(l.Quantity) // int64(...) is a type conversion
	}
	body, _ := json.Marshal(events.StockReservedEvent{OrderID: e.OrderID, Items: items, TotalCents: total})
	log.Printf("order %d: stock reserved (total %d cents)", e.OrderID, total)
	return s.bus.Publish(ctx, events.StockReserved, body)
}

// Lowercase name = unexported: callable only inside this package.
func (s *Service) publishRejected(ctx context.Context, orderID int64, reason string) error {
	body, _ := json.Marshal(events.StockRejectedEvent{OrderID: orderID, Reason: reason})
	log.Printf("order %d: stock rejected (%s)", orderID, reason)
	return s.bus.Publish(ctx, events.StockRejected, body)
}
