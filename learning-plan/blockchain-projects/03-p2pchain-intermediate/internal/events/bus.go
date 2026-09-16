// Package events is a tiny in-process pub/sub used to tell the miner "there is
// a new tip, restart" and to feed the /events SSE stream.
package events

import (
	"sync"
	"time"
)

// Kind is the type of a chain event.
type Kind string

const (
	BlockAdded       Kind = "block_added"    // a new canonical tip on top of the old one
	Reorg            Kind = "reorg"          // the canonical chain switched branches
	SideBlock        Kind = "side_block"     // a valid block that lost the fork choice
	TxAdded          Kind = "tx_added"       // a transaction entered the mempool
	PeerConnected    Kind = "peer_connected" //
	PeerDisconnected Kind = "peer_disconnected"
	SyncStarted      Kind = "sync_started"
	SyncFinished     Kind = "sync_finished"
)

// Event is one thing that happened, with free-form details for the UI.
type Event struct {
	Kind   Kind           `json:"kind"`
	Time   time.Time      `json:"time"`
	Fields map[string]any `json:"fields,omitempty"`
}

// Bus fans events out to subscribers.
//
// Slow subscribers are never allowed to block the chain: each subscriber has a
// buffered channel, and an event that doesn't fit is dropped for that
// subscriber only. A UI that can't keep up misses lines; the node keeps running.
type Bus struct {
	mu     sync.Mutex
	nextID int
	subs   map[int]chan Event
}

func NewBus() *Bus { return &Bus{subs: make(map[int]chan Event)} }

// Subscribe returns a channel of events and a function to stop listening.
func (b *Bus) Subscribe(buffer int) (<-chan Event, func()) {
	ch := make(chan Event, buffer)
	b.mu.Lock()
	id := b.nextID
	b.nextID++
	b.subs[id] = ch
	b.mu.Unlock()

	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if sub, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(sub)
		}
	}
}

// Publish delivers an event to every subscriber that has room for it.
func (b *Bus) Publish(kind Kind, fields map[string]any) {
	ev := Event{Kind: kind, Time: time.Now(), Fields: fields}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs {
		select {
		case ch <- ev:
		default: // subscriber is behind: drop rather than block the caller
		}
	}
}
