package domain

import (
	"context"
	"time"
)

// Chargeback is a disputed transaction the team must respond to.
type Chargeback struct {
	ID                       int64
	TenantID                 int64
	ChargebackGUID           string
	EventGUID                string
	CaseNumber               string
	ARN                      string
	MID                      string
	Amount                   int64
	Currency                 string
	CardBrand                string
	CardFirst6               string
	CardLast4                string
	ReasonCode               string
	ReasonDescription        string
	ChargebackDate           *time.Time
	DateReceived             *time.Time
	DueDate                  *time.Time
	ProcessorTransactionID   string
	ProcessorTransactionDate *time.Time
	AuthCode                 string
	AlternateTransactionID   string
	Result                   string
	DNFReason                string
	DNFTimestamp             *time.Time
	OrderGUID                string
	OrderID                  string
	CRMAccountGUID           string
	AssignedTo               *int64
	Stage                    string
	IsHidden                 bool
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// ChargebackRepository is the chargeback store: the shared CaseRepository plus
// typed read/write used by the dedicated endpoints, webhook worker, and seed.
type ChargebackRepository interface {
	CaseRepository
	Get(ctx context.Context, tenantID, id int64) (*Chargeback, error)
	Update(ctx context.Context, tenantID, id int64, fields map[string]any) (*Chargeback, error)
	Insert(ctx context.Context, c *Chargeback) (*Chargeback, error)
	BulkSetHidden(ctx context.Context, tenantID int64, ids []int64, hidden bool) (int, error)
}
