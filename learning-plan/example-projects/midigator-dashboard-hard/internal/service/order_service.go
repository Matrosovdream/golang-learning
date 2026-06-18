package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"midigator/internal/domain"
)

// OrderService holds the order-specific operations: list, show, create, edit
// whitelisted fields, and submit to Midigator. Orders have no workflow stage, so
// there is no generic CaseService half — everything typed lives here.
type OrderService struct {
	repo     domain.OrderRepository
	activity activityLogger
}

// NewOrderService wires the order service.
func NewOrderService(repo domain.OrderRepository, activity domain.ActivityRepository) *OrderService {
	return &OrderService{repo: repo, activity: activityLogger{repo: activity}}
}

// List paginates the tenant's orders.
func (s *OrderService) List(ctx context.Context, f domain.CaseFilter) ([]domain.Order, int, error) {
	_, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, 0, err
	}
	return s.repo.List(ctx, tenantID, f)
}

// Get returns one order.
func (s *OrderService) Get(ctx context.Context, id int64) (*domain.Order, error) {
	_, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, tenantID, id)
}

// Create records a new order, generating an order_guid when absent and pinning
// the submission lifecycle to pending.
func (s *OrderService) Create(ctx context.Context, o *domain.Order) (*domain.Order, error) {
	userID, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	if o.OrderAmount <= 0 {
		return nil, domain.Invalid("order_amount", "must be greater than zero")
	}
	o.TenantID = tenantID
	if o.OrderGUID == "" {
		o.OrderGUID = orderGUID()
	}
	o.SubmissionStatus = domain.SubmissionPending
	created, err := s.repo.Create(ctx, o)
	if err != nil {
		return nil, err
	}
	s.activity.log(ctx, tenantID, userID, "order", int64Ptr(created.ID), "created", nil)
	return created, nil
}

// Update edits an order's whitelisted fields.
func (s *OrderService) Update(ctx context.Context, id int64, fields map[string]any) (*domain.Order, error) {
	userID, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	o, err := s.repo.Update(ctx, tenantID, id, fields)
	if err != nil {
		return nil, err
	}
	s.activity.log(ctx, tenantID, userID, "order", int64Ptr(id), "updated", nil)
	return o, nil
}

// Submit forwards the order to Midigator. The external call is stubbed/no-op; on
// success we mark the order submitted with the current timestamp.
func (s *OrderService) Submit(ctx context.Context, id int64) (*domain.Order, error) {
	userID, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	o, err := s.repo.SetSubmission(ctx, tenantID, id, domain.SubmissionSubmitted, "", &now)
	if err != nil {
		return nil, err
	}
	s.activity.log(ctx, tenantID, userID, "order", int64Ptr(id), "submitted", nil)
	return o, nil
}

// orderGUID generates a random, prefixed identifier for new orders.
func orderGUID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "order-" + hex.EncodeToString(b)
}
