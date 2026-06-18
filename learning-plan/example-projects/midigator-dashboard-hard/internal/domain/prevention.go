package domain

import (
	"context"
	"time"
)

// PreventionAlert is an early-warning alert (Ethoca / Verifi / TC40) that can be
// resolved before it becomes a chargeback.
type PreventionAlert struct {
	ID                    int64
	TenantID              int64
	PreventionGUID        string
	EventGUID             string
	PreventionCaseNumber  string
	PreventionType        string
	ARN                   string
	MID                   string
	Amount                int64
	Currency              string
	CardFirst6            string
	CardLast4             string
	MerchantDescriptor    string
	PreventionTimestamp   *time.Time
	TransactionTimestamp  *time.Time
	ResolutionType        string
	ResolutionSubmittedAt *time.Time
	OrderGUID             string
	OrderID               string
	CRMAccountGUID        string
	AssignedTo            *int64
	Stage                 string
	IsHidden              bool
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// PreventionRepository is the prevention-alert store.
type PreventionRepository interface {
	CaseRepository
	Get(ctx context.Context, tenantID, id int64) (*PreventionAlert, error)
	Update(ctx context.Context, tenantID, id int64, fields map[string]any) (*PreventionAlert, error)
	Insert(ctx context.Context, p *PreventionAlert) (*PreventionAlert, error)
}
