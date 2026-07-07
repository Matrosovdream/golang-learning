# Step 39 — Observability: Distributed Tracing · Examples

A library of **15 runnable examples**, split into three files by difficulty. Every example is a
complete `package main` program you **retype** and run with `go run .`. They reinforce
[39-observability-tracing.md](../../39-observability-tracing.md): spans, trace/span ids, context
propagation, the span tree, log↔trace correlation, and sampling.

## One-time setup

```bash
mkdir -p /tmp/trace-ex && cd /tmp/trace-ex
go mod init scratch
```

For each example, put the code in **`main.go`** (replacing the previous one) and run it:

```bash
go run .
```

Every example was compiled, `go vet`-ed, and run before being added; the **Output** is real stdout.
Standard-library only: these build a **mini in-memory tracer** standing in for OpenTelemetry — ids are
counter-based and timing uses an injectable clock, so everything is **deterministic**. The concepts
(spans, trace/span ids, `traceparent` propagation, the span tree, trace_id in logs, sampling) are
exactly what OTel does for real.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–5 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 6–10 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 11–15 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Ask and I'll append them.

## Index

### 🟢 [Easy](1-easy.md) — spans, ids, attributes
- [1. A span](1-easy.md#1-a-span)
- [2. Trace & span ids](1-easy.md#2-trace--span-ids)
- [3. Parent/child via context](1-easy.md#3-parentchild-via-context)
- [4. Span attributes](1-easy.md#4-span-attributes)
- [5. Span status & errors](1-easy.md#5-span-status--errors)

### 🟡 [Medium](2-medium.md) — trees, propagation, correlation
- [6. The span tree](2-medium.md#6-the-span-tree)
- [7. Context propagation in-process](2-medium.md#7-context-propagation-in-process)
- [8. Propagation across a hop (traceparent)](2-medium.md#8-propagation-across-a-hop-traceparent)
- [9. Correlate logs with trace_id](2-medium.md#9-correlate-logs-with-trace_id)
- [10. The three pillars](2-medium.md#10-the-three-pillars)

### 🔴 [Hard](3-hard.md) — sampling, timing, capstone
- [11. Head sampling](3-hard.md#11-head-sampling)
- [12. Timing a span](3-hard.md#12-timing-a-span)
- [13. Span links](3-hard.md#13-span-links)
- [14. Low-cardinality attributes](3-hard.md#14-low-cardinality-attributes)
- [15. Capstone: a mini distributed trace](3-hard.md#15-capstone-a-mini-distributed-trace)
