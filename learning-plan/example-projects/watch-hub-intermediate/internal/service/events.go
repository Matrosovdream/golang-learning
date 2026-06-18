package service

// Event is a status-change notification fanned out to every SSE subscriber.
type Event struct {
	MonitorID  int64  `json:"monitor_id"`
	URL        string `json:"url"`
	Status     string `json:"status"`
	HTTPStatus int    `json:"http_status,omitempty"`
	LatencyMS  int64  `json:"latency_ms"`
	At         string `json:"at"`
}

// EventHub is a publish/subscribe broadcaster for the /events SSE endpoint.
//
// It follows the actor pattern: a single goroutine (run) owns the subscribers
// map, and everyone else talks to it over channels. Because only one goroutine
// ever touches the map, there is NO mutex here — "don't communicate by sharing
// memory; share memory by communicating."
type EventHub struct {
	subscribe   chan chan Event
	unsubscribe chan chan Event
	broadcast   chan Event
	quit        chan struct{}
}

func NewEventHub() *EventHub {
	h := &EventHub{
		subscribe:   make(chan chan Event),
		unsubscribe: make(chan chan Event),
		broadcast:   make(chan Event),
		quit:        make(chan struct{}),
	}
	go h.run()
	return h
}

func (h *EventHub) run() {
	subscribers := make(map[chan Event]struct{})
	for {
		select {
		case ch := <-h.subscribe:
			subscribers[ch] = struct{}{}
		case ch := <-h.unsubscribe:
			if _, ok := subscribers[ch]; ok {
				delete(subscribers, ch)
				close(ch)
			}
		case ev := <-h.broadcast:
			for ch := range subscribers {
				// Non-blocking send: a slow client that isn't draining its
				// channel gets this event dropped rather than stalling the hub.
				select {
				case ch <- ev:
				default:
				}
			}
		case <-h.quit:
			for ch := range subscribers {
				close(ch)
			}
			return
		}
	}
}

// Subscribe returns a new buffered channel registered with the hub. The caller
// must Unsubscribe when done.
func (h *EventHub) Subscribe() chan Event {
	ch := make(chan Event, 16)
	select {
	case h.subscribe <- ch:
	case <-h.quit:
	}
	return ch
}

// Unsubscribe removes ch; the hub closes it (so the reader's range/recv ends).
func (h *EventHub) Unsubscribe(ch chan Event) {
	select {
	case h.unsubscribe <- ch:
	case <-h.quit:
	}
}

// Publish broadcasts an event to all subscribers (best-effort).
func (h *EventHub) Publish(ev Event) {
	select {
	case h.broadcast <- ev:
	case <-h.quit:
	}
}

// Close shuts the hub down and closes every subscriber channel.
func (h *EventHub) Close() {
	select {
	case <-h.quit: // already closed
	default:
		close(h.quit)
	}
}
