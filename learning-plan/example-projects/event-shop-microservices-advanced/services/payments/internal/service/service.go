package service

import (
	"context"
	"encoding/json"
	"log"

	"eventshop/pkg/broker"
	"eventshop/pkg/events"
	"eventshop/services/payments/internal/domain"
)

// Service charges orders once their stock is reserved. The charge is simulated
// here (it always succeeds), then a payment.settled event is published.
type Service struct {
	repo domain.Repository
	bus  *broker.Broker
}

func New(repo domain.Repository, bus *broker.Broker) *Service {
	return &Service{repo: repo, bus: bus}
}

func (s *Service) OnStockReserved(ctx context.Context, e events.StockReservedEvent) error {
	payment := &domain.Payment{OrderID: e.OrderID, AmountCents: e.TotalCents, Status: "settled"}
	if err := s.repo.Create(ctx, payment); err != nil {
		return err // infrastructure error -> let the broker retry
	}
	body, _ := json.Marshal(events.PaymentSettledEvent{OrderID: e.OrderID, AmountCents: e.TotalCents})
	log.Printf("order %d: payment settled (%d cents)", e.OrderID, e.TotalCents)
	return s.bus.Publish(ctx, events.PaymentSettled, body)
}
