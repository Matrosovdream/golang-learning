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

// Service holds product admin logic plus the order.placed event handler.
type Service struct {
	repo domain.Repository
	bus  *broker.Broker
}

func New(repo domain.Repository, bus *broker.Broker) *Service {
	return &Service{repo: repo, bus: bus}
}

// --- product admin (HTTP) ---

func (s *Service) CreateProduct(ctx context.Context, name string, priceCents int64, stock int) (*domain.Product, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, domain.ValidationError{Field: "name", Message: "is required"}
	}
	if priceCents < 0 {
		return nil, domain.ValidationError{Field: "price_cents", Message: "must be >= 0"}
	}
	if stock < 0 {
		return nil, domain.ValidationError{Field: "stock", Message: "must be >= 0"}
	}
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

// OnOrderPlaced reserves stock for the order. On success it publishes
// stock.reserved; on a business rejection (short/unknown stock) it publishes
// stock.rejected. Either way the message is handled (acked); only infrastructure
// failures are returned so the broker can retry them.
func (s *Service) OnOrderPlaced(ctx context.Context, e events.OrderPlacedEvent) error {
	lines := make([]domain.StockLine, len(e.Items))
	for i, it := range e.Items {
		lines[i] = domain.StockLine{ProductID: it.ProductID, Quantity: it.Quantity}
	}

	reserved, err := s.repo.Reserve(ctx, lines)
	if err != nil {
		var stockErr domain.InsufficientStockError
		switch {
		case errors.As(err, &stockErr):
			return s.publishRejected(ctx, e.OrderID, stockErr.Error())
		case errors.Is(err, domain.ErrProductNotFound):
			return s.publishRejected(ctx, e.OrderID, "a product does not exist")
		default:
			return err // infrastructure error -> let the broker retry
		}
	}

	var total int64
	items := make([]events.ReservedItem, len(reserved))
	for i, l := range reserved {
		items[i] = events.ReservedItem{
			ProductID:      l.ProductID,
			ProductName:    l.ProductName,
			Quantity:       l.Quantity,
			UnitPriceCents: l.UnitPriceCents,
		}
		total += l.UnitPriceCents * int64(l.Quantity)
	}
	body, _ := json.Marshal(events.StockReservedEvent{OrderID: e.OrderID, Items: items, TotalCents: total})
	log.Printf("order %d: stock reserved (total %d cents)", e.OrderID, total)
	return s.bus.Publish(ctx, events.StockReserved, body)
}

func (s *Service) publishRejected(ctx context.Context, orderID int64, reason string) error {
	body, _ := json.Marshal(events.StockRejectedEvent{OrderID: orderID, Reason: reason})
	log.Printf("order %d: stock rejected (%s)", orderID, reason)
	return s.bus.Publish(ctx, events.StockRejected, body)
}
