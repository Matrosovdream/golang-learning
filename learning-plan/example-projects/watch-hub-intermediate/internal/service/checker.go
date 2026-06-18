package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"watchhub/internal/domain"
)

// Checker probes a single URL with three guards layered on top of the raw GET:
//
//   - a process-wide semaphore (bounded concurrency, shared by every caller),
//   - a token-bucket rate limiter (politeness),
//   - retry-with-backoff on transport errors, each attempt under its own timeout.
type Checker struct {
	client     *http.Client
	sem        chan struct{} // buffered channel used as a counting semaphore
	limiter    *Limiter
	maxRetries int

	inFlight    atomic.Int64 // live gauge: probes running right now
	totalChecks atomic.Int64 // monotonic counter: probes ever started
}

func NewChecker(maxConcurrent int, limiter *Limiter, maxRetries int) *Checker {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	if maxRetries < 1 {
		maxRetries = 1
	}
	return &Checker{
		client:     &http.Client{}, // per-attempt timeout comes from context, not the client
		sem:        make(chan struct{}, maxConcurrent),
		limiter:    limiter,
		maxRetries: maxRetries,
	}
}

func (c *Checker) InFlight() int64    { return c.inFlight.Load() }
func (c *Checker) TotalChecks() int64 { return c.totalChecks.Load() }

// Check probes url, returning the final result after any retries.
func (c *Checker) Check(ctx context.Context, url string, timeout time.Duration) domain.CheckResult {
	// Global concurrency gate: acquire a slot or give up if cancelled. Sending
	// into a buffered channel blocks once `maxConcurrent` slots are taken.
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }() // release the slot on the way out
	case <-ctx.Done():
		return domain.CheckResult{CheckedAt: time.Now(), Status: domain.StatusError, Error: "cancelled before start"}
	}
	c.inFlight.Add(1)
	defer c.inFlight.Add(-1)

	var last domain.CheckResult
	for attempt := 1; attempt <= c.maxRetries; attempt++ {
		// Rate limit: wait for a token so we stay polite under load.
		if err := c.limiter.Wait(ctx); err != nil {
			return domain.CheckResult{CheckedAt: time.Now(), Status: domain.StatusError, Attempts: attempt, Error: "cancelled"}
		}
		last = c.probe(ctx, url, timeout)
		last.Attempts = attempt

		// A 4xx/5xx is a real answer — don't retry it. Only retry transport
		// errors (timeout, refused, DNS), which may be transient.
		if last.Status != domain.StatusError {
			return last
		}
		if attempt < c.maxRetries && !sleepCtx(ctx, backoff(attempt)) {
			return last // ctx cancelled during the backoff sleep
		}
	}
	return last
}

func (c *Checker) probe(ctx context.Context, url string, timeout time.Duration) domain.CheckResult {
	c.totalChecks.Add(1)

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return domain.CheckResult{CheckedAt: start, Status: domain.StatusError, Error: "bad request: " + shortErr(err)}
	}

	resp, err := c.client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return domain.CheckResult{CheckedAt: start, Status: domain.StatusError, LatencyMS: latency, Error: shortErr(err)}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body) // drain so the connection can be reused

	res := domain.CheckResult{CheckedAt: start, HTTPStatus: resp.StatusCode, LatencyMS: latency}
	if resp.StatusCode < 400 {
		res.Status = domain.StatusUp
	} else {
		res.Status = domain.StatusDown
	}
	return res
}

// backoff grows exponentially: 100ms, 200ms, 400ms, ...
func backoff(attempt int) time.Duration {
	return time.Duration(100*(1<<(attempt-1))) * time.Millisecond
}

// sleepCtx waits for d, returning false if ctx is cancelled first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// shortErr trims net/http's wrapping (`Get "url": <reason>`) down to the reason.
func shortErr(err error) string {
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i != -1 {
		return msg[i+2:]
	}
	return msg
}
