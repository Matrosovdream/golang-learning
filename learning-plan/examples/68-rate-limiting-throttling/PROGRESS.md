# Step 68 — Rate Limiting & Throttling · Progress

Type & run each example; tick once your output matches. Examples are split by tier:
[🟢 easy](1-easy.md) · [🟡 medium](2-medium.md) · [🔴 hard](3-hard.md).

> ▶ **Resume here:** 🟢 **easy** tier — start with example **1. Pace outbound calls with a ticker**. None ticked yet.


### 🟢 easy — [1-easy.md](1-easy.md)
- [ ] 1. Pace outbound calls with a ticker
- [ ] 2. Fixed window — and its 2× boundary burst
- [ ] 3. Allow() — the inbound shape
- [ ] 4. Wait(ctx) — the outbound shape, and its error trap
- [ ] 5. Reserve(): compute Retry-After, and always Cancel()
- [ ] 6. Rate vs burst: what NewLimiter(r, b) permits
- [ ] 7. Middleware: 429, Retry-After and RateLimit-*
- [ ] 8. Pace an http.Client with a RoundTripper

### 🟡 medium — [2-medium.md](2-medium.md)
- [ ] 9. Sliding window log — exact, and what it costs
- [ ] 10. Sliding window counter — the production compromise
- [ ] 11. Leaky bucket — shaping, not just limiting
- [ ] 12. Token bucket by hand — lazy refill, no goroutine
- [ ] 13. Per-key limiters: one budget per tenant
- [ ] 14. Evicting idle keys — the leak every per-key limiter has
- [ ] 15. Tiered limits: the budget comes from the plan
- [ ] 16. Several limits at once — and why Cancel() can't save you
- [ ] 17. Rate limit vs concurrency limit
- [ ] 18. Load shedding on queue depth
- [ ] 19. Be a polite client: honour Retry-After

### 🔴 hard — [3-hard.md](3-hard.md)
- [ ] 20. Distributed fixed window with an atomic INCR
- [ ] 21. Distributed token bucket in one atomic script
- [ ] 22. GCRA — one timestamp per key, exact Retry-After
- [ ] 23. Fail open, fail closed, or fail local
- [ ] 24. A rate-limited, bounded, cancellable fetcher
- [ ] 25. Metrics: the three numbers that matter
- [ ] 26. Capstone: a tenant-aware rate-limited service

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
