// Package service handles notification events. This service is intentionally
// stateless — it just reacts to terminal events (a real one would send email /
// push). It owns no database.
package service

import (
	"encoding/json"
	"log"

	"eventshop/pkg/events"
)

type Service struct{}

func New() *Service { return &Service{} }

// Handle logs a "notification" for the events this service cares about.
func (s *Service) Handle(routingKey string, body []byte) error {
	switch routingKey {
	case events.PaymentSettled:
		var e events.PaymentSettledEvent
		if err := json.Unmarshal(body, &e); err != nil {
			return err
		}
		log.Printf("NOTIFY order %d: confirmed — payment of %d cents settled", e.OrderID, e.AmountCents)
	case events.StockRejected:
		var e events.StockRejectedEvent
		if err := json.Unmarshal(body, &e); err != nil {
			return err
		}
		log.Printf("NOTIFY order %d: cancelled — %s", e.OrderID, e.Reason)
	}
	return nil
}
