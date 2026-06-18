package service

import (
	"context"
	"fmt"

	"midigator/internal/domain"
	"midigator/internal/events"
)

// RegisterEventSubscribers wires the in-process event handlers (the Go analogue
// of the Laravel observers): a new case notifies the tenant's active users, and
// an assignment notifies the assignee. Call this on each process's bus — the API
// publishes CaseAssigned/StageChanged, the worker publishes CaseReceived.
func RegisterEventSubscribers(bus *events.Bus, notifications domain.NotificationRepository, users domain.UserRepository) {
	bus.Subscribe(events.CaseReceived{}.Name(), func(ctx context.Context, e events.Event) {
		ev, ok := e.(events.CaseReceived)
		if !ok {
			return
		}
		members, err := users.ListByTenant(ctx, ev.TenantID)
		if err != nil {
			return
		}
		for _, u := range members {
			if !u.IsActive {
				continue
			}
			id := ev.CaseID
			_, _ = notifications.Create(ctx, &domain.Notification{
				UserID:         u.ID,
				TenantID:       ev.TenantID,
				Type:           "case_received",
				Title:          "New " + ev.CaseType + " received",
				Body:           fmt.Sprintf("A new %s (%s) for %d %s arrived.", ev.CaseType, ev.GUID, ev.Amount, ev.Currency),
				NotifiableType: ev.CaseType,
				NotifiableID:   &id,
			})
		}
	})

	bus.Subscribe(events.CaseAssigned{}.Name(), func(ctx context.Context, e events.Event) {
		ev, ok := e.(events.CaseAssigned)
		if !ok {
			return
		}
		id := ev.CaseID
		_, _ = notifications.Create(ctx, &domain.Notification{
			UserID:         ev.AssignedTo,
			TenantID:       ev.TenantID,
			Type:           "case_assigned",
			Title:          "Case assigned to you",
			Body:           fmt.Sprintf("You were assigned a %s (#%d).", ev.CaseType, ev.CaseID),
			NotifiableType: ev.CaseType,
			NotifiableID:   &id,
		})
	})
}
