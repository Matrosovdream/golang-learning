package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"watchhub/internal/service"
)

// EventsHandler streams status-change events to clients over Server-Sent
// Events (SSE). Every connected client is one goroutine parked in Stream,
// reading its own subscriber channel from the EventHub.
type EventsHandler struct {
	hub *service.EventHub
}

func NewEventsHandler(hub *service.EventHub) *EventsHandler {
	return &EventsHandler{hub: hub}
}

// Stream holds the connection open and writes an SSE frame per event. Try it
// with:  curl -N localhost:8080/events
func (h *EventsHandler) Stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	events := h.hub.Subscribe()
	defer h.hub.Unsubscribe(events)

	fmt.Fprint(w, ": connected\n\n") // SSE comment to open the stream
	flusher.Flush()

	// A heartbeat keeps proxies from closing an idle connection.
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	ctx := r.Context() // cancelled when the client disconnects
	for {
		select {
		case ev, open := <-events:
			if !open {
				return // hub shut down / unsubscribed
			}
			payload, _ := json.Marshal(ev)
			fmt.Fprintf(w, "event: status\ndata: %s\n\n", payload)
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case <-ctx.Done():
			return
		}
	}
}
