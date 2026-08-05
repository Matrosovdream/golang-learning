# Step 64 — The HTTP Protocol & `net/http` Internals · Progress

Type & run each example, then tick it once your output matches. Tiers: [🟢 easy](1-easy.md) · [🟡 medium](2-medium.md) · [🔴 hard](3-hard.md).

> ▶ **Resume here:** 🟢 **easy** tier — start with example **1. HTTP/1.1 by hand over a raw socket**. None ticked yet.

### 🟢 easy — [1-easy.md](1-easy.md)
- [ ] 1. HTTP/1.1 by hand over a raw socket
- [ ] 2. Dumping requests and responses
- [ ] 3. Content-Length vs chunked
- [ ] 4. Chunked encoding, decoded by hand
- [ ] 5. Build the server explicitly — the five timeouts
- [ ] 6. Headers must be set before the first Write
- [ ] 7. Content-Type sniffing
- [ ] 8. Everything the server knows about a request

### 🟡 medium — [2-medium.md](2-medium.md)
- [ ] 9. ConnState: the accept loop, observed
- [ ] 10. Keep-alive: 10 requests, 1 connection
- [ ] 11. ReadHeaderTimeout defeats the slow loris
- [ ] 12. WriteTimeout breaks streaming — ResponseController fixes it
- [ ] 13. Flusher: sending bytes before the handler returns
- [ ] 14. Hijacker: taking the raw connection
- [ ] 15. The Transport is a connection pool
- [ ] 16. Drain the body, or lose the connection
- [ ] 17. Client timeouts: blanket vs per-request context

### 🔴 hard — [3-hard.md](3-hard.md)
- [ ] 18. TLS end to end with a generated certificate
- [ ] 19. ALPN: where HTTP/2 actually comes from
- [ ] 20. Multiplexing: 20 concurrent requests, 1 connection
- [ ] 21. Head-of-line blocking, measured
- [ ] 22. TimeoutHandler: a real 503 instead of a dropped connection
- [ ] 23. X-Forwarded-For: trust only your own hop
- [ ] 24. A reverse proxy in ten lines
- [ ] 25. Wrapping ResponseWriter without breaking Flush
- [ ] 26. Capstone: a server built properly, and a client that talks to it right

---

**Notes**

- Every example runs standalone: `go run main.go`. No ports to configure, no servers to start.
- Examples 1–4 are the foundation: do them in order, and read the raw bytes rather than skimming.
- Examples 15 and 16 are the two client habits that matter most in production code.
- Example 18 generates a real certificate in-process — no `openssl` needed.
