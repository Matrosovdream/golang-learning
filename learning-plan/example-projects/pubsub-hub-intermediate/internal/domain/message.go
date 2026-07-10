package domain

import "time"

// Message is one published event, fanned out to every subscriber of its topic.
// Plain fields, no json tags — the handler has its own DTO for the wire format.
type Message struct {
	ID        int64
	Topic     string
	Body      string
	CreatedAt time.Time
}

// TopicInfo is a topic name paired with its current live-subscriber count.
type TopicInfo struct {
	Name        string
	Subscribers int
}

// A struct carrying one field. Giving it an Error() method (below) makes it
// satisfy the built-in `error` interface, so a validation failure is just a value.
type ValidationError struct{ Message string }

// Implementing `Error() string` is all it takes to be an error. Value receiver
// (e ValidationError): it only reads, so a copy is fine.
func (e ValidationError) Error() string { return e.Message }
