# 67 — Multi-User State in One Process

> The bridge between [15 — Sync, Context & Patterns](15-sync-context.md) (the *primitives*) and
> [20 — HTTP Server Fundamentals](20-http-server.md) (the *server*). Lesson 15 taught you what a
> mutex is. This one is about the situation mutexes exist for: **one process, many users, shared
> memory.**

## Goals
- Internalize the Go server memory model: one long-lived process, one goroutine per request, memory shared by default.
- Decide correctly, for any piece of state, whether it is **request-scoped**, **user-scoped**, or **process-scoped** — and what protects it.
- Carry identity from a credential to the bottom of the call stack via `context`.
- Build the four things every multi-user service needs: per-user state, presence, fan-out, graceful shutdown.
- Recognize the bugs that only appear with concurrent users: check-then-act, lost updates, escaped pointers, the `r.Context()` trap.
- Know exactly what breaks the moment you run a second replica.

## Concepts

### The memory model shift
- **PHP/CGI:** every request is a **fresh process**. Nothing is shared, globals reset, memory dies at the end of the request. "Session" means a file or a Redis key, because there is nowhere else to put it.
- **Go:** the server is **one process that runs for weeks**. `net/http` accepts a connection and calls your handler **in its own goroutine** — so at 500 requests/sec you have hundreds of copies of your handler running *simultaneously, in the same address space*.
- Consequence: **every package-level variable, every field of a struct your handler closes over, every map you built at startup is shared by all users at once.** Lesson 15's primitives are not academic — they are the price of admission.

### The three scopes of state
Everything in a server is exactly one of these. Getting the classification right is 90% of the work:

| Scope | Lives for | Examples | Protection |
|---|---|---|---|
| **Request** | one handler call | locals, `r.Context()` values, decoded body | **none needed** — one goroutine owns it |
| **User** | many requests, one user | rate-limit budget, session, presence, cart | shared map → **mutex** |
| **Process** | the whole server | config, DB pool, metrics, the hub, caches | **mutex / atomic / single-owner goroutine** |

- **Request-scoped state is free.** Locals declared inside a handler live on that goroutine's stack; no other request can reach them. This is why handlers should prefer locals and why "just make it a global" is how you create races.
- **User-scoped state is the interesting one** — it's shared (the map holding it) *and* per-user (each entry). Both levels need thought.

### Identity: credential → context → handler
- Middleware verifies the credential **once**, then stashes the identity in the request context under an **unexported key type** (`type userKey struct{}`), same idiom as [15](15-sync-context.md) example 10 and [43](43-authorization-rbac-multitenancy.md).
- `r.WithContext(ctx)` returns a **copy** of the request — you never mutate a `*http.Request` in place.
- Handlers read the identity through a helper (`userFrom(ctx)`), never by re-parsing the token.
- Middleware order matters: you cannot rate-limit *per user* before you know who the user is.

### `r.Context()` — what it is and the trap
- It is **cancelled when the client disconnects** *and* **when the handler returns**. Watching it (`<-r.Context().Done()`) is how you stop work nobody will receive.
- **The trap:** background work started with `go doWork(r.Context())` is killed the instant the handler returns. Use **`context.WithoutCancel(r.Context())`** (Go 1.21+) to keep the request's *values* (request id, trace, logger) while dropping its *cancellation*. Better still, hand the job to a real queue ([44](44-background-jobs-queues.md)).

### The patterns
- **Guarded store** — `struct { mu sync.Mutex; m map[K]V }` with methods. The lock lives *inside* the type so no caller can forget it. **Return copies, not pointers** — a pointer handed out past the unlock is unprotected shared state.
- **Per-user data** — `map[userID]*state`. One lock for the map is the right default; **lock striping** (a mutex per user) only when profiling shows users blocking each other.
- **Read-mostly config** — `atomic.Pointer[Config]`, swapped whole. Readers never block, never take a lock, and always see a complete snapshot.
- **Presence** — `map[userID]time.Time` + `RWMutex` + a **background sweeper goroutine** on a ticker that deletes stale entries. Filtering stale users on read is not enough: the map grows forever without a sweeper.
- **Fan-out** — one subscriber channel per user, published to with a **non-blocking send** (`select { case ch <- m: default: drop }`) so one slow reader can't stall everyone.
- **The hub (actor)** — a single goroutine owns the client map and serves `register`/`unregister`/`broadcast` channels. **No mutex exists** because only one goroutine touches the state. Request/response works by sending a reply channel. This is [58](58-realtime-websockets-sse.md)'s hub, and lesson 15 example 11(b) scaled up.
- **Graceful shutdown** — `srv.Shutdown(ctx)` stops the listener and **blocks until in-flight requests finish**. Pair with a `WaitGroup` for your own background goroutines and cancel their (server-scoped, not request-scoped) context.

### The concurrency bugs that only appear with many users
- **Check-then-act.** `if seats > 0 { seats-- }` is two operations. **Atomics do not fix this** — `atomic.Load` then `atomic.Add` is still two operations, and 50 goroutines can all read "1 left". Only one critical section covering *both* steps is correct.
- **Lost update.** Two users load the same record, both edit, both save; the second silently overwrites the first. A mutex cannot help — the reads happened in different requests. Fix with **optimistic locking**: a `version` column and `UPDATE … WHERE id = ? AND version = ?`, checking `RowsAffected == 1`, returning **409 Conflict** to the loser.
- **Escaped pointer.** A getter that takes the lock and returns `*T` protected nothing; the caller reads and writes it lock-free.
- **Counter under `RLock`.** `RLock` admits *many* concurrent readers, so mutating anything under it is a data race. Use an atomic, or take the write lock.
- **Idempotency.** A double-clicked "Pay" is two goroutines with the same key. Check-and-insert in one critical section (or a unique index) so the card is charged once — see [35](35-sagas-distributed-transactions.md), [41](41-api-design-evolution.md).

### What breaks at two replicas
The hard boundary: **a mutex only coordinates goroutines inside one process.** The moment a second replica starts, everything in memory silently diverges — sessions, rate limits, caches, locks, presence, the hub. Fixes: move the state to a shared backend (Redis/Postgres), add a pub/sub backplane for broadcast ([58](58-realtime-websockets-sse.md)), and use the **database** as the cross-process lock (optimistic locking, `SELECT … FOR UPDATE`). Program against a small interface from day one so the in-memory and shared implementations are swappable.

## Exercises
1. Write a handler that increments a package-level `visits` counter, drive it with 200 concurrent `httptest` requests, and watch `go run -race .` report the race and the count come up short. Then fix it with a `Counter` type that owns its mutex.
2. Build an auth middleware that reads a header, stashes a `User` under an unexported context key, and a handler that reads it back — plus the 401 path where the handler never runs.
3. Build a `Store` with `Get` returning a **copy** and `GetPointer` returning a pointer; prove that a caller can corrupt the store through the pointer.
4. Build a per-user rate limiter (`map[string]int` + mutex, N tokens each) and prove with concurrent requests that each user gets exactly N `200`s and the rest `429`.
5. Build a presence tracker with `RWMutex` and a **sweeper goroutine** that expires stale users on a ticker and stops cleanly on `ctx` cancellation + `wg.Wait()`.
6. Demonstrate the `r.Context()` trap: start background work with `r.Context()` and with `context.WithoutCancel(r.Context())` from a **real** `httptest.NewServer`, and show only the second one survives.
7. Write the seat-booking stampede: 50 goroutines, 10 seats, once with `atomic.Load`/`Add` (oversells) and once with a mutex around check *and* decrement (exact).
8. Build a hub goroutine that owns a client map with **no mutex anywhere**, supporting register, unregister, broadcast, and a roster query via reply channel.
9. Wire `srv.Shutdown(ctx)`: fire a slow request, shut down mid-flight, and show the in-flight request completes while new ones are refused.
10. Take any of the above and run `go run -race .` with a concurrent load loop. Make it part of how you finish a feature.

## Best Practices & Pitfalls
- **Classify every piece of state as request / user / process before writing the code.** Most concurrency bugs are misclassifications.
- **Prefer request-scoped locals.** State that never leaves the handler goroutine needs no protection and can't be raced.
- **Put the lock inside the type that owns the data**, and never export a field that needs the lock. Callers cannot be trusted to remember.
- **Return copies from getters.** If the value contains a slice, map, or pointer, copy it *inside* the critical section.
- **Never do I/O while holding a lock.** Compute, then lock only for the write ([15](15-sync-context.md) example 13).
- **Pitfall — atomics on check-then-act.** Atomic steps ≠ atomic transaction. If the decision depends on the value you're about to change, you need one critical section.
- **Pitfall — `go work(r.Context())`.** The request context dies with the response. `context.WithoutCancel` or a job queue.
- **Pitfall — mutating anything under `RLock`.** Read lock means *read*.
- **Pitfall — a getter that returns a pointer into a guarded map.** The lock ends at the `return`; the aliasing doesn't.
- **Pitfall — assuming one replica.** Ask of every in-memory design: "what happens when this runs twice?" Put it behind an interface now, so swapping in Redis later is a constructor change.
- **Non-blocking sends for fan-out.** One slow subscriber must never be able to stall the publisher.
- **Always `srv.Shutdown(ctx)`**, and `wg.Wait()` your own background goroutines. Cancelling isn't the same as having stopped.
- **`-race` under concurrent load, every time.** A `-race` run that only exercises one request proves nothing — races only report on code paths that actually execute concurrently.

## Checklist
- [ ] I can explain why a package-level variable in a Go server is shared but in PHP is not.
- [ ] I can classify state as request-, user-, or process-scoped and pick the right protection.
- [ ] I can write auth middleware that puts identity in the context, and read it back downstream.
- [ ] I can build a guarded per-user store that returns copies, not pointers.
- [ ] I can build presence with a sweeper that shuts down cleanly.
- [ ] I know why `go doWork(r.Context())` is a bug and what to use instead.
- [ ] I can spot check-then-act and know atomics don't fix it.
- [ ] I can explain a lost update and fix it with optimistic locking + 409.
- [ ] I can write a hub goroutine that needs no mutex at all.
- [ ] I can implement graceful shutdown and prove in-flight requests complete.
- [ ] I can name what breaks when a second replica starts.

## Resources
- `net/http` — https://pkg.go.dev/net/http
- `net/http/httptest` — https://pkg.go.dev/net/http/httptest
- `context.WithoutCancel` (Go 1.21+) — https://pkg.go.dev/context#WithoutCancel
- `http.Server.Shutdown` — https://pkg.go.dev/net/http#Server.Shutdown
- Blog — Go Concurrency Patterns: Context: https://go.dev/blog/context
- The Go Memory Model: https://go.dev/ref/mem
- Related lessons: [15](15-sync-context.md) primitives · [20](20-http-server.md) HTTP · [43](43-authorization-rbac-multitenancy.md) authZ & tenancy · [56](56-authentication-sessions.md) sessions · [58](58-realtime-websockets-sse.md) hub & backplane · [44](44-background-jobs-queues.md) job queues

---
*Examples: [examples/67-multi-user-state/](examples/67-multi-user-state/) (24) · Global progress: [PROGRESS.md](PROGRESS.md).*
