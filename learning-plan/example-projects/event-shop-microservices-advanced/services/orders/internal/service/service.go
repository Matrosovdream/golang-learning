package service

import (
	"context"
	"encoding/json"
	"fmt"

	"eventshop/pkg/broker"
	"eventshop/pkg/events"
	"eventshop/services/orders/internal/domain"
)

// Service holds the order business logic. It persists orders and publishes the
// order.placed event; the downstream status changes arrive back as events.
type Service struct {
	repo domain.Repository
	bus  *broker.Broker
}

func New(repo domain.Repository, bus *broker.Broker) *Service {
	return &Service{repo: repo, bus: bus}
}

// PlaceOrder validates input, stores a pending order, and publishes
// order.placed. It returns immediately — fulfilment happens asynchronously.
func (s *Service) PlaceOrder(ctx context.Context, userID int64, items []events.RequestedItem) (*domain.Order, error) {
	if userID <= 0 {
		return nil, domain.ValidationError{Field: "user_id", Message: "is required"}
	}
	if len(items) == 0 {
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

	order := &domain.Order{UserID: userID, Status: domain.StatusPending}
	if err := s.repo.Create(ctx, order); err != nil {
		return nil, err
	}

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
	items := make([]domain.OrderItem, len(e.Items))
	for i, it := range e.Items {
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
