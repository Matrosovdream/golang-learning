# Step 27 — gRPC & Microservices · Progress

Do the [one-time setup](README.md#one-time-setup-do-this-first) once, then type & run each example;
tick it once your output matches. Examples are split by tier:
[🟢 easy](1-easy.md) · [🟡 medium](2-medium.md) · [🔴 hard](3-hard.md).

> ▶ **Resume here:** 🟢 **easy** tier — do the one-time setup, then start with example **1. Your first .proto and what protoc generates**. None ticked yet.


### 🟢 easy — [1-easy.md](1-easy.md)
- [ ] 1. Your first .proto and what protoc generates
- [ ] 2. Hello gRPC in one process (bufconn)
- [ ] 3. The wire format: proto.Marshal vs JSON
- [ ] 4. Every call gets a deadline
- [ ] 5. Status codes, not error strings
- [ ] 6. A real TCP server (two terminals) and codes.Unavailable
- [ ] 7. proto3 field presence: zero value vs optional
- [ ] 8. Server-streaming: many replies, read to io.EOF

### 🟡 medium — [2-medium.md](2-medium.md)
- [ ] 9. Client-streaming: one summary reply
- [ ] 10. Bidirectional streaming: Chat
- [ ] 11. Metadata: the request's headers
- [ ] 12. A unary server interceptor that logs
- [ ] 13. Chaining interceptors
- [ ] 14. A client interceptor that injects a request-id
- [ ] 15. Propagate a request-id across a hop
- [ ] 16. A recovery interceptor: panic → codes.Internal
- [ ] 17. Time every RPC (the seed of a metric)

### 🔴 hard — [3-hard.md](3-hard.md)
- [ ] 18. A stream interceptor (wrapping ServerStream)
- [ ] 19. Structured logging with slog + request_id
- [ ] 20. Prometheus metrics interceptor + /metrics
- [ ] 21. Health checking (grpc_health_v1)
- [ ] 22. Server reflection (talk to it with grpcurl)
- [ ] 23. Client retry with backoff on Unavailable
- [ ] 24. Capstone: two services, logging + metrics wired

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
