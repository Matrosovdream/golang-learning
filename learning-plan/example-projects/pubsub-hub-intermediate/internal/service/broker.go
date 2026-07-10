package service

import (
	"sync"
	"sync/atomic"
	"time"

	"pubsubhub/internal/domain"
)

// subscriber is one live listener on one topic. Its ch is a BUFFERED channel
// (make(chan T, n)) acting as a private mailbox: a publisher can drop up to
// `bufSize` messages in without the receiver being ready. Past that it's full,
// and the non-blocking send in Publish will skip this subscriber.
type subscriber struct {
	id    int64
	topic string
	ch    chan domain.Message
}

// Broker is an in-memory publish/subscribe hub. One published message fans out
// to every subscriber of its topic. All shared state lives here, guarded by mu
// — there is no database on purpose (that IS the lesson; see the README).
type Broker struct {
	// sync.RWMutex allows many concurrent readers (RLock) OR one writer (Lock).
	// Publish only reads the topic map, so it takes RLock and many publishes to
	// different topics proceed in parallel; Subscribe/Unsubscribe mutate the map
	// and take the exclusive Lock.
	mu sync.RWMutex

	// A map of topic -> (map of subscriber id -> *subscriber). The inner map keyed
	// by id makes Unsubscribe an O(1) delete. Pointer values (*subscriber) so every
	// holder shares the one struct rather than copying it.
	topics map[string]map[int64]*subscriber

	// atomic types are safe to touch from many goroutines without the mutex.
	// nextID hands out unique subscriber ids; the counters tally traffic. A plain
	// int++ from concurrent goroutines is a data race (-race flags it); .Add is
	// the lock-free fix for a single shared number.
	nextID    atomic.Int64
	published atomic.Int64
	delivered atomic.Int64
	dropped   atomic.Int64

	bufSize int // capacity given to each subscriber's mailbox channel
}

// Returns *Broker (a pointer) so every caller shares the one instance and its
// single copy of the mutex and maps.
func NewBroker(bufSize int) *Broker {
	return &Broker{
		// make is required for a map before you can write to it; a nil map panics on write.
		topics:  make(map[string]map[int64]*subscriber),
		bufSize: bufSize,
	}
}

// Subscribe registers a new listener on topic and returns its id plus a
// RECEIVE-ONLY view of its mailbox.
//
// The return type `<-chan domain.Message` is a DIRECTIONAL channel type: the
// caller may only receive (<-ch) from it, never send or close it. Sending and
// closing stay the broker's job — the compiler enforces that split.
func (b *Broker) Subscribe(topic string) (int64, <-chan domain.Message) {
	// nextID.Add(1) atomically increments and returns the new value — unique even
	// if two goroutines subscribe at the same instant.
	id := b.nextID.Add(1)
	sub := &subscriber{
		id:    id,
		topic: topic,
		// make(chan T, n) is a BUFFERED channel of capacity n. Sends succeed
		// without a waiting receiver until n messages are queued.
		ch: make(chan domain.Message, b.bufSize),
	}

	b.mu.Lock()         // exclusive lock: we're mutating the maps
	defer b.mu.Unlock() // defer runs on return, even if a panic unwinds through here
	if b.topics[topic] == nil {
		// First subscriber for this topic: create its inner map lazily.
		b.topics[topic] = make(map[int64]*subscriber)
	}
	b.topics[topic][id] = sub

	return id, sub.ch // the chan is returned as <-chan by the signature's conversion
}

// Unsubscribe removes a listener and closes its channel so the reader's receive
// loop ends. Safe to call twice (the map lookup guards it).
func (b *Broker) Unsubscribe(topic string, id int64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs := b.topics[topic]
	if subs == nil {
		return
	}
	// Comma-ok form on a map: sub is the value, ok reports whether the key existed.
	if sub, ok := subs[id]; ok {
		delete(subs, id) // delete removes a key from a map (no-op if absent)
		// close(ch) makes the reader's `v, ok := <-ch` return ok==false, ending its
		// loop. Only the sender side (the broker) may close — never the receiver.
		close(sub.ch)
	}
	if len(subs) == 0 {
		delete(b.topics, topic) // drop empty topics so Topics() stays tidy
	}
}

// Publish builds a Message and fans it out to every current subscriber of topic.
func (b *Broker) Publish(topic, body string) domain.Message {
	msg := domain.Message{
		ID:        b.nextID.Add(1),
		Topic:     topic,
		Body:      body,
		CreatedAt: time.Now(),
	}
	b.published.Add(1)

	// RLock (read lock): we only read the map here, so any number of concurrent
	// publishes may run at once. We snapshot the pointers under the lock and send
	// while still holding it — sends are non-blocking (below), so we never sleep
	// with the lock held.
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, sub := range b.topics[topic] {
		// *** THE KEY TEACHING POINT: a NON-BLOCKING SEND. ***
		// A `select` with a `default` case never blocks: if the send in the first
		// case can't proceed *right now* (the subscriber's buffered channel is
		// full), Go takes `default` instead. So one slow consumer is DROPPED rather
		// than stalling the publisher or every other subscriber of this topic.
		select {
		case sub.ch <- msg:
			b.delivered.Add(1)
		default:
			b.dropped.Add(1) // mailbox full: this is backpressure — drop, don't block
		}
	}
	return msg
}

// Topics returns a snapshot of every live topic and its subscriber count.
func (b *Broker) Topics() []domain.TopicInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Pre-size the slice's capacity to the number of topics; length starts at 0
	// and grows via append. Avoids re-allocating as we fill it.
	out := make([]domain.TopicInfo, 0, len(b.topics))
	for name, subs := range b.topics { // range over a map gives (key, value)
		out = append(out, domain.TopicInfo{Name: name, Subscribers: len(subs)})
	}
	return out
}

// Stats returns the atomic counters plus current topic/subscriber totals.
// map[string]int64 marshals straight to a small JSON object in the handler.
func (b *Broker) Stats() map[string]int64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var subs int64
	for _, m := range b.topics {
		subs += int64(len(m)) // int64(...) is a type conversion, not a cast
	}
	return map[string]int64{
		"published":   b.published.Load(), // .Load atomically reads the counter
		"delivered":   b.delivered.Load(),
		"dropped":     b.dropped.Load(),
		"topics":      int64(len(b.topics)),
		"subscribers": subs,
	}
}

// Shutdown closes every subscriber channel so all SSE handlers unblock and
// return. Called once from main on graceful shutdown.
func (b *Broker) Shutdown() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for topic, subs := range b.topics {
		for id, sub := range subs {
			close(sub.ch) // ends each SSE reader's receive loop (v, ok := <-ch → ok false)
			delete(subs, id)
		}
		delete(b.topics, topic)
	}
}
