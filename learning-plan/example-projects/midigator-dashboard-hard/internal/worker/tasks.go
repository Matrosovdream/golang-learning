// Package worker holds the asynq background-job layer: the enqueuer used by the
// API to publish jobs, and the processor + server run by the worker binary that
// drains them. Today the one queue is webhook:process (ingest a Midigator
// webhook into the right case type).
package worker

import (
	"context"
	"encoding/json"

	"github.com/hibiken/asynq"
)

// TypeWebhookProcess is the asynq task type for processing an inbound webhook.
const TypeWebhookProcess = "webhook:process"

// WebhookProcessPayload is the job body for TypeWebhookProcess.
type WebhookProcessPayload struct {
	WebhookLogID int64           `json:"webhook_log_id"`
	TenantID     int64           `json:"tenant_id"`
	EventType    string          `json:"event_type"`
	EventGUID    string          `json:"event_guid"`
	Data         json.RawMessage `json:"data"`
}

// Enqueuer publishes jobs to Redis via asynq. The API holds one.
type Enqueuer struct {
	client *asynq.Client
}

// NewEnqueuer connects an asynq client to Redis.
func NewEnqueuer(redisAddr string) *Enqueuer {
	return &Enqueuer{client: asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})}
}

// EnqueueWebhook queues a webhook for asynchronous processing.
func (e *Enqueuer) EnqueueWebhook(ctx context.Context, p WebhookProcessPayload) error {
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = e.client.EnqueueContext(ctx, asynq.NewTask(TypeWebhookProcess, b))
	return err
}

// Close releases the client connection.
func (e *Enqueuer) Close() error { return e.client.Close() }
