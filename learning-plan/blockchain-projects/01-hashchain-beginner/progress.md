# Hashchain — beginner · Progress

The walkthrough checklist. One phase at a time: 📖 explanation → you type into
**`projects/hashchain/`** (repo root, `module hashchain`) → ▶ checkpoint → ❓ questions → `next`.

Tags per file: 📋 **Copy** (paste, read once) · 🔁 **Repeat** (retype by hand) · 🧠 **Rebuild**
(read, close, write from memory, then diff against the reference).
You can copy the reference `*_test.go` files into your copy. They are how the checkpoints verify your code.

> ▶ **Resume here:** reference built & verified (2026-09-14): `go vet` clean, `go test -race ./...` green,
> live run with all tamper modes, retargeting, cancellation, and graceful shutdown.
> Docker files written but **not built** (no Docker on the build machine).
> **Walkthrough started 2026-09-14 — current: Phase 2 (Block & canonical encoding).** Phase 1 done 2026-09-15.

### 1 · Scaffold & the Hash type
- [x] 📋 `go.mod` (`module hashchain`, `go 1.26`, no dependencies)
- [x] 📋 `internal/domain/hash.go` — why `[32]byte` and not `[]byte`
- ▶ `go build ./...`
- ❓ Why can a `Hash` be compared with `==` and used as a map key, but a `[]byte` can't?

### 2 · Block & canonical encoding
- [ ] 🔁 `internal/domain/block.go` — `Header`, `Block`, `Encode`, `ComputeHash`, `HashData`
- [ ] 📋 `internal/domain/block_test.go`
- ▶ `go test ./internal/domain -run 'Encode|Golden|AnyField' -v`
- ❓ Why not `json.Marshal`? What is ambiguous about variable-length fields? Why is `Data` outside the header?

### 3 · Proof-of-work
- [ ] 🧠 `internal/domain/pow.go` — `LeadingZeroBits`, `Mine` (rebuild these two from memory); 🔁 `Target`, `MeetsDifficulty`, `MineBlock`
- [ ] 📋 `internal/domain/pow_test.go`
- [ ] 📋 `cmd/powbench/main.go`
- ▶ `go test ./internal/domain -run 'Mine|LeadingZero' -v` and `go run ./cmd/powbench -from 12 -to 22`
- ❓ Why are "top d bits zero" and "hash < 2^(256−d)" the same? Why is the nonce encoded last? Why check `ctx` every 16,384 hashes?

### 4 · Genesis & validation
- [ ] 📋 `internal/domain/genesis.go`
- [ ] 🧠 first write the **7 rule names in order** from memory as comments, then 🔁 `internal/domain/validate.go`
- [ ] 📋 `internal/domain/validate_test.go`
- ▶ `go test ./internal/domain -run Validat -v` (every tamper case reports the expected height and rule)
- ❓ Why compare `PrevHash` with the parent's *recomputed* hash? What attack does the `difficulty` rule stop?

### 5 · Difficulty retargeting
- [ ] 🔁 `internal/domain/difficulty.go` — `Params`, `NextDifficulty`
- [ ] 📋 `internal/domain/difficulty_test.go`
- ▶ `go test ./internal/domain -run NextDifficulty -v`
- ❓ Why integers only? Why the dead band? Why does the first retarget happen at height 2N?

### 6 · Storage
- [ ] 🔁 `internal/domain/repository.go` — the port + errors
- [ ] 🔁 `internal/repository/memory/chain_repository.go`
- ▶ `go build ./...`
- ❓ Why does `Append` check "extends the tip" under the same lock as the append? Why does `Range` copy?

### 7 · Service (the concurrency)
- [ ] 🔁 `internal/service/chain_service.go` — mining slot, `MineBlock`, auto-miner, `Validate`, `Stats`
- [ ] 🔁 `internal/service/tamper.go`
- [ ] 📋 `internal/service/chain_service_test.go`
- ▶ `go test -race ./internal/service -v`
- ❓ Why a `chan struct{}` instead of `sync.Mutex`? Why does `StopAutoMiner` unlock before `<-done`? Why is a cancelled rewrite all-or-nothing?

### 8 · HTTP layer & main
- [ ] 📋 `internal/config/config.go`
- [ ] 📋 `internal/handler/response.go`
- [ ] 📋 `internal/handler/chain_handler.go` DTOs and reads · 🔁 `Mine`, `Tamper`, `writeServiceError`
- [ ] 📋 `internal/router/router.go` + `router_test.go`
- [ ] 🔁 `cmd/node/main.go` — especially `BaseContext` + `cancelBase()` at shutdown
- ▶ `go vet ./... && go test -race ./...`, then `go run ./cmd/node` and try every route with `curl`
- ❓ Where does a client disconnect travel to? Why is `/blocks/latest` registered next to `/blocks/{height}` without conflict?

### 9 · Infra
- [ ] 📋 `Dockerfile`, `docker-compose.yml`, `.dockerignore`
- ▶ `docker compose up --build` → `curl localhost:8080/stats`

### 10 · Experiments (see README "Experiments") — write down your numbers
- [ ] E1 doubling: at how many `-runs` does "time vs previous" approach 2×? → …
- [ ] E2 tamper ladder at difficulty 22: rules reported → … ; rewrite of block 3 vs block 9 `took_ms` → …
- [ ] E3 cancel at difficulty 28: `cancelled` in the log? CPU drops? → …
- [ ] E4 retarget: settles at difficulty … ; `log₂(your hashrate × 1s)` = …
- [ ] E5 shutdown under load: with `cancelBase()` → … s ; without → … s

### 🧠 Rebuild scoreboard (also in the track's [PROGRESS.md](../PROGRESS.md))
- [ ] `Header.Encode` + `ComputeHash`
- [ ] `LeadingZeroBits` + `Mine`

---
*Project description: [README.md](README.md) · Track plan: [../README.md](../README.md).*
