package domain

import (
	"context"
	"time"
)

// OrderValidation is a validation event tied to an order/transaction.
type OrderValidation struct {
	ID                       int64
	TenantID                 int64
	OrderValidationGUID      string
	EventGUID                string
	OrderValidationType      string
	OrderValidationTimestamp *time.Time
	TransactionTimestamp     *time.Time
	Amount                   int64
	Currency                 string
	CardFirst6               string
	CardLast4                string
	ARN                      string
	AuthCode                 string
	CardBrand                string
	OrderGUID                string
	OrderID                  string
	CRMAccountGUID           string
	MID                      string
	AssignedTo               *int64
	Stage                    string
	IsHidden                 bool
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// OrderValidationRepository is the order-validation store.
type OrderValidationRepository interface {
	CaseRepository
	Get(ctx context.Context, tenantID, id int64) (*OrderValidation, error)
	Update(ctx context.Context, tenantID, id int64, fields map[string]any) (*OrderValidation, error)
	Insert(ctx context.Context, v *OrderValidation) (*OrderValidation, error)
}
