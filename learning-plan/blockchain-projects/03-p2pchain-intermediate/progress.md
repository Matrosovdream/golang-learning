# P2Pchain — intermediate · Progress

The walkthrough checklist. One phase at a time: 📖 explanation → you type into
**`projects/p2pchain/`** (repo root, `module p2pchain`) → ▶ checkpoint → ❓ questions → `next`.

Tags: 📋 **Copy** · 🔁 **Repeat** (retype by hand) · 🧠 **Rebuild** (read, close, write from memory,
then diff). Copy the reference `*_test.go` files: they are how each checkpoint verifies your code.

> ▶ **Resume here:** reference built & verified (2026-09-16): `go vet` clean, `go test -race ./...`
> green (3×, including multi-node tests with real libp2p hosts), and a live four-node network on one
> machine covering gossip, natural forks, partitions, a double spend, healing reorgs, a 51% attack,
> and a fresh node syncing 432 blocks. Docker files written and the compose file parses, but were
> **not** built here (no Docker on the build machine).
> **Walkthrough not started.** Do Projects 1 and 2 first — this one reuses both.

### 1 · Scaffold & what carries over
- [ ] 📋 `go mod init p2pchain`; copy coinledger's `internal/domain` (minus `store.go`) and `internal/mempool`
- [ ] 🔁 `internal/domain/work.go` — `BlockWork`, `AddWork`, `MoreWork` (and why `big.Int`)
- [ ] 🔁 `internal/domain/pow.go` — add `MineFrom(ctx, header, start, stride)`
- [ ] 📋 `internal/domain/devkeys.go`
- ▶ `go build ./...`
- ❓ Why is a chain's work the sum of `2^difficulty`, not the number of blocks?

### 2 · The store becomes a tree
- [ ] 🔁 `internal/domain/store.go` — the new port: `PutBlock`, `Meta`, `Tips`, `Switch`, `UndoDiff`
- [ ] 🔁 `internal/repository/boltstore/store.go` — buckets `blocks · meta · tips · canonical · txindex · undo · state · chain`
- [ ] 🧠 `Switch` (disconnect tip-first, connect oldest-first, one bbolt transaction)
- [ ] 📋 `store_test.go`
- ▶ `go test ./internal/repository/... -v`
- ❓ Why keep losing blocks? Why must a reorg be one transaction? What is an undo diff?

### 3 · The chain: genesis, reads, locator
- [ ] 🔁 `internal/chain/chain.go` — `New` (genesis / load / rebuild), `Replay`, `Tip`, `State`, `Tips`
- [ ] 🔁 `Locator` + `BlocksAfter`
- ▶ `go build ./...` (tests come with phase 5)
- ❓ Why does the locator get sparser with distance, and why must it end at genesis?

### 4 · Fork choice
- [ ] 🧠 `CheckStateless` — write the rule list from memory first
- [ ] 🧠 `AddBlock` — known → stateless → parent? → store → more work? → switch
- ▶ `go test ./internal/chain -run 'Extending|EqualWork|Orphan|Invalid' -v`
- ❓ Why is an equal-work block a side branch? Why is an orphan *not* stored? Which checks need no state, and why does that matter for gossip?

### 5 · Reorgs (the heart of this project)
- [ ] 🧠 `paths` — walk both branches back to their common ancestor
- [ ] 🧠 `switchTo` — undo, execute forward, one atomic `Switch`, mempool follow-up
- [ ] 📋 `chain_test.go`
- ▶ `go test -race ./internal/chain -v`
- ❓ In what order is state undone and re-applied? What happens if block 3 of the new branch is invalid? Which transactions come back to the mempool, and which don't?

### 6 · Events & mining
- [ ] 📋 `internal/events/bus.go` (note: drop for slow subscribers, never block)
- [ ] 🔁 `internal/miner/miner.go` — `mineRound` (select txs, execute, build header)
- [ ] 🧠 `search` + the watchdog: threads over disjoint nonce ranges, cancel on a new tip
- [ ] 📋 `miner_test.go`
- ▶ `go test -race ./internal/miner -v`
- ❓ Why cancel a round when someone else's block arrives? Who owns the `done` channel, and what happens if two goroutines close it?

### 7 · The libp2p host
- [ ] 🔁 `internal/p2p/host.go` — identity from a seed, listen addrs, `Peers`, the partition switch
- [ ] 🔁 `dialBootstrap` with retries (the bug from the README)
- [ ] 📋 `cmd/peerid`
- ▶ `go run ./cmd/peerid node1 node2` (same seed ⇒ same peer ID, every time)
- ❓ What is in a multiaddr? Why does the peer ID at the end matter? Why retry bootstrap forever?

### 8 · Gossip
- [ ] 🔁 `internal/p2p/gossip.go` — topics, `messageID`, subscription readers
- [ ] 🧠 the validators: when to Accept / Reject / Ignore
- ❓ Why is a duplicate `Ignore` but a bad proof-of-work `Reject`? Why isn't a bad nonce checked here?

### 9 · Sync
- [ ] 🔁 `internal/p2p/sync.go` — the stream protocol: `status`, `blocks`, `block`
- ▶ `go build ./...`
- ❓ Why a separate request/response protocol when gossip already spreads blocks?

### 10 · The node
- [ ] 🔁 `internal/service/node.go` — `OnBlock`, `OnTx`, `Status`, `OnPeerStatus`, `SubmitTx`
- [ ] 🧠 `SyncFrom` + `fetchAncestors` + `connectOrphansOf`
- [ ] 📋 `internal/service/orphans.go`, `queries.go`
- [ ] 📋 `network_test.go` (real libp2p nodes in-process)
- ▶ `go test -race ./internal/service -v`
- ❓ Trace a gossiped block whose parent is missing, from arrival to connection. What is the periodic status poll for?

### 11 · HTTP layer
- [ ] 📋 `internal/handler/response.go`, most of `node_handler.go`, `internal/router/router.go`
- [ ] 🔁 `Events` (SSE), `Partition`, `SetMiner`, `Forks`
- ▶ `go run ./cmd/node` then `curl -N localhost:8080/events`
- ❓ Why must the SSE handler never block on a slow client? Why does `/blocks/{ref}` accept a hash?

### 12 · Wiring & infra
- [ ] 🔁 `cmd/node/main.go` — attach order (node → network → miner), shutdown order
- [ ] 📋 `cmd/chainctl`, `Dockerfile`, `docker-compose.yml`
- ▶ four nodes locally (README "Run it"), then `chainctl status`
- ❓ Why do `service.Node.net` and `.miner` use `atomic.Pointer` instead of plain fields?

### 13 · Experiments (README "Experiments") — write down your results
- [ ] E1 watch a fork happen: heights … 
- [ ] E2 orphan rate at DIFFICULTY=25: … % ; at a lower difficulty: … %
- [ ] E3 double spend: which side won, and what happened to the loser's transaction: …
- [ ] E4 51%: threads needed … ; reorg depth … ; safe confirmations: …
- [ ] E5 kill & restart: back at the tip after … s, via gossip / status poll (circle one)
- [ ] E6 a node that breaks the reward rule: the others log …
- [ ] E7 naive locator: sync breaks when …

### 🧠 Rebuild scoreboard (also in the track's [PROGRESS.md](../PROGRESS.md))
- [ ] `AddBlock` (fork choice)
- [ ] `paths` + `switchTo` (reorg)
- [ ] gossip validator semantics
- [ ] `SyncFrom` / `fetchAncestors`
- [ ] `Miner.search` (threads + cancellation)

---
*Project description: [README.md](README.md) · Track plan: [../README.md](../README.md).*
