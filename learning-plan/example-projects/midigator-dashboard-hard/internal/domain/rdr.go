package domain

import (
	"context"
	"time"
)

// RdrCase is a Rapid Dispute Resolution case (Visa RDR).
type RdrCase struct {
	ID                 int64
	TenantID           int64
	RdrGUID            string
	EventGUID          string
	RdrCaseNumber      string
	RdrDate            *time.Time
	RdrResolution      string
	ARN                string
	AuthCode           string
	Amount             int64
	Currency           string
	CardFirst6         string
	CardLast4          string
	MerchantDescriptor string
	PreventionType     string
	TransactionDate    *time.Time
	OrderGUID          string
	OrderID            string
	CRMAccountGUID     string
	AssignedTo         *int64
	Stage              string
	IsHidden           bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// RdrRepository is the RDR-case store.
type RdrRepository interface {
	CaseRepository
	Get(ctx context.Context, tenantID, id int64) (*RdrCase, error)
	Update(ctx context.Context, tenantID, id int64, fields map[string]any) (*RdrCase, error)
	Insert(ctx context.Context, r *RdrCase) (*RdrCase, error)
}
