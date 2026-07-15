# 47 — Low-Latency Go II: GC, Layout & Contention · Examples Progress

Tick each example as you retype and run it. 15 total.

## 🟢 Easy ([1-easy.md](1-easy.md))
- [ ] 1. Struct padding: field order changes size
- [ ] 2. Find the padding holes with `Offsetof`
- [ ] 3. Field order → cache-line packing
- [ ] 4. Index-based references beat pointers
- [ ] 5. `GOGC`: pacing the collector

## 🟡 Medium ([2-medium.md](2-medium.md))
- [ ] 6. `GOMEMLIMIT`: a soft cap that triggers GC
- [ ] 7. `sync.Pool`: the reset footgun
- [ ] 8. Atomic vs mutex counter
- [ ] 9. Sharded (striped) counter
- [ ] 10. `RWMutex` for read-mostly data

## 🔴 Hard ([3-hard.md](3-hard.md))
- [ ] 11. Array-of-structs vs struct-of-arrays
- [ ] 12. Contention: mutex vs atomic vs sharded
- [ ] 13. False sharing: pad to a cache line
- [ ] 14. Reading a heap profile: `alloc_space` vs `inuse_space`
- [ ] 15. Capstone: a profile-guided fix

---
**Status:** not started · **Lesson:** [47-low-latency-gc-contention.md](../../47-low-latency-gc-contention.md)
</content>
