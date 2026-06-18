// Package events is a tiny in-process publish/subscribe bus, the Go analogue of
// the Laravel app's domain events + observers. Services publish facts ("a
// chargeback arrived"); subscribers react (create a notification, write the
// activity log) without the publisher knowing who listens.
package events

import (
	"context"
	"log"
	"sync"
)

// Event is any domain fact. Name routes it to subscribers.
type Event interface {
	Name() string
}

// Handler reacts to an event.
type Handler func(ctx context.Context, e Event)

// Bus dispatches events to registered handlers synchronously, isolating panics
// so one bad subscriber can't take down the request.
type Bus struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
}

// NewBus builds an empty bus.
func NewBus() *Bus {
	return &Bus{handlers: make(map[string][]Handler)}
}

// Subscribe registers h for events with the given name.
func (b *Bus) Subscribe(name string, h Handler) {
	b.mu.Lock()
	b.handlers[name] = append(b.handlers[name], h)
	b.mu.Unlock()
}

// Publish delivers e to every subscriber, recovering from handler panics.
func (b *Bus) Publish(ctx context.Context, e Event) {
	b.mu.RLock()
	hs := b.handlers[e.Name()]
	b.mu.RUnlock()
	for _, h := range hs {
		func(h Handler) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("event handler for %q panicked: %v", e.Name(), r)
				}
			}()
			h(ctx, e)
		}(h)
	}
}

// ---- concrete events -------------------------------------------------------

// CaseReceived fires when a case (chargeback/prevention/validation/rdr) is
// ingested from a webhook.
type CaseReceived struct {
	TenantID   int64
	CaseType   string
	CaseID     int64
	GUID       string
	Amount     int64
	Currency   string
	AssignedTo *int64
}

func (CaseReceived) Name() string { return "case.received" }

// StageChanged fires after a case moves between workflow stages.
type StageChanged struct {
	TenantID  int64
	CaseType  string
	CaseID    int64
	FromStage string
	ToStage   string
	UserID    int64
}

func (StageChanged) Name() string { return "case.stage_changed" }

// CaseAssigned fires when a case is assigned to a user.
type CaseAssigned struct {
	TenantID   int64
	CaseType   string
	CaseID     int64
	AssignedTo int64
	ByUserID   int64
}

func (CaseAssigned) Name() string { return "case.assigned" }
