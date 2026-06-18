package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"pinghub/internal/domain"
)

// Checker runs concurrent URL health checks using a bounded worker pool.
type Checker struct {
	repo        domain.CheckRepository
	client      *http.Client
	workerCount int

	// totalChecks counts every URL ever probed, across all jobs and all
	// goroutines. It is written from many workers at once, so it is an
	// atomic counter rather than a plain int (a plain int++ would race).
	totalChecks atomic.Int64
}

func NewChecker(repo domain.CheckRepository, workerCount int) *Checker {
	return &Checker{
		// One shared http.Client is safe to use from many goroutines.
		// We give it no timeout of its own — each request carries its own
		// deadline via context (see probe), which is the idiomatic way.
		repo:        repo,
		client:      &http.Client{},
		workerCount: workerCount,
	}
}

// Run probes every URL concurrently and returns the stored job summary.
//
// The shape is the classic fan-out / fan-in pipeline:
//
//		urls ──► [jobs chan] ──► N worker goroutines ──► [results chan] ──► collector
//		          (fan-out)                                (fan-in)
//
//	  - ctx cancels the whole batch (e.g. the HTTP client disconnected).
//	  - perURL is the timeout applied to each individual request.
//
// Results come back in completion order (fastest first), NOT request order —
// that ordering is itself a visible consequence of running concurrently.
func (c *Checker) Run(ctx context.Context, urls []string, perURL time.Duration) *domain.CheckJob {
	start := time.Now()

	jobs := make(chan string)
	results := make(chan domain.CheckResult)

	// Fan-out: start a FIXED number of workers. This is what bounds
	// concurrency — 1000 URLs still only ever open `workerCount` requests at
	// once, instead of spawning 1000 goroutines that hammer the network.
	var wg sync.WaitGroup
	for i := 0; i < c.workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for url := range jobs { // pull work until `jobs` is closed
				results <- c.probe(ctx, url, perURL)
			}
		}()
	}

	// Feed the jobs channel from its own goroutine so the collector below can
	// read results at the same time. (With unbuffered channels, sending all
	// jobs before reading any result would deadlock.)
	go func() {
		for _, url := range urls {
			select {
			case jobs <- url:
			case <-ctx.Done(): // caller cancelled: stop handing out work
			}
		}
		close(jobs) // closing makes every worker's `range jobs` loop end
	}()

	// Closer: once every worker has returned, nobody will send to `results`
	// again, so it is safe to close it — which ends the collector loop.
	go func() {
		wg.Wait()
		close(results)
	}()

	// Fan-in: collect on this goroutine until `results` is closed.
	job := &domain.CheckJob{Requested: len(urls), CreatedAt: start}
	for res := range results {
		job.Results = append(job.Results, res)
		switch res.Status {
		case domain.StatusUp:
			job.Up++
		case domain.StatusDown:
			job.Down++
		default:
			job.Errored++
		}
	}

	job.DurationMS = time.Since(start).Milliseconds()
	c.repo.Save(job)
	return job
}

// Get returns a previously stored job.
func (c *Checker) Get(id int64) (*domain.CheckJob, error) { return c.repo.Get(id) }

// List returns all stored jobs, newest first.
func (c *Checker) List() []*domain.CheckJob { return c.repo.List() }

// TotalChecks returns how many URLs have been probed since startup.
func (c *Checker) TotalChecks() int64 { return c.totalChecks.Load() }

// probe performs a single GET under a per-request timeout derived from ctx.
func (c *Checker) probe(ctx context.Context, url string, timeout time.Duration) domain.CheckResult {
	c.totalChecks.Add(1) // atomic: safe even though many workers call this at once

	// context.WithTimeout gives this one request its own deadline. It also
	// inherits cancellation from the parent ctx, so cancelling the batch
	// cancels in-flight requests too. Always cancel() to release resources.
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return domain.CheckResult{URL: url, Status: domain.StatusError, Error: "bad request: " + shortErr(err)}
	}

	resp, err := c.client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		// timeouts, DNS failures, connection refused all land here
		return domain.CheckResult{URL: url, Status: domain.StatusError, LatencyMS: latency, Error: shortErr(err)}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body) // drain so the connection can be reused

	res := domain.CheckResult{URL: url, HTTPStatus: resp.StatusCode, LatencyMS: latency}
	if resp.StatusCode < 400 {
		res.Status = domain.StatusUp
	} else {
		res.Status = domain.StatusDown
	}
	return res
}

// shortErr trims net/http's wrapping (`Get "url": <reason>`) down to the reason.
func shortErr(err error) string {
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i != -1 {
		return msg[i+2:]
	}
	return msg
}
