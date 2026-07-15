# 48 — Low-Latency Go III: Lock-Free, Zero-Copy & Tail Latency · Examples Progress

Tick each example as you retype and run it. 15 total.

## 🟢 Easy ([1-easy.md](1-easy.md))
- [ ] 1. Copy-on-write config with `atomic.Pointer`
- [ ] 2. A CAS retry loop (lock-free float add)
- [ ] 3. `unsafe.String`: zero-copy `[]byte`→`string`
- [ ] 4. Stream with `io.Copy`, don't buffer with `io.ReadAll`
- [ ] 5. `bufio` batches the syscalls

## 🟡 Medium ([2-medium.md](2-medium.md))
- [ ] 6. `net.Buffers`: one vectored write
- [ ] 7. Batch to amortise
- [ ] 8. Coalesce duplicate work (mini singleflight)
- [ ] 9. A zero-allocation ring buffer
- [ ] 10. Hedged requests cut the tail

## 🔴 Hard ([3-hard.md](3-hard.md))
- [ ] 11. A lock-free stack (Treiber)
- [ ] 12. Copy-on-write snapshots never tear
- [ ] 13. Zero allocations → zero GC on the hot path
- [ ] 14. Zero-alloc serialization with a pooled buffer
- [ ] 15. Capstone: a zero-allocation hot handler

---
**Status:** not started · **Lesson:** [48-low-latency-lockfree-tail.md](../../48-low-latency-lockfree-tail.md)
</content>
