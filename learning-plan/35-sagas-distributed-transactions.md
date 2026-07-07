# 35 — Sagas & Distributed Transactions

> Part 9, Track B: [34 Event-Driven & Outbox](34-event-driven-outbox.md) → **35 Sagas** → [36 Resilience](36-resilience-patterns.md) → [37 CQRS & Event Sourcing](37-cqrs-event-sourcing.md).
> You can't wrap a business operation that spans several services in one ACID transaction ([27](27-grpc-microservices.md)'s trap). The **saga** is how you keep such an operation consistent anyway: a chain of local transactions, each with a **compensating** action to undo it.

## Goals
- Explain why cross-service ACID (and two-phase commit) is impractical, and what a **saga** replaces it with.
- Implement both saga styles: **orchestration** (a central coordinator) and **choreography** (services react to events).
- Design **compensating actions** and make every step and compensation **idempotent**.
- Handle the lack of isolation between saga steps with the standard countermeasures.

## Concepts

### Why not 2PC / distributed ACID
A single transaction gives Atomicity, Consistency, Isolation, Durability. Across services with separate databases you lose it: there's no shared transaction manager, and the classic fix — **two-phase commit (2PC)** — is a poor fit for microservices. 2PC holds locks across the network while every participant votes, so one slow or crashed participant blocks all the others (it's a *blocking* protocol), it doesn't scale, and many brokers/NoSQL stores don't support it. So we give up global atomicity and instead build a sequence we can **undo**.

### The saga: local transactions + compensations
A **saga** is a sequence of steps `T1, T2, … Tn`, each a *local* ACID transaction in one service. If step `Ti` fails, the saga runs **compensating** transactions `Ci-1, … C1` to semantically undo the completed steps. Example — placing an order:
```
T1 Order:   create order (PENDING)      C1: cancel order
T2 Stock:   reserve items               C2: release reservation
T3 Payment: charge card                 C3: refund
T4 Shipping:create shipment             (pivot: after this, we commit forward)
```
Each `Ti` commits independently, so between steps the system is in a **valid but intermediate** state (order PENDING, stock reserved). The saga's job is to drive it to a terminal state — **completed** (all forward) or **compensated** (all undone).

### Compensation is *semantic* undo, not rollback
You cannot "roll back" a committed local transaction in another service — you issue a **new** transaction that reverses its business effect. Charge → **Refund**. Reserve → **Release**. Send-email → **Send-correction** (some effects can't truly be undone — you compensate as best the domain allows). Compensations must be:
- **Idempotent** — they'll be retried (at-least-once, [34](34-event-driven-outbox.md)). Refunding twice must refund once.
- **Commutative-ish / always-available** — a compensation should not itself fail for business reasons; design it to (nearly) always succeed, and retry with backoff if it hits transient errors.
- **Ordered in reverse** — undo the most recent completed step first.

### Orchestration — a central coordinator
An **orchestrator** owns the saga's state machine: it calls each service (command via gRPC or a command message), waits for success/failure, and on failure walks the compensations backward. This is the [30](30-patterns-behavioral.md) state-machine + command patterns applied across services:
```go
type Step struct {
    Name       string
    Do         func(ctx context.Context, s *SagaState) error
    Compensate func(ctx context.Context, s *SagaState) error
}

func RunSaga(ctx context.Context, s *SagaState, steps []Step) error {
    done := 0
    for i, step := range steps {
        if err := step.Do(ctx, s); err != nil {          // forward failed at step i
            for j := i - 1; j >= 0; j-- {                // compensate completed steps
                if cerr := steps[j].Compensate(ctx, s); cerr != nil {
                    return fmt.Errorf("compensation %s failed: %w", steps[j].Name, cerr)
                }
            }
            return fmt.Errorf("saga aborted at %s: %w", step.Name, err)
        }
        done = i + 1
        s.Persist(ctx, done)                             // checkpoint after each step
    }
    return nil
}
```
Pros: the whole flow lives in **one place** you can read, log, and visualise; easy to see "where is this order?" Cons: the orchestrator is a component to build and keep available. Persist saga state after each step so a crashed orchestrator resumes instead of restarting.

### Choreography — services react to events
No coordinator: each service does its local transaction and **emits an event**; the next service **subscribes** and reacts; failures emit failure events that trigger compensations. It's pure [34](34-event-driven-outbox.md) pub/sub:
```
Order → OrderCreated → Stock reserves → StockReserved → Payment charges
                                     ↘ StockFailed → Order cancels (C1)
Payment → PaymentFailed → Stock releases (C2) → Order cancels (C1)
```
Pros: no central component, maximal decoupling, services added by subscribing. Cons: the flow is **implicit** — spread across N services' event handlers — so it's hard to follow, debug, and reason about cycles. Good for short (2–3 step) sagas; painful for long ones.

**Rule of thumb:** choreography for simple, short flows; **orchestration once a saga has several steps, branches, or you need to answer "what's the status of this transaction?"**

### The hard part: no isolation between steps
Because each step commits immediately, other transactions can see intermediate state (an order that's PENDING with stock reserved but not yet paid). Sagas lack the **I** in ACID. Countermeasures (from Richardson/Garcia-Molina):
- **Semantic lock** — a status flag (`PENDING`, `APPROVED`) marking a record as "in a saga"; other operations check it and wait/refuse. Compensation clears it.
- **Commutative updates** — design updates that yield the same result regardless of order (increment/decrement rather than set-absolute), so interleaving is safe.
- **Pessimistic view / reordering** — order the steps so the least-reversible or most-contended action happens last, minimising the window and the compensations needed.
- **Re-read value** — before acting, re-read to detect that a concurrent saga changed things.

### Idempotency keys tie it together
Every step and compensation is delivered at-least-once, so each carries an **idempotency key** (often `saga_id + step_name`); the target service dedups on it ([34](34-event-driven-outbox.md), [41](41-api-design-evolution.md)). That's what makes "retry the whole saga after a crash" safe.

## Exercises
1. Write, in a comment, why 2PC is a blocking protocol and why that's unacceptable for a microservice checkout. Name the property a saga gives up in exchange.
2. Design the order saga's four steps with a compensation for each; identify which effect (e.g. a shipped package) can't be perfectly undone and how the domain compensates anyway.
3. Implement the `RunSaga` orchestrator above with in-memory step funcs; make step 3 fail and assert compensations 2 then 1 run in reverse order.
4. Add persistence: checkpoint `done` after each step; simulate an orchestrator crash mid-saga and resume from the checkpoint instead of restarting.
5. Make every step and compensation idempotent with a `saga_id+step` key and a processed-set; replay a compensation twice and prove the effect happens once.
6. Re-model the same saga as **choreography**: three services emitting/handling events (`OrderCreated`, `StockReserved`, `PaymentFailed`, …). Then write two sentences on why tracing this flow is harder than the orchestrated version.
7. Introduce a **semantic lock** (`status = PENDING`) so a concurrent operation can't act on an order mid-saga; show the compensation clearing it.

## Best Practices & Pitfalls
- **Compensate, don't roll back.** A completed local transaction is undone by a new, reversing transaction — refund, release, cancel — not by database rollback.
- **Every step and compensation is idempotent.** Sagas run over at-least-once messaging; retries and resumes will re-deliver. Key on `saga_id+step` and dedup.
- **Persist saga state after each step.** A crash must resume, not restart. The orchestrator's state machine is durable, not in-memory only.
- **Prefer orchestration as flows grow.** One coordinator you can read and monitor beats logic smeared across many event handlers once there are branches or more than a couple of steps.
- **Design compensations to (almost) always succeed.** A compensation that can fail for *business* reasons leaves the saga stuck. If it can only fail transiently, retry with backoff ([36](36-resilience-patterns.md)); if it truly can't compensate, escalate to a human/dead-letter.
- **Pitfall — assuming isolation.** Other transactions see intermediate saga state. Use semantic locks, commutative updates, or step reordering — don't pretend the saga is isolated.
- **Pitfall — a compensation that needs data it no longer has.** Capture what each compensation will need (amounts, ids) in the saga state at forward time; don't assume you can re-derive it after failure.
- **Pitfall — infinite compensation loops.** If a compensation keeps failing, cap retries and dead-letter to an operator. Don't spin forever.
- **Pitfall — choreography for a long saga.** Beyond ~3 steps, the implicit event web becomes unmaintainable. Switch to orchestration.

## Checklist
- [ ] I can explain why 2PC doesn't fit microservices and what consistency model a saga provides instead.
- [ ] I can decompose a business transaction into local steps each with a compensating action.
- [ ] I can implement an orchestration saga with reverse-order compensation and durable checkpoints.
- [ ] I can implement the same saga as choreography and articulate the observability trade-off.
- [ ] I can make steps/compensations idempotent with idempotency keys.
- [ ] I can apply semantic locks / commutative updates to handle the missing isolation.

## Resources
- Chris Richardson, microservices.io — Saga pattern: https://microservices.io/patterns/data/saga.html
- Hector Garcia-Molina & Kenneth Salem, original "Sagas" paper (1987): https://www.cs.cornell.edu/andru/cs711/2002fa/reading/sagas.pdf
- Caitie McCaffrey, "Applying the Saga Pattern" (GOTO talk): https://www.youtube.com/watch?v=xDuwrtwYHu8
- temporal.io / Cadence (durable orchestration engines you'd reach for in production): https://temporal.io/
- Builds on: [34 — Event-Driven & Outbox](34-event-driven-outbox.md), [30 — Behavioral (state machine, command)](30-patterns-behavioral.md).
- Next: [36 — Resilience Patterns](36-resilience-patterns.md).
