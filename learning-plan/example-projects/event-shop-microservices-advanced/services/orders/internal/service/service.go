package service

import (
	"context"
	"encoding/json"
	"fmt"

	"eventshop/pkg/broker"
	"eventshop/pkg/events"
	"eventshop/services/orders/internal/domain"
)

// repo is the interface type domain.Repository (any implementation fits); bus is
// a *broker.Broker pointer. Depending on an interface rather than a concrete
// type is how Go does dependency inversion.
type Service struct {
	repo domain.Repository
	bus  *broker.Broker
}

func New(repo domain.Repository, bus *broker.Broker) *Service {
	return &Service{repo: repo, bus: bus}
}

func (s *Service) PlaceOrder(ctx context.Context, userID int64, items []events.RequestedItem) (*domain.Order, error) {
	if userID <= 0 {
		// Returning a struct value where an `error` is expected works because
		// ValidationError satisfies the error interface.
		return nil, domain.ValidationError{Field: "user_id", Message: "is required"}
	}
	if len(items) == 0 { // len() works on slices, arrays, maps, strings, channels
		return nil, domain.ValidationError{Field: "items", Message: "at least one item is required"}
	}
	for _, it := range items {
		if it.ProductID <= 0 {
			return nil, domain.ValidationError{Field: "product_id", Message: "is invalid"}
		}
		if it.Quantity <= 0 {
			return nil, domain.ValidationError{Field: "quantity", Message: "must be > 0"}
		}
	}

	// &domain.Order{...} is a pointer to a new struct; Create scans the generated
	// id back into it through that pointer.
	order := &domain.Order{UserID: userID, Status: domain.StatusPending}
	if err := s.repo.Create(ctx, order); err != nil {
		return nil, err
	}

	// json.Marshal returns (bytes, error); the blank identifier _ drops the error.
	body, _ := json.Marshal(events.OrderPlacedEvent{OrderID: order.ID, UserID: userID, Items: items})
	if err := s.bus.Publish(ctx, events.OrderPlaced, body); err != nil {
		return nil, fmt.Errorf("publish order.placed: %w", err)
	}
	return order, nil
}

func (s *Service) GetOrder(ctx context.Context, id int64) (*domain.Order, error) {
	return s.repo.GetByID(ctx, id)
}

// --- event handlers (called by the consumer) ---

func (s *Service) OnStockReserved(ctx context.Context, e events.StockReservedEvent) error {
	// make([]T, n) preallocates a slice of length n, filled by index below.
	items := make([]domain.OrderItem, len(e.Items))
	for i, it := range e.Items { // two-value range: index i, value it
		items[i] = domain.OrderItem{
			ProductID:      it.ProductID,
			ProductName:    it.ProductName,
			Quantity:       it.Quantity,
			UnitPriceCents: it.UnitPriceCents,
		}
	}
	return s.repo.SetReserved(ctx, e.OrderID, items, e.TotalCents)
}

func (s *Service) OnStockRejected(ctx context.Context, e events.StockRejectedEvent) error {
	return s.repo.SetStatus(ctx, e.OrderID, domain.StatusCancelled)
}

func (s *Service) OnPaymentSettled(ctx context.Context, e events.PaymentSettledEvent) error {
	return s.repo.SetStatus(ctx, e.OrderID, domain.StatusConfirmed)
}
