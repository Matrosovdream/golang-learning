# Step 63 — Networking Fundamentals · Progress

Type & run each example, then tick it once your output matches. Tiers: [🟢 easy](1-easy.md) · [🟡 medium](2-medium.md) · [🔴 hard](3-hard.md).

> ▶ **Resume here:** 🟢 **easy** tier — start with example **1. Your first TCP server**. None ticked yet.

### 🟢 easy — [1-easy.md](1-easy.md)
- [ ] 1. Your first TCP server
- [ ] 2. A TCP client with net.Dial
- [ ] 3. One goroutine per connection
- [ ] 4. Port :0 — let the kernel choose
- [ ] 5. The 4-tuple: why one port serves many clients
- [ ] 6. TCP has no message boundaries
- [ ] 7. Framing I — newline-delimited
- [ ] 8. Framing II — length-prefixed

### 🟡 medium — [2-medium.md](2-medium.md)
- [ ] 9. EOF means the peer closed
- [ ] 10. Half-close with CloseWrite
- [ ] 11. Read deadlines and net.Error
- [ ] 12. An idle timeout that drops silent clients
- [ ] 13. Unblocking a stuck Read by closing
- [ ] 14. Graceful shutdown of a TCP server
- [ ] 15. Capping concurrent connections
- [ ] 16. UDP: datagrams keep their boundaries
- [ ] 17. The UDP truncation trap

### 🔴 hard — [3-hard.md](3-hard.md)
- [ ] 18. Unix domain sockets
- [ ] 19. Dialing with a context
- [ ] 20. DNS lookups you can cancel
- [ ] 21. Reading network errors: refused / timeout / EOF / reset
- [ ] 22. Connection reuse vs dial-per-request
- [ ] 23. Socket options: Dialer, ListenConfig and Control
- [ ] 24. 2000 connections, 2000 goroutines
- [ ] 25. A TCP proxy in two io.Copy calls
- [ ] 26. Capstone: a line-protocol key-value server

---

**Notes**

- Every example runs standalone: `go run main.go`. No ports to configure — each binds `127.0.0.1:0`.
- Examples 6, 7 and 8 are the heart of the tier: read them back to back.
- If an example hangs, you found the lesson's point — check which side is waiting for a delimiter that never came.
