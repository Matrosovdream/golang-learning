# 46 — Low-Latency Go I: Measuring & Allocation Basics · Examples Progress

Tick each example as you retype and run it. 17 total.

## 🟢 Easy ([1-easy.md](1-easy.md))
- [ ] 1. Percentiles and the tail
- [ ] 2. `testing.AllocsPerRun` — a deterministic count
- [ ] 3. Preallocate a slice
- [ ] 4. `strings.Builder` vs `+=`
- [ ] 5. `strconv.AppendInt` into a reused buffer → 0 allocs

## 🟡 Medium ([2-medium.md](2-medium.md))
- [ ] 6. Escape analysis with `-gcflags=-m`
- [ ] 7. Interface boxing allocates
- [ ] 8. `[]byte`→`string` conversion & the map-lookup elision
- [ ] 9. Presize a map with a size hint
- [ ] 10. A real benchmark with `go test -bench`

## 🔴 Hard ([3-hard.md](3-hard.md))
- [ ] 11. Reuse buffers with `sync.Pool`
- [ ] 12. A zero-allocation log line
- [ ] 13. Watch allocations drive the GC (`runtime.MemStats`)
- [ ] 14. `[]T` vs `[]*T`: the pointer tax
- [ ] 15. Capstone: a hot path driven to zero allocations
- [ ] 16. Inlining & devirtualization
- [ ] 17. Generics keep values unboxed

---
**Status:** not started · **Lesson:** [46-low-latency-measuring.md](../../46-low-latency-measuring.md)
</content>
