# Coinledger — beginner+ · Progress

The walkthrough checklist. One phase at a time: 📖 explanation → you type into
**`projects/coinledger/`** (repo root, `module coinledger`) → ▶ checkpoint → ❓ questions → `next`.

Tags per file: 📋 **Copy** (paste, read once) · 🔁 **Repeat** (retype by hand) · 🧠 **Rebuild**
(read, close, write from memory, then diff against the reference).
Copy the reference `*_test.go` files into your copy: they are how each checkpoint verifies your code.

> ▶ **Resume here:** reference built & verified (2026-09-15): `go vet` clean, `go test -race ./...` green
> (concurrency tests ×15), and a live run of both binaries covering keys, mining, transfers, queued/RBF,
> error codes, offline signing, light-client verify, validate, restart, rebuild, lock and params guards,
> tampered and cross-chain txs. Docker files written but **not built** (no Docker on the build machine).
> **Walkthrough not started.** Recommended after finishing Project 1's walkthrough.

### 1 · Scaffold & what carries over from Project 1
- [ ] 📋 `go mod init coinledger` + `go get github.com/ethereum/go-ethereum@v1.17.5 go.etcd.io/bbolt@v1.5.0`
- [ ] 📋 `internal/domain/hash.go` — P1's `Hash` + `Hex`, `ParseHash`, `Keccak256`, `SHA256` (which hash for what)
- [ ] 📋 `internal/domain/pow.go` — P1's `Mine`, now `Seal(ctx, header, txs)`
- [ ] 📋 `internal/domain/errors.go`
- ▶ `go build ./...`
- ❓ Why two hash functions? Which data uses which?

### 2 · Keys & addresses
- [ ] 🧠 lab in `tmp/main.go`: generate a key, print the address, sign a hash, recover the address (no project code yet)
- [ ] 🔁 `internal/domain/address.go` — `Address` alias, `ParseAddress` with EIP-55
- ▶ `go test ./internal/domain -run 'KnownKeys|ParseAddress' -v` (after copying `helpers_test.go` + `tx_test.go` in Phase 3)
- ❓ Private key → public key → address: which steps are one-way? What does the mixed case in an address do?

### 3 · Transactions
- [ ] 🧠 `internal/domain/tx.go` — `SigningHash` and `Sender` from memory; 🔁 `SignTx`, `WithSignature`, `Encode`, `DecodeTx`, `Hash`, `Cost`
- [ ] 📋 `internal/domain/helpers_test.go`, `internal/domain/tx_test.go`
- ▶ `go test ./internal/domain -run 'Sender|Editing|WalletStyle|HighS|Unsigned|RawEncoding|Golden|Cost' -v`
- ❓ Why no `From` field? What happens when a signed tx's amount is edited? Signing hash vs tx hash? What is high-s malleability?

### 4 · The state transition function
- [ ] 🧠 `ApplyTx` in `internal/domain/state.go` — write the 6 steps as comments first, then the code
- [ ] 🔁 the rest of `state.go` — `Account`, `State` (`set` deletes empties), `Clone`, `Root`, `TotalSupply`, `Diff`, `credit`, `ApplyBlock`
- [ ] 📋 `internal/domain/state_test.go`
- ▶ `go test ./internal/domain -run 'ApplyTx|SelfTransfer|AllOrNothing|Reward|Empty|Root|Diff' -v`
- ❓ Which step stops double spends? Why `bits.Add64`? Why must `ApplyBlock` clone? Why sort accounts in `Root`?

### 5 · Merkle trees
- [ ] 🔁 `internal/domain/merkle.go` — `hashLeaf`, `hashNode`, `MerkleRoot`, `MerkleProof`, `nextLevel`
- [ ] 🧠 `VerifyProof` from memory
- [ ] 📋 `internal/domain/merkle_test.go`
- ▶ `go test ./internal/domain -run 'Proof|Order|Duplicated|InnerNode|EmptyRoot' -v`
- ❓ Why the 0x00/0x01 prefixes? Why promote the odd node instead of duplicating it? How long is a proof for 1M txs?

### 6 · Blocks, params & validation
- [ ] 🔁 `internal/domain/block.go` — 141-byte header, `DecodeHeader`, `EncodeBlock`/`DecodeBlock`, `ComputeTxRoot`
- [ ] 📋 `internal/domain/params.go` · 🔁 its `Fingerprint`
- [ ] 🔁 `internal/domain/validate.go` — the 9 rules
- [ ] 📋 `internal/domain/store.go` (the persistence port)
- [ ] 📋 `internal/domain/block_test.go`
- ▶ `go test ./internal/domain -v` (whole package green)
- ❓ What do `tx_root` and `state_root` commit to? How does a validator catch a miner paying itself extra?

### 7 · Mempool
- [ ] 🔁 `internal/mempool/pool.go` — `Add` (admission + RBF), `statusLocked`, `PendingNonce`, `Prune`, `Remove`, `Snapshot`
- [ ] 🧠 `Select` + the `byFee` heap
- [ ] 📋 `internal/mempool/pool_test.go`
- ▶ `go test -race ./internal/mempool -v`
- ❓ Pending vs queued? Why +10% for replacement? Why can't Alice's fee-100 nonce 1 go before her fee-1 nonce 0? Why is the pool "policy"?

### 8 · Persistence
- [ ] 🔁 `internal/repository/boltstore/store.go` — focus on `Open` (lock), `CommitBlock` (one tx), `LoadState`, `ReplaceState`
- [ ] 📋 `internal/repository/boltstore/store_test.go`
- ▶ `go test ./internal/repository/... -v`
- ❓ What happens if the process dies halfway through `CommitBlock`? Why are keys big-endian? Why copy data out of a bbolt transaction?

### 9 · The node
- [ ] 🧠 `Mine`'s four steps as comments first (choose → execute → seal → commit), then 🔁 the code
- [ ] 🔁 the rest of `internal/service/node.go` — `NewNode` (genesis, fingerprint, rebuild), `replay`, `SubmitTx`, queries, `Validate`
- [ ] 📋 `internal/service/node_test.go`
- ▶ `go test -race ./internal/service -v`
- ❓ Why hold `mu.RLock` across `pool.Add`? Why may readers keep using a `State` after unlocking? What does `REBUILD_STATE` prove?

### 10 · HTTP layer
- [ ] 📋 `internal/config/config.go`
- [ ] 📋 `internal/handler/response.go` · 🔁 its `errorMapping` table (400 vs 422 vs 409)
- [ ] 📋 `internal/handler/node_handler.go`
- [ ] 📋 `internal/router/router.go` + `router_test.go`
- ▶ `go test -race ./internal/router -v`
- ❓ Why are amounts JSON strings? Why does the proof endpoint return the *raw* header?

### 11 · Wallet
- [ ] 🔁 `internal/wallet/keys.go` — keystore, `SignTx`
- [ ] 🔁 `internal/wallet/lightclient.go` — `VerifyInclusion`
- [ ] 📋 `internal/wallet/client.go`, `cmd/wallet/main.go`, `internal/wallet/wallet_test.go`
- ▶ `go test -race ./internal/wallet -v`
- ❓ Where does the private key exist, and for how long? What can `verify` prove, and what can't it?

### 12 · main & infra
- [ ] 🔁 `cmd/node/main.go` — block producer goroutine, `BaseContext`, shutdown order (cancel → wait → shutdown → close store)
- [ ] 📋 `Dockerfile`, `docker-compose.yml`, `.dockerignore`, `.gitignore`
- ▶ the full "What you'll see" tour from the README against your own binaries

### 13 · Experiments (README "Experiments") — write down your results
- [ ] E1 RLP by hand: amount 200 encodes as … because …
- [ ] E2 tampered tx: error … because …
- [ ] E3 double spend: paid … ; why not both: …
- [ ] E4 cross-chain replay: error …
- [ ] E5 replay 1,000 blocks: … ms → estimate for 20M blocks: …
- [ ] E6 broken supply: the rule that fires is …
- [ ] E7 UTXO on paper: balances match? …

### 🧠 Rebuild scoreboard (also in the track's [PROGRESS.md](../PROGRESS.md))
- [ ] `Tx.SigningHash` + `Tx.Sender`
- [ ] `ApplyTx`
- [ ] `VerifyProof`
- [ ] `Pool.Select` (heap)
- [ ] `Node.Mine` (4 steps)

---
*Project description: [README.md](README.md) · Track plan: [../README.md](../README.md).*
