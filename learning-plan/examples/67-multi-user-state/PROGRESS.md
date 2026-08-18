# Step 67 — Multi-User State in One Process · Progress

Type & run each example; tick once your output matches. Examples are split by tier:
[🟢 easy](1-easy.md) · [🟡 medium](2-medium.md) · [🔴 hard](3-hard.md).

> ▶ **Resume here:** 🟢 **easy** tier — start with example **1. One process, many users: the shared global**. None ticked yet.


### 🟢 easy — [1-easy.md](1-easy.md)
- [ ] 1. One process, many users: the shared global
- [ ] 2. The fix: a type that owns its lock
- [ ] 3. Request-scoped state is free
- [ ] 4. httptest: calling a handler with no server
- [ ] 5. A real server, 100 real clients
- [ ] 6. Identity: credential → context → handler
- [ ] 7. r.Context() is cancelled when the client leaves
- [ ] 8. Per-user data in one shared map

### 🟡 medium — [2-medium.md](2-medium.md)
- [ ] 9. The escaped pointer: a lock that protects nothing
- [ ] 10. A per-user rate limiter
- [ ] 11. Presence: who is online right now
- [ ] 12. The sweeper: a background goroutine that shuts down cleanly
- [ ] 13. Hot-reloaded config with atomic.Pointer
- [ ] 14. The r.Context() trap: background work that dies with the response
- [ ] 15. Idempotency: the double-clicked "Pay" button
- [ ] 16. Lock striping: don't let alice block bob
- [ ] 17. Fan-out: alice posts, everyone sees it

### 🔴 hard — [3-hard.md](3-hard.md)
- [ ] 18. The hub: a single owner and no mutex anywhere
- [ ] 19. Graceful shutdown: finish what you started
- [ ] 20. Check-then-act: atomics do not save you
- [ ] 21. Request IDs: making 200 concurrent users readable
- [ ] 22. What breaks when you run a second replica
- [ ] 23. Lost updates and optimistic locking
- [ ] 24. Capstone: a small multi-user service

---
*Lesson: [../../67-multi-user-state.md](../../67-multi-user-state.md) · Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
