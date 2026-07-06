package service

import (
	"context"
	"encoding/json"
	"log"

	"eventshop/pkg/broker"
	"eventshop/pkg/events"
	"eventshop/services/payments/internal/domain"
)

type Service struct {
	repo domain.Repository // interface field
	bus  *broker.Broker    // pointer field
}

func New(repo domain.Repository, bus *broker.Broker) *Service {
	return &Service{repo: repo, bus: bus}
}

func (s *Service) OnStockReserved(ctx context.Context, e events.StockReservedEvent) error {
	// &domain.Payment{...}: pointer to a new struct, passed to Create.
	payment := &domain.Payment{OrderID: e.OrderID, AmountCents: e.TotalCents, Status: "settled"}
	if err := s.repo.Create(ctx, payment); err != nil {
		return err
	}
	// json.Marshal returns (bytes, error); _ drops the error.
	body, _ := json.Marshal(events.PaymentSettledEvent{OrderID: e.OrderID, AmountCents: e.TotalCents})
	log.Printf("order %d: payment settled (%d cents)", e.OrderID, e.TotalCents)
	return s.bus.Publish(ctx, events.PaymentSettled, body)
}
