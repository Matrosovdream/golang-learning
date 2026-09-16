# P2Pchain — intermediate · Blockchain Project 3

**Coinledger, multiplied.** Several nodes, each mining, each with its own mempool, connected
peer-to-peer with **libp2p**. Blocks and transactions spread by **gossip**. Nodes race, produce
competing blocks, and sometimes disagree — so a node no longer has *a chain*, it has a **tree of
blocks** and a rule for picking the branch to believe. Then you break the network on purpose:
partition it, spend the same coins on both sides, heal it, and watch one payment get undone.

| Concept | Where it lives |
|---|---|
| **peer identity, bootstrap, retries** | [`p2p/host.go`](internal/p2p/host.go) |
| **GossipSub topics, validate-before-relay, dedup** | [`p2p/gossip.go`](internal/p2p/gossip.go) |
| **sync: status, block locator, ancestor fetch** | [`p2p/sync.go`](internal/p2p/sync.go), [`service/node.go`](internal/service/node.go) |
| **the block tree & cumulative work** | [`domain/work.go`](internal/domain/work.go), [`repository/boltstore`](internal/repository/boltstore/store.go) |
| **fork choice & reorgs** | [`chain/forkchoice.go`](internal/chain/forkchoice.go) — `AddBlock`, `switchTo`, `paths` |
| **orphan blocks** | [`service/orphans.go`](internal/service/orphans.go) |
| **mining with cancellation** | [`miner/miner.go`](internal/miner/miner.go) |
| **partitions & 51% attacks** | `POST /debug/partition`, `POST /debug/miner` |

> Everything below is copied from real runs of four nodes on one machine.
> Theory: Part 1 §1.8–1.9 of the [track plan](../README.md#part-1--blockchain-from-first-principles).

---

## What you'll see

### Four nodes agree on their own

```
$ chainctl status -nodes http://localhost:18101,...,http://localhost:18104
node                          height            tip  peers pending   mined    thr flags
http://localhost:18101            34 0x0000001f666f      3       0       9      1
http://localhost:18102            34 0x0000001f666f      3       0      10      1
http://localhost:18103            34 0x0000001f666f      3       0      10      1
http://localhost:18104            34 0x0000001f666f      3       0       7      1
→ all nodes agree on the same tip
```

Nobody coordinates this. Each node mines on what it believes is the best chain and gossips what it
finds; the fork choice rule does the rest.

### Racing wastes blocks — that is where "orphan rate" comes from

After a few minutes of four nodes mining at ~1 block/second:

```
mined=102 canonical_height=87 orphaned=15        # 15% of all mined blocks lost their race
tips known: 5 · side branches: 4
```

Every one of those 15 blocks was valid. They lost because someone else's block of the same height
reached the network first. **This is the cost of a fast block time**: the closer the block interval
gets to the time it takes a block to cross the network, the more work is wasted. It is why Bitcoin
chose 10 minutes and why faster chains work hard on propagation.

### A partition splits the chain, and a double spend becomes possible

Split `{node1, node2}` from `{node3, node4}`, then spend the same coins on both sides:

```
$ chainctl double-spend -a http://localhost:18101 -b http://localhost:18103 -from 2 -to-a 0 -to-b 1 -amount 500000
nonce 2 spent twice:
  A http://localhost:18101 → dev 0 : 0x334e6ce00c23…
  B http://localhost:18103 → dev 1 : 0xdf8e04b2116f…
both are valid on their own side; only one can survive the merge

$ chainctl status …
http://localhost:18101            81 0x00000017deec   …        # side A, dev0 was paid
http://localhost:18103            85 0x0000002d8403   …        # side B, dev1 was paid
→ 2 different tips: the network has forked
```

Both sides are internally consistent. Both recipients see a confirmed payment. Neither side is
"wrong" — they simply haven't heard from each other.

### Healing: the heavier branch wins and the other payment is undone

```
$ curl -X DELETE localhost:1810N/debug/partition      # on every node
$ chainctl status …
http://localhost:18101           126 0x00000078ae45  …
http://localhost:18102           126 0x00000078ae45  …
http://localhost:18103           126 0x00000078ae45  …
http://localhost:18104           126 0x00000078ae45  …
→ all nodes agree on the same tip

dev0: 1,501,097 → 1,001,146     # side A's payment was UNDONE by the reorg
dev1: 1,501,258                 # side B's payment survived
GET /txs/0x334e6ce0…            # the losing transaction: {"error":"not found"}
```

And in the node logs, from a later run of the same experiment:

```
REORG to block 30 0x0000001672: 10 block(s) undone, 0 tx(s) back in the mempool
```

Zero transactions came back to the mempool because the loser's nonce had already been used by the
winner — it can never be mined again. A transaction that *isn't* in conflict does come back
(`TestReorgReturnsTransactionsToTheMempool`).

### A 51% attack, in 25 seconds

One node isolated, mining with 8 threads, against three honest nodes with one thread each:

```
$ curl -X POST localhost:18101/debug/miner -d '{"threads":8}'
…25 seconds later…
http://localhost:18101           285 0x00000040af04   …    # the attacker's private chain
http://localhost:18102           254 0x000000472a80   …    # the honest network
http://localhost:18103           254 0x000000472a80   …
http://localhost:18104           254 0x000000472a80   …
→ 2 different tips: the network has forked

…partition healed…
http://localhost:1810*           318 0x000000351491   …    # everyone adopted the attacker's chain
→ all nodes agree on the same tip
```

The honest nodes did nothing wrong, and no rule was broken. The attacker simply had more work, and
**the rule is "most work wins"**. That is what "51%" means, and why a payment needs confirmations:
undoing N blocks costs the attacker N blocks' worth of work that the rest of the network is
simultaneously outpacing.

### A new node catches up from nothing

```
$ DATA_DIR=./data5 MINER_THREADS=0 ./node        # empty database
synced 432 blocks from <peer.ID 12*X1ffq8>, now at height 458     # ~7 seconds
$ curl localhost:18105/chain/validate
{"valid":true,"blocks":460,"state_matches_live":true,"total_supply":"5022950","expected_supply":"5022950"}
```

It validated every block itself: signatures, proof-of-work, and the state root after executing each
block's transactions. It trusted its peers for *delivery*, never for *truth*.

### The live event stream

```
$ chainctl watch -node http://localhost:18101
12:44:51.754 tx_added         map[from:0xeA2A55A910… hash:0x3dea51c842… status:pending]
12:44:52.187 block_added      map[hash:0x0000001d7843… height:163 miner:0x699C67b2e8… txs:0]
12:44:53.042 block_added      map[hash:0x000000171d7b… height:164 miner:0x699C67b2e8… txs:3]
```

---

## Routes

Everything from coinledger (`/txs`, `/accounts/{address}`, `/mempool`, `/blocks…`, `/chain/validate`),
plus `GET /blocks/{ref}` now accepting a **hash** as well as a height (so you can inspect blocks that
are *not* on the canonical chain), and:

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/node/status` | peer id, height, tip, **total work**, peers, syncing, orphans, mempool, miner threads |
| `GET` | `/peers` | connected peers, direction, and whether they are blocked by a simulated partition |
| `GET` | `/forks` | every branch tip this node knows, with work, and which one is canonical |
| `GET` | `/events` | SSE stream: `block_added`, `reorg`, `side_block`, `tx_added`, `sync_*`, `peer_*` |
| `POST` | `/debug/partition` | `{"peers": ["12D3…"]}` — ignore those peers; `{}` blocks every connected peer |
| `DELETE` | `/debug/partition` | heal |
| `POST` | `/debug/miner` | `{"threads": 8}` — change this node's hashpower at runtime |

---

## 1. One node was simple. Many nodes need a tree.

In coinledger the chain was a list and "the state" was unambiguous. Now blocks arrive from
strangers, at any time, in any order, and two valid blocks can claim the same height. So a node
keeps:

```
                    ┌── B3' ── B4'          ← a side branch: stored, valid, not in use
   G ── B1 ── B2 ───┤
                    └── B3 ── B4 ── B5      ← the canonical branch: state, tx index, "the balance"
```

- **Every valid block is stored**, winner or not (`PutBlock`). A branch that looks lost today can
  win tomorrow, and re-downloading it would be wasteful.
- **Exactly one branch is canonical.** The account state, the transaction index, and every API
  answer describe *that* branch.
- **`state` is a function of a branch, not of the world.** "Alice has 5 coins" is always short for
  "on the branch I currently believe".

Each block carries its **cumulative work** (`BlockMeta.Work`): the parent's work plus `2^difficulty`.

---

## 2. Identity, addresses, and getting connected

A node's identity is a key pair; its **peer ID** is derived from the public key. Here the key comes
from a seed string, so the ID is predictable before the node exists:

```
$ go run ./cmd/peerid node1 node2
node1      12D3KooWNJMvu24iarjknQr7P8miTyi4ZdynE5jjZCnRhyX1ffq8
node2      12D3KooWHay9m7dumiMo8EvFYeqnDuwWGBSjn69NWprGTMSNkYEf
```

That is a demo convenience (it lets `docker-compose.yml` hard-code bootstrap addresses); a real node
generates a random key once and keeps the file.

A **multiaddr** describes *how* to reach a peer, layer by layer:

```
/ip4/127.0.0.1/tcp/14001/p2p/12D3KooWNJMvu24iarjknQr7P8miTyi4ZdynE5jjZCnRhyX1ffq8
└─ network ─┘└─ transport ─┘└────────────── who (verified by the handshake) ──────────────┘
```

The peer ID at the end matters: libp2p checks that the node answering really holds that key, so a
machine-in-the-middle can't impersonate a peer.

> **A bug this project hit for real.** Bootstrap was a single dial attempt at startup. Nodes 3 and 4
> came up while node 1 was still mining its genesis block, got `connection refused`, and sat alone
> forever with 0 peers. Peers restart, links flap, and startup order is never guaranteed — so
> `dialBootstrap` now **retries every 5 seconds** any bootstrap peer it isn't connected to.

---

## 3. Gossip: how a block reaches everyone

Blocks and transactions travel on two **GossipSub** topics, namespaced by chain id so two networks
can share a machine without mixing:

```
p2pchain/1337/blocks/1          p2pchain/1337/txs/1
```

Two mechanics do the heavy lifting:

**Deduplication by content.** A message's ID is the hash of its payload
(`messageID`), so the same block arriving from four peers is processed and forwarded **once**.

**Validation before relay.** `RegisterTopicValidator` runs *before* a message is forwarded. This is
the network's immune system: junk stops at the first honest node instead of being amplified.

| Result | When | Effect |
|---|---|---|
| `Accept` | the block passes the stateless checks | forwarded to our peers, handed to the node |
| `Reject` | provably invalid: bad proof-of-work, wrong difficulty, tx root mismatch | not forwarded, **and the sender's gossip score drops** |
| `Ignore` | nothing to blame the sender for (a partitioned peer, a duplicate) | not forwarded, no penalty |

**What is deliberately *not* checked at gossip time:** whether a transaction's nonce and balance fit
*our* state, or whether a block's transactions execute. Those depend on which branch we are on, and
our branch may legitimately differ from the sender's. Rejecting on state would punish honest peers.
Full execution happens later, when the block is actually connected.

---

## 4. Fork choice: most work, not longest

```go
if !domain.MoreWork(meta.Work, c.tipMeta.Work) {
    return StatusSide   // stored, but we stay where we are
}
```

- **Work, not height.** With fixed difficulty these are the same, but the code compares accumulated
  work because that is the rule that survives difficulty changes. A short chain of hard blocks beats
  a long chain of easy ones — otherwise an attacker would just lower their difficulty.
- **Strictly more.** On an exact tie we keep the tip we already have, so a node doesn't flip-flop
  between two equally good branches. Each node keeps whichever it saw first, and the tie is broken
  by whoever mines the next block.
- **`big.Int`, not `uint64`.** Bitcoin's accumulated work passed 2^93 long ago.

---

## 5. Reorgs: switching branches safely

When a heavier branch appears, the node must move — and moving means **undoing** blocks it already
applied:

```
current tip:  G ─ B1 ─ B2 ─ B3                 disconnect B3, B2   (tip first)
new branch:   G ─ B1 ─ B2' ─ B3' ─ B4'         connect   B2', B3', B4' (oldest first)
                   ↑ common ancestor
```

`switchTo` does it in four steps:

1. **Walk back to the common ancestor** (`paths`), applying each block's stored **undo diff** — the
   account values it replaced. Collect the transactions of the disconnected blocks.
2. **Execute the new branch** on top of the ancestor's state with the ordinary `ValidateBlock`. If
   any block fails here, the whole switch is abandoned — and **nothing has been written yet**
   (`TestFailedSwitchLeavesTheChainUntouched`).
3. **One atomic store transaction** (`Switch`): undo diffs, forward diffs, canonical height index,
   transaction index, and the new tip all move together. A crash mid-reorg is impossible to observe.
4. **The mempool follows the chain**: disconnected transactions are re-admitted (unless the new
   branch already contains them, in which case the nonce rule drops them), then stale ones pruned.

The undo diffs are what make this cheap: a block's diff is "the accounts it touched, as they were".
Real nodes do the same (geth calls it a state journal / reverse diffs).

**A consequence worth internalising:** `GET /txs/{hash}` answering `included` with 1 confirmation is
a *probability*, not a fact. The measured reorgs above undid 10 blocks.

---

## 6. Sync: catching up on what gossip didn't deliver

Gossip only carries *new* blocks. A node that was offline, partitioned, or just started needs a pull
mechanism, so there is a small request/response protocol on its own stream
(`/p2pchain/sync/1.0.0`) with three requests: `status`, `blocks`, `block`.

**Status exchange.** On connect (and every `PEER_SYNC_S` seconds), we ask each peer where its chain
is. If the peer reports more work, we pull. The periodic poll is the safety net: gossip messages can
be missed, but a node that keeps asking "what's your tip?" always recovers.

**The block locator.** We can't say "send me everything after block 42": on a fork we may not agree
what block 42 *is*. So we send a list of our canonical hashes, dense near the tip and exponentially
sparser going back, always ending at genesis:

```
tip, tip-1, tip-2, … tip-10, tip-12, tip-16, tip-24, tip-40, … genesis
```

The peer walks that list, finds the first hash that is canonical *for it*, and replies with the
blocks after it. Both "I'm 400 blocks behind" and "we forked 12 blocks ago" are answered by the same
request, in ~log₂(height) hashes. Bitcoin's design.

**Orphans.** A gossiped block whose parent we don't have is parked in a capped
[orphan pool](internal/service/orphans.go), and we walk backwards asking the sender for each missing
ancestor (`fetchAncestors`); when the chain connects, the parked children are connected too. The cap
matters: without it, blocks with parents that will never exist are a memory-exhaustion attack.

---

## 7. Mining, and the work that gets thrown away

```go
for i := range threads {
    // thread i tries nonces i, i+threads, i+2·threads, … — disjoint, no coordination
    mined, tried, err := domain.MineFrom(ctx, header, uint64(i), uint64(threads))
}
```

The important part isn't the parallelism, it's the **cancellation**: the moment a peer's block
arrives, everything being mined is worthless because it points at the wrong parent. The miner
subscribes to the event bus and cancels the round on `block_added` / `reorg`, then rebuilds a
candidate on the new tip. The work already spent is simply lost — that is the mining race.

> **A bug this project hit for real.** The round's watchdog goroutine closed its `done` channel in a
> `defer`, and the mining loop closed it too: `panic: close of closed channel` a few seconds after
> startup. One channel, one owner. `TestRunMinesBlocksContinuously` mines many rounds in a row and
> would have caught it; it was written after the panic.

---

## 8. Breaking the network on purpose

`POST /debug/partition` doesn't close connections — it makes a node **ignore** the blocked peers'
gossip and refuse their sync requests. That is deliberately what a real network failure looks like
from inside: the socket is fine, the packets don't arrive. (`ValidationIgnore`, not `Reject`: a
partitioned peer has done nothing wrong.)

With that one switch you can stage the three classic scenarios:

| Experiment | Setup | What it shows |
|---|---|---|
| **Split brain** | block `{1,2}` ↔ `{3,4}` | two valid chains, both "confirmed", diverging |
| **Double spend** | same nonce, one tx per side | confirmations on a partitioned network mean nothing |
| **51% attack** | isolate one node, give it more threads | more work rewrites history, without breaking any rule |

The defence against all three is the same and it is not cryptographic: **wait for confirmations**,
and know that "confirmed" means "expensive to undo", never "impossible".

---

## 9. Concurrency map (lessons 13–15)

| Primitive | Where | Why |
|---|---|---|
| `sync.RWMutex` + immutable state snapshots | `chain.Chain` | one writer (`AddBlock`), many readers; a committed `State` map is replaced, never mutated, so readers keep using what they read |
| single-writer serialization | `AddBlock` holds the write lock for the whole switch | a reorg must be atomic with respect to every reader |
| `context` cancellation | miner rounds, sync requests, HTTP handlers | a new tip cancels mining; shutdown cancels everything at once |
| buffered channels + drop-on-full | `events.Bus` | a slow SSE client must never block the chain |
| `atomic.Pointer` | `service.Node.net`, `.miner` | networking and mining attach after the node exists (they reference each other) |
| `atomic.Bool` (compare-and-swap) | `Node.syncing` | exactly one sync at a time |
| goroutine per subscription/stream | gossip readers, sync handlers, peer poll | libp2p hands each stream to its own handler |
| `sync.WaitGroup.Go` | `cmd/node` shutdown | cancel, then wait for the miner and the poller before closing the store |

All of it runs under `go test -race`, including the multi-node tests that start real libp2p hosts
in-process.

---

## Architecture

```
                    ┌──────────── p2p (libp2p) ────────────┐
   gossip in/out ──►│ host · gossip · sync protocol        │◄── peers
                    └───────────────┬──────────────────────┘
                                    │ Handler (OnBlock, OnTx, Status, BlocksAfter…)
                          ┌─────────▼─────────┐
   HTTP ──► handler ──►   │  service.Node     │ ──► mempool
                          │  orphans, syncing │
                          └─────────┬─────────┘
                                    │
                    ┌───────────────▼───────────────┐      ┌──────────┐
                    │ chain.Chain: tree, fork choice│◄────►│ boltstore│
                    │ reorg, state                  │      └──────────┘
                    └───────────────▲───────────────┘
                                    │ AddBlock
                              miner.Miner (threads, restart-on-new-tip)
```

```
03-p2pchain-intermediate/
├── cmd/
│   ├── node/main.go          # wiring: store → chain → node → p2p → miner → HTTP
│   ├── chainctl/main.go      # keys · status · send · spam · double-spend · watch
│   └── peerid/main.go        # seed → peer ID (used by docker-compose)
├── internal/
│   ├── domain/               # coinledger's rules, plus work.go (cumulative work) and devkeys.go
│   ├── chain/                # ★ block tree, fork choice, reorg, locator
│   ├── p2p/                  # ★ host, gossip (+validators), sync protocol, partition switch
│   ├── service/              # node orchestration: gossip handling, sync, orphans, queries
│   ├── miner/                # parallel mining, cancel on new tip
│   ├── mempool/ · repository/boltstore/ · events/ · handler/ · router/ · config/
├── Dockerfile · docker-compose.yml   # 4 miners in a mesh + 1 follower
└── README.md · progress.md
```

---

## Configuration

Consensus (identical on every node — the params fingerprint refuses a mismatched data dir):
`CHAIN_ID` (1337) · `DIFFICULTY` (25) · `BLOCK_REWARD` (50) · `MIN_FEE` (1) · `MAX_BLOCK_TXS` (100) ·
`DEV_ACCOUNTS` (5) · `DEV_BALANCE` (1000000).

Node-local: `APP_PORT` (8080) · `P2P_PORT` (4001) · `DATA_DIR` (./data) · `NODE_SEED` (identity) ·
`BOOTSTRAP` (comma-separated multiaddrs) · `MINER_THREADS` (1, 0 = don't mine) ·
`MINER_DEV_ACCOUNT` / `MINER_ADDRESS` (where rewards go) · `PEER_SYNC_S` (10) · `MEMPOOL_SIZE` ·
`MAX_NONCE_GAP` · `REBUILD_STATE`.

`DIFFICULTY=25` gives roughly one block per second per thread on an Apple M4 (~12 MH/s). Lower it if
your machine is slower; raise it if forks are too frequent to follow.

---

## Run it

```bash
# five nodes in containers (4 miners in a mesh + 1 follower)
docker compose up --build
docker compose exec node1 /app/chainctl status -nodes http://node1:8080,http://node2:8080,http://node3:8080,http://node4:8080,http://node5:8080

# or locally, without Docker
go run ./cmd/peerid node1 node2 node3 node4          # note the peer IDs
# node 1:
APP_PORT=18101 P2P_PORT=14001 NODE_SEED=node1 DATA_DIR=./data1 MINER_DEV_ACCOUNT=0 go run ./cmd/node
# node 2 (and so on; BOOTSTRAP lists the other nodes):
APP_PORT=18102 P2P_PORT=14002 NODE_SEED=node2 DATA_DIR=./data2 MINER_DEV_ACCOUNT=1 \
  BOOTSTRAP=/ip4/127.0.0.1/tcp/14001/p2p/<node1-peer-id> go run ./cmd/node

go run ./cmd/chainctl status -nodes http://localhost:18101,http://localhost:18102
go test -race ./...
```

> Verified: `go build`, `go vet`, `go test -race ./...` (3×), and every transcript in this README,
> from a four-node network on one machine. The Docker files follow the earlier projects' pattern and
> the compose file parses, but were **not** built here (no Docker on the build machine).

---

## Experiments

1. **Watch a fork happen.** Run `chainctl watch` on two nodes side by side and wait for a
   `side_block` on one and a `block_added` on the other for the same height.
2. **Measure the orphan rate.** Sum `blocks_mined` across nodes and compare with the canonical
   height. Halve `DIFFICULTY` (faster blocks) and measure again. What does that say about block time
   vs propagation time?
3. **Double spend.** Partition, `chainctl double-spend`, heal, and follow one payment's
   `GET /txs/{hash}` from `included` to gone.
4. **51%.** Isolate a node, raise its threads until it out-mines the rest, heal, and record the reorg
   depth from the logs. How many confirmations would have been safe?
5. **Kill and restart a node** mid-flight. How long until it is back at the tip, and which mechanism
   got it there — gossip or the periodic status poll?
6. **Break the rules.** Patch a node to mine `BlockReward + 1` and connect it. What do the others
   log, and what happens to its blocks? (Look for `state_root`.)
7. **Make the locator naive.** Replace `Locator()` with just the tip hash and re-run experiment 3.
   Where does sync get stuck?

---

## What's still missing

| Missing | Why it matters | Arrives in |
|---|---|---|
| finality | with proof-of-work, "confirmed" is always probabilistic | Ethereum's `finalized` tag — Project 4 |
| peer discovery beyond bootstrap (DHT, peer exchange) | a real network can't hard-code its peers | mentioned only; `go-libp2p-kad-dht` |
| headers-first sync, parallel block download | syncing 20M blocks one request at a time is too slow | real clients (see `btcd/netsync`) |
| transaction fees that adapt, mempool eviction | a real fee market | Project 4's gas oracle |
| smart contracts and a real chain | this is where the toy ends | Projects 4–7 |

---

## Check questions

1. Why does a node store blocks that lost the fork choice?
2. Why compare *work* rather than *length*, when both are the same here?
3. Why does a tie keep the current tip instead of switching?
4. Walk through a reorg: what is undone, in what order, and what happens to the transactions?
5. Why is the whole branch switch a single store transaction?
6. Which checks run at gossip time, and which only when a block is connected? Why the split?
7. What problem does the block locator solve that "send me everything after height N" doesn't?
8. A node is partitioned for an hour. Name two mechanisms that get it back to the tip.
9. Your exchange credits a deposit after 1 confirmation. Use the 51% experiment to explain the risk.
10. Why does the miner cancel its round when a peer's block arrives, and what does that wasted work cost the network?
