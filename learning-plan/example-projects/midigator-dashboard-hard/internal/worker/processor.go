package worker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hibiken/asynq"

	"midigator/internal/domain"
	"midigator/internal/events"
)

// Processor handles background jobs. It inserts a webhook's case into the right
// table, marks the webhook log processed, and publishes a CaseReceived event so
// subscribers (notifications) react.
type Processor struct {
	chargebacks domain.ChargebackRepository
	preventions domain.PreventionRepository
	validations domain.OrderValidationRepository
	rdr         domain.RdrRepository
	webhooks    domain.WebhookRepository
	bus         *events.Bus
}

// NewProcessor wires the processor's dependencies.
func NewProcessor(
	chargebacks domain.ChargebackRepository,
	preventions domain.PreventionRepository,
	validations domain.OrderValidationRepository,
	rdr domain.RdrRepository,
	webhooks domain.WebhookRepository,
	bus *events.Bus,
) *Processor {
	return &Processor{chargebacks: chargebacks, preventions: preventions, validations: validations, rdr: rdr, webhooks: webhooks, bus: bus}
}

// webhookData is the common inbound payload shape (a Midigator-ish event body).
type webhookData struct {
	GUID               string `json:"guid"`
	MID                string `json:"mid"`
	Amount             int64  `json:"amount"`
	Currency           string `json:"currency"`
	CardFirst6         string `json:"card_first_6"`
	CardLast4          string `json:"card_last_4"`
	ReasonCode         string `json:"reason_code"`
	ReasonDescription  string `json:"reason_description"`
	ARN                string `json:"arn"`
	CaseNumber         string `json:"case_number"`
	OrderID            string `json:"order_id"`
	OrderGUID          string `json:"order_guid"`
	PreventionType     string `json:"prevention_type"`
	MerchantDescriptor string `json:"merchant_descriptor"`
}

// HandleWebhookProcess is the asynq handler for TypeWebhookProcess.
func (p *Processor) HandleWebhookProcess(ctx context.Context, t *asynq.Task) error {
	var pl WebhookProcessPayload
	if err := json.Unmarshal(t.Payload(), &pl); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}

	caseID, caseType, amount, currency, guid, err := p.ingest(ctx, pl)
	if err != nil {
		// A duplicate (same event replayed) is not a failure — it's already done.
		var conflict domain.ConflictError
		if errors.As(err, &conflict) {
			_ = p.webhooks.MarkProcessed(ctx, pl.WebhookLogID, time.Now())
			return nil
		}
		_ = p.webhooks.MarkFailed(ctx, pl.WebhookLogID, err.Error())
		return err
	}

	_ = p.webhooks.MarkProcessed(ctx, pl.WebhookLogID, time.Now())
	p.bus.Publish(ctx, events.CaseReceived{
		TenantID: pl.TenantID, CaseType: string(caseType), CaseID: caseID,
		GUID: guid, Amount: amount, Currency: currency,
	})
	return nil
}

func (p *Processor) ingest(ctx context.Context, pl WebhookProcessPayload) (int64, domain.CaseType, int64, string, string, error) {
	var d webhookData
	if len(pl.Data) > 0 {
		if err := json.Unmarshal(pl.Data, &d); err != nil {
			return 0, "", 0, "", "", fmt.Errorf("decode data: %w", err)
		}
	}
	cur := d.Currency
	if cur == "" {
		cur = "USD"
	}

	switch {
	case strings.HasPrefix(pl.EventType, "chargeback"):
		guid := orGen(d.GUID, "cb")
		mid := d.MID
		if mid == "" {
			mid = "UNKNOWN"
		}
		ins, err := p.chargebacks.Insert(ctx, &domain.Chargeback{
			TenantID: pl.TenantID, ChargebackGUID: guid, EventGUID: pl.EventGUID, MID: mid,
			Amount: d.Amount, Currency: cur, CardFirst6: d.CardFirst6, CardLast4: d.CardLast4,
			ReasonCode: d.ReasonCode, ReasonDescription: d.ReasonDescription, CaseNumber: d.CaseNumber,
			ARN: d.ARN, OrderID: d.OrderID, OrderGUID: d.OrderGUID,
		})
		if err != nil {
			return 0, "", 0, "", "", err
		}
		return ins.ID, domain.CaseChargeback, d.Amount, cur, guid, nil

	case strings.HasPrefix(pl.EventType, "prevention"):
		guid := orGen(d.GUID, "prev")
		ptype := d.PreventionType
		switch ptype {
		case "ethoca", "verifi", "tc40":
		default:
			ptype = "ethoca"
		}
		ins, err := p.preventions.Insert(ctx, &domain.PreventionAlert{
			TenantID: pl.TenantID, PreventionGUID: guid, EventGUID: pl.EventGUID, PreventionType: ptype,
			MID: d.MID, Amount: d.Amount, Currency: cur, CardFirst6: d.CardFirst6, CardLast4: d.CardLast4,
			MerchantDescriptor: d.MerchantDescriptor, ARN: d.ARN, OrderID: d.OrderID, OrderGUID: d.OrderGUID,
		})
		if err != nil {
			return 0, "", 0, "", "", err
		}
		return ins.ID, domain.CasePrevention, d.Amount, cur, guid, nil

	case strings.HasPrefix(pl.EventType, "order_validation"):
		guid := orGen(d.GUID, "ov")
		ins, err := p.validations.Insert(ctx, &domain.OrderValidation{
			TenantID: pl.TenantID, OrderValidationGUID: guid, EventGUID: pl.EventGUID,
			MID: d.MID, Amount: d.Amount, Currency: cur, CardFirst6: d.CardFirst6, CardLast4: d.CardLast4,
			ARN: d.ARN, OrderID: d.OrderID, OrderGUID: d.OrderGUID,
		})
		if err != nil {
			return 0, "", 0, "", "", err
		}
		return ins.ID, domain.CaseOrderValidation, d.Amount, cur, guid, nil

	case strings.HasPrefix(pl.EventType, "rdr"):
		guid := orGen(d.GUID, "rdr")
		ins, err := p.rdr.Insert(ctx, &domain.RdrCase{
			TenantID: pl.TenantID, RdrGUID: guid, EventGUID: pl.EventGUID,
			Amount: d.Amount, Currency: cur, CardFirst6: d.CardFirst6, CardLast4: d.CardLast4,
			MerchantDescriptor: d.MerchantDescriptor, ARN: d.ARN, OrderID: d.OrderID, OrderGUID: d.OrderGUID,
			RdrCaseNumber: d.CaseNumber,
		})
		if err != nil {
			return 0, "", 0, "", "", err
		}
		return ins.ID, domain.CaseRdr, d.Amount, cur, guid, nil

	default:
		return 0, "", 0, "", "", fmt.Errorf("unknown event type %q", pl.EventType)
	}
}

func orGen(guid, prefix string) string {
	if guid != "" {
		return guid
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + "-" + hex.EncodeToString(b)
}
