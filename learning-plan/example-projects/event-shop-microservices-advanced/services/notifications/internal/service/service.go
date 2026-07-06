// Package service handles notification events.
package service

import (
	"encoding/json"
	"log"

	"eventshop/pkg/events"
)

// An empty struct{}: zero fields, occupies no memory — used here just as a
// receiver to hang methods on.
type Service struct{}

// New returns *Service (a pointer to a freshly allocated empty struct).
func New() *Service { return &Service{} }

func (s *Service) Handle(routingKey string, body []byte) error {
	// A switch on the event-name string; each case decodes into its own struct.
	switch routingKey {
	case events.PaymentSettled:
		var e events.PaymentSettledEvent
		if err := json.Unmarshal(body, &e); err != nil { // &e: decode into e
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
