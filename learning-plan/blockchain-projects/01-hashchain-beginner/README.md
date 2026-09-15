# Hashchain — beginner · Blockchain Project 1

A **proof-of-work blockchain in standard-library Go**, running as one node with an HTTP API.
You can mine blocks, inspect them, **tamper** with history in four increasingly careful ways and
watch validation catch each one, and turn on **difficulty retargeting** to watch the chain tune itself
toward a target block time.

There are no transactions, keys, or network yet. This project is the data structure and the work,
and nothing else. Project 2 adds signed transactions; Project 3 adds many nodes that disagree.

| Concept | Where it lives |
|---|---|
| **canonical encoding** | [`domain/block.go`](internal/domain/block.go) — `Header.Encode` |
| **hash pointers** | `Header.PrevHash`, checked in [`domain/validate.go`](internal/domain/validate.go) |
| **proof-of-work** | [`domain/pow.go`](internal/domain/pow.go) — `Mine`, `LeadingZeroBits`, `Target` |
| **validation** | [`domain/validate.go`](internal/domain/validate.go) — 7 rules, first failure wins |
| **difficulty retargeting** | [`domain/difficulty.go`](internal/domain/difficulty.go) — `NextDifficulty` |
| **concurrency** (lessons 13–15) | [`service/chain_service.go`](internal/service/chain_service.go) — mining slot, auto-miner goroutine, atomics, context |
| **tampering** | [`service/tamper.go`](internal/service/tamper.go) |

> Part 1 of the [track plan](../README.md#part-1--blockchain-from-first-principles) has the theory.
> This README explains how *this* code implements it. Every output below is from a real run on an
> Apple M4 (about 20 MH/s on one core).

---

## What you'll see

```bash
DIFFICULTY=22 go run ./cmd/node
# in another terminal:
for i in 1 2 3 4 5; do curl -s -X POST localhost:8080/blocks -d "{\"data\":\"payment #$i\"}"; done
curl -s "localhost:8080/blocks?limit=6"
```

Condensed (hashes shortened to 16 hex characters):

| height | hash | prev_hash | nonce | leading zero bits | mined in |
|---|---|---|---|---|---|
| 0 | `00000036a0af51b2` | `0000000000000000` | 518,153 | **26** | 35 ms |
| 1 | `000001b15114129d` | `00000036a0af51b2` | 2,046,568 | 23 | 122 ms |
| 2 | `0000032e1196d8ad` | `000001b15114129d` | 3,978,820 | 22 | 200 ms |
| 3 | `000000d3c81fa2bb` | `0000032e1196d8ad` | 1,653,242 | 24 | 82 ms |
| 4 | `00000317d1c34eca` | `000000d3c81fa2bb` | 10,528,961 | 22 | 520 ms |
| 5 | `00000074b4ebf756` | `00000317d1c34eca` | 1,440,561 | 25 | 71 ms |

Three things to notice before reading any code:

- **Each `prev_hash` is the previous row's `hash`.** That's the chain.
- **Every hash starts with zeros.** Difficulty 22 requires at least 22 leading zero *bits*: five
  zero hex digits (20 bits) plus two more bits. That's the proof-of-work.
- **Mining time is random.** Block 4 needed 10.5 million attempts (520 ms), block 5 only 1.4 million
  (71 ms), at the same difficulty. Genesis got 26 zero bits when 22 were enough. The expected
  attempts are 2²² ≈ 4.2 million; the actual number is luck.

---

## Routes

| Method | Path | Purpose | Success |
|---|---|---|---|
| `GET` | `/blocks?from=0&limit=20` | list blocks (limit 1–100) | 200 |
| `GET` | `/blocks/latest` | the tip | 200 |
| `GET` | `/blocks/{height}` | one block | 200 |
| `POST` | `/blocks` | mine a block with `{"data": "..."}` and append it; waits until mined | 201 |
| `GET` | `/chain/validate` | re-check the whole chain from genesis | 200 |
| `POST` | `/miner/start` | start the background auto-miner; optional `{"max_blocks": n}` (0 = until stopped) | 202 |
| `POST` | `/miner/stop` | stop it and wait for the goroutine to exit | 200 |
| `GET` | `/stats` | height, next difficulty and target, average block time, total hashes | 200 |
| `POST` | `/debug/tamper/{height}` | edit a block's data: `{"data": "...", "mode": "body\|header\|remine\|rewrite"}` (default `header`) | 200 |
| `GET` | `/healthz` | liveness | 200 |

Errors: `400` for bad input (unknown JSON field, data over `MAX_DATA_BYTES`, bad height or mode),
`404` for an unknown height, `409` when the miner is already running, `504` when mining exceeds
`MINE_TIMEOUT_S`.

A block looks like this:

```json
{
  "height": 3,
  "hash": "00000ef90d1fe951b0908df02bc12a91f4d453b7d1eb4eee98273080786870fe",
  "prev_hash": "00000d2769e06a35d1a0d81d1f9dfe543d79a4ec353ff9775959eed02815546e",
  "timestamp": "2026-09-14T12:35:22.644Z",
  "data": "carol pays dave 1",
  "data_hash": "eb1b6671721f0150fb26262ac9613af7e40366fa9ec0b4c1c0e700e05c79610e",
  "difficulty": 20,
  "nonce": 1644086,
  "leading_zero_bits": 20,
  "attempts": 1644087,
  "mined_in_ms": 676.76
}
```

`attempts` and `mined_in_ms` are **local bookkeeping**: how *this* node mined the block. They aren't
part of the header, aren't hashed, and another node would report different numbers.

---

## 1. How a block is built

A block is a **header** plus a **body**:

```go
type Header struct {
	Height     uint64 // position in the chain; genesis = 0
	Timestamp  int64  // Unix milliseconds, set when mining starts
	PrevHash   Hash   // hash pointer: the previous block's header hash
	DataHash   Hash   // SHA-256 of Data — commits the body into the header
	Difficulty uint8  // required number of leading zero bits in this header's hash
	Nonce      uint64 // the number the miner varies; encoded LAST
}

type Block struct {
	Header
	Data     []byte        // the body
	Hash     Hash          // cached Header.ComputeHash() — never trusted
	MinedIn  time.Duration // local metadata
	Attempts uint64        // local metadata
}
```

**The block hash is `SHA-256(header)`**, and only the header. The body gets in through `DataHash`.

### Canonical encoding

SHA-256 hashes *bytes*, so a struct has to become bytes first. `Header.Encode` writes the fields in
a fixed order with fixed widths, big-endian, with no names or separators. It always produces exactly
**89 bytes**:

```
offset  0        8        16                        48                        80  81       89
        ┌────────┬────────┬─────────────────────────┬─────────────────────────┬───┬────────┐
        │ height │  time  │        prev_hash        │        data_hash        │ d │ nonce  │
        └────────┴────────┴─────────────────────────┴─────────────────────────┴───┴────────┘
          u64 BE   i64 BE          32 bytes                  32 bytes          u8   u64 BE
```

Why not `json.Marshal(header)`? Because two nodes must produce **byte-for-byte identical** input or they
compute different hashes for the same block and disagree about the chain. JSON output depends on field
order, whitespace, number formatting, and escaping. None of those are a contract. Real chains specify
their encodings exactly: Bitcoin's 80-byte header, Ethereum's RLP (Project 2).

### Why `DataHash` instead of putting `Data` in the header

Variable-length fields concatenated without lengths are **ambiguous**. The data `"ab"` followed by a
field `"c"` produces the same bytes as `"a"` followed by `"bc"`, so different blocks would share a hash.
There are two standard fixes:

1. **Length-prefix** every variable field (`len(data) ‖ data`).
2. **Hash the variable part** into a fixed 32 bytes and put *that* in the header.

This project uses fix 2 because it's what real chains do: Bitcoin's header holds a Merkle root of the
transactions, not the transactions. It also keeps the header small and fixed-size, which makes mining
fast (next section), and lets a light client verify headers without downloading bodies (Project 2).

There's a golden test that pins the encoding (`TestComputeHashGolden`). Reorder two fields and every
hash on the chain changes, so the test fails loudly.

---

## 2. Proof-of-work

**Rule:** a header is valid when its hash has at least `difficulty` leading zero bits.

Two ways to say the same thing:

| "Leading zero bits" view | "Target" view (Bitcoin's wording) |
|---|---|
| the top `d` bits of the hash are 0 | the hash, read as a 256-bit number, is `< 2^(256−d)` |
| `LeadingZeroBits(hash) >= d` | `hash < Target(d)` |
| no allocations; stops at the first non-zero byte | needs `big.Int` |

They're equivalent. `TestLeadingZeroBitsAgreesWithTarget` checks every combination of 0–256 zero bits
against difficulties 0–255. `Mine` uses the cheap form; `/stats` shows the target for intuition:

```
"next_difficulty": 22,
"next_target": "0000040000000000000000000000000000000000000000000000000000000000",
"expected_attempts": "4194304"
```

### The mining loop

```go
func Mine(ctx context.Context, h Header) (Header, uint64, error) {
	buf := h.Encode()
	nonceAt := len(buf) - 8

	for nonce := uint64(0); ; nonce++ {
		if nonce%ctxCheckInterval == 0 {           // every 16,384 hashes
			if err := ctx.Err(); err != nil {
				return Header{}, nonce, err
			}
		}
		binary.BigEndian.PutUint64(buf[nonceAt:], nonce)
		if MeetsDifficulty(sha256.Sum256(buf), h.Difficulty) {
			h.Nonce = nonce
			return h, nonce + 1, nil
		}
		// ...
	}
}
```

Three design decisions worth understanding:

- **Encode once, patch the nonce.** The nonce is the *last* field, so each attempt overwrites 8 bytes
  instead of re-encoding the header. Real miners go further: SHA-256 processes 64-byte chunks, so the
  first chunk's state (the "midstate") is computed once and only the tail is re-hashed.
- **Why hashes are "random enough" to mine:** changing the nonce by 1 changes about half the output bits
  (the avalanche effect). No nonce is better than another, so the only strategy is trying them. Each
  attempt succeeds with probability `2^−d`, so the expected number of attempts is `2^d`, and **each extra
  bit doubles the work**.
- **Cancellation is checked every 16,384 hashes, not every hash.** `ctx.Err()` takes a mutex. Checking
  every attempt would cost a noticeable fraction of the hash rate. At about 20 MH/s, 16,384 hashes take under
  a millisecond, so cancellation still feels instant.

### Measured: `go run ./cmd/powbench -from 12 -to 26 -runs 5`

```
  bits  expected attempts  avg attempts   avg time  time vs previous   hashrate
    16              65536         38094    2.857ms              0.9×  13.3 MH/s
    17             131072        133829    7.858ms              2.8×  17.0 MH/s
    18             262144        141356    7.066ms              0.9×  20.0 MH/s
    19             524288        388697   19.267ms              2.7×  20.2 MH/s
    20            1048576       1815885   89.788ms              4.7×  20.2 MH/s
    21            2097152       1884060   93.356ms              1.0×  20.2 MH/s
    22            4194304       5585259  275.996ms              3.0×  20.2 MH/s
    23            8388608      11186683  554.034ms              2.0×  20.2 MH/s
    24           16777216      11430122  567.812ms              1.0×  20.1 MH/s
    25           33554432      73166821   3.65908s              6.4×  20.0 MH/s
    26           67108864     180418609  9.273937s              2.5×  19.5 MH/s
```

Read the "time vs previous" column. It isn't a clean 2.0× per row: 24 bits happened to take as long as 23,
and 25 took 6.4× longer. With 5 samples, luck dominates. The number of attempts until success follows a
**geometric distribution**: its standard deviation is about as large as its mean. Only averaged over many
blocks does the doubling show. (Run `-runs 50` if you have time.) This is why a single block time tells a
miner nothing, and why retargeting (§5) averages over a window.

The hash rate column settles at 20 MH/s. The lower numbers at small difficulties are startup overhead
dominating tiny runs.

---

## 3. Validation: the seven rules

`ValidateChain` re-checks everything from genesis, **trusting nothing that was stored**. That's
exactly what a node does with a chain it downloads from a stranger. For each block, in this order,
the first failure is reported:

| # | Rule (`rule` in the response) | Check | Catches |
|---|---|---|---|
| 1 | `height` | `Height` equals the block's position | skipped or duplicated heights |
| 2 | `data_hash` | `SHA-256(Data) == DataHash` | the body was edited |
| 3 | `stale_hash` | the cached `Hash` equals the recomputed header hash | the header was edited but the cache wasn't |
| 4 | `prev_hash` | genesis: all zeros. Others: `PrevHash ==` the **recomputed** hash of the parent | broken links, a tampered parent |
| 5 | `timestamp` | not earlier than the parent's | time running backwards |
| 6 | `difficulty` | equals `NextDifficulty` computed from history | a miner claiming an easier difficulty |
| 7 | `work` | the recomputed hash has ≥ `Difficulty` leading zero bits | the proof-of-work was never done |

Two subtle points:

- **Rule 4 compares against the parent's *recomputed* hash, not its cached `Hash`.** A validator that
  trusted the cached value could be fooled by someone who edited the parent *and* its cache.
- **Rule 6 matters as much as rule 7.** Without it, a miner could set `Difficulty: 1`, do almost no work, and
  satisfy rule 7. The difficulty is a *consensus rule* computed from history, not a field a miner picks.

Simplifications compared to Bitcoin: Bitcoin requires a timestamp greater than the **median of the last 11
blocks** (so one miner with a wrong clock can't stall the chain), rejects blocks more than 2 hours in the
future, and hard-codes the genesis hash.

---

## 4. Tampering with history

The tamper endpoint edits a block's data, then covers the tracks with increasing effort. A real run at
difficulty 22 with blocks 0–5, editing block 2 each time (every mode starts from the result of the previous one):

| `mode` | What the "attacker" does | Result of `/chain/validate` |
|---|---|---|
| `body` | changes `Data` only | ❌ block **2**, `data_hash`: *data does not match the header's data_hash* |
| `header` | also updates `DataHash` and the cached hash, but no new proof-of-work | ❌ block **2**, `work`: *hash has 0 leading zero bits, difficulty requires 22* |
| `remine` | re-mines block 2 (3.07 M attempts, 153 ms) | ❌ block **3**, `prev_hash`: *prev_hash 0000032e1196… does not match block 2's hash 0000022ed878…* |
| `rewrite` | re-mines block 2 **and every block after it** (4 blocks, 6.6 M attempts, 333 ms) | ✅ **valid** |

Reading the table top to bottom is the whole idea of a blockchain:

1. You can't change data without changing `DataHash`.
2. You can't change `DataHash` without changing the block's hash, which then no longer has the zeros.
3. If you redo the work for that block, its hash is *different*, so the next block's `prev_hash` breaks.
4. So you must redo the work for **every block from the edit to the tip**. Then the chain is valid again.

**Step 4 is the uncomfortable one.** On a single node the rewrite succeeds, and it took 333 ms, less than the
825 ms the blocks originally took (luck again). Hashing alone doesn't make history immutable. What does:
- **Cost grows with depth:** rewriting block `h` costs the work of blocks `h…tip`. Deep blocks are expensive.
- **Everyone else keeps mining:** in a network (Project 3), honest nodes extend the *real* chain while the
  attacker re-mines. To win, the attacker must out-compute everyone else combined: the **51% attack**.

A cancelled rewrite (e.g. `curl -m 1` on a slow one) changes **nothing**. Re-mined blocks are collected in a
slice and written only once all of them are done (`TestCancelledRewriteLeavesChainUntouched`).

---

## 5. Difficulty retargeting

With fixed difficulty, block time depends on hardware: a faster machine mines faster. Real networks want a
**stable block time** (Bitcoin ~10 min), so they adjust difficulty to the hash rate they observe. Enable it with
`RETARGET_INTERVAL`:

```go
// every N blocks (from height 2N on), over the last N intervals:
actual   := prev.Timestamp - first.Timestamp   // how long they took
expected := N * TargetBlockTime                // how long they should have taken

actual*4 <= expected → +2 bits    (≥4× too fast)
actual*2 <= expected → +1 bit     (≥2× too fast)
actual >= expected*4 → −2 bits
actual >= expected*2 → −1 bit
otherwise            → unchanged  (the dead band)
```
then clamped to `[MIN_DIFFICULTY, MAX_DIFFICULTY]`.

Design notes:
- **Integers only.** Consensus code avoids floats: if two machines rounded one value differently, they would
  disagree about validity and the network would split.
- **The dead band** (no change between ½× and 2× the target) exists because block times are so noisy (§2). Without
  it, difficulty would jump up and down on random streaks.
- **At most ±2 bits (×4) per retarget**, like Bitcoin's ×4 clamp, so no single window can swing difficulty wildly.
- **The first retarget is at height 2N**, so the window never includes genesis, whose timestamp is a fixed date in the past.

### Measured: `DIFFICULTY=16 RETARGET_INTERVAL=10 TARGET_BLOCK_TIME_MS=1000`, auto-miner for 45 s

```
blocks   0-9   difficulty [16] avg mining 6 ms
blocks  10-19  difficulty [16] avg mining 3 ms
blocks  20-29  difficulty [18] avg mining 20 ms      ← way too fast: +2
blocks  30-39  difficulty [20] avg mining 52 ms      ← +2
blocks  40-49  difficulty [22] avg mining 182 ms     ← +2
blocks  50-59  difficulty [24] avg mining 982 ms     ← +2, now close to 1 s
blocks  60-69  difficulty [24] avg mining 352 ms     ← inside the dead band: unchanged
blocks  70-79  difficulty [25] avg mining 2634 ms    ← the previous window was a lucky 2.8× fast: +1
blocks  80-89  difficulty [24] avg mining 1381 ms    ← now 2.6× too slow: −1
```

It converges in about 50 blocks, then **oscillates between 24 and 25**. That's the interesting part. The "ideal"
difficulty is `log₂(20 MH/s × 1 s) ≈ 24.25` bits, which isn't a whole number. Difficulty 24 is a little too fast
and 25 is about 1.7× too slow, so random streaks push it back and forth. This is the price of power-of-two
granularity, and it's why **Bitcoin's target is an arbitrary 256-bit number** rather than a count of zero bits: it
can scale the target by exactly `actual / expected`. (Bitcoin retargets every 2,016 blocks, about two weeks, with
a ×4 clamp. Its window famously measures 2,015 intervals instead of 2,016, an off-by-one that is now permanent
because fixing it would change consensus.)

---

## 6. The concurrency in this project (lessons 13–15)

| Primitive | Where | Why |
|---|---|---|
| **`chan struct{}` with capacity 1** as a mutex | `ChainService.miningSlot` | only one miner may extend the tip. Unlike `sync.Mutex`, waiting can be **abandoned** when a `ctx` ends: `select { case slot <- struct{}{}: … case <-ctx.Done(): … }` |
| **goroutine + cancel func + `done` channel** | auto-miner (`StartAutoMiner` / `StopAutoMiner`) | `Stop` cancels **and waits for `done`**, so when it returns no half-mined block can still be appended |
| **lock ordering** | `StopAutoMiner` unlocks `autoMu` *before* `<-done` | the exiting goroutine's `clearAutoMiner` needs `autoMu`. Waiting while holding it would deadlock |
| **`context`** chain | request ctx → `WithTimeout(MINE_TIMEOUT_S)` → `Mine` | disconnect or timeout stops the nonce search; the log shows `POST /blocks: cancelled` |
| **`http.Server.BaseContext`** | `main.go` | on SIGTERM, cancelling the base context aborts in-flight mining requests immediately instead of waiting for them |
| **`sync.RWMutex`** | `memory.ChainStore` | many readers, one writer; `Append` checks "extends the tip" and appends under **one** lock |
| **`atomic.Uint64`** | `totalHashes`, `autoMined` | written by the miner goroutine and handler goroutines, read by `/stats` |

A real run of the shutdown path, with SIGTERM sent while the auto-miner was mining:
```
2026/09/14 19:38:12 shutting down...
2026/09/14 19:38:12 auto-miner stopped
2026/09/14 19:38:12 bye
```

All tests pass under `go test -race`, and the service tests were run 20 times in a row under `-race`.

---

## Architecture

The same clean-architecture rule as the other example projects, with dependencies pointing **inward**:

```
handler  ->  service  ->  domain  <-  repository
                          (core)
```

`domain` has no dependencies at all, only the standard library. Blocks, encoding, proof-of-work, and
validation are **pure functions**, which is why they're easy to test and why Project 2 and 3 can reuse them.

```
01-hashchain-beginner/
├── cmd/
│   ├── node/main.go                        # wiring, genesis, graceful shutdown (BaseContext)
│   └── powbench/main.go                    # difficulty vs time measurement tool
├── internal/
│   ├── domain/                             # ★ the blockchain — stdlib only, no I/O
│   │   ├── hash.go                         # Hash type
│   │   ├── block.go                        # Header, Block, canonical Encode, ComputeHash
│   │   ├── pow.go                          # LeadingZeroBits, Target, Mine, MineBlock
│   │   ├── difficulty.go                   # Params, NextDifficulty (retargeting)
│   │   ├── genesis.go                      # deterministic genesis block
│   │   ├── validate.go                     # the 7 rules, InvalidBlockError
│   │   ├── repository.go                   # ChainRepository port + errors
│   │   └── *_test.go                       # golden hash, PoW, retarget table, tamper table
│   ├── repository/memory/chain_repository.go  # RWMutex-guarded slice
│   ├── service/
│   │   ├── chain_service.go                # mining slot, auto-miner, validate, stats
│   │   ├── tamper.go                       # the four tamper modes
│   │   └── chain_service_test.go
│   ├── handler/                            # DTOs, validation, error → status mapping
│   ├── router/                             # routes + middleware (+ HTTP tests)
│   └── config/config.go                    # env → Params
├── Dockerfile · docker-compose.yml · .dockerignore
├── go.mod                                  # module hashchain — no dependencies
├── progress.md                             # the walkthrough checklist
└── README.md
```

---

## Configuration

| Env var | Default | Meaning |
|---|---|---|
| `APP_PORT` | `8080` | HTTP port |
| `DIFFICULTY` | `20` | initial difficulty in bits (fixed if retargeting is off). 20 ≈ 50 ms per block at 20 MH/s |
| `MIN_DIFFICULTY` / `MAX_DIFFICULTY` | `8` / `32` | retargeting bounds |
| `RETARGET_INTERVAL` | `0` | retarget every N blocks; `0` = fixed difficulty (`1` is bumped to `2`) |
| `TARGET_BLOCK_TIME_MS` | `2000` | the block time retargeting aims for |
| `MINE_TIMEOUT_S` | `60` | per-request mining limit → `504` |
| `MAX_DATA_BYTES` | `1024` | maximum block data size |

`DIFFICULTY` and the retarget settings are **consensus parameters**. On a network, every node must use the
same values or they reject each other's blocks. Here they only apply to the one chain in memory, which resets
on restart.

---

## Run it

```bash
docker compose up --build                   # the node on :8080
DIFFICULTY=24 docker compose up --build     # slower, more dramatic tampering

# or without Docker (no dependencies to download):
go run ./cmd/node
go run -race ./cmd/node                     # race detector on; mines about 3× slower

go test -race ./...                         # all tests
go run ./cmd/powbench -from 12 -to 24       # experiment 1
```

> The Go build, tests, and a live run were verified. The Docker files follow the other example projects but
> were **not** built here (no Docker on the build machine), so run `docker compose up --build` once and report
> anything odd.

---

## Experiments

Do these after the walkthrough and record your numbers in [progress.md](progress.md).

1. **Doubling.** `go run ./cmd/powbench -from 16 -to 24 -runs 20`. Does "time vs previous" approach 2×? How
   many runs do you need before it does?
2. **Tamper ladder.** Start at `DIFFICULTY=22`, mine 10 blocks, then tamper with block 3 using `body` → `header` →
   `remine` → `rewrite`, calling `/chain/validate` after each. Then tamper with block **9** in `rewrite` mode.
   Compare `took_ms`: why is it so different?
3. **Cancel.** `DIFFICULTY=28 go run ./cmd/node` (about 13 s per block, genesis included, so wait for `listening`). `curl -m 1 -X POST localhost:8080/blocks -d '{}'`.
   Watch the log print `cancelled` and CPU usage drop.
4. **Retarget.** `DIFFICULTY=16 RETARGET_INTERVAL=10 TARGET_BLOCK_TIME_MS=1000`, `POST /miner/start`, poll `/stats`
   for a minute. Where does it settle for *your* machine? Compute `log₂(hashrate × 1 s)` and compare.
5. **Shutdown under load.** With the auto-miner running, press Ctrl-C. How long until `bye`? Comment out
   `cancelBase()` in `main.go` and try again with `DIFFICULTY=30` and a pending `POST /blocks`.

---

## What this chain does *not* have (yet)

| Missing | Why it matters | Arrives in |
|---|---|---|
| transactions and signatures | anyone can write any data; no ownership | Project 2 |
| a Merkle tree | `DataHash` proves the whole body, not one item inside it | Project 2 |
| persistence | restart = new chain | Project 2 (bbolt) |
| other nodes | no forks, no disagreement, so the rewrite in §4 has no competition | Project 3 |
| fork choice ("most work wins") | needed as soon as two valid chains exist | Project 3 |
| median-time-past, future-time limit | a single miner's clock can distort retargeting | discussed only |

---

## Check questions

1. Why does editing block 3 invalidate blocks 3…tip, but not blocks 0–2?
2. What exactly does "difficulty 22" mean, and how many hashes does it take on average? Why can one block take 10× longer than another at the same difficulty?
3. Why is the header encoded by hand instead of with `json.Marshal`? What goes wrong with variable-length fields without a length prefix?
4. Why is `Nonce` the last field of the encoding?
5. Rule 6 (`difficulty`) looks redundant next to rule 7 (`work`). What attack does it stop?
6. In §4, `rewrite` produced a valid chain. So what actually makes old blocks hard to change?
7. Why does the difficulty oscillate between 24 and 25 instead of settling? What would fix it?
8. `StopAutoMiner` waits on `<-done` *after* unlocking `autoMu`. What would happen if it waited while holding the lock?
9. Two miners find block 10 at the same moment. What happens? (A preview of Project 3.)
