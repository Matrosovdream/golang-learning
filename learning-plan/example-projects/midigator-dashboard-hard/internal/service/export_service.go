package service

import (
	"context"
	"encoding/csv"
	"io"
	"strconv"
	"time"

	"midigator/internal/domain"
)

// ExportService streams tenant-scoped CSV exports. It reuses the CaseRegistry so
// every case type exports through the same CaseSummary projection, and the order
// repository for the order export, which has its own (workflow-less) shape.
type ExportService struct {
	registry *CaseRegistry
	orders   domain.OrderRepository
}

// NewExportService wires the CSV export service.
func NewExportService(registry *CaseRegistry, orders domain.OrderRepository) *ExportService {
	return &ExportService{registry: registry, orders: orders}
}

// WriteCSV validates kind, then streams the matching rows as CSV to w. The kind
// is either "order" or one of the four case types; any other value is rejected
// before a single row is written so the handler can still set an error status.
func (s *ExportService) WriteCSV(ctx context.Context, w io.Writer, kind string) error {
	_, tenantID, err := tenantActor(ctx)
	if err != nil {
		return err
	}

	cw := csv.NewWriter(w)
	defer cw.Flush()

	if kind == "order" {
		orders, _, err := s.orders.List(ctx, tenantID, domain.CaseFilter{Limit: 200, IncludeHidden: true})
		if err != nil {
			return err
		}
		if err := cw.Write([]string{"id", "order_guid", "order_id", "mid", "order_amount", "currency", "submission_status", "created_at"}); err != nil {
			return err
		}
		for _, o := range orders {
			if err := cw.Write([]string{
				strconv.FormatInt(o.ID, 10),
				o.OrderGUID,
				o.OrderID,
				o.MID,
				strconv.FormatInt(o.OrderAmount, 10),
				o.Currency,
				o.SubmissionStatus,
				o.CreatedAt.Format(time.RFC3339),
			}); err != nil {
				return err
			}
		}
		return cw.Error()
	}

	t, ok := domain.ParseCaseType(kind)
	if !ok {
		return domain.Invalid("type", "must be one of order, chargeback, prevention, order_validation, rdr")
	}
	repo, err := s.registry.RepoFor(t)
	if err != nil {
		return err
	}
	summaries, _, err := repo.ListSummaries(ctx, tenantID, domain.CaseFilter{Limit: 200, IncludeHidden: true})
	if err != nil {
		return err
	}
	if err := cw.Write([]string{"type", "id", "guid", "mid", "amount", "currency", "stage", "assigned_to", "is_hidden", "created_at"}); err != nil {
		return err
	}
	for _, c := range summaries {
		assigned := ""
		if c.AssignedTo != nil {
			assigned = strconv.FormatInt(*c.AssignedTo, 10)
		}
		if err := cw.Write([]string{
			string(c.Type),
			strconv.FormatInt(c.ID, 10),
			c.GUID,
			c.MID,
			strconv.FormatInt(c.Amount, 10),
			c.Currency,
			c.Stage,
			assigned,
			strconv.FormatBool(c.IsHidden),
			c.CreatedAt.Format(time.RFC3339),
		}); err != nil {
			return err
		}
	}
	return cw.Error()
}
