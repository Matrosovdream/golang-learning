package service

import (
	"context"
	"time"
)

// Limiter is a TOKEN-BUCKET rate limiter built from stdlib only. A time.Ticker
// drips one token into a buffered channel every 1/rate seconds; Wait() consumes
// one token per call and blocks when the bucket is empty. The buffer's capacity
// is the maximum BURST — how many fetches may fire back-to-back before throttling.
type Limiter struct {
	tokens chan struct{} // the bucket: a receive takes a token, a send adds one
	ticker *time.Ticker  // fires on a fixed interval to refill the bucket
	done   chan struct{} // closed by Stop() to end the refiller goroutine
}

// NewLimiter starts the refiller goroutine and returns a ready limiter. The
// bucket starts FULL so an initial burst of `ratePerSec` fetches isn't throttled.
func NewLimiter(ratePerSec int) *Limiter {
	if ratePerSec < 1 {
		ratePerSec = 1
	}
	l := &Limiter{
		tokens: make(chan struct{}, ratePerSec), // buffer == burst size
		ticker: time.NewTicker(time.Second / time.Duration(ratePerSec)),
		done:   make(chan struct{}),
	}
	for i := 0; i < ratePerSec; i++ {
		l.tokens <- struct{}{} // pre-fill: the buffer has room for exactly ratePerSec
	}

	// Refiller: on every tick add one token; if the bucket is already full the
	// default branch drops it — that non-blocking send is what CAPS the burst.
	go func() {
		for {
			select {
			case <-l.ticker.C: // Ticker delivers ticks on its C channel
				select {
				case l.tokens <- struct{}{}: // room for a token: add it
				default: // bucket full: discard, don't block
				}
			case <-l.done: // Stop() was called: exit the goroutine
				return
			}
		}
	}()
	return l
}

// Wait blocks until a token is available or ctx is cancelled. It returns
// ctx.Err() on cancellation so the caller can abandon a dead crawl. The select
// races the two channels: whichever is ready first wins.
func (l *Limiter) Wait(ctx context.Context) error {
	select {
	case <-l.tokens: // took a token: proceed
		return nil
	case <-ctx.Done(): // crawl cancelled/timed out: give up
		return ctx.Err()
	}
}

// Stop ends the refiller goroutine and stops the ticker (releasing its timer).
// close(done) makes the `<-l.done` receive in the goroutine return immediately.
func (l *Limiter) Stop() {
	l.ticker.Stop()
	close(l.done)
}
