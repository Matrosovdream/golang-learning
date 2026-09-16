# Coinledger — beginner+ · Blockchain Project 2

**Hashchain becomes a cryptocurrency.** Accounts are secp256k1 keys with real Ethereum-format
addresses. A wallet signs transfers locally, and the node never sees a private key. The node checks
signatures, nonces, and balances, keeps a mempool, mines blocks that commit to their transactions
(Merkle root) *and* to the resulting balances (state root), and stores everything in a crash-safe
bbolt file. A light-client command proves a transaction is in a block using one 141-byte header and
a few hashes.

| Concept | Where it lives |
|---|---|
| **keys, addresses, EIP-55** | [`domain/address.go`](internal/domain/address.go), go-ethereum `crypto` |
| **signed transactions, RLP, sender recovery** | [`domain/tx.go`](internal/domain/tx.go) — `SigningHash`, `Sender`, `Encode` |
| **the state transition function** | [`domain/state.go`](internal/domain/state.go) — `ApplyTx`, `ApplyBlock` |
| **Merkle trees & proofs** | [`domain/merkle.go`](internal/domain/merkle.go) |
| **block validation by re-execution** | [`domain/validate.go`](internal/domain/validate.go) |
| **mempool: pending/queued, replace-by-fee, fee-priority selection** | [`mempool/pool.go`](internal/mempool/pool.go) |
| **persistence, state as derived data** | [`repository/boltstore/store.go`](internal/repository/boltstore/store.go), `Node.replay` |
| **block production** | [`service/node.go`](internal/service/node.go) — `Mine` |
| **wallet: keystore, offline signing, light client** | [`wallet/`](internal/wallet/), [`cmd/wallet`](cmd/wallet/main.go) |

> Theory: Part 1 §1.4–1.7 of the [track plan](../README.md#part-1--blockchain-from-first-principles).
> Everything below, every output and every error message, is copied from real runs of these binaries.

---

## What you'll see

```bash
export WALLET_PASSWORD=demo              # or type it when asked
go run ./cmd/wallet new                   # alice
go run ./cmd/wallet new                   # bob
MINER_ADDRESS=<alice> go run ./cmd/node   # alice earns block rewards
```

**1. Keys.** Each `new` writes an encrypted key file and prints the address:
```
address: 0x4D373110998C27573A44F4A31C0Bf57b927E93a0
key file: …/keystore/UTC--2026-09-15T07-20-55.180581000Z--4d373110998c27573a44f4a31c0bf57b927e93a0
```

**2. Coins come from mining.** Two `curl -X POST localhost:8080/mine` later:
```
$ wallet balance -address 0x4D37…93a0
balance:       100
nonce:         0  (transactions mined)
pending nonce: 0  (next nonce to use)
at height:     2
```

**3. A transfer, signed locally:**
```
$ wallet send -from 0x4D37…93a0 -to 0x2B2F…f889 -amount 30 -fee 2
signed tx 0xac6644e435ed0929d0238842c9aace738697ebd7461932c4ed3130d9b4c39218 (nonce 0, 96 bytes)
accepted by node: 0xac66…9218 from 0x4D373110998C27573A44F4A31C0Bf57b927E93a0 (pending)
```
The node reports `from`, but the request never said who sent it. The node **recovered** the sender from the signature (§1).

**4. The mempool at work.** Alice signs nonce 2 while nonce 1 doesn't exist yet, then fills the gap and
tries to replace nonce 1 twice:
```
$ wallet send … -nonce 2        → accepted (queued)          ← gap at nonce 1
$ wallet send … -nonce 1 -fee 1 → accepted (pending)         ← gap filled; nonce 2 becomes pending too
$ wallet send … -nonce 1 -fee 1 → 422 replacement_underpriced: fee 1, need at least 2 to replace
$ wallet send … -nonce 1 -fee 5 → accepted (pending)         ← replace-by-fee: the fee-1 version is gone
$ wallet send … -chain-id 1     → 400 wrong_chain_id: tx is for chain 1, this is chain 1337
$ wallet send … -amount 500     → 422 insufficient_funds: balance 100, cost 501
```

**5. Mine, and every balance follows the rules:**
```
$ curl -X POST localhost:8080/mine   → block 3 with 3 txs
bob:   43   = 30 + 8 + 5
alice: 107  = 100 − (32 + 13 + 6) spent  + 8 fees back (she's the miner)  + 50 block reward
```

**6. A light-client check** of the first transfer, using only the header and two sibling hashes:
```
$ wallet verify -hash 0xac66…9218
header (141 bytes) for block 3
  ✔ block hash computed locally: 0x00003467589145b7c10b2c7fc5592dac0769204cfee38c5e19f818725468b2d9
  ✔ proof-of-work: 18 leading zero bits, header difficulty 18, minimum 18
merkle path (2 steps, index 0):
  1. sibling on the right 0x11c499b8164168bfc8f72e30f2917a69be0a087433195ebcb39c46dd0e528d3d
  2. sibling on the right 0x1a10028f12dc26a57244ed5ddd044b8e7458723b4711cc30c16913a60724a4bc
  ✔ recomputed root matches header tx_root 0x687f11cabaff7e0005506391467cc6c6e94b973730d1da7a948b5f59fd53f97c
verified: the transaction is committed to by a header with valid proof-of-work
```

**7. Replaying the whole chain** (`GET /chain/validate`) agrees with the live node, and no coin appeared from nowhere:
```json
{ "valid": true, "blocks": 5, "state_matches_live": true, "total_supply": "200", "expected_supply": "200" }
```

**8. Restarts:**
```
tip: block 4 0x0000388f350b…d1c5                                   ← normal restart: data is on disk
state rebuilt by replaying 5 blocks in 8ms (root 0xcb2dcba4…c2fd)   ← REBUILD_STATE=true
store: open …/chain.db: locked by another process — is another node using this data dir? (timeout)
node: data dir was created with different consensus params (chain id, difficulty, reward, min fee, max block txs, or genesis alloc changed); use a new DATA_DIR
```

---

## Routes

| Method | Path | Purpose | Success |
|---|---|---|---|
| `POST` | `/txs` | submit `{"raw": "0x…"}`, a signed RLP transaction | 202 `{hash, from, status: pending\|queued}` |
| `GET` | `/txs/{hash}` | status `pending` / `queued` / `included` (+ block, index, confirmations) | 200 |
| `GET` | `/accounts/{address}` | `balance`, `nonce`, `pending_nonce` (what a wallet should use next) | 200 |
| `GET` | `/mempool` | pooled txs split into `pending` and `queued` | 200 |
| `POST` | `/mine` | mine one block from the mempool now | 201 |
| `GET` | `/blocks?from=&limit=` · `/blocks/latest` · `/blocks/{height}` | headers + tx hashes | 200 |
| `GET` | `/blocks/{height}/txs/{hash}/proof` | Merkle proof + the raw 141-byte header | 200 |
| `GET` | `/chain/validate` | replay every block; compare with live state; check total supply | 200 |
| `GET` | `/stats` | height, accounts, supply vs expected supply, mempool sizes, params | 200 |
| `GET` | `/healthz` | liveness | 200 |

**Errors** carry a stable `code`, so a wallet can react to `nonce_too_low` without parsing English:

| Status | `code` | Meaning |
|---|---|---|
| 400 | `invalid_input`, `invalid_json` | malformed hex, RLP, address (incl. a bad EIP-55 checksum), height |
| 400 | `wrong_chain_id`, `invalid_signature` | can never be valid on this chain |
| 422 | `nonce_too_low`, `nonce_gap`, `fee_too_low`, `insufficient_funds`, `overflow`, `replacement_underpriced` | well-formed, but not valid *against the current state* (the same bytes may be fine later) |
| 409 | `already_known` | this exact tx is already pooled or mined |
| 404 | `not_found` | unknown tx, block, or proof |
| 503 / 504 | `mempool_full`, `cancelled` / `timeout` | try again later |

Coin amounts are **strings** in JSON (`"balance": "107"`). JSON parsers in JavaScript use float64, which
silently loses precision above 2⁵³, and a `uint64` can exceed that. This is the rule from Part 3 §3.3 of the plan.

---

## 1. Keys, addresses, and signatures

Ownership is **knowing a private key**. There are no usernames and no passwords on the chain:

```
private key (32 random bytes)
    │  elliptic-curve multiplication on secp256k1 — easy forward, infeasible backward
    ▼
public key (64 bytes)
    │  Keccak-256, keep the last 20 bytes
    ▼
address 0x4D373110998C27573A44F4A31C0Bf57b927E93a0
```

These are *exactly* Ethereum addresses. Private key `1` gives `0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf`
in this code and in every Ethereum tool (`TestKnownKeysGiveKnownAddresses`). The mixed capitalisation is
the **EIP-55 checksum**: `ParseAddress` rejects a mixed-case address whose case pattern is wrong, so a
typo fails instead of sending coins to nowhere.

### The sender is recovered, not declared

A `Tx` has **no `From` field**:

```go
type Tx struct {
	ChainID uint64
	Nonce   uint64
	To      Address
	Amount  uint64
	Fee     uint64
	Sig     [65]byte // r ‖ s ‖ v
}
```

ECDSA has a property called **public key recovery**: from `(message hash, r, s, v)` the math yields exactly
one public key, and so one address. `Tx.Sender()` does that. The consequence is the most important security
property of the whole project:

> A client **cannot claim** to be someone else, because there is no field to lie in. The only way to produce a
> signature that recovers to Alice's address is to hold Alice's key.

What happens if someone edits a signed transaction? It doesn't fail with "bad signature". It recovers a
**different, random** address. From a real run, changing one byte (amount 10 → 11) of Alice's signed raw tx:

```
error: node returned 422 insufficient_funds: insufficient funds for amount + fee: balance 0, cost 13
```

The balance is `0` because the "sender" is now a stranger nobody owns (`TestEditingASignedTxChangesTheSender`).

Two signature details handled in `tx.go`:
- **`v` is 0/1 here, but 27/28 from wallets** like MetaMask. `WithSignature` normalises it.
- **Malleability (EIP-2).** For every valid signature `(r, s)` there is a twin `(r, n − s)` that verifies too.
  If both were accepted, anyone could change a transaction's bytes, and therefore its hash, without the key.
  `Sender` rejects the "high-s" twin (`TestHighSTwinSignatureIsRejected`).

---

## 2. The transaction format

### What gets signed

```go
func (tx Tx) SigningHash() Hash   // Keccak-256( RLP([chainID, nonce, to, amount, fee]) )
func (tx Tx) Hash() Hash          // Keccak-256( RLP([chainID, nonce, to, amount, fee, sig]) )
```

- The **signing hash** covers every field except the signature (a signature can't sign itself).
- The **transaction hash** (its ID) covers the signed bytes.
- **`ChainID` is signed**, so a transaction for this chain can't be replayed on another. Real run: the same raw
  bytes sent to a node with `CHAIN_ID=1` → `400 wrong_chain_id: tx is for chain 1337, this is chain 1`.

### RLP, byte by byte

Transactions use **RLP** (Recursive Length Prefix), Ethereum's encoding. Here is a real 96-byte raw transaction
from `wallet send -offline`:

```
f85e 820539 80 94 2b2f89db537770a53b5d9a515149ff9fc2b9f889 0a 02 b841 b223…7401
```

| Bytes | Rule | Meaning |
|---|---|---|
| `f8 5e` | list whose length needs 1 byte (`0xf7 + 1`); length `0x5e` = 94 | the transaction (2 + 94 = 96 bytes) |
| `82 0539` | string of 2 bytes (`0x80 + 2`) | chain ID `0x0539` = **1337** |
| `80` | empty string | nonce **0** (zero is encoded as *nothing*) |
| `94 2b2f…f889` | string of 20 bytes (`0x80 + 20`) | `To` address |
| `0a` | a single byte below `0x80` is itself | amount **10** |
| `02` | same | fee **2** |
| `b8 41 b223…` | string longer than 55 bytes (`0xb7 + 1`), length `0x41` = 65 | the signature |

RLP decoding is **strict**: integers must have no leading zeros, and a single small byte must not be prefixed.
Every value has exactly one valid encoding, so every transaction has exactly one hash. Editing that amount byte
to `ff` gives `400 invalid_input: invalid raw transaction: rlp: element is larger than containing list`.

---

## 3. The state transition function

The account model (Ethereum-style): the **state** is a map `address → {balance, nonce}`. `ApplyTx` is the single
definition of what a valid transaction is and what it does, and **every node runs exactly this code**:

```go
func ApplyTx(p Params, s State, tx Tx, miner Address) (Address, error) {
	// 1. Meant for this chain?                         → ErrWrongChainID
	// 2. Who sent it? (recover from the signature)     → ErrInvalidSignature
	// 3. tx.Nonce == account nonce?                    → ErrNonceTooLow / ErrNonceTooHigh
	// 4. Fee ≥ minimum?                                → ErrFeeTooLow
	// 5. Balance ≥ amount + fee (overflow-checked)?    → ErrInsufficientFunds / ErrOverflow
	// 6. sender −= amount+fee, nonce++; recipient += amount; miner += fee
}
```

Details that matter:

- **The nonce rule is double-spend protection.** Alice's account nonce is a counter. A transaction must use
  *exactly* the next value. Replaying a mined transaction (same nonce) fails; so does a different transaction
  reusing that nonce. Two transactions paying Bob and Carol with nonce 0 can never *both* happen
  (`TestDoubleSpendWithTheSameNonce`).
- **Overflow.** `amount + fee` is computed with `bits.Add64`. Without that, `amount = 2⁶⁴−1, fee = 1` would wrap
  around to a cost of 0 and pass the balance check: a classic smart-contract bug class.
- **Read-modify-write, one account at a time.** Step 6 is correct even when sender, recipient, and miner are
  the same address (`TestSelfTransferOnlyCostsTheFee`).
- **Empty accounts are deleted** (Ethereum's EIP-161). "Received 0 coins" and "never existed" must produce the
  same state, or they would produce different state roots.
- **`ApplyBlock` is all-or-nothing.** It works on a `Clone()`. One invalid transaction rejects the whole block
  and leaves the parent state untouched. A block is never "partly" valid.
- **Money supply.** New coins appear only in `ApplyBlock`, as the block reward. Fees *move* coins. So the total
  supply must always equal `genesis allocation + height × BlockReward`, and `/chain/validate` checks exactly that.

### Sidebar: the UTXO model (Bitcoin)

The same history in Bitcoin's model: there are no balances, only **unspent outputs** ("coins"):

```
block 1 reward → output #1: 50 → alice
block 2 reward → output #2: 50 → alice
tx: alice pays bob 30, fee 2
    inputs:  #1 (50)                     ← whole coins are consumed
    outputs: #3: 30 → bob
             #4: 18 → alice              ← "change" back to yourself
             (50 − 30 − 18 = 2 is the fee, implicitly)
alice's balance = sum of unspent outputs she can sign for = #2 + #4 = 68
```

Double spending means spending output #1 twice, and it's prevented because an output can be spent exactly once.
No nonces are needed. Try the exercise in [progress.md](progress.md): recompute this chain's balances as UTXOs.

---

## 4. Blocks that commit to their contents *and* their result

The header grew from Project 1's 89 bytes to **141 bytes**:

```
height (8) · timestamp (8) · prev_hash (32) · tx_root (32) · state_root (32) · miner (20) · difficulty (1) · nonce (8)
```

| Field | Commits to | Checked how |
|---|---|---|
| `tx_root` | the ordered list of transactions (Merkle root) | recompute from the block's txs |
| `state_root` | **every account after this block** | re-execute all txs on the parent state, hash the result |
| `miner` | who gets reward + fees | it's inside the state root |

`ValidateBlock` never believes the producer. Its rules, in order: `height`, `stale_hash`, `prev_hash`, `timestamp`,
`difficulty`, `work`, `tx_root`, `txs` (re-execute), `state_root`. A miner who pays itself an extra million coins
produces a valid-looking header, but its state root doesn't match what honest validators compute
(`TestValidateBlock/miner_paid_itself_extra` → rule `state_root`).

The block producer (`Node.Mine`) runs **the same `ApplyBlock`** a validator will run, and puts the resulting root
in the header. That's the whole trick: the producer and every validator execute identical deterministic code.

A side effect you can measure: 141 bytes need **three** SHA-256 compression rounds (the input is padded to a
multiple of 64 bytes), where Project 1's 89-byte header needed two. The node's hash rate dropped from about
20 MH/s to about 12 MH/s (the real log: *1,425,390 attempts in 120 ms*). Real miners care about this. Bitcoin's
header is 80 bytes for a reason.

`State.Root()` sorts accounts by address before hashing. Go's map iteration order is deliberately random, so
without sorting two nodes with identical balances would compute different roots.

---

## 5. Merkle trees and light clients

`tx_root` is a Merkle root over the transaction hashes. The tree uses **domain separation**:

```
leaf = SHA-256(0x00 ‖ tx hash)
node = SHA-256(0x01 ‖ left ‖ right)
odd node at the end of a level → promoted unchanged
```

Two design decisions, each fixing a real attack:

| Decision | Without it |
|---|---|
| `0x00`/`0x01` prefixes | an inner node's 64 bytes `left ‖ right` are also valid "leaf data", so a subtree could be passed off as a transaction (a second-preimage attack; `TestInnerNodeCannotPoseAsALeaf`) |
| promote the odd node | Bitcoin duplicates it instead, so `[a b c]` and `[a b c c]` share a root (CVE-2012-2459; `TestDuplicatedLastItemChangesTheRoot`) |

A **proof** is the list of sibling hashes from a leaf to the root, about log₂(n) of them: 10 for 1,000
transactions (`TestProofLengthIsLogarithmic`). `wallet verify` (output above) uses the proof endpoint's **raw
header bytes**. It hashes them itself, checks the proof-of-work, and then checks the Merkle path against that
header's `tx_root`. It never trusts a `hash` or `root` field the node merely states.

**Know the limit.** This proves that *some* header with valid work commits to the transaction. It doesn't prove
the header is part of the chain everyone else follows. At difficulty 18, forging a header takes a laptop about
20 ms. A real light client (Bitcoin SPV) downloads *every* header and follows the chain with the most work, and
that only becomes meaningful with many independent miners. Project 3.

---

## 6. The mempool

Transactions wait in the node's **mempool** until mined. The pool is **policy, not consensus**: a block is valid
or invalid regardless of what any pool contains, and two nodes may hold different transactions.

| | Consensus (`domain.Params`) | Policy (`mempool.Config`, node options) |
|---|---|---|
| Who decides | the whole network, identically | each node for itself |
| Examples | chain ID, difficulty, reward, min fee, max block txs, genesis alloc | pool size, max nonce gap, the miner's address, block time |
| Getting it wrong | your node rejects everyone's blocks | your node is just less useful |

What the pool does:

- **Admission** (against the tip state): nonce not already used, not more than `MAX_NONCE_GAP` ahead, affordable
  on its own.
- **Pending vs queued.** A tx is *pending* if every lower nonce of its sender is on-chain or pooled; otherwise it
  is *queued* behind a gap. Real Ethereum nodes make the same split. `pending_nonce` in `/accounts` is the first
  unused nonce: what a wallet should sign next.
- **Replace-by-fee.** Same sender and nonce, at least **+10% fee** → the old transaction is replaced. This is how
  wallets "speed up" or "cancel" stuck transactions. The bump makes spam replacements cost something. Geth uses the
  same 10% (Project 7 builds a transaction manager on this).
- **Selection** (`Select`): a max-heap holds only each sender's *next* executable tx. Pop the highest fee, push that
  sender's following nonce. Alice's nonce-1 tx paying 100 still can't go before her nonce-0 tx paying 1
  (`TestSelectRespectsNonceOrderThenFee`).
- **Block building** trial-applies selected txs to a scratch state. Two txs that are each affordable but not together:
  the first is included, the second is dropped (`TestTxsThatDontFitTogether`). After a block, `Prune` removes
  everything whose nonce is now used.

---

## 7. Persistence: the chain on disk

One bbolt file, `DATA_DIR/chain.db`. bbolt is an embedded B+tree with ACID transactions.

| Bucket | Key → value |
|---|---|
| `blocks` | height (8, big-endian) → 141-byte header ‖ RLP(transactions) |
| `txindex` | tx hash → height ‖ index (what makes `GET /txs/{hash}` fast) |
| `state` | address (20) → balance (8) ‖ nonce (8) |
| `meta` | `tip` → height; `params` → consensus params fingerprint |

**One bbolt transaction per block.** `CommitBlock` writes the block, its tx index entries, the account changes, and
the new tip together. Kill the process at any moment and the file holds either the old tip with the old state or the
new tip with the new state, never a block without its balances.

**Only changes are written.** `domain.Diff(parent, next)` returns the accounts a block touched. That's how real nodes
avoid rewriting millions of accounts per block.

**State is derived data.** The `state` bucket is a cache of "replay every block from genesis". Two consequences,
both implemented:
- On every start, the stored accounts are hashed and must equal the tip's `state_root`. If not, the node refuses to
  start (`TestCorruptStateIsDetectedAndRebuilt`).
- `REBUILD_STATE=true` throws the cache away and replays: *"state rebuilt by replaying 5 blocks in 8ms"*.

**Guards:**
- bbolt holds an exclusive file lock, so a second node on the same directory fails with a clear message instead of
  corrupting the file.
- The **params fingerprint** stored at genesis catches a restart with different consensus params. The genesis hash
  alone only covers the allocation and difficulty, not the reward, fee, or chain ID. Geth stores its chain config for
  the same reason.

---

## 8. The wallet

- **Keystore files** are the "Web3 Secret Storage" JSON format used by geth and many Ethereum wallets. The private key
  is encrypted with a key derived from your password by **scrypt**, a deliberately slow, memory-hungry function that
  makes password guessing expensive. The demo uses `LightScryptN` (2¹²) for speed; real wallets use `StandardScryptN`
  (2¹⁸), 64× the work per guess.
- **The key never leaves the wallet process.** `SignHashWithPassphrase` decrypts it for one signature. The node only ever
  receives signed bytes.
- **The one thing a wallet must ask the chain is the nonce** (`pending_nonce`). Everything else it knows.
- **Offline signing:** `wallet send … -nonce 0 -offline` prints raw bytes without touching the network. `wallet submit
  -raw 0x…` broadcasts them later, from anywhere. Hardware wallets and air-gapped cold storage work this way.

---

## 9. Concurrency map (lessons 13–15)

| Primitive | Where | Why |
|---|---|---|
| `chan struct{}` capacity 1 | `Node.mineSlot` | one block producer at a time; waiting is cancellable (`select` with `ctx.Done()`) |
| `sync.RWMutex` + **immutable state snapshots** | `Node.mu`, `tip`, `state` | a block commit installs a *new* `State` map and never mutates the old one, so readers keep using what they read without holding the lock (copy-on-write) |
| lock ordering `Node.mu` → `Pool.mu` | `SubmitTx`, `Mine` | submit holds the read lock across `pool.Add` so a block can't commit between "read the account" and "admit the tx" |
| `sync.Mutex` | `mempool.Pool` | many HTTP goroutines submit at once |
| `time.Ticker` + `ctx` + `sync.WaitGroup.Go` | `RunBlockProducer`, `main` | `BLOCK_TIME_MS` mining; shutdown cancels, then waits until no block is half-committed |
| `http.Server.BaseContext` | `main` | shutdown cancels in-flight `/mine` requests too |

`TestConcurrentSubmitsWhileMining` hammers submit, reads, and mining together under `-race`.

---

## Architecture

```
handler  ->  service (Node)  ->  domain  <-  repository/boltstore
                 │                  ↑
                 └──> mempool ──────┘          wallet (client side) -> domain
```

`domain` is pure: no I/O, no clocks, no goroutines. Transactions, state transitions, Merkle trees, blocks, and
validation are functions of their inputs, so Project 3 can reuse all of it when blocks arrive from peers.

```
02-coinledger-beginner/
├── cmd/
│   ├── node/main.go                 # wiring, block producer, graceful shutdown
│   └── wallet/main.go               # new · list · balance · send · submit · tx · verify
├── internal/
│   ├── domain/                      # ★ the cryptocurrency — pure functions
│   │   ├── hash.go · address.go     # Hash, Keccak/SHA-256, EIP-55 parsing
│   │   ├── tx.go                    # Tx, SigningHash, Sender, RLP Encode/Decode
│   │   ├── state.go                 # Account, State, Root, Diff, ApplyTx, ApplyBlock
│   │   ├── merkle.go                # MerkleRoot, MerkleProof, VerifyProof
│   │   ├── block.go · pow.go        # 141-byte header, block encoding, mining
│   │   ├── params.go                # consensus Params, genesis, fingerprint, supply
│   │   ├── validate.go              # ValidateBlock — 9 rules
│   │   ├── store.go · errors.go     # persistence port, error values
│   │   └── *_test.go
│   ├── mempool/pool.go              # admission, pending/queued, RBF, heap selection, prune
│   ├── repository/boltstore/        # bbolt buckets, one tx per block
│   ├── service/node.go              # submit, mine, replay, queries
│   ├── wallet/                      # HTTP client, keystore signing, light-client verify
│   ├── handler/ · router/ · config/
├── Dockerfile · docker-compose.yml
├── go.mod                           # go-ethereum v1.17.5 (crypto, rlp, keystore), bbolt v1.5.0
├── progress.md
└── README.md
```

---

## Configuration

**Consensus**. Every node of one network must use identical values; the node refuses to reuse a data dir created with different ones.

| Env var | Default | Meaning |
|---|---|---|
| `CHAIN_ID` | `1337` | signed into every tx |
| `DIFFICULTY` | `18` | fixed proof-of-work (≈ 20 ms per block on an M4) |
| `BLOCK_REWARD` | `50` | new coins per block |
| `MIN_FEE` | `1` | smaller fees make a tx invalid |
| `MAX_BLOCK_TXS` | `100` | transactions per block |
| `GENESIS_ALLOC` | *(empty)* | `0xADDR=amount,0xADDR=amount`, balances before block 1 |

**Node policy and runtime**:

| Env var | Default | Meaning |
|---|---|---|
| `APP_PORT` | `8080` | HTTP port |
| `DATA_DIR` | `./data` | where `chain.db` lives |
| `MINER_ADDRESS` | *(zero address — rewards burned, with a warning)* | who gets this node's rewards and fees |
| `BLOCK_TIME_MS` | `0` | `0` = mine only on `POST /mine`; otherwise one block per interval |
| `MEMPOOL_SIZE` | `1000` | max pooled txs |
| `MAX_NONCE_GAP` | `16` | how far ahead of the account nonce a tx may queue |
| `REBUILD_STATE` | `false` | replay all blocks at startup and rewrite the stored state |
| `MINE_TIMEOUT_S` | `60` | limit for `POST /mine` |

Wallet: `-node` (or `$COINLEDGER_NODE`), `-keystore` (default `./keystore`), password from `$WALLET_PASSWORD` or stdin.

---

## Run it

```bash
# locally
export WALLET_PASSWORD=demo
go run ./cmd/wallet new                                        # note the address
MINER_ADDRESS=0x… BLOCK_TIME_MS=5000 go run ./cmd/node          # a block every 5 s
go run ./cmd/wallet send -from 0x… -to 0x… -amount 5 -wait     # after a few blocks give you coins

# or with Docker (the chain lives in the `chaindata` volume)
MINER_ADDRESS=0x… BLOCK_TIME_MS=5000 docker compose up --build

go test -race ./...
```

> Build, `go vet`, all tests under `-race` (concurrency tests repeated 15×), and every command above were verified.
> The Docker files follow Project 1's pattern but were **not** built here (no Docker on the build machine).

---

## Experiments

1. **RLP by hand.** `wallet send … -offline` and decode your raw hex with the table in §2. Change the amount to 200
   (`81 c8`). Why does 200 need a prefix but 10 doesn't?
2. **Tamper with a signed tx.** Edit the amount byte of a raw tx and `wallet submit` it. Explain the error you get.
3. **Double spend.** Sign nonce 0 → Bob (fee 5), then nonce 0 → Carol (fee 5, then fee 6). Mine. Who got paid, and why can't both?
4. **Replay across chains.** Run a second node with `CHAIN_ID=1 DATA_DIR=./data2 APP_PORT=8081 GENESIS_ALLOC=<you>=1000`
   and submit a raw tx signed for 1337.
5. **State is derived.** Produce 1,000 blocks (`BLOCK_TIME_MS=20` for a minute), stop, restart with `REBUILD_STATE=true`.
   How long does the replay take? What would it take for Ethereum's ~20 M blocks?
6. **Break the supply.** In a scratch copy, change `ApplyBlock` to pay `BlockReward+1`. Mine a block with the modified node,
   then start the original code on the same data dir with `REBUILD_STATE=true`. Which rule fires?
7. **UTXO on paper.** Take 3 blocks from `/blocks` and rewrite their transfers as UTXO inputs/outputs with change. Check the balances match.

---

## What's still missing

| Missing | Arrives in |
|---|---|
| more than one node — no gossip, no forks, no competing miners | Project 3 |
| fork choice and reorgs (what if two valid blocks 5 appear?) | Project 3 |
| a real light client that follows the heaviest header chain | Project 3 (discussed) |
| `uint256` amounts, decimals, gas-metered execution, smart contracts | Projects 4–6 on real Ethereum |
| HD wallets (one seed → many addresses) | Project 7 |

---

## Check questions

1. Why doesn't a transaction carry a `from` field? What happens when someone edits a signed transaction's amount?
2. What exactly does a nonce protect against? What does the chain ID protect against?
3. Why is the *signing hash* different from the *transaction hash*?
4. Walk through the 6 steps of `ApplyTx`. Why must `ApplyBlock` be all-or-nothing?
5. A miner claims a state root in the header. How do other nodes know it's honest?
6. Why do Merkle leaves and nodes get different prefixes? What goes wrong if the odd node is duplicated?
7. What can `wallet verify` prove, and what can't it?
8. Pending vs queued: what's the difference, and why does `pending_nonce` matter to a wallet?
9. Why is the mempool "policy" while `MinFee` is "consensus"?
10. The `state` bucket could be deleted and rebuilt. Why store it at all? And why is the params fingerprint needed on top of the genesis hash?
