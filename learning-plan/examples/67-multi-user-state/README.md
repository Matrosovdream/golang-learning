# Step 67 — Multi-User State in One Process · Examples

A library of **24 runnable examples**, split into three files by difficulty. Each is a complete
`package main` program: read the concept and steps, then **retype the code block** into a scratch
folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .        # every example here is concurrent — run go run -race . too
```

Every example was compiled, `gofmt`-checked, `go vet`-ed, and run — under `-race` too — before
being added; the **Output** under each one is real stdout. (One deliberate exception: example 1
*is* a data race, which is exactly what it teaches.)

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–8 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 9–17 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 18–24 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## What this lesson is for

[15 — Sync, Context & Patterns](../../15-sync-context.md) taught the **primitives**: mutex, atomic,
context. This lesson is the **situation** those primitives exist for — one long-lived Go process,
hundreds of users inside it at once, memory shared by default. It's the bridge to
[20 — HTTP Server Fundamentals](../../20-http-server.md), and the examples build toward a small but
complete multi-user service.

The organizing idea, which examples 1–8 establish and the rest apply:

| Scope | Lives for | Protection |
|---|---|---|
| **Request** | one handler call | none needed — one goroutine owns it |
| **User** | many requests, one user | shared map → mutex |
| **Process** | the whole server | mutex / atomic / single-owner goroutine |

## Index

### 🟢 [Easy](1-easy.md) — the memory model

- [1. One process, many users: the shared global](1-easy.md#1-one-process-many-users-the-shared-global)
- [2. The fix: a type that owns its lock](1-easy.md#2-the-fix-a-type-that-owns-its-lock)
- [3. Request-scoped state is free](1-easy.md#3-request-scoped-state-is-free)
- [4. httptest: calling a handler with no server](1-easy.md#4-httptest-calling-a-handler-with-no-server)
- [5. A real server, 100 real clients](1-easy.md#5-a-real-server-100-real-clients)
- [6. Identity: credential → context → handler](1-easy.md#6-identity-credential--context--handler)
- [7. `r.Context()` is cancelled when the client leaves](1-easy.md#7-rcontext-is-cancelled-when-the-client-leaves)
- [8. Per-user data in one shared map](1-easy.md#8-per-user-data-in-one-shared-map)

### 🟡 [Medium](2-medium.md) — the patterns

- [9. The escaped pointer: a lock that protects nothing](2-medium.md#9-the-escaped-pointer-a-lock-that-protects-nothing)
- [10. A per-user rate limiter](2-medium.md#10-a-per-user-rate-limiter)
- [11. Presence: who is online right now](2-medium.md#11-presence-who-is-online-right-now)
- [12. The sweeper: a background goroutine that shuts down cleanly](2-medium.md#12-the-sweeper-a-background-goroutine-that-shuts-down-cleanly)
- [13. Hot-reloaded config with `atomic.Pointer`](2-medium.md#13-hot-reloaded-config-with-atomicpointer)
- [14. The `r.Context()` trap: background work that dies with the response](2-medium.md#14-the-rcontext-trap-background-work-that-dies-with-the-response)
- [15. Idempotency: the double-clicked "Pay" button](2-medium.md#15-idempotency-the-double-clicked-pay-button)
- [16. Lock striping: don't let alice block bob](2-medium.md#16-lock-striping-dont-let-alice-block-bob)
- [17. Fan-out: alice posts, everyone sees it](2-medium.md#17-fan-out-alice-posts-everyone-sees-it)

### 🔴 [Hard](3-hard.md) — the hard parts

- [18. The hub: a single owner and no mutex anywhere](3-hard.md#18-the-hub-a-single-owner-and-no-mutex-anywhere)
- [19. Graceful shutdown: finish what you started](3-hard.md#19-graceful-shutdown-finish-what-you-started)
- [20. Check-then-act: atomics do not save you](3-hard.md#20-check-then-act-atomics-do-not-save-you)
- [21. Request IDs: making 200 concurrent users readable](3-hard.md#21-request-ids-making-200-concurrent-users-readable)
- [22. What breaks when you run a second replica](3-hard.md#22-what-breaks-when-you-run-a-second-replica)
- [23. Lost updates and optimistic locking](3-hard.md#23-lost-updates-and-optimistic-locking)
- [24. Capstone: a small multi-user service](3-hard.md#24-capstone-a-small-multi-user-service)

---
*Lesson: [../../67-multi-user-state.md](../../67-multi-user-state.md) · Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
