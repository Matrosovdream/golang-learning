package service

import (
	"context"
	"time"
)

// Limiter is a token-bucket rate limiter built from a ticker and a buffered
// channel — no third-party library. The buffer is the burst size; a background
// goroutine drips one token per interval. Callers block in Wait until a token
// is available (or their context is cancelled).
type Limiter struct {
	tokens chan struct{}
	stop   chan struct{}
}

// NewLimiter allows up to perSec checks per second, with a burst of `burst`
// queued at once. perSec <= 0 disables limiting (Wait returns immediately).
func NewLimiter(perSec, burst int) *Limiter {
	if burst < 1 {
		burst = 1
	}
	l := &Limiter{
		tokens: make(chan struct{}, burst),
		stop:   make(chan struct{}),
	}
	for i := 0; i < burst; i++ { // start full so the first burst goes through
		l.tokens <- struct{}{}
	}
	if perSec > 0 {
		go l.refill(time.Second / time.Duration(perSec))
	}
	return l
}

func (l *Limiter) refill(every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			select {
			case l.tokens <- struct{}{}: // add a token...
			default: // ...unless the bucket is already full
			}
		case <-l.stop:
			return
		}
	}
}

// Wait blocks until a token is free or ctx is done.
func (l *Limiter) Wait(ctx context.Context) error {
	if l == nil {
		return nil
	}
	select {
	case <-l.tokens:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close stops the refill goroutine.
func (l *Limiter) Close() { close(l.stop) }
