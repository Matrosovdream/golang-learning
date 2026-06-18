package domain

import (
	"context"
	"encoding/json"
	"time"
)

// Order is a purchase record. Unlike the four case types it has no workflow
// stage; instead it is submitted to Midigator (submission_status lifecycle).
type Order struct {
	ID                     int64
	TenantID               int64
	OrderGUID              string
	OrderID                string
	MID                    string
	OrderDate              *time.Time
	OrderAmount            int64
	Currency               string
	CardBrand              string
	CardFirst6             string
	CardLast4              string
	CardExpMonth           string
	CardExpYear            string
	AVS                    string
	CVV                    string
	ProcessorAuthCode      string
	ProcessorTransactionID string
	Email                  string
	Phone                  string
	IPAddress              string
	BillingFirstName       string
	BillingLastName        string
	BillingAddress         json.RawMessage
	Refunded               bool
	RefundedAmount         *int64
	SubscriptionCycle      *int
	SubscriptionParentID   string
	MarketingSource        string
	SubMarketingSources    json.RawMessage
	Items                  json.RawMessage
	Evidence               json.RawMessage
	IsHidden               bool
	SubmissionStatus       string
	SubmissionError        string
	SubmittedAt            *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// Submission status values for orders.
const (
	SubmissionPending   = "pending"
	SubmissionSubmitted = "submitted"
	SubmissionFailed    = "failed"
)

// OrderRepository is the order store.
type OrderRepository interface {
	Create(ctx context.Context, o *Order) (*Order, error)
	Get(ctx context.Context, tenantID, id int64) (*Order, error)
	Update(ctx context.Context, tenantID, id int64, fields map[string]any) (*Order, error)
	List(ctx context.Context, tenantID int64, f CaseFilter) ([]Order, int, error)
	SetSubmission(ctx context.Context, tenantID, id int64, status, errMsg string, submittedAt *time.Time) (*Order, error)
	ExistsForTenant(ctx context.Context, tenantID, id int64) (bool, error)
}
