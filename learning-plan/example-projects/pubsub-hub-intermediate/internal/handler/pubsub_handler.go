package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"pubsubhub/internal/config"
	"pubsubhub/internal/domain"
	"pubsubhub/internal/service"
)

// PubSubHandler turns HTTP requests into Broker calls and streams events out.
// It holds a *service.Broker (shared pointer) and the config by value.
type PubSubHandler struct {
	broker *service.Broker
	cfg    config.Config
}

func NewPubSubHandler(broker *service.Broker, cfg config.Config) *PubSubHandler {
	return &PubSubHandler{broker: broker, cfg: cfg}
}

// Wire DTOs. The `json:"..."` struct tags map Go fields to JSON keys.
type publishRequest struct {
	Message string `json:"message"`
}

type messageResponse struct {
	ID        int64  `json:"id"`
	Topic     string `json:"topic"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

type topicResponse struct {
	Name        string `json:"name"`
	Subscribers int    `json:"subscribers"`
}

// Subscribe streams messages for a topic as Server-Sent Events (SSE). This is
// the centerpiece: one goroutine per connection (the HTTP server gives us that
// for free) blocks here receiving from this subscriber's mailbox channel and
// flushes each message down the wire.
func (h *PubSubHandler) Subscribe(w http.ResponseWriter, r *http.Request) {
	topic := r.PathValue("topic")
	if err := h.validateTopic(topic); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// SSE handshake: a long-lived text/event-stream, never cached, kept open.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// TYPE ASSERTION: w is the http.ResponseWriter interface; w.(http.Flusher)
	// asks "does the concrete value behind it also implement Flusher?". The
	// comma-ok form (flusher, ok) avoids a panic if it doesn't — without Flush we
	// can't push bytes before the handler returns, so SSE is impossible.
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	// Subscribe returns a RECEIVE-ONLY channel (<-chan). defer guarantees we
	// Unsubscribe no matter how this handler exits (client disconnect, shutdown,
	// or panic) — that closes our channel and frees the slot in the broker.
	id, ch := h.broker.Subscribe(topic)
	defer h.broker.Unsubscribe(topic, id)

	// An SSE comment line (":" prefix) as an immediate heartbeat so the client's
	// EventSource fires `open` right away, before any real message arrives.
	fmt.Fprintf(w, ": subscribed to %q\n\n", topic)
	flusher.Flush() // push the buffered bytes to the client now

	// This blocking loop is fine: the net/http server runs every request on its
	// OWN goroutine, so blocking here parks just this one connection, not the
	// server. select waits on whichever case is ready first.
	for {
		select {
		case msg, ok := <-ch:
			// v, ok := <-ch: ok is false once the channel is closed and drained —
			// i.e. Unsubscribe or Shutdown closed our mailbox. Time to leave.
			if !ok {
				return
			}
			// SSE frame format: "data: <payload>\n\n". One JSON object per event.
			fmt.Fprintf(w, "data: %s\n\n", encodeMessage(msg))
			flusher.Flush()
		case <-r.Context().Done():
			// The request context is cancelled when the client disconnects. Return
			// so the deferred Unsubscribe runs and reclaims this subscriber.
			return
		}
	}
}

// Publish validates the body and fans a message out to every subscriber.
// Returns 202 Accepted: delivery is fire-and-forget (slow consumers get dropped).
func (h *PubSubHandler) Publish(w http.ResponseWriter, r *http.Request) {
	topic := r.PathValue("topic")
	if err := h.validateTopic(topic); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Cap how many bytes we'll read so a huge body can't exhaust memory.
	r.Body = http.MaxBytesReader(w, r.Body, int64(h.cfg.MaxBodyBytes)+512)
	var req publishRequest // zero-valued struct to decode into
	// Decode reads the request body into &req (a pointer, so it can fill it).
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if err := h.validateBody(req.Message); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	msg := h.broker.Publish(topic, req.Message)
	writeJSON(w, http.StatusAccepted, toMessageResponse(msg))
}

// Topics lists every live topic with its current subscriber count.
func (h *PubSubHandler) Topics(w http.ResponseWriter, r *http.Request) {
	infos := h.broker.Topics()
	resp := make([]topicResponse, len(infos)) // pre-size, then fill by index
	for i, t := range infos {
		resp[i] = topicResponse{Name: t.Name, Subscribers: t.Subscribers}
	}
	writeJSON(w, http.StatusOK, resp)
}

// Stats exposes the broker's atomic counters and current totals.
func (h *PubSubHandler) Stats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.broker.Stats())
}

// Health is a trivial liveness probe.
func (h *PubSubHandler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Home serves a tiny self-contained demo page (no external assets) so the
// fan-out is visible in a browser. See demoPage below.
func (h *PubSubHandler) Home(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(demoPage)) // []byte(string) is a type conversion
}

// --- helpers ---

// validateTopic rejects an empty or oversized topic name.
func (h *PubSubHandler) validateTopic(topic string) error {
	if topic == "" {
		return domain.ValidationError{Message: "topic must not be empty"}
	}
	if len(topic) > h.cfg.MaxTopicLen {
		return domain.ValidationError{Message: fmt.Sprintf("topic too long (max %d)", h.cfg.MaxTopicLen)}
	}
	return nil
}

// validateBody rejects an empty or oversized message body.
func (h *PubSubHandler) validateBody(body string) error {
	if body == "" {
		return domain.ValidationError{Message: "message must not be empty"}
	}
	if len(body) > h.cfg.MaxBodyBytes {
		return domain.ValidationError{Message: fmt.Sprintf("message too long (max %d bytes)", h.cfg.MaxBodyBytes)}
	}
	return nil
}

// encodeMessage marshals a Message to a compact JSON string for one SSE frame.
func encodeMessage(msg domain.Message) []byte {
	b, err := json.Marshal(toMessageResponse(msg))
	if err != nil {
		return []byte(`{"error":"encode failed"}`)
	}
	return b
}

// toMessageResponse maps a domain.Message onto the wire DTO (decoupling the two).
func toMessageResponse(msg domain.Message) messageResponse {
	return messageResponse{
		ID:        msg.ID,
		Topic:     msg.Topic,
		Body:      msg.Body,
		CreatedAt: msg.CreatedAt.Format(time.RFC3339), // format a time.Time as a string
	}
}
