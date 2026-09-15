# Blockchain Projects Track — Go

A hands-on track of **seven blockchain projects**, easy → hard. The first three build a
blockchain **from first principles** so the concepts stop being magic; the last four build
the **Go backend services that companies actually run on real chains**: gateways, wallet
login, indexers, and a payment/withdrawal system.

> **Who builds what:** Claude builds each project as a verified reference. You learn it
> through a guided walkthrough: explanations first, then you retype the key parts
> (🔁 Repeat) and rebuild the core algorithms from memory (🧠 Rebuild). See [Part 0](#part-0--how-this-track-works).
>
> **Verified:** every Go snippet in this document was compiled and run against
> **Go 1.27.1** and **go-ethereum v1.17.5**. Library versions were checked on the Go module
> proxy on **2026-09-14**.

---

## Contents

- [Part 0 — How this track works](#part-0--how-this-track-works)
- [Part 1 — Blockchain from first principles](#part-1--blockchain-from-first-principles)
- [Part 2 — Building blockchain backends in Go](#part-2--building-blockchain-backends-in-go)
- [Part 3 — Designing APIs for blockchain services](#part-3--designing-apis-for-blockchain-services)
- [Part 4 — The seven projects](#part-4--the-seven-projects)
- [Part 5 — Libraries & technologies](#part-5--libraries--technologies)
- [Part 6 — Real codebases to read](#part-6--real-codebases-to-read)
- [Part 7 — Glossary](#part-7--glossary)

---

# Part 0 — How this track works

## Roles

| Who | Does what |
|---|---|
| **Claude** | Builds the reference project, verifies it (`go build`, `go vet`, `go test -race`, `docker compose up`), writes its `README.md` + `progress.md`, then runs the walkthrough. |
| **You** | Read, ask questions, retype the marked parts into your own copy, run the checkpoints, answer the check questions, and run the experiments. |

## The walkthrough loop

Every project is split into **phases** (usually 6–12). Each phase follows the same loop, the
same "one piece at a time, `next` moves on" rhythm used in the lesson examples:

```
 ┌──────────────────────────────────────────────────────────────────────┐
 │ 1. 📖 Explain   Claude explains the concept + the file(s) of the phase │
 │ 2. 🔁 Repeat    you retype the marked code into projects/<name>/     │
 │ 3. ▶  Run       a checkpoint command proves it works                   │
 │ 4. ❓ Check     2–3 questions — answer in your own words                │
 │ 5. next         the phase is ticked in progress.md, next phase starts   │
 └──────────────────────────────────────────────────────────────────────┘
```

## Three kinds of code

Not every line deserves to be typed by hand. Each file in a phase is tagged:

| Tag | Meaning | Typical files |
|---|---|---|
| 📋 **Copy** | Paste it, read it once. You've written this kind of code before. | config loaders, JSON response helpers, Dockerfile, router wiring, migrations boilerplate |
| 🔁 **Repeat** | Retype it by hand while reading. Typing is how the shape gets into your fingers. | repositories, handlers with non-trivial validation, adapters to the chain |
| 🧠 **Rebuild** | Read it, close it, write it **from memory**, then diff against the reference. | the core algorithms: block hashing, proof-of-work, the state transition function, Merkle proofs, fork choice, reorg rollback, nonce manager |

The 🧠 parts are few (1–3 per project) and they are the actual blockchain knowledge. If you
can rebuild them from memory, you understand the project.

## Folder layout

```
learning-plan/blockchain-projects/
├── README.md                     # this plan
├── PROGRESS.md                   # track-level progress (which project, which phase)
├── 01-hashchain-beginner/        # reference copies built by Claude (don't edit)
├── 02-coinledger-beginner/
├── ...
projects/                         # repo root: YOUR rebuilds
├── hashchain/
├── coinledger/
└── ...
```

Each reference project has the same files as the existing `example-projects/`:
`README.md` (what, why, routes, how to run), `progress.md` (phase checklist with
📋/🔁/🧠 tags and a `▶ Resume here` line), `docker-compose.yml`, `go.mod`.

## Prerequisites map

| Project | Needs these lessons (at least read) |
|---|---|
| 1 hashchain | 13–15 (goroutines, channels, context), 20–21 (HTTP/REST) |
| 2 coinledger | + 12 (errors), 18 (testing), 22 (persistence ideas) |
| 3 p2pchain | + 63 (networking), 58 (realtime), 65 (docker networking) |
| 4 eth-gateway | + 36 (resilience), 38 (caching), 68 (rate limiting), 41 (API design) |
| 5 web3-auth | + 56 (authentication & sessions), 57 (web security), 43 (authorization) |
| 6 token-indexer | + 22/59 (Postgres), 34 (outbox), 55 (data pipelines), 39 (observability) |
| 7 crypto-payments | + 27 (gRPC), 35 (sagas), 44 (background jobs) — basically everything |

You are currently on lesson 15, so **Projects 1–2 can start right away**. They're built to
use exactly the concurrency you're practising now: mining in a goroutine, cancelling with
`context`, a mempool behind a mutex.

---

# Part 1 — Blockchain from first principles

This part is the theory you need before and during the projects. Every concept is tagged
with the project where you implement it.

## 1.1 What a blockchain actually is

Remove the hype and a blockchain is three ordinary computer-science ideas combined:

1. **An append-only log** of transactions, grouped into **blocks**.
2. **A replicated state machine**: every node starts from the same initial state (the
   *genesis*), applies the same transactions in the same order, and therefore arrives at the
   same state (balances, contract storage). State is *derived* from the log.
3. **A consensus protocol** that lets nodes that **don't trust each other** agree on *which*
   log is the real one, even when some nodes lie.

Databases already do 1 and 2 (a Postgres WAL plus replicas). The new part is 3, agreement
**without a trusted operator**. Everything strange about blockchains (slow, expensive,
probabilistic finality, fees) comes from that one requirement.

> **Rule of thumb for backend engineers:** if all participants trust one operator, you want a
> database. Blockchains are for when they explicitly don't.

## 1.2 Hashes and hash pointers — *Project 1*

A cryptographic hash function (SHA-256 in Bitcoin, Keccak-256 in Ethereum) maps any bytes to
a fixed 32-byte digest with three properties that matter here:

- **Deterministic**: same input, same hash, on every machine, forever.
- **Avalanche**: flip one input bit and about half the output bits flip.
- **One-way and collision-resistant**: you can't find an input for a given hash, or two
  inputs with the same hash, in any practical amount of time.

A **hash pointer** is "the hash of the previous thing". Put the previous block's hash inside
each block and you get a chain where **changing any old block changes its hash, which breaks
the pointer in the next block, which changes *its* hash, and so on to the tip.** That's the
whole "immutability" story: not that edits are impossible, but that they are **instantly
detectable** and (with proof-of-work) **expensive to redo**.

**Canonical encoding matters.** You hash *bytes*, not structs. If two nodes serialize the
same block differently (field order, whitespace, JSON escaping), they compute different
hashes and disagree. Real chains use strict binary encodings (Bitcoin's wire format,
Ethereum's RLP, SSZ on the consensus layer). In Project 1 you write the encoding by hand:

```go
type Block struct {
	Height    uint64
	PrevHash  [32]byte
	Timestamp int64
	Data      []byte
	Nonce     uint64
}

func (b *Block) Hash() [32]byte {
	buf := make([]byte, 0, 8+32+8+len(b.Data)+8)
	buf = binary.BigEndian.AppendUint64(buf, b.Height)
	buf = append(buf, b.PrevHash[:]...)
	buf = binary.BigEndian.AppendUint64(buf, uint64(b.Timestamp))
	buf = append(buf, b.Data...)
	buf = binary.BigEndian.AppendUint64(buf, b.Nonce)
	return sha256.Sum256(buf)
}
```

(Variable-length `Data` makes plain concatenation ambiguous. Project 1 fixes that by putting only
`SHA-256(Data)` in a fixed-size header — see its README.)

## 1.3 Blocks and headers — *Projects 1–2*

A block has a small **header** and a larger **body**:

| Header field | Purpose |
|---|---|
| `parentHash` | the hash pointer to the previous block |
| `number` / height | position in the chain (genesis = 0) |
| `timestamp` | roughly when it was produced (loosely validated) |
| `txRoot` | Merkle root of the transactions in the body |
| `stateRoot` | commitment to the *entire* state after this block (Ethereum) |
| `nonce`, `difficulty`/`bits` | proof-of-work fields (Bitcoin; Ethereum pre-2022) |

The **block hash is the hash of the header only**. Because the header contains the Merkle
root of the transactions, the header still commits to every transaction. This is why *light
clients* can follow the chain by downloading only headers (80 bytes each in Bitcoin).

## 1.4 Merkle trees — *Project 2*

A Merkle tree hashes transactions in pairs, level by level, until one hash remains: the
**Merkle root**.

```
                 root = H(H12 ‖ H34)
                /                   \
        H12 = H(H1‖H2)         H34 = H(H3‖H4)
         /        \              /        \
     H1=H(tx1) H2=H(tx2)    H3=H(tx3)  H4=H(tx4)
```

What it buys you: a **Merkle proof**. To prove `tx3` is in the block, you don't send all
transactions. You send the siblings on the path (`H4`, `H12`), and the verifier recomputes
the root: `H(H12 ‖ H(H(tx3) ‖ H4))`. That's **log₂(n)** hashes: about 20 hashes for a million
transactions.

Details you'll meet in Project 2:
- **Odd counts:** Bitcoin duplicates the last hash on a level with an odd count. That rule
  famously enabled a bug (CVE-2012-2459), where two different transaction lists produce the
  same root. You'll see why when you implement it.
- **Order-dependent:** a proof must say whether each sibling is on the left or right.
- Ethereum uses a **Merkle Patricia Trie** (a Merkle tree crossed with a prefix trie) so that
  *state* (address → account) also has a root and supports proofs.

## 1.5 Keys, signatures, and addresses — *Projects 2, 5, 7*

Ownership on a blockchain = **knowing a private key**. There are no usernames.

- **Private key:** a random 256-bit number. Both Bitcoin and Ethereum use the **secp256k1**
  elliptic curve.
- **Public key:** a point on the curve, computed from the private key. Easy one way,
  infeasible the other way.
- **Signature (ECDSA):** proves "the holder of the private key approved *this exact message
  hash*", without revealing the key. It is 65 bytes in Ethereum: `r (32) ‖ s (32) ‖ v (1)`.
- **Public key recovery:** from `(hash, signature)` you can **recover the public key**. So
  Ethereum transactions **don't carry a `from` field**; the sender is *derived* from the
  signature. Your code must never trust a client-supplied `from`.

**Addresses** are short, human-copyable fingerprints of public keys:

| Chain | Address derivation | Looks like |
|---|---|---|
| Ethereum | last 20 bytes of `Keccak256(uncompressed pubkey without the 0x04 prefix)` | `0xd8dA6BF2…6045` |
| Bitcoin (legacy P2PKH) | `Base58Check(version ‖ RIPEMD160(SHA256(pubkey)))` | `1A1zP1…` |
| Bitcoin (SegWit / Taproot) | bech32 / bech32m encoding of a witness program | `bc1q…` / `bc1p…` |

**EIP-55 checksum:** Ethereum addresses are hex, and hex is case-insensitive. EIP-55 uses the
*capitalisation* as a checksum, so a typo is detectable. Always **store and compare addresses
as `common.Address` (20 bytes)**, never as strings. Display them checksummed:

```go
common.HexToAddress("0xd8da6bf26964af9d7eed9e03e53415d37aa96045").Hex()
// 0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045
```

**Two signature gotchas every Go dev hits** (you'll handle both in Project 5):
1. go-ethereum's `crypto.Sign` produces `v ∈ {0,1}`. Wallets (MetaMask etc.) return
   `v ∈ {27,28}`. Normalise before recovering.
2. **Malleability:** for every valid `(r, s)` there is a second valid `(r, n−s)`. Ethereum
   (EIP-2) only accepts the "low-s" form. Never use a signature's bytes as a unique ID.

```go
// RecoverPersonalSign returns the address that produced sig over msg (EIP-191).
func RecoverPersonalSign(msg string, sigHex string) (common.Address, error) {
	sig, err := hex.DecodeString(strings.TrimPrefix(sigHex, "0x"))
	if err != nil || len(sig) != 65 {
		return common.Address{}, fmt.Errorf("bad signature")
	}
	if sig[64] >= 27 { // wallets send V as 27/28; go-ethereum wants 0/1
		sig[64] -= 27
	}
	pub, err := crypto.SigToPub(accounts.TextHash([]byte(msg)), sig)
	if err != nil {
		return common.Address{}, err
	}
	return crypto.PubkeyToAddress(*pub), nil
}
```

## 1.6 Transactions: UTXO vs account model — *Project 2*

A transaction is a **signed request to change state**. Two ways to model "who owns what":

| | **UTXO model** (Bitcoin) | **Account model** (Ethereum) |
|---|---|---|
| State is | a set of *unspent transaction outputs* ("coins") | a map `address → {balance, nonce, code, storage}` |
| A payment | consumes whole coins as inputs, creates new outputs (one to the payee, one "change" back to you) | `balance[from] -= v; balance[to] += v` |
| Double-spend prevention | an output can be spent exactly once | per-account **nonce** must equal the next expected value |
| Balance | sum of your UTXOs (must scan or index) | a single lookup |
| Parallelism | independent coins → easy to validate in parallel | shared accounts → ordering matters |
| Privacy | new address per payment is natural | one address reused |
| Smart contracts | limited (Bitcoin Script) | first-class |

**The nonce** in the account model is the counter of transactions sent from an account:
`0, 1, 2, …`. A transaction is valid only if `tx.nonce == account.nonce`. That gives you:
- **replay protection:** the same signed tx can't be applied twice;
- **ordering:** tx 5 can't be mined before tx 4;
- **replacement:** sending a *different* tx with the *same* nonce and a higher fee replaces the
  pending one (this is how "speed up" and "cancel" work in wallets — Project 7).

**Chain ID** (EIP-155) is included in the signed payload so a transaction signed for a
testnet can't be replayed on mainnet.

## 1.7 The mempool — *Projects 2–3, 7*

Transactions don't go straight into blocks. A node first validates them (signature, nonce,
balance, fee) and holds them in the **mempool** (Ethereum: *txpool*), a node-local waiting
room. Block producers pick transactions from it, usually highest fee first.

Important facts for backend work:
- **Every node has its own mempool.** "Pending" is not a global truth. Your tx may be in one
  node's pool and unknown to another.
- **Pools evict** under pressure: low fee, too many txs per sender, or too old. A "sent"
  transaction can silently disappear.
- **Nonce gaps block everything behind them:** if nonce 7 is missing, nonces 8, 9, 10 wait
  (Ethereum keeps them as "queued", not "pending").

## 1.8 Consensus: who gets to write the next block — *Projects 1, 3, bonus 8*

| | **Proof of Work** | **Proof of Stake** (Ethereum) | **BFT** (CometBFT / Tendermint) |
|---|---|---|---|
| Right to propose | whoever finds a hash below the target first | a validator picked in proportion to staked ETH | round-robin weighted by stake among a known validator set |
| Cost of attack | out-compute the rest of the network (51% of hashpower) | acquire a large share of stake — and get **slashed** | control ≥ ⅓ of voting power |
| Finality | **probabilistic** — deeper = safer | **economic finality** after ~2 epochs (~13 min) | **instant** — once committed, final |
| Forks | common, short | rare, shallow before finality | none under the fault assumption |
| Examples | Bitcoin | Ethereum since Sep 2022 | Cosmos chains |

**Proof of work in one sentence:** find a `nonce` such that `hash(header) < target`. Because
hash outputs are effectively random, the only strategy is trying nonces. Each extra
*difficulty bit* halves the target and so **doubles** the expected work. Checking a solution
takes one hash. That asymmetry, *hard to produce and easy to verify*, is the point.

```go
// Mine searches for a nonce whose hash, read as a big number, is below target.
func Mine(b *Block, difficultyBits uint) {
	target := new(big.Int).Lsh(big.NewInt(1), 256-difficultyBits)
	for b.Nonce = 0; ; b.Nonce++ {
		h := b.Hash()
		if new(big.Int).SetBytes(h[:]).Cmp(target) < 0 {
			return
		}
	}
}
// 16 bits → about 65,536 tries on average; verified output: nonce 9097, hash 000058f3…
```

**Fork choice rule:** when a node sees two competing chains, it picks the one with the **most
cumulative work** (Bitcoin), or the one with the most attesting stake (Ethereum's LMD-GHOST).
Note that it's *not* "most blocks": a long chain of easy blocks must lose to a shorter chain of
hard ones. Project 3 implements this.

## 1.9 Forks, reorgs, confirmations, finality — *Projects 3, 6, 7*

This is **the most important concept for backend engineers**, and the most common source
of real money-losing bugs.

Two miners/validators produce a block at the same height at about the same time. For a while
the network has two tips:

```
            ┌── B3 ── B4'          (chain A)
 B1 ── B2 ──┤
            └── B3'' ── B4'' ── B5''   (chain B wins: more work)
```

Nodes that were following chain A switch to chain B: they **reorganise** (a **reorg**).
Blocks `B3`, `B4'` become **orphaned** (Ethereum calls them *uncled* or just reorged).
Transactions in them are **not lost**: they go back to the mempool and are usually re-included
in a later block of chain B, **possibly in a different block, at a different position, or not
at all** (if a conflicting transaction with the same nonce made it in).

Consequences for your code:
- A transaction "in block 1000" can be in block 1001 five seconds later, or gone.
- Anything you derived from block 1000 (a credited deposit, an indexed transfer, a sent
  webhook) may need to be **undone**.
- **Confirmations** = how many blocks sit on top of yours (`head − txBlock + 1`). The deeper,
  the less likely the reorg. The classic Bitcoin convention is 6.

**Ethereum block tags** (use these instead of guessing confirmation counts):

| Tag | Meaning | Reorg risk |
|---|---|---|
| `latest` | the head this node currently sees | real, usually 1–2 blocks |
| `safe` | justified by the attestation process | very low |
| `finalized` | finalised by ⅔ of stake (~2 epochs, ~13 min on mainnet) | reverting it requires burning ≥ ⅓ of all staked ETH |
| `pending` | the node's own view of the next block | not a real block — don't store it |

> **Design rule:** read `latest` to show "incoming", act on `safe`/`finalized` (or N
> confirmations) to move money. Always store the **block hash** alongside the block number,
> because the number alone doesn't identify a block after a reorg.

## 1.10 State, the EVM, and smart contracts — *Projects 4–7*

In Ethereum, an account is either:
- an **EOA** (externally owned account): controlled by a private key, has a balance and a
  nonce, no code;
- a **contract account**: has **code** (EVM bytecode) and **storage** (a `uint256 →
  uint256` key-value map), and is controlled by that code.

A **smart contract** is a program deployed on-chain. Sending a transaction to it runs its code
on every node inside the **EVM** (Ethereum Virtual Machine), deterministically. Contracts are
usually written in **Solidity** and compiled to bytecode plus an **ABI**.

Two ways to interact with a contract from Go:

| | `eth_call` | transaction (`eth_sendRawTransaction`) |
|---|---|---|
| Changes state? | no — simulated on a node, result discarded | yes — mined into a block |
| Costs gas? | no | yes |
| Needs a signature? | no | yes |
| Returns | the function's return value immediately | a tx hash; the *effect* shows up later in the receipt |
| Used for | reads: `balanceOf`, `decimals`, dry runs | writes: `transfer`, `approve`, `mint` |

## 1.11 ABI, events, and logs — *Projects 4, 6*

**ABI (Application Binary Interface)** is the calling convention. A call's `data` field is:

```
4-byte selector            = first 4 bytes of keccak256("balanceOf(address)") = 0x70a08231
+ 32-byte encoded argument  = the address, left-padded with zeros
```

Selectors verified with go-ethereum: `balanceOf(address)` → `70a08231`,
`transfer(address,uint256)` → `a9059cbb`.

**Events / logs** are how contracts tell the outside world something happened. A log lives in
the transaction **receipt** and has:
- `address`: the contract that emitted it;
- `topics[0]`: `keccak256` of the event signature, e.g.
  `Transfer(address,address,uint256)` →
  `0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef`;
- `topics[1..3]`: up to 3 **indexed** parameters (filterable);
- `data`: the non-indexed parameters, ABI-encoded.

Logs are **cheap for contracts to emit, can't be read by other contracts, and are the
backbone of every indexer, explorer, and wallet backend.** Each block header contains a 2048-bit
**bloom filter** of the addresses and topics in its logs, so nodes can skip blocks quickly when
serving `eth_getLogs`.

> **Trap you'll meet in Project 7:** a *plain ETH transfer* to an EOA **emits no log**. You
> find it by scanning transactions (`to`, `value`). ETH sent *by a contract* (an "internal
> transfer") doesn't even appear as a transaction; you need **traces** (`debug_traceBlock`),
> which many providers charge extra for. ERC-20 transfers always emit `Transfer` logs.

## 1.12 Token standards — *Projects 5–7*

A token is **not a coin**. It's a contract that keeps its own `mapping(address => uint256)`
ledger.

| Standard | What | Key functions / events |
|---|---|---|
| **ERC-20** | fungible tokens (USDC, DAI) | `balanceOf`, `transfer`, `approve`, `transferFrom`, `allowance`, `decimals`; event `Transfer(from, to, value)` |
| **ERC-721** | NFTs — unique token IDs | `ownerOf(id)`, `safeTransferFrom`; event `Transfer(from, to, tokenId)` with **tokenId indexed** (4 topics vs ERC-20's 3) |
| **ERC-1155** | multi-token (fungible and non-fungible in one contract) | `balanceOf(account, id)`; events `TransferSingle`, `TransferBatch` |

**Decimals:** ERC-20 amounts are integers in *base units*. `decimals()` says where the decimal
point goes: USDC uses 6, most tokens 18. `1.5 USDC` on-chain is `1500000`. Mint and burn show up
as `Transfer` from/to the zero address.

## 1.13 Gas and fees (EIP-1559) — *Projects 4, 7*

Every EVM operation costs **gas**, a unit of computation. A plain ETH transfer costs exactly
21,000 gas; contract calls cost more. The price per gas unit is denominated in **wei**
(1 ETH = 10¹⁸ wei; 1 **gwei** = 10⁹ wei).

Since EIP-1559 (London, 2021), a type-2 transaction specifies:

| Field | Meaning |
|---|---|
| `gasLimit` | the most gas it may use (estimate it with `eth_estimateGas`, add a margin) |
| `maxFeePerGas` | the absolute most you'll pay per gas |
| `maxPriorityFeePerGas` | the tip to the block proposer |

Each block has a protocol-set **`baseFee`** that moves up to ±12.5% per block depending on how
full the previous block was. The base fee is **burned**. You pay:

```
effectiveGasPrice = min(maxFeePerGas, baseFee + maxPriorityFeePerGas)
fee               = gasUsed × effectiveGasPrice
```

A common strategy: `maxFeePerGas = 2 × currentBaseFee + tip`. That survives about 6 consecutive
full blocks of base-fee increases, and you only pay what's actually needed.

**Transaction types** you'll see in receipts: `0` legacy, `1` access list (EIP-2930), `2` dynamic
fee (EIP-1559), `3` blob (EIP-4844, used by L2s), `4` set-code (EIP-7702, lets an EOA delegate to
contract code).

## 1.14 Nodes, RPC providers, and JSON-RPC — *Projects 4–7*

Your Go service doesn't "connect to the blockchain". It talks to **a node** over **JSON-RPC
2.0** (HTTP for requests, WebSocket for subscriptions). You either run the node yourself
(geth + a consensus client, Erigon, Reth…) or rent access from a provider (Alchemy, Infura,
QuickNode, and others).

The methods you'll use constantly:

| Method | Returns | Go (`ethclient`) |
|---|---|---|
| `eth_chainId` | chain ID | `ChainID` |
| `eth_blockNumber` | head number | `BlockNumber` |
| `eth_getBlockByNumber` | block (header + tx hashes or full txs) | `BlockByNumber`, `HeaderByNumber` |
| `eth_getTransactionByHash` | tx | `TransactionByHash` |
| `eth_getTransactionReceipt` | status, gasUsed, logs | `TransactionReceipt` |
| `eth_getBlockReceipts` | all receipts of a block in one call | `BlockReceipts` |
| `eth_getBalance` | wei at a block | `BalanceAt` |
| `eth_getTransactionCount` | nonce (at `latest` or `pending`) | `NonceAt`, `PendingNonceAt` |
| `eth_call` | contract read | `CallContract` |
| `eth_estimateGas` | gas estimate | `EstimateGas` |
| `eth_feeHistory` / `eth_maxPriorityFeePerGas` | fee data | `FeeHistory`, `SuggestGasTipCap` |
| `eth_sendRawTransaction` | tx hash | `SendTransaction` |
| `eth_getLogs` | logs matching a filter over a block range | `FilterLogs` |
| `eth_subscribe` (WS) | `newHeads`, `logs` streams | `SubscribeNewHead`, `SubscribeFilterLogs` |

What providers don't put on the front page:
- **Rate limits** and **compute-unit pricing**. `eth_getLogs` and traces are expensive.
- **`eth_getLogs` caps** on block range or result count. Your indexer must **chunk adaptively**.
- **Load-balanced nodes can disagree:** request 1 hits a node at block 100, request 2 hits one
  at block 99. "Block not found" right after "new head" is normal.
- **WebSockets drop.** Subscriptions don't replay missed events after a reconnect.

## 1.15 Wallets and HD derivation (BIP-39/32/44) — *Project 7*

- **BIP-39:** a **mnemonic** (12 or 24 words) encodes random entropy and is stretched into a
  512-bit **seed**.
- **BIP-32:** from the seed, derive a *tree* of keys (**HD** = hierarchical deterministic).
  An **extended private key (xprv)** derives child private keys. An **extended public key
  (xpub)** derives the **matching child public keys (and therefore addresses) without any
  private key**, for non-hardened children only.
- **BIP-44:** a path convention: `m / 44' / coin' / account' / change / index`. Ethereum's coin
  type is `60`, Bitcoin's `0`. `'` marks a *hardened* step.

Why this is gold for a payments backend: **the public-facing API holds only the xpub.** It can
generate a fresh deposit address for every invoice (`…/0/0`, `…/0/1`, …), and a full compromise
of the API server steals **zero** funds. Only the isolated signer holds the xprv. Verified:

```go
const h = hdkeychain.HardenedKeyStart
seed := bip39.NewSeed("test test test test test test test test test test test junk", "")
master, _ := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)

account := derive(master, 44+h, 60+h, 0+h, 0) // m/44'/60'/0'/0 — treasury side
xpub, _ := account.Neuter()                    // API side: public only

pub, _ := derive(xpub, 0).ECPubKey()
crypto.PubkeyToAddress(*pub.ToECDSA())         // 0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266
// …the same address the xprv derives, and anvil's default account #0.
```

## 1.16 Beyond Ethereum mainnet, in one paragraph each

- **Layer 2 rollups** (Arbitrum, Optimism/OP Stack, Base, zkSync, Scroll…) execute
  transactions off mainnet and post data/proofs to it. They are **EVM-compatible**: the same
  JSON-RPC, the same `ethclient`, the same ABIs, a different chain ID. Everything in Projects
  4–7 works on them; only the finality and fee details differ. Much of the OP Stack is written in Go.
- **Bitcoin** is the UTXO world. From Go you'd use `btcd`'s RPC client and `btcutil/psbt` for
  partially signed transactions (bonus project).
- **Cosmos / CometBFT** is *the* Go-native way to build your **own** chain: CometBFT does
  networking and BFT consensus, and your Go application implements the state machine through
  the ABCI interface (bonus project).
- **Solana** is an account-based high-throughput chain with a different programming model;
  `solana-go` is the Go client.

---

# Part 2 — Building blockchain backends in Go

Projects 4–7 are ordinary Go services with one unusual dependency. This part describes
what's different about that dependency and the patterns that handle it.

## 2.1 The mindset: the chain is a hostile, slow, eventually consistent dependency

Treat the blockchain like the worst third-party API you've ever integrated:

| Property | What it means for your code |
|---|---|
| **Slow** | Seconds to minutes per write. Every write endpoint is asynchronous (`202 Accepted`). |
| **Eventually consistent** | "Latest" can be rolled back (reorgs). Store block hashes, support rollback. |
| **Irreversible once final** | A wrong transfer can't be refunded by support. Validation, limits, and idempotency are not optional. |
| **Adversarial** | Anyone can call your contract, send you dust, craft weird tokens, replay signatures. |
| **Unreliable transport** | RPC nodes lag, disagree, rate-limit, drop WebSockets, and time out *after* accepting your transaction. |
| **Public** | Everything on-chain is visible to everyone, forever. Never put secrets or personal data there. |
| **Money** | Bugs have a price tag. Invariant checks and reconciliation are part of the design. |

## 2.2 Architecture: the same clean shape, plus a chain port

You already know the layout from `example-projects/`. The blockchain becomes **one more driven
adapter behind an interface** that the domain owns:

```
handler (HTTP/gRPC)  →  service (use cases)  →  domain (entities + ports)
                                                  ↑            ↑
                              repository/postgres ┘            └ chain/ethereum (ethclient adapter)
                                                                 chain/fake     (tests)
```

```go
// internal/domain/chain.go — the port. Domain types only; no go-ethereum types leak out.
type ChainReader interface {
	Head(ctx context.Context, finality Finality) (BlockRef, error)
	BlockByNumber(ctx context.Context, n uint64) (Block, error)
	Balance(ctx context.Context, addr Address, at BlockTag) (Amount, error)
	TransfersInRange(ctx context.Context, token Address, from, to uint64) ([]Transfer, error)
}

type ChainWriter interface {
	PendingNonce(ctx context.Context, addr Address) (uint64, error)
	SuggestFees(ctx context.Context) (Fees, error)
	Broadcast(ctx context.Context, rawTx []byte) (TxHash, error)
	Receipt(ctx context.Context, h TxHash) (*Receipt, error) // nil, nil = not mined (yet)
}
```

Why bother wrapping `ethclient`:
- **Tests** run against a fake or `ethclient/simulated` without Docker.
- **Error classification** happens once, in the adapter: `nonce too low` becomes
  `domain.ErrNonceTooLow`, and so on (see 3.8).
- **Multi-chain support** comes down to a map `chainID → adapter`.
- **Resilience** (timeouts, retries, fallback provider, rate limiting) wraps the adapter as a
  decorator (lesson 29), invisible to the service layer.

## 2.3 Money: `*big.Int` everywhere, never `float64`

On-chain values are `uint256`: up to 2²⁵⁶−1, far beyond `int64` (9.2×10¹⁸, which is about 9.2 ETH
in wei). Rules:

1. **In Go:** `*big.Int` in base units (wei, token units). For hot paths go-ethereum internally uses
   `holiman/uint256` (fixed-size, no allocations).
2. **In Postgres:** `NUMERIC(78,0)`. 78 digits hold any `uint256`. Scan and write as strings.
3. **In JSON:** **strings**. JavaScript numbers lose precision above 2⁵³.
4. **Formatting for humans** happens only at the edge, with the token's `decimals`:

```go
// FormatUnits turns base units into a decimal string: (1500000, 6) -> "1.5".
func FormatUnits(amount *big.Int, decimals int) string {
	s := amount.String()
	if len(s) <= decimals {
		s = strings.Repeat("0", decimals-len(s)+1) + s
	}
	whole, frac := s[:len(s)-decimals], strings.TrimRight(s[len(s)-decimals:], "0")
	if frac == "" {
		return whole
	}
	return whole + "." + frac
}
// FormatUnits(1500000, 6) = "1.5"; FormatUnits(5, 6) = "0.000005"; FormatUnits(10^18, 18) = "1"
```

5. `*big.Int` is a **pointer with mutable internals**. `a.Add(a, b)` mutates `a`. Never share one
   between goroutines or struct copies without `new(big.Int).Set(x)`. This is a lesson-10 pointer trap
   with real money attached.

## 2.4 Idempotency: every chain event has a natural key

Your consumers **will** see the same event twice: restarts, retries, reorg re-processing,
WebSocket reconnects. Design every write to be idempotent, keyed by what the chain already
makes unique:

| Thing | Natural unique key |
|---|---|
| block | `(chain_id, block_hash)` |
| transaction | `(chain_id, tx_hash)` |
| log / transfer | `(chain_id, block_hash, log_index)` — include the *block hash*, since after a reorg the same tx can land in another block with another log index |
| API-initiated withdrawal | the client's `Idempotency-Key` |

Then `INSERT … ON CONFLICT DO NOTHING`, and advance the checkpoint **in the same DB
transaction** as the data (lesson 34's outbox logic, applied to reads).

## 2.5 Confirmations and reorg handling — *Projects 3, 6, 7*

The standard indexer algorithm is **parent-hash chaining**. For each new block `N` you fetch:

```
if block[N].parentHash == stored[N-1].hash → normal: process N, store its hash
else                                         → reorg: walk back until parentHash matches
                                               (the common ancestor), then in ONE DB tx:
                                               delete/mark data from orphaned blocks,
                                               reverse derived state (balances),
                                               move checkpoint back to the ancestor;
                                               then re-process forward on the new branch.
```

Three strategies, from simplest to most complete:

| Strategy | How | Trade-off |
|---|---|---|
| **Lag behind** | only process blocks ≤ `finalized` (or head − N) | no reorg code at all; data is ~13 min late on Ethereum mainnet |
| **Process latest + rollback** | the algorithm above | real-time, but you must write and test the rollback path |
| **Two tiers** | index latest as `unconfirmed`, promote to `confirmed` at depth N / `finalized` | the UI gets real-time data, money moves only on final — what exchanges do (Project 7) |

`SubscribeFilterLogs` also delivers logs with **`Removed: true`** when a reorg reverts them. Use it
as a signal, but don't rely on it alone: a disconnect loses those notifications.

**Money-moving rule:** never *delete* ledger entries because of a reorg. Post **reversing
entries**, so the audit trail shows what happened (Project 7).

## 2.6 Nonce management — *Project 7*

If your service sends transactions from its own hot wallet, **you** own that account's nonce
sequence. Asking the node (`PendingNonceAt`) for every send breaks as soon as two goroutines
or two replicas send at the same time: both get nonce 42, one tx is rejected or replaces the
other.

The production pattern:

```
BEGIN;
SELECT next_nonce FROM wallets WHERE address = $1 FOR UPDATE;   -- serialise senders (lesson 59)
INSERT INTO chain_txs (wallet, nonce, status, …) VALUES ($1, next_nonce, 'created', …);
UPDATE wallets SET next_nonce = next_nonce + 1 WHERE address = $1;
COMMIT;
-- only then: sign → broadcast → track
```

- The **database is the source of truth** for nonces. At startup, reconcile it with the chain
  (`NonceAt(latest)` ≤ DB nonce, and look for gaps).
- **A gap is an outage:** if the tx for nonce 42 is never broadcast, 43+ are stuck forever. A
  background job re-broadcasts any `created`/`signed` tx that isn't on-chain.
- **"Nonce too low"** on broadcast isn't necessarily a failure: maybe *your own* tx was already
  mined (a previous attempt timed out after reaching the node). Check for a receipt by hash first.

## 2.7 Fees and stuck transactions — *Projects 4, 7*

A tx whose `maxFeePerGas` is below the current base fee just sits in the pool. The fix is a
**replacement**: same nonce, new signature, higher fees. geth's txpool requires **both**
`maxFeePerGas` and `maxPriorityFeePerGas` to increase by **≥ 10%** (`replacement transaction
underpriced` otherwise).

Tx-manager loop (per pending tx):
1. Broadcast. Record `(nonce, hash, fees, sent_at_block)`.
2. Every new block: is there a receipt for **any** hash we sent with this nonce? If yes, that one
   won. Mark the others `replaced` and track confirmations.
3. No receipt after `K` blocks: bump fees by ≥ 10% (capped by a configured maximum), re-sign,
   re-broadcast, append the new hash to the attempt list.
4. **Cancel** = replace with a 0-value self-transfer at the same nonce.

Optimism's `op-service/txmgr` (Go) is a production implementation of exactly this. Read it in Project 7.

## 2.8 Key management

| Level | Where the key lives | Use for |
|---|---|---|
| 0 | hard-coded / env var | local anvil only — anvil keys are public knowledge |
| 1 | encrypted **keystore file** (scrypt; `accounts/keystore`) + password from a secret store | dev, testnets |
| 2 | a **separate signer service**: tiny, no public ingress, policy checks (limits, allowlists), audit log | Project 7's design |
| 3 | **KMS / HSM** (AWS KMS and GCP KMS support secp256k1), or **MPC** custody providers | production custody |

Principles you'll implement: API servers get **xpubs, never xprvs**. The signer **re-validates**
what it signs (don't sign an arbitrary hash from the network). **Hot wallets hold a working float;
the bulk sits in cold storage.** Every signature is logged with who requested it and why.

## 2.9 RPC reliability patterns — *Project 4, reused after that*

Straight from lessons 36, 38, and 68:
- **Deadline on every call** (`context.WithTimeout`). A hung RPC otherwise hangs a worker forever.
- **Retry reads, not writes.** Reads are idempotent. For `eth_sendRawTransaction`, *re-sending the
  same signed bytes* is safe (same hash), but only because you track the hash. Never re-*build* a
  tx on retry.
- **Classify errors:** retryable (timeouts, 429, 5xx, "header not found" right after a new head)
  vs permanent (invalid params, `execution reverted`, `insufficient funds`).
- **Outbound rate limiting** with `x/time/rate` sized to your provider plan, plus `Wait(ctx)`.
- **Primary + fallback providers**, a circuit breaker per provider, and a **head-lag check**
  (don't read from a provider 20 blocks behind).
- **JSON-RPC batching** (`rpc.Client.BatchCallContext`) to fetch 50 balances in one round trip.
- **Cache by finality:** data at or below `finalized` is immutable, so cache it forever.
  `latest`-dependent data gets a TTL of about one block time. `singleflight` stops a stampede
  when a new block arrives.

## 2.10 Concurrency patterns you'll reuse (lessons 13–15)

| Pattern | Where |
|---|---|
| goroutine + `context` cancellation | mining loop (P1) cancelled when a peer's block arrives (P3) |
| `sync.RWMutex`-guarded state | chain + mempool (P1–P3) |
| fan-out/fan-in worker pool with bounded concurrency | parallel block/receipt fetching during backfill (P6) |
| pipeline: fetch → decode → persist, with ordering preserved | indexer (P6) |
| `select` over subscription channel, error channel, ticker, `ctx.Done()` | live tail with polling fallback (P6) |
| `errgroup` | run indexer + API + metrics server and shut down together |
| single-writer goroutine | per-wallet tx manager (P7) — ordering by construction |

## 2.11 Testing strategy

| Layer | Tool | What it catches |
|---|---|---|
| Pure logic (hashing, Merkle, fork choice, formatting, fee bump math) | table tests, fuzz tests (lesson 49) | off-by-one, encoding bugs |
| Chain adapter + contracts, in-process | **`ethclient/simulated`** (a real geth EVM in memory; `Commit()` mines a block) | ABI mistakes, revert handling, event decoding |
| Full integration | **anvil** in Docker via **testcontainers-go** | subscriptions, reorgs (`evm_snapshot`/`evm_revert`, `anvil_reorg`), mining control, base-fee control |
| Against real data | anvil `--fork-url <mainnet RPC>` | real token quirks (USDT doesn't return `bool`, fee-on-transfer tokens) |
| System invariants | a reconciliation job | ledger sum = 0, indexed balances = `balanceOf`, no nonce gaps |

## 2.12 Observability — the metrics that matter

- `chain_head_block`, `indexer_processed_block`, and their difference: **lag in blocks**, the #1 alert.
- `rpc_requests_total{method,provider,code}`, `rpc_latency_seconds`, rate-limit hits.
- `reorgs_total`, `reorg_depth` (histogram).
- `txmgr_pending_txs`, `txmgr_oldest_pending_age_seconds`, `txmgr_fee_bumps_total`, `wallet_balance_wei` (alert before the hot wallet runs dry of gas).
- `webhook_deliveries_total{result}`, outbox backlog.

## 2.13 Security checklist (applies from Project 4 on)

- [ ] Validate addresses (`common.IsHexAddress`) and hex input lengths **before** doing anything.
- [ ] Compare addresses as `common.Address`, not strings.
- [ ] Every signed message includes **domain, chain ID, nonce, expiry**. Nonces are single-use.
- [ ] Never trust a client-provided `from`, amount, or "tx succeeded". Verify on-chain (receipt `status == 1`, correct contract, correct log, enough confirmations).
- [ ] Check the **token contract address**, not the symbol: anyone can deploy a token called "USDC".
- [ ] Withdrawal limits, allowlists, and manual approval above a threshold.
- [ ] Keys never in logs, env dumps, error messages, or the database in plaintext.
- [ ] Webhooks are HMAC-signed with timestamps; incoming webhooks are verified.
- [ ] Rate-limit auth-nonce and write endpoints (lesson 68).

---

# Part 3 — Designing APIs for blockchain services

Your clients (frontends, merchants, internal services) should never have to understand reorgs,
nonces, or wei. **Hiding that complexity behind a clean contract is the main job of the API.**

## 3.1 Pick the right API style per audience

| Style | Good for | In this track |
|---|---|---|
| **REST + JSON** | public/merchant APIs, frontends, cacheable reads | P1–P7 external APIs |
| **JSON-RPC 2.0** | node-like interfaces, compatibility with Ethereum tooling | P3 (node API mimics `eth_*`), and what you *consume* in P4–P7 |
| **gRPC** | strict internal contracts, the signer boundary, streaming between services | P7 (api ↔ signer, api ↔ txmgr) |
| **SSE / WebSocket** | live feeds: new blocks, transfers, payment status | P6 SSE, P7 invoice status |
| **Webhooks** | notifying merchants' servers asynchronously | P7 |

## 3.2 Resource modelling and identifiers

- **Put the chain in the identifier.** The same address exists on Ethereum, Base, and Arbitrum,
  and it's a different account on each. Use `/v1/chains/{chainId}/…` or
  [CAIP-2](https://chainagnostic.org/CAIPs/caip-2) / CAIP-10 strings (`eip155:1`,
  `eip155:1:0xab16…`) when you need a single string.
- **Use chain IDs for chain objects** (`txHash`, `address`, `blockHash`). Use **your own IDs**
  (UUID/ULID) for your business objects (`invoice`, `withdrawal`). A withdrawal is *not* a tx hash:
  it may have several hashes (replacements) or none yet.
- **Separate "what the user asked for" from "what happened on-chain"**:
  `withdrawal { id, status, amount, to, attempts: [ {txHash, nonce, fees, status} ] }`.

## 3.3 Representing chain data in JSON

```jsonc
{
  "chainId": 11155111,
  "address": "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045", // EIP-55 checksummed on output
  "token": {
    "address": "0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238",
    "symbol": "USDC",
    "decimals": 6
  },
  "balance": {
    "raw": "1500000",        // base units, STRING — the source of truth
    "formatted": "1.5"      // convenience only, never parse this back
  },
  "block": {
    "number": 6123456,       // safe as a JSON number (< 2^53)
    "hash": "0x9f…",
    "timestamp": "2026-09-14T09:12:00Z"   // RFC 3339, UTC
  },
  "finality": "finalized"   // latest | safe | finalized — what this answer is based on
}
```

Rules: amounts as **decimal strings in base units**; block numbers as numbers; hashes and
addresses as 0x-hex strings; **accept** addresses in any case, **emit** checksummed; always say
**which block** an answer is based on.

## 3.4 Reads: make consistency explicit

Let clients choose, and default to something safe:

```
GET /v1/chains/1/addresses/0xd8dA…/balance?finality=finalized   (default)
GET /v1/chains/1/addresses/0xd8dA…/balance?finality=latest
GET /v1/chains/1/addresses/0xd8dA…/balance?block=21000000
```

Caching follows directly: `finalized` and block-pinned responses can be `Cache-Control:
immutable` with a strong `ETag` (lesson 41). `latest` gets `max-age` of about one block time.

## 3.5 Writes are asynchronous: accept, then track

Never block an HTTP request waiting for mining. The standard shape:

```
POST /v1/withdrawals                      → 202 Accepted
  Idempotency-Key: 7c1e…                    Location: /v1/withdrawals/wd_01J8…
  { "to": "0x…", "asset": "USDC",           { "id": "wd_01J8…", "status": "requested" }
    "amount": "250000000" }

GET /v1/withdrawals/wd_01J8…              → 200 { "status": "broadcast",
                                               "attempts": [{"txHash":"0x…","nonce":42}],
                                               "confirmations": 0 }
```

Publish the **state machine** in the docs, so clients know every status and transition:

```
requested ─► approved ─► signed ─► broadcast ─► mined ─► confirmed (final)
    │            │                     │   ▲        │
    └─► rejected └─► rejected          │   └─ replaced (fee bump, new attempt)
                                       └─► failed (reverted / dropped after max attempts)
                                     mined ─► reorged ─► broadcast   (back into the pool)
```

Offer three ways to learn about transitions: **polling** (always), **SSE** (for UIs), and
**webhooks** (for servers).

## 3.6 Idempotency keys for money-moving endpoints

A mobile client times out and retries `POST /withdrawals`. Without protection, the user is
paid twice, **irreversibly**. Implementation (lesson 41, made stricter):

1. Require `Idempotency-Key` on `POST /withdrawals` and `POST /invoices`.
2. Store `(merchant_id, key) → request_hash, response, status` with a unique constraint, **in the
   same DB transaction** as the withdrawal row.
3. Same key + same body → replay the stored response. Same key + different body → `422`.
4. A concurrent duplicate hits the unique constraint → `409` ("in progress").

## 3.7 Pagination over chain history

Offset pagination breaks when new blocks arrive and when reorgs remove rows. Use **cursor
pagination on the chain's natural order**:

```
GET /v1/tokens/0x…/transfers?limit=100&cursor=eyJiIjo2MTIzNDU2LCJsIjoxN30
cursor = base64({"b": blockNumber, "l": logIndex})   → WHERE (block_number, log_index) < ($b, $l)
                                                        ORDER BY block_number DESC, log_index DESC
```

Treat the cursor as opaque to clients, stable under inserts, and backed by an index on
`(token, block_number DESC, log_index DESC)`.

## 3.8 Errors: problem+json, with chain errors translated

Use RFC 9457 `application/problem+json` (lesson 41) with **stable machine-readable `type`s**.
The adapter layer maps node error strings to domain errors, and the handler maps those to HTTP:

| Node says (substring) | Domain error | HTTP | Client should |
|---|---|---|---|
| `insufficient funds for gas * price + value` | `ErrInsufficientFunds` | 422 | top up / lower amount |
| `nonce too low` | `ErrNonceTooLow` | *(internal: check receipt, never shown to clients)* | — |
| `replacement transaction underpriced` | `ErrReplacementUnderpriced` | *(internal: bump more)* | — |
| `already known` | *(success — same tx already in pool)* | — | — |
| `execution reverted: <reason>` | `ErrReverted{Reason}` | 422 | fix input |
| `max fee per gas less than block base fee` | `ErrFeeTooLow` | *(internal: re-price)* | — |
| timeout / 429 / 5xx from provider | `ErrChainUnavailable` | 503 + `Retry-After` | retry later |
| invalid address / hex | `ErrInvalidAddress` | 400 | fix input |

```json
{
  "type": "https://api.example.com/problems/insufficient-funds",
  "title": "Insufficient funds",
  "status": 422,
  "detail": "Balance 1.2 USDC is lower than requested 250 USDC.",
  "instance": "/v1/withdrawals",
  "available": "1200000",
  "requested": "250000000"
}
```

Match error strings **only in the adapter**, and cover them with tests: node implementations word
errors differently.

## 3.9 Webhooks done right — *Project 7*

- **Event envelope:** `{ id, type, createdAt, data }`. `id` is unique, so receivers deduplicate.
- **Signature:** `X-Signature: t=<unix>,v1=HMAC_SHA256(secret, t + "." + body)`. Receivers check the
  HMAC with `hmac.Equal` and reject stale timestamps (replay protection).
- **Delivery:** written to the **outbox** in the same transaction as the state change (lesson 34), then
  sent by a worker with exponential backoff + jitter, then dead-lettered after N attempts.
- **At-least-once, unordered.** Document it. Put the full current state in `data`, so a receiver that
  sees `invoice.confirmed` before `invoice.pending` still ends up correct.
- **Replay endpoint:** `GET /v1/events?cursor=` lets merchants catch up after their own outage.
- **Reorg events are real events:** `invoice.reverted` must exist, even if it's rare.

## 3.10 Authentication for web3 APIs — *Project 5*

| Caller | Auth |
|---|---|
| end user with a wallet | **Sign-In with Ethereum (EIP-4361)** → session / JWT |
| merchant server | API key (hashed at rest), scoped, rotatable |
| internal service | mTLS or service JWT on gRPC |
| signed user *intent* (order, permit) | **EIP-712 typed-data signature**, verified per request |
| smart-contract wallet (Safe, ERC-4337) | **EIP-1271**: call `isValidSignature(hash, sig)` on the wallet contract |

## 3.11 Versioning and documentation

- URL major version (`/v1`), additive changes only within a version (lesson 41).
- Write the **OpenAPI** spec, including every `status` enum value and every problem `type`.
- Document **finality semantics** per endpoint: "balances are based on `finalized` unless…". This
  is the part integrators get wrong.

## 3.12 A worked design: `POST /v1/withdrawals`, end to end

1. **Handler:** authenticate the API key, require `Idempotency-Key`, validate the address (checksum
   if mixed case), check `amount` is a positive integer string, check `asset` is supported on the
   chain.
2. **Service, in one DB transaction:** idempotency check → lock the merchant ledger account `FOR UPDATE`
   → available balance ≥ amount + fee estimate → insert withdrawal `requested` → post a
   ledger **hold** entry (debit available, credit pending) → insert outbox event
   `withdrawal.requested`.
3. **Response:** `202` + `Location`.
4. **Policy worker:** amount above threshold → `pending_approval` (admin endpoint), else `approved`.
5. **Tx manager:** allocate nonce (2.6) → ask **signer** over gRPC (the signer re-checks limits and the
   allowlist) → store raw tx + hash → broadcast → `broadcast`.
6. **Watcher:** receipt found → `mined` → at `finalized` → `confirmed` → ledger: release the hold, post the
   final transfer and the gas-fee expense.
7. **Stuck:** bump and replace (2.7). **Reverted:** `failed` → ledger reversal → webhook
   `withdrawal.failed`.
8. **Every transition** writes an outbox event → webhook + SSE.

If you can explain every step and the failure mode it handles, you can design a payment system
at a real exchange.

---

# Part 4 — The seven projects

## Overview

| # | Project | Level | One-liner | Big new ideas | Main tech |
|---|---|---|---|---|---|
| 1 | **hashchain** | 🟢 beginner | a single-node PoW blockchain with an HTTP API | hashing, hash pointers, PoW, difficulty, validation | stdlib only |
| 2 | **coinledger** | 🟢 beginner+ | turn it into a cryptocurrency: keys, signed txs, mempool, Merkle, persistence, wallet CLI | signatures, account model, nonces, state transition, Merkle proofs | go-ethereum `crypto`+`rlp`, bbolt |
| 3 | **p2pchain** | 🟡 intermediate | many coinledger nodes over libp2p that fork, reorg, and converge | gossip, fork choice, reorgs, sync, partitions | go-libp2p, GossipSub, Docker |
| 4 | **eth-gateway** | 🟡 intermediate | a clean, cached, resilient REST API over real Ethereum JSON-RPC | JSON-RPC, blocks/receipts, ABI calls, finality tags, gas, big numbers | `ethclient`, anvil, Redis |
| 5 | **web3-auth** | 🟡 intermediate | wallet login (SIWE), EIP-712 intents, token-gated routes | ECDSA recovery, EIP-191/712/4361/1271, replay protection | go-ethereum `crypto`/`apitypes`, JWT, Redis |
| 6 | **token-indexer** | 🔴 hard | deploy an ERC-20, index its events into Postgres, reorg-safe, with API + live feed | Solidity/Foundry, abigen, logs, backfill vs tail, reorg rollback | Foundry, abigen v2, pgx, SSE, Prometheus |
| 7 | **crypto-payments** | 🟣 advanced | a custodial payment gateway: invoices, deposits, withdrawals, webhooks | HD wallets, double-entry ledger, nonce manager, fee bumping, signer isolation | gRPC, RabbitMQ/asynq, outbox, btcutil `hdkeychain` |

```
 From first principles                      On real chains
 ─────────────────────                      ──────────────
 1 hashchain ─► 2 coinledger ─► 3 p2pchain
                    │ (signatures)
                    ├───────────────────────► 5 web3-auth ───┐
                    │                                         │
                    └──► 4 eth-gateway ─► 6 token-indexer ─► 7 crypto-payments
                          (chain port,      (reorg engine     (uses 4's port,
                           resilience)       reused)           5's signing, 6's indexer)
```

Estimated walkthrough time: P1 1–2 sessions · P2 3–4 · P3 3–4 · P4 3 · P5 3 · P6 5–6 · P7 8–10.

---

## Project 1 — `hashchain` 🟢

> ✅ **Built:** [`01-hashchain-beginner/`](01-hashchain-beginner/README.md), with a walkthrough checklist in its
> [`progress.md`](01-hashchain-beginner/progress.md). The sections below are the summary; the project README has
> the full explanation and real measured output.

### Pitch
A proof-of-work blockchain in stdlib Go. One node, blocks with arbitrary data, mining, full-chain
validation, an auto-miner with difficulty retargeting, and an HTTP API to mine, inspect, and **tamper
with history in four ways**, so you can watch validation catch each one, until the last one succeeds.
No transactions or keys yet: just the data structure and the work.

**Real-world analog:** the header chain of Bitcoin, stripped down.

### Concepts you'll own
- Canonical binary encoding (an 89-byte header), and why `json.Marshal` and unprefixed variable-length fields are bad inputs for hashing.
- Header vs body: only `SHA-256(Data)` goes in the header, a preview of Merkle roots.
- Hash pointers, and why editing block 3 invalidates blocks 3…tip.
- Proof-of-work: leading zero bits ⇔ target, expected work = 2^bits, and why block times are so random.
- The 7 validation rules, and why "difficulty" is a consensus rule, not a miner's choice.
- **Difficulty retargeting** with integer math, a dead band, and a ±2-bit clamp, including why it oscillates.
- Concurrency: a ctx-aware mutex (`chan struct{}`), an auto-miner goroutine with cancel + `done`, atomics, and `BaseContext` for shutdown.

### Architecture
```
01-hashchain-beginner/
├── cmd/node/main.go · cmd/powbench/main.go
├── internal/
│   ├── domain/       hash · block (Encode) · pow (Mine) · difficulty · genesis · validate · repository port
│   ├── repository/memory/chain_repository.go   (RWMutex)
│   ├── service/      chain_service.go (mining slot, auto-miner, stats) · tamper.go
│   ├── handler/ · router/ · config/
├── Dockerfile · docker-compose.yml · README.md · progress.md
```

### API
| Method & path | Does |
|---|---|
| `GET /blocks?from=&limit=` · `GET /blocks/{height}` · `GET /blocks/latest` | read blocks (hash, prev_hash, nonce, leading zero bits, attempts, mining time) |
| `POST /blocks` `{ "data": "…" }` | mine and append; the request context cancels mining → `201` |
| `GET /chain/validate` | `{ "valid": false, "first_invalid_height": 3, "rule": "prev_hash", "reason": "…" }` |
| `POST /miner/start` `{ "max_blocks": n }` · `POST /miner/stop` | background auto-miner |
| `POST /debug/tamper/{height}` `{ "data": "…", "mode": "body\|header\|remine\|rewrite" }` | edit history, then report validation |
| `GET /stats` | height, next difficulty and target, average block time, total hashes |

### Phases (details in `progress.md`)
| # | Phase | Tag | Checkpoint |
|---|---|---|---|
| 1 | Scaffold + `Hash` type | 📋 | `go build ./...` |
| 2 | `Header` + canonical `Encode` + `ComputeHash` | 🔁 | golden-hash test |
| 3 | `LeadingZeroBits` + `Mine` | 🧠 | first-valid-nonce and ctx-cancel tests; `powbench` |
| 4 | Genesis + the 7 validation rules | 🧠 rule list · 🔁 code | tamper table test: exact height and rule |
| 5 | `NextDifficulty` retargeting | 🔁 | retarget table test |
| 6 | Repository port + RWMutex store | 🔁 | build |
| 7 | Service: mining slot, auto-miner, tamper | 🔁 | `go test -race ./internal/service` |
| 8 | Config, handlers, router, main (`BaseContext`) | 📋 + 🔁 | `curl` every route |
| 9 | Docker | 📋 | `docker compose up --build` |
| 10 | **Experiments** | notes | numbers in progress.md |

### Experiments
1. `powbench` with many runs: when does "+1 bit ≈ 2× time" actually show? (It doesn't with 5 runs.)
2. Tamper ladder `body → header → remine → rewrite`: each is caught by a different rule, and `rewrite` passes. Compare rewriting from block 3 vs block 9.
3. Cancel a difficulty-28 mine with `curl -m 1`.
4. Retarget toward a 1 s block time: it settles near `log₂(hashrate)` and oscillates between two values.
5. Ctrl-C under load, with and without `cancelBase()`.

### Done when
`go test -race ./...` is green in your copy, the tamper ladder behaves as in the README, and you can rebuild
`Encode` + `Mine` from memory.

### Read in the wild
- `Jeiwan/blockchain_go` — `block.go`, `proofofwork.go` (the classic Go tutorial chain).
- `btcd/blockchain/difficulty.go` — `CompactToBig`, `CalcWork`: how Bitcoin packs a 256-bit target into 4 bytes (`bits`), and why its target isn't a power of two.

### Check questions
- Why does changing one old block break every block after it, but not before it?
- What exactly does "difficulty 22" mean, and why can one block take 10× longer than another?
- Why is the header encoded by hand, and why is `Data` outside it?
- The `rewrite` tamper produced a valid chain. So what makes old blocks hard to change?
- Two miners find block 10 at the same moment. What happens? (A preview of Project 3.)

---

## Project 2 — `coinledger` 🟢+

### Pitch
Turn hashchain into a **cryptocurrency**. Accounts are secp256k1 keys with **real
Ethereum-format addresses**. Users sign transfers offline with a wallet CLI. The node validates
signatures, nonces, and balances, keeps a mempool, mines blocks that pay a reward, commits
transactions with a Merkle root, persists everything to disk, and can prove a transaction's
inclusion with a Merkle proof.

**Real-world analog:** a minimal Ethereum execution layer without the EVM, and a wallet.

### Concepts you'll own
- Keypairs and addresses; **deriving the sender from the signature**.
- Transaction encoding with **RLP** and the signing hash (what exactly gets signed, and why `chainID` is in it).
- The **state transition function** `ApplyTx(state, tx) → (state', error)`, the heart of every blockchain.
- Account model: balances + **nonces** → replay and double-spend protection.
- Mempool admission, per-sender nonce ordering, fee priority, eviction.
- **Merkle root + inclusion proofs** (and the odd-leaf trap).
- **State is derived:** wipe the state bucket, replay the blocks, get identical balances.
- Keystore files (scrypt-encrypted keys). The private key never leaves the CLI.
- A sidebar exercise on the **UTXO model**: compute balances the Bitcoin way from the same blocks.

### Architecture
```
coinledger/
├── cmd/node/main.go
├── cmd/wallet/main.go                 # new · address · balance · send
├── internal/
│   ├── domain/
│   │   ├── tx.go          # Tx, SigningHash (RLP + Keccak), Sender() via recovery
│   │   ├── state.go       # Account{Balance, Nonce}, ApplyTx, ApplyBlock
│   │   ├── merkle.go      # Root, Proof, VerifyProof
│   │   └── block.go       # header now has TxRoot, Miner; body has Txs
│   ├── mempool/pool.go    # admission, per-sender queues, fee ordering, max size
│   ├── repository/bolt/   # buckets: blocks, heightIndex, state, meta
│   ├── service/           # node.go (submit, mine, query) · miner.go
│   └── handler/ …
```

### API
| Method & path | Does |
|---|---|
| `POST /txs` `{ "raw": "0x…" }` | submit a signed tx → `202 { hash, status: "pending" }`; `400` bad sig; `422` bad nonce / insufficient funds |
| `GET /txs/{hash}` | `pending` / `included { block, index, confirmations }` / `404` |
| `GET /accounts/{address}` | `{ balance: "…", nonce: n }` (balance as a string) |
| `GET /mempool` | pending txs grouped by sender |
| `POST /mine` | mine a block from the mempool (reward → configured miner address) |
| `GET /blocks/{height}` | header + tx hashes |
| `GET /blocks/{height}/txs/{hash}/proof` | Merkle proof `{ leaf, siblings: [{hash, left}], root }` |

Wallet CLI: `wallet new` (keystore + password) · `wallet address` · `wallet balance` ·
`wallet send --to 0x… --amount 5 --fee 1` (fetch nonce → build → sign locally → POST raw) ·
`wallet verify-proof --tx 0x… --block 12` (a mini **light client**: checks the proof against the header only).

### Phases
| # | Phase | Tag | Checkpoint |
|---|---|---|---|
| 1 | Keys & addresses lab: generate, sign, recover, EIP-55 | 🧠 | recovered address == signer |
| 2 | `Tx` + RLP signing hash + `Sender()` | 🧠 | tampered amount → different sender / invalid |
| 3 | State & `ApplyTx` (sig → nonce → balance → debit/credit → nonce++) | 🧠 | table test: replay, wrong nonce, overspend all rejected |
| 4 | Mempool | 🔁 | nonce 2 waits until nonce 1 is included |
| 5 | Merkle root + proof + verify | 🔁 / 🧠 verify | proof for every leaf, for 1…9 leaves (odd counts) |
| 6 | Blocks with txs + reward; `ApplyBlock` atomic (all or nothing) | 🔁 | a block with one invalid tx is rejected entirely |
| 7 | bbolt repository (one bolt tx per block) | 📋 + 🔁 | restart node → same tip, same balances |
| 8 | Wallet CLI with keystore | 🔁 | send from CLI, see it included |
| 9 | Handlers, run, double-spend demo | 📋 | demo script passes |

### Done when
Replay and same-nonce double spend are rejected. A restart rebuilds identical state. `verify-proof`
works using only the block header. You can rebuild `Sender()`, `ApplyTx`, and `VerifyProof` from memory.

### Read in the wild
- `go-ethereum/core/types/transaction_signing.go` — signers, chain ID, how `Sender` is cached.
- `go-ethereum/core/state_transition.go` — `preCheck`: nonce and balance checks, the real version of your `ApplyTx`.
- `go-ethereum/core/txpool/legacypool/list.go` — per-account nonce-sorted lists.
- `btcd/blockchain/merkle.go` — `BuildMerkleTreeStore` and the duplicate-last-leaf rule.
- `Jeiwan/blockchain_go` — `transaction.go`, `utxo_set.go` for the UTXO version.

### Check questions
- Why doesn't an Ethereum transaction carry a `from` field?
- What would go wrong without nonces? Without chain ID?
- How can a phone verify a transaction is in a block without downloading the block?
- State is "derived". So what is actually the source of truth, and what's a cache?

---

## Project 3 — `p2pchain` 🟡

### Pitch
Run **4–5 coinledger nodes** in Docker, each mining independently and connected **peer-to-peer
with libp2p**. Transactions and blocks spread by gossip. Nodes produce competing blocks, forks
appear on their own, and the network converges on the heaviest chain. Then you break it on purpose:
partition the network, double-spend across the partition, heal it, and watch a reorg push
transactions back into the mempool.

**Real-world analog:** Bitcoin's network layer and Nakamoto consensus, on your laptop.

### Concepts you'll own
- Peer identity (a key pair → peer ID), multiaddrs, bootstrap peers, discovery.
- **GossipSub** topics, message validation *before relay* (DoS defence), dedup "seen" caches.
- **Orphan blocks** (parent unknown yet) and fetching missing ancestors.
- **Fork choice:** heaviest cumulative work, with tie-breaking.
- **Reorg:** find the common ancestor, undo blocks (return their txs to the mempool), apply the new branch.
- **Initial sync:** headers-first download from a peer over a request/response stream protocol.
- Partitions, 51% attacks, and why confirmations exist, shown by experiment.

### Architecture (adds to coinledger)
```
internal/
├── p2p/
│   ├── host.go         # libp2p host, persisted identity key, bootstrap dial
│   ├── gossip.go       # topics "coinledger/txs/1", "coinledger/blocks/1" + validators
│   └── syncproto.go    # stream protocol "/coinledger/sync/1.0.0": GetHeaders, GetBlocks
├── consensus/
│   ├── forkchoice.go   # block tree, total work per tip, best tip
│   ├── reorg.go        # common ancestor, undo/redo with state journal
│   └── orphans.go      # orphan pool keyed by missing parent
└── chaos/partition.go  # debug: drop traffic to/from selected peers
docker-compose.yml       # node1..node5, each with its own volume + miner key
```

### API additions
| Method & path | Does |
|---|---|
| `GET /node/status` | tip height + hash, total work, peer count, mempool size, syncing flag |
| `GET /peers` | peer IDs, addresses, latency |
| `GET /forks` | all known tips with work; which one is best |
| `GET /events` (SSE) | live `block_added`, `reorg { depth, oldTip, newTip }`, `peer_connected` |
| `POST /debug/partition` `{ "block": ["peerID", …] }` · `DELETE /debug/partition` | simulate a network split |
| `POST /debug/miner` `{ "threads": 4 }` | change a node's "hashpower" |

### Phases
| # | Phase | Tag | Checkpoint |
|---|---|---|---|
| 1 | **Naive version first:** nodes poll each other's HTTP `/blocks/latest` | 📋 | it "works"… then list its 4 problems |
| 2 | libp2p host, identity key persisted, bootstrap peers from env | 🔁 | 5 nodes connected in `/peers` |
| 3 | GossipSub topics + validators + seen cache | 🔁 | tx submitted on node1 appears in node5's mempool |
| 4 | Block tree, orphan pool, fetch-missing-parent | 🔁 | out-of-order delivery still builds the right tree |
| 5 | **Fork choice + reorg** with state undo | 🧠 | unit test: 3-block reorg restores exact balances and mempool |
| 6 | Sync protocol (headers → blocks) | 🔁 | wipe node3's volume, restart, it catches up |
| 7 | Chaos endpoints + SSE events | 📋 | — |
| 8 | **Experiments** | 🔁 notes | results table in progress.md |

### Experiments
1. **Natural forks:** lower the difficulty so blocks come fast, and count reorgs per minute in SSE.
2. **Partition:** split {1,2} vs {3,4,5} for 60 s → heal → which side reorged, how deep, which txs returned to the mempool?
3. **Double spend across a partition:** spend the same nonce to Bob on side A and to Carol on side B.
   After healing, who got paid? What did each side's "1 confirmation" mean?
4. **51%:** give node1 more threads than all the others combined, partition it, let it mine privately, heal. It rewrites history.

### Done when
Every node converges to the same tip after each experiment. The reorg unit test is green under `-race`.
You can explain experiment 3 to someone else.

### Read in the wild
- `btcd/blockchain/chain.go` — `connectBestChain`, `reorganizeChain`, `getReorganizeNodes`.
- `btcd/netsync/manager.go` — headers-first sync.
- `go-libp2p` GitHub repo `examples/` (chat, pubsub — a nested module, so not in the downloaded module cache) and `go-libp2p-pubsub` docs on validators.
- `OffchainLabs/prysm` `beacon-chain/p2p` — a production Ethereum client on libp2p GossipSub.

### Check questions
- Why must a node validate a block *before* relaying it?
- What happens to the transactions of an orphaned block?
- Why "most work" and not "longest"?
- A merchant accepted a payment with 1 confirmation during experiment 3. What went wrong, and what rule fixes it?

---

## Project 4 — `eth-gateway` 🟡

### Pitch
Leave the toy chain. Build the service that sits between your company's apps and **real
Ethereum**: a typed, cached, rate-limited, multi-provider REST API over JSON-RPC. It serves blocks,
transactions with fees, ETH and ERC-20 balances, token metadata, a gas oracle, and node health.
It runs against **anvil** locally and against **Sepolia** through a provider.

**Real-world analog:** the internal "chain gateway" at wallets, exchanges, and analytics companies;
a small Alchemy/Infura-style enhanced API.

### Concepts you'll own
- JSON-RPC 2.0 by hand (`curl`), then through `ethclient` and `rpc.Client` batch calls.
- Blocks, transactions, **receipts** (`status`, `gasUsed`, `effectiveGasPrice`, logs).
- `latest` / `safe` / `finalized` and what each is safe for.
- ABI encoding: calling `balanceOf`, `decimals`, `symbol` with `accounts/abi` (by hand before generated bindings).
- `*big.Int` money handling, `FormatUnits` / `ParseUnits`.
- EIP-1559 fee estimation from `eth_feeHistory`.
- Resilience around a flaky, rate-limited upstream: deadlines, classified retries, fallback, circuit breaker, head-lag checks.
- **Finality-aware caching** + `singleflight`.
- Testing with `ethclient/simulated`.

### Architecture
```
eth-gateway/
├── cmd/api/main.go
├── contracts/                    # tiny Foundry project: a test ERC-20 for local dev
├── internal/
│   ├── domain/                   # Block, Tx, Receipt, Amount, Token, Finality, ChainReader port, errors
│   ├── chain/ethereum/           # ethclient adapter + error classification + batch calls
│   ├── chain/resilient/          # decorator: timeout, retry, breaker, rate limit, fallback, head-lag
│   ├── cache/                    # finality-aware cache (in-memory LRU → Redis) + singleflight
│   ├── service/                  # blocks, accounts, tokens, gas
│   └── handler/                  # DTOs (strings for amounts), problem+json
├── docker-compose.yml            # anvil + deployer (forge create) + redis + api + prometheus
```

### API
| Method & path | Does |
|---|---|
| `GET /v1/chains/{chainId}/blocks/{ref}` | `ref` = number, hash, `latest`, `safe`, `finalized` |
| `GET /v1/chains/{chainId}/txs/{hash}` | tx + receipt + computed fee (`gasUsed × effectiveGasPrice`), confirmations, finality |
| `GET /v1/chains/{chainId}/addresses/{addr}/balance?finality=` | ETH balance (raw + formatted) and the block it's based on |
| `GET /v1/chains/{chainId}/addresses/{addr}/tokens?contracts=0x…,0x…` | many ERC-20 balances in **one batched RPC round trip** |
| `GET /v1/chains/{chainId}/tokens/{contract}` | name, symbol, decimals, totalSupply (cached forever except supply) |
| `GET /v1/chains/{chainId}/gas` | base fee, suggested tips (slow/normal/fast from fee-history percentiles), estimated cost of a transfer |
| `GET /v1/status` | per chain and provider: head, finalized, lag, breaker state |

### Phases
| # | Phase | Tag | Checkpoint |
|---|---|---|---|
| 1 | Local chain: anvil in Docker; raw JSON-RPC with `curl`; `cast` basics | 📋 + 🔁 commands | you read a block and a receipt by hand |
| 2 | Domain types + `ChainReader` port + ethclient adapter | 🔁 | adapter test against `ethclient/simulated` |
| 3 | Money: `FormatUnits`, `ParseUnits`, JSON string amounts | 🧠 | fuzz test: `Parse(Format(x)) == x` |
| 4 | ABI by hand: pack `balanceOf`, unpack result, handle non-standard tokens | 🔁 | balance matches `cast call` |
| 5 | Batch RPC for multi-token balances | 🔁 | 20 tokens = 1 HTTP request (log it) |
| 6 | Finality-aware cache + singleflight | 🔁 | finalized block hit twice → one RPC call |
| 7 | Resilient decorator: deadline, classify, retry reads, breaker, fallback, rate limit, head-lag | 📋 (lessons 36/68) + 🔁 classification | kill primary provider → requests still succeed |
| 8 | Gas oracle from `FeeHistory` | 🔁 | numbers match `cast base-fee` / `cast gas-price` sanity |
| 9 | Handlers, problem+json, metrics | 📋 | full `curl` tour against Sepolia |

### Done when
All endpoints work on anvil **and** Sepolia. The kill-the-primary test passes. No `float64` anywhere
(`grep` it). The simulated-backend tests are green.

### Read in the wild
- `go-ethereum/ethclient/ethclient.go` — each method is a thin JSON-RPC call; notice `toBlockNumArg`.
- `go-ethereum/rpc/client.go` — `BatchCallContext`, subscriptions, reconnect behaviour.
- `go-ethereum/eth/gasprice/feehistory.go` — how the node computes fee-history percentiles.
- `go-ethereum/accounts/abi/abi.go` — `Pack` / `Unpack`.

### Check questions
- Why can you cache `finalized` data forever but not `latest`?
- What's the difference between `eth_call` and sending a transaction?
- A token has `decimals = 6`. How is "12.34" sent on-chain, and why is it a string in your JSON?
- Your provider returns "header not found" for a block it just announced. Why, and what do you do?

---

## Project 5 — `web3-auth` 🟡

### Pitch
Users log in with their **wallet** instead of a password: **Sign-In with Ethereum (EIP-4361)**.
The server issues a nonce, the wallet signs a structured message, the server verifies it and
issues JWTs. On top of that come **EIP-712 signed intents** (a "gasless" signed order the backend
verifies and stores), **smart-contract wallet** support (EIP-1271), and **token-gated** routes where
access depends on on-chain holdings. It extends your `auth-service-intermediate`.

**Real-world analog:** login and order signing at OpenSea, Uniswap-style order books, Snapshot voting, and most dApp backends.

### Concepts you'll own
- ECDSA public-key recovery in practice; `v` normalisation; low-s.
- **EIP-191** (`personal_sign` prefix) vs **EIP-712** (typed structured data with a domain separator).
- The **EIP-4361** message format and *why each field exists* (domain → anti-phishing, nonce → anti-replay, chain ID, expiry).
- Single-use nonces with atomic consume (`GETDEL` in Redis).
- **EIP-1271** signatures from contract wallets (`isValidSignature` via `eth_call`, reusing P4's chain port).
- Token gating → roles (lesson 43), cached at `finalized`.
- Attack-driven testing: replay, wrong domain, wrong chain, expired, tampered, wrong signer.

### The login flow
```
Browser/wallet                         web3-auth                               Redis
     │ GET /v1/auth/nonce ───────────────►│ nonce = random 96 bits ─────────────►│ SET nonce EX 300
     │◄──────────────────── { nonce } ────│                                        │
     │ build EIP-4361 message:            │                                        │
     │  "app.example.com wants you to sign in with your Ethereum account:\n0xAb…\n\n
     │   Sign in to Example\n\nURI: https://app.example.com\nVersion: 1\n
     │   Chain ID: 1\nNonce: …\nIssued At: 2026-09-14T09:00:00Z"
     │ wallet personal_sign(message)      │                                        │
     │ POST /v1/auth/verify {message,sig}►│ parse → check domain, URI, chain, time │
     │                                    │ GETDEL nonce (single use) ────────────►│
     │                                    │ recover addr (EOA) or EIP-1271 (contract)
     │                                    │ addr == message.address ?              │
     │◄──── { accessToken, refreshToken } │ JWT sub = checksummed address          │
```

### API
| Method & path | Does |
|---|---|
| `GET /v1/auth/nonce` | new single-use nonce (rate-limited per IP) |
| `POST /v1/auth/verify` | verify SIWE message + signature → tokens |
| `POST /v1/auth/refresh` · `POST /v1/auth/logout` | rotate / revoke refresh token |
| `GET /v1/me` | address, ENS-less profile, roles derived from holdings |
| `POST /v1/intents/orders` | body: EIP-712 typed `Order{maker, token, amount, price, nonce, deadline}` + signature → verify, check nonce and deadline, store |
| `GET /v1/intents/orders?maker=` | list stored intents |
| `GET /v1/gated/holders-lounge` | requires ≥ N of the configured ERC-20 or ≥ 1 ERC-721 at `finalized` |

A static `web/index.html` uses the browser wallet (`window.ethereum`) for the real flow. A
`cmd/devwallet` CLI signs with an anvil key for scripted tests.

### Phases
| # | Phase | Tag | Checkpoint |
|---|---|---|---|
| 1 | Signatures lab: `personal_sign` round trip incl. v=27/28 | 🧠 | recover works for Go-signed and MetaMask-signed messages |
| 2 | EIP-4361 message builder + strict parser | 🔁 | parser rejects extra whitespace / missing fields |
| 3 | Nonce store with atomic single-use consume | 🔁 | concurrent verify of one nonce → exactly one success |
| 4 | Verify use case (all checks, in the right order) | 🧠 | attack table test green |
| 5 | JWT issuing + auth middleware | 📋 (auth-service) | `/v1/me` works |
| 6 | EIP-712 typed-data verification (`apitypes.TypedDataAndHash`) | 🔁 | intent signed by devwallet verifies; changed amount fails |
| 7 | EIP-1271 contract-wallet path via the P4 chain port | 🔁 | a tiny test wallet contract on anvil verifies |
| 8 | Token-gating middleware + cached role derivation | 🔁 | transfer the token away → access lost after cache TTL |
| 9 | Browser page + devwallet CLI | 📋 | real wallet login on Sepolia |

### Done when
The attack table (replay · wrong domain · wrong chain · expired · not-yet-valid · tampered message ·
signature from another key · reused EIP-712 nonce · past deadline) is all red→green. A real browser
wallet logs in.

### Read in the wild
- `spruceid/siwe-go` — message parsing and verification (last release 2023: read it, don't depend on it).
- `go-ethereum/signer/core/apitypes/types.go` — `TypedData`, `HashStruct`, `TypedDataAndHash`.
- `go-ethereum/accounts/accounts.go` — `TextHash` (the EIP-191 prefix).
- EIP-4361, EIP-712, EIP-1271 specs (short and readable).

### Check questions
- Why not just have the user sign the word "login"?
- What does the `domain` field defend against?
- `personal_sign` vs `eth_signTypedData_v4`: what does the user see, and what changes for verification?
- How does a Safe multisig "sign" a message when it has no private key?

---

## Project 6 — `token-indexer` 🔴

### Pitch
Write an **ERC-20 token in Solidity**, deploy it with **Foundry**, generate **Go bindings with
abigen**, and build a production-shaped **indexer**. It backfills history, tails new blocks live,
survives restarts and WebSocket drops, and **handles reorgs correctly**. It turns `Transfer` events into
Postgres tables (transfers, holders, balances), exposes them through a REST API with cursor pagination
and a **live SSE feed**, and proves its own correctness with a reconciliation job.

**Real-world analog:** the core of Etherscan's token pages, The Graph subgraphs, Dune-style
datasets, or any wallet's "transaction history" backend.

### Concepts you'll own
- Solidity basics (enough to read contracts), Foundry `forge`/`cast`/`anvil`, OpenZeppelin.
- ABI → **abigen v2 bindings** → typed event decoding (and decoding topics by hand first).
- `eth_getLogs` filters (address + topic0 + indexed topics), **adaptive range chunking**.
- **Backfill vs live tail**, and the handover between them without gaps or duplicates.
- **Parent-hash chaining, reorg detection, and atomic rollback.**
- One DB transaction per block batch: data + derived balances + checkpoint → crash-safe and idempotent.
- WS subscription with reconnect + gap fill + polling fallback.
- `NUMERIC(78,0)`, cursor pagination on `(block_number, log_index)`.
- Postgres `LISTEN/NOTIFY` → SSE (lesson 58).
- Metrics: head, indexed, lag, reorg depth; a reconciliation job as a correctness oracle.

### Architecture
```
token-indexer/
├── contracts/                    # Foundry: src/Token.sol (OpenZeppelin ERC20), script/Deploy.s.sol
├── bindings/token/               # abigen --v2 output (📋 generated — never edit)
├── cmd/
│   ├── indexer/main.go           # the worker
│   ├── api/main.go               # read-only API + SSE
│   ├── trafficgen/main.go        # random transfers from anvil accounts
│   └── reconcile/main.go         # compares DB balances with on-chain balanceOf at finalized
├── internal/
│   ├── domain/                   # Transfer, BlockRef, Checkpoint, ports
│   ├── chain/                    # adapter (from P4) + log decoding
│   ├── indexer/
│   │   ├── backfill.go           # chunked FilterLogs, worker pool, ordered commit
│   │   ├── tail.go               # newHeads subscription, reconnect, polling fallback
│   │   ├── processor.go          # ★ block batch → one DB tx
│   │   └── reorg.go              # ★ parent-hash check, find ancestor, rollback
│   ├── repository/postgres/      # pgx, ON CONFLICT DO NOTHING, balances as deltas
│   └── handler/                  # cursor pagination, SSE
├── migrations/                   # goose
├── docker-compose.yml            # anvil · deployer · postgres · indexer · api · trafficgen · prometheus · grafana
```

### Data model (core)
```sql
CREATE TABLE blocks (
  number      BIGINT PRIMARY KEY,
  hash        BYTEA  NOT NULL UNIQUE,
  parent_hash BYTEA  NOT NULL,
  timestamp   TIMESTAMPTZ NOT NULL
);
CREATE TABLE transfers (
  block_hash   BYTEA  NOT NULL REFERENCES blocks(hash) ON DELETE CASCADE,
  log_index    INT    NOT NULL,
  block_number BIGINT NOT NULL,
  tx_hash      BYTEA  NOT NULL,
  token        BYTEA  NOT NULL,
  from_addr    BYTEA  NOT NULL,
  to_addr      BYTEA  NOT NULL,
  value        NUMERIC(78,0) NOT NULL,
  PRIMARY KEY (block_hash, log_index)
);
CREATE TABLE balances (token BYTEA, holder BYTEA, balance NUMERIC(78,0) NOT NULL, PRIMARY KEY (token, holder));
CREATE TABLE checkpoints (name TEXT PRIMARY KEY, block_number BIGINT NOT NULL, block_hash BYTEA NOT NULL);
```
Rollback = for each orphaned block: subtract its transfers from `balances`, delete the block
(cascade removes transfers), move the checkpoint to the ancestor, all in one transaction.

### API
| Method & path | Does |
|---|---|
| `GET /v1/tokens/{token}` | metadata + holders count + transfers count + indexed-through block |
| `GET /v1/tokens/{token}/holders?limit=&cursor=` | sorted by balance desc |
| `GET /v1/tokens/{token}/transfers?limit=&cursor=` | newest first, cursor on `(block, logIndex)` |
| `GET /v1/addresses/{addr}/transfers?token=&direction=in|out` | history for one address |
| `GET /v1/tokens/{token}/balances/{holder}` | balance + `asOfBlock` + finality |
| `GET /v1/stream/transfers?token=` | **SSE**: live transfers, plus `reorg` events |
| `GET /v1/indexer/status` | chain head, finalized, indexed, lag, last reorg (depth, time) |

### Phases
| # | Phase | Tag | Checkpoint |
|---|---|---|---|
| 1 | Solidity + Foundry crash course; deploy Token to anvil; `cast send`/`cast logs` | 📋 contract · 🔁 commands | you see a `Transfer` log in the raw receipt |
| 2 | abigen v2 bindings; decode a log **by hand** (topics → addresses, data → uint256), then with bindings | 🔁 · 🧠 by-hand decode | both decoders agree in a test |
| 3 | Schema + goose migrations + repository | 🔁 | `ON CONFLICT` makes double-insert a no-op |
| 4 | Backfill: adaptive chunking (halve on "too many results"), bounded worker pool, **ordered** commit | 🔁 | 50k transfers backfilled; row count == `cast logs` count |
| 5 | **Processor:** batch → one DB tx (blocks, transfers, balance deltas, checkpoint) | 🧠 | `kill -9` mid-backfill → restart → identical totals |
| 6 | **Reorg detection & rollback** | 🧠 | forced reorg via `evm_snapshot`/`evm_revert` (or `anvil_reorg`) → balances correct |
| 7 | Live tail: `SubscribeNewHead` + reconnect + gap fill + polling fallback | 🔁 | stop anvil 30 s → restart → no gap |
| 8 | API: cursor pagination + SSE via LISTEN/NOTIFY | 📋 handlers · 🔁 cursor + SSE | two browser tabs receive live transfers |
| 9 | Metrics + Grafana dashboard + reconciliation job | 📋 dashboard · 🔁 reconcile | reconcile reports 0 mismatches after a chaos run |

### Done when
A chaos script (random `kill -9`, forced reorgs of depth 1–5, WS drops, a trafficgen burst) runs for
10 minutes and **reconcile reports zero mismatches**: every holder's balance equals `balanceOf` at
`finalized`, and `Σ balances == totalSupply`.

### Read in the wild
- `lightningnetwork/lnd/chainntnfs` — reorg-aware confirmation and spend notifications, production-grade (Bitcoin, same ideas).
- `go-ethereum/eth/filters/filter.go` — how nodes serve `eth_getLogs`, including bloom-filter skipping.
- `go-ethereum/core/types/log.go` — note the `Removed` field.
- `go-ethereum/accounts/abi/bind/v2` — what your generated bindings call under the hood.
- OpenZeppelin `ERC20.sol` — `_update`, and why mint/burn are `Transfer`s from/to `address(0)`.

### Check questions
- Why is the transfers primary key `(block_hash, log_index)` and not `(tx_hash, log_index)`?
- Walk through what happens when block N's `parentHash` doesn't match your stored N−1.
- Why must the checkpoint be updated in the same transaction as the data?
- Your WebSocket was down for 40 blocks. What did you miss, and how do you recover?

---

## Project 7 — `crypto-payments` 🟣

### Pitch
A **custodial crypto payment gateway** for ETH and one ERC-20 stablecoin. Merchants create
**invoices**. Each gets a **fresh deposit address derived from an xpub**. The system detects
payments, waits for finality, credits a **double-entry ledger**, **sweeps** deposits to a treasury,
processes **merchant withdrawals** through an isolated **signer** and a **transaction manager** (nonces,
fee bumping, replacements), and notifies merchants through **signed webhooks**. Everything is built to
survive crashes, duplicates, RPC lies, and reorgs.

**Real-world analog:** Coinbase Commerce / BitPay, or the deposit and withdrawal system inside any exchange.

### Services
```
                   ┌──────────────┐  REST (API keys, Idempotency-Key)
 merchants ───────►│     api      │─────────────┐
                   │ invoices     │             │ outbox → RabbitMQ / asynq
                   │ withdrawals  │             ▼
                   │ ledger read  │     ┌───────────────┐   HMAC webhooks
                   │ xpub → addrs │     │   notifier    │──────────────► merchant servers
                   └──────┬───────┘     └───────────────┘
                          │ events                ▲
         ┌────────────────┼────────────────┬──────┘
         ▼                ▼                ▼
 ┌──────────────┐ ┌──────────────┐ ┌──────────────┐        gRPC (mTLS)   ┌──────────────┐
 │   watcher    │ │   ledger     │ │    txmgr     │─────────────────────►│    signer    │
 │ (P6 engine): │ │ double-entry │ │ nonce, fees, │                      │ xprv / KMS   │
 │ ETH txs +    │ │ invariants   │ │ broadcast,   │                      │ policy:limits│
 │ ERC-20 logs  │ │              │ │ bump, track  │                      │ allowlist    │
 │ to deposit   │ └──────────────┘ └──────┬───────┘                      │ audit log    │
 │ addresses    │                         │                              └──────────────┘
 └──────┬───────┘                ┌────────┴───────┐
        └───────── Ethereum ◄────┤    sweeper     │  deposit addrs → treasury (gas top-up for ERC-20)
                   (anvil)       └────────────────┘
```

### Concepts you'll own
- **HD wallets:** mnemonic → seed → xprv/xpub, BIP-44 paths, address generation without private keys.
- **Double-entry ledger:** every movement is balanced debits/credits; balances are sums; corrections are reversing entries.
- **Deposit detection:** native ETH (scan txs; the "internal transfer" caveat) vs ERC-20 (logs), under/over/late payments.
- **Two-tier confirmation:** show `pending` at first sight, credit at `finalized`, **reverse on reorg**.
- **Signer isolation** and signing policy.
- **Transaction manager:** DB-backed nonces, EIP-1559 pricing, replacement ≥ 10%, stuck-tx handling, "nonce too low" ≠ failure.
- **Sweeping** and the gas-funding problem for token deposits (top-up vs forwarder contracts vs EIP-7702, as a design discussion).
- Sagas across services (lesson 35), outbox (34), idempotency (41), gRPC (27), job queues (44), rate limits (68), RBAC for admin approvals (43).

### State machines
```
Invoice:     created ─► seen ─► confirmed ─► settled
                │         │          └─► reverted (reorg after confirm — rare, but modelled)
                │         ├─► underpaid ─(top-up)─► confirmed
                │         └─► overpaid (credit + flag)
                └─► expired ─(late payment)─► confirmed_late

Withdrawal:  requested ─► approved ─► signed ─► broadcast ─► mined ─► confirmed
                 └─► rejected         ▲             │   └─► failed (reverted → ledger reversal)
                                      └─ replaced ◄─┘ (fee bump: same nonce, new hash)
```

### API (merchant-facing)
| Method & path | Does |
|---|---|
| `POST /v1/invoices` *(Idempotency-Key)* | amount + asset → invoice with deposit address, expiry, payment URI (EIP-681) |
| `GET /v1/invoices/{id}` | status, received amount, txs, confirmations |
| `GET /v1/invoices/{id}/events` (SSE) | live status for a checkout page |
| `GET /v1/balances` | available / pending / held per asset (from the ledger) |
| `GET /v1/ledger/entries?cursor=` | the merchant's entries |
| `POST /v1/withdrawals` *(Idempotency-Key)* | → `202` + status resource (see 3.12) |
| `GET /v1/withdrawals/{id}` | status + attempts (tx hashes, nonces, fees) |
| `POST /v1/webhook-endpoints` · `GET /v1/events?cursor=` | register a URL (secret returned once) · replay events |

Admin (RBAC): `GET /admin/wallets` (hot wallet balance, next nonce, pending txs, oldest pending age) ·
`POST /admin/withdrawals/{id}/approve|reject` · `POST /admin/txs/{id}/bump|cancel`.

### Phases
| # | Phase | Tag | Checkpoint |
|---|---|---|---|
| 1 | Design doc: read the system design + failure-mode table; write your answers to 12 "what if…" questions | 📖 + 🔁 answers | discussion |
| 2 | HD wallet lab: mnemonic → xprv/xpub → addresses; prove xpub-derived == xprv-derived | 🧠 | test with anvil's known accounts |
| 3 | Ledger: accounts, balanced entries, holds, reversals, invariant checks | 🧠 | property test: any sequence keeps Σ = 0 |
| 4 | api: invoices + address allocation + idempotency + outbox | 🔁 | duplicate POST → same invoice |
| 5 | watcher: reuse the P6 engine; ERC-20 logs + native ETH tx scan to watched addresses | 🔁 | pay an invoice from anvil → `seen` |
| 6 | Confirmation tiers + reorg → ledger reversal | 🧠 | forced reorg after `seen` and after `confirmed`: both handled |
| 7 | signer: gRPC service, policy (per-tx max, daily limit, allowlist), audit log | 🔁 | over-limit request refused by the signer itself |
| 8 | **txmgr:** nonce allocation, pricing, broadcast, receipt tracking, bump/replace, restart recovery | 🧠 | spike base fee on anvil → tx gets bumped and mined |
| 9 | sweeper + gas top-up flow | 🔁 | deposits end in treasury; ledger matches chain |
| 10 | notifier: HMAC-signed webhooks, retries, DLQ, replay endpoint | 📋 (lessons 34/44) + 🔁 signing | a receiver verifies signatures; a 500 receiver gets retries |
| 11 | Admin + RBAC + metrics + dashboards | 📋 | Grafana shows pending txs and hot wallet balance |
| 12 | **Chaos suite** (below) + invariant checker | 🔁 | all invariants hold |

### Chaos suite: the scenarios real systems fail on
1. Kill `txmgr` **after signing, before broadcast** → restart → the tx is broadcast exactly once, with no nonce gap.
2. The RPC **times out on `eth_sendRawTransaction` but actually accepted it** → the retry sees `already known` / `nonce too low` → the receipt is found → no double spend.
3. **Reorg removes a deposit** after it was `seen` (and, separately, after `confirmed` on `latest`-mode config) → correct reversal + `invoice.reverted` webhook.
4. **Double-click withdrawal** (same Idempotency-Key, concurrent) → one withdrawal.
5. **Gas spike** (`anvil_setNextBlockBaseFeePerGas`) → stuck tx → bumped → mined; fee accounted in the ledger.
6. **Hot wallet runs out of ETH for gas** → withdrawals pause with a clear status, alert fires, nothing corrupts.
7. **Webhook endpoint down for an hour** → retries with backoff → DLQ → replay via `/v1/events`.

**Invariants** (checked after every run): ledger Σ = 0 · on-chain treasury + hot wallet balances ==
ledger custody accounts ± in-flight txs · no nonce gaps · each confirmed withdrawal has exactly one mined tx ·
no invoice credited twice.

### Done when
The chaos suite passes with all invariants green, and you can whiteboard the withdrawal flow
(3.12) and explain what protects each step.

### Read in the wild
- `ethereum-optimism/optimism/op-service/txmgr` — a production Go tx manager: nonce queue, fee estimation, price bumps, send state.
- `go-ethereum/core/txpool/legacypool` — replacement rules and the 10% price bump from the node's side.
- `btcsuite/btcd/btcutil/v2/hdkeychain` — BIP-32 in Go.
- `lightningnetwork/lnd/sweep` + `chainntnfs` — sweeping and confirmation tracking in a money-critical Go system.
- `coinbase/mesh-sdk-go` (formerly Rosetta) — a standardised API for exchange-grade blockchain integrations; compare its model with yours.

### Check questions
- Why does the API server hold only an xpub?
- The broadcast call timed out. Is the withdrawal sent or not? How do you find out safely?
- Why never delete a ledger entry, even when a reorg proves it wrong?
- An ERC-20 deposit address has tokens but no ETH. How do you move the tokens? List two approaches and their trade-offs.

---

## Bonus projects (after the seven, optional)

| # | Project | What it adds | Tech |
|---|---|---|---|
| 8 | **cometbft-ledger** — rewrite coinledger's state machine as an **ABCI app** on CometBFT | a real BFT consensus engine replaces your P3 consensus; instant finality; `CheckTx`/`FinalizeBlock`/`Commit` | `cometbft/cometbft` (then `cosmos-sdk` modules) |
| 9 | **btc-watcher** — Bitcoin deposit watcher on regtest/signet | UTXO deposits, address types, PSBT, RBF — the UTXO side of P7 | `btcsuite/btcd` (`rpcclient`), `btcutil/v2/psbt` |
| 10 | **solana-balance-api** — P4 for Solana | a different account model, commitment levels (`processed`/`confirmed`/`finalized`), SPL tokens | `gagliardetto/solana-go` |

---

# Part 5 — Libraries & technologies

Versions checked on the Go module proxy on **2026-09-14**. Pin the version in `go.mod`, and re-check when
you start a project.

## 5.1 Blockchain & cryptography libraries

| Library (import path) | Version | What it's for | Projects |
|---|---|---|---|
| **`github.com/ethereum/go-ethereum`** | v1.17.5 | The official Ethereum client, used as a library. The packages you'll use: | 2, 4–7 |
| ↳ `…/crypto` | | secp256k1 keys, `Sign`, `SigToPub`, `Keccak256`, `PubkeyToAddress` | 2, 5, 7 |
| ↳ `…/rlp` | | Ethereum's canonical encoding | 2 |
| ↳ `…/common`, `…/common/hexutil` | | `Address`, `Hash`, EIP-55, hex helpers | 2, 4–7 |
| ↳ `…/ethclient`, `…/rpc` | | typed JSON-RPC client; raw client with batching + subscriptions | 4–7 |
| ↳ `…/core/types` | | `Block`, `Header`, `Transaction`, `Receipt`, `Log`, `DynamicFeeTx`, `LatestSignerForChainID` | 4–7 |
| ↳ `…/accounts/abi` | | ABI `Pack`/`Unpack`, event decoding | 4, 6 |
| ↳ `…/accounts/abi/bind/v2` + `cmd/abigen --v2` | | generated type-safe contract bindings | 6, 7 |
| ↳ `…/ethclient/simulated` | | in-memory chain with a real EVM for tests (`NewBackend`, `Commit`) | 4–7 |
| ↳ `…/accounts` (`TextHash`), `…/signer/core/apitypes` | | EIP-191 and EIP-712 hashing | 5 |
| ↳ `…/accounts/keystore` | | scrypt-encrypted key files | 2, 7 |
| `github.com/holiman/uint256` | v1.3.2 | fixed-width 256-bit ints (no allocations); what geth uses internally | 6–7 (hot paths, optional) |
| `github.com/btcsuite/btcd/btcutil/v2` (`hdkeychain`) | v2.0.1 | BIP-32 extended keys, xpub derivation | 7 |
| `github.com/btcsuite/btcd/chaincfg/v2` | v2.0.0 | network params needed by `hdkeychain.NewMaster` | 7 |
| `github.com/cosmos/go-bip39` | v1.0.0 | BIP-39 mnemonic ↔ seed. The original `tyler-smith/go-bip39` GitHub repo is gone (the proxy still serves it); the cosmos fork is the common drop-in | 7 |
| `github.com/btcsuite/btcd` | v0.26.2 | Bitcoin full node in Go: `rpcclient`, `wire`, `txscript`, `blockchain` (reading) | 1–3 (read), 9 |
| `github.com/btcsuite/btcd/btcec/v2` | v2.5.0 | secp256k1 (+ Schnorr) for Bitcoin | 9 |
| `github.com/decred/dcrd/dcrec/secp256k1/v4` | v4.4.1 | pure-Go secp256k1 underneath btcec. Readable implementation of the curve | reading |
| `github.com/libp2p/go-libp2p` | v0.49.0 | P2P host, peer identity, streams, mDNS discovery (`p2p/discovery/mdns`) | 3 |
| `github.com/libp2p/go-libp2p-pubsub` | v0.17.0 | GossipSub with message validators | 3 |
| `github.com/libp2p/go-libp2p-kad-dht` | v0.42.2 | Kademlia DHT for internet-scale peer discovery (discussed, optional) | 3 |
| `go.etcd.io/bbolt` | v1.5.0 | embedded B+tree KV store, single file, ACID transactions | 2–3 |
| `github.com/cockroachdb/pebble/v2` | v2.1.7 | LSM KV store (geth uses Pebble); for reading / optional swap | 3 (optional) |
| `github.com/cometbft/cometbft` | v1.0.1 | BFT consensus engine + ABCI | 8 |
| `github.com/cosmos/cosmos-sdk` | v0.55.0 | framework for app-specific chains | 8 |
| `github.com/gagliardetto/solana-go` | v1.23.0 | Solana RPC client, transactions, SPL | 10 |
| `github.com/spruceid/siwe-go` | v0.2.1 (2023) | SIWE reference implementation. Stale, so **read** it and implement yourself | 5 (reading) |
| `github.com/coinbase/mesh-sdk-go` | v0.12.0 | Mesh (ex-Rosetta) API spec SDK | 7 (reading) |

> **Heads-up on btcd paths:** btcd was recently split into `/v2` submodules
> (`btcutil/v2`, `chaincfg/v2`, `address/v2`, `chainhash/v2`, `wire/v2`). Most tutorials and
> older code still import `github.com/btcsuite/btcd/btcutil` + `…/chaincfg`. **Don't mix the two
> generations in one module.** The P7 HD snippet in 1.15 was compiled with the v2 paths.

## 5.2 Backend libraries (the ones you already know from the lessons)

| Library | Version | Used for | Projects |
|---|---|---|---|
| stdlib `net/http` (1.22+ routing), `log/slog`, `crypto/sha256`, `crypto/hmac`, `math/big`, `encoding/binary` | Go 1.27 | everything | all |
| `github.com/jackc/pgx/v5` | v5.11.0 | Postgres driver, `NUMERIC` as strings, `LISTEN/NOTIFY` | 6, 7 |
| `github.com/pressly/goose/v3` | v3.28.0 | SQL migrations | 6, 7 |
| `github.com/redis/go-redis/v9` | v9.22.0 | nonce store, cache, rate-limit counters | 4, 5, 7 |
| `github.com/golang-jwt/jwt/v5` | v5.3.1 | access/refresh tokens | 5, 7 |
| `golang.org/x/time/rate` | v0.16.0 | outbound RPC limits + inbound API limits | 4–7 |
| `golang.org/x/sync` (`singleflight`, `errgroup`) | v0.23.0 | cache stampede protection, lifecycle | 4, 6, 7 |
| `google.golang.org/grpc` | v1.83.2 | api ↔ signer / txmgr | 7 |
| `github.com/rabbitmq/amqp091-go` | v1.14.0 | events between services | 7 |
| `github.com/hibiken/asynq` | v0.26.0 | webhook delivery jobs, sweeps, retries | 7 |
| `github.com/prometheus/client_golang` | v1.24.1 | metrics | 3–7 |
| `github.com/testcontainers/testcontainers-go` | v0.44.0 | anvil + Postgres + Redis in integration tests | 4–7 |

## 5.3 Tools

| Tool | What | Where it shows up |
|---|---|---|
| **Foundry** (`forge`, `cast`, `anvil`, `chisel`) | Solidity toolchain in Rust. `anvil`: local Ethereum node (chain ID **31337**, instant mining, 10 funded accounts from the mnemonic `test test … junk`). `cast`: a JSON-RPC Swiss-army knife. `forge`: compile/test/deploy. | P4–P7. Docker image `ghcr.io/foundry-rs/foundry` |
| **OpenZeppelin Contracts** | audited ERC-20/721 base contracts (`forge install OpenZeppelin/openzeppelin-contracts`) | P5–P7 |
| **abigen** | `go install github.com/ethereum/go-ethereum/cmd/abigen@v1.17.5`; generate with `--v2` | P6–P7 |
| **solc** | the Solidity compiler (Foundry downloads it for you) | P6 |
| `geth --dev` | alternative single-node dev chain | optional |
| **Docker Compose** | multi-node and multi-service setups, as in `example-projects/` | all |
| **Postgres, Redis, RabbitMQ, Prometheus, Grafana** | same as your earlier projects | 4–7 |
| A browser wallet (MetaMask, Rabby…) | real SIWE and EIP-712 signing. **Use a fresh, test-only account** | P5 |
| Sepolia faucet + Sepolia Etherscan | free test ETH; inspect your transactions publicly | P4–P5 |

## 5.4 Networks

| Network | Chain ID | Use |
|---|---|---|
| **anvil** (local) | 31337 | everyday development, tests, reorg and fee-spike simulation, mainnet forking (`--fork-url`) |
| **Sepolia** | 11155111 | the Ethereum testnet for application developers; "real network" runs of P4/P5 |
| **Ethereum mainnet** | 1 | **read-only** in this track (P4 against a provider). Never put real funds in learning code. |
| Bitcoin **regtest / signet** | — | bonus 9 |

## 5.5 Deliberately avoided (and why)

- **Floats and `shopspring/decimal` for on-chain amounts.** Base-unit `*big.Int` is exact and matches the chain. Decimals are for fiat display only.
- **Keeping private keys in the API process** (beyond anvil). The architecture lesson of P7 is that it doesn't need them.
- **Reaching for a web3 "framework" early.** `ethclient` + your own port teaches you what's happening. Frameworks hide exactly the parts that break (reorgs, nonces).
- **Mainnet funds.** Everything money-moving runs on anvil or Sepolia.

---

# Part 6 — Real codebases to read

Reading production code is how the concepts become concrete. Each project lists the precise files;
here is the whole map.

| Codebase | Language | Why read it | Start with |
|---|---|---|---|
| **Jeiwan/blockchain_go** | Go | the classic minimal Bitcoin-like chain (BoltDB, PoW, UTXO, wallets) | `block.go`, `proofofwork.go`, `transaction.go`, `utxo_set.go` |
| **ethereum/go-ethereum** (geth) | Go | the most-used Ethereum execution client; you import it as a library | `core/types/`, `core/state_transition.go`, `core/txpool/legacypool/`, `ethclient/`, `rpc/`, `eth/filters/`, `accounts/abi/` |
| **btcsuite/btcd** | Go | a full Bitcoin node with a very readable structure | `blockchain/chain.go` (reorgs), `blockchain/difficulty.go`, `blockchain/merkle.go`, `netsync/`, `mempool/` |
| **libp2p/go-libp2p** + **go-libp2p-pubsub** | Go | the P2P stack behind Ethereum consensus clients, IPFS, Filecoin | `examples/`, `pubsub.go`, `gossipsub.go` |
| **OffchainLabs/prysm** | Go | an Ethereum consensus (proof-of-stake) client | `beacon-chain/p2p/`, `beacon-chain/forkchoice/` |
| **erigontech/erigon** | Go | an alternative execution client built for efficiency (staged sync) | its staged-sync docs, then search the repo for `stagedsync` (package paths move between releases) |
| **ethereum-optimism/optimism** | Go | the OP Stack L2; production chain services in Go | `op-service/txmgr/`, `op-node/` |
| **lightningnetwork/lnd** | Go | Lightning Network node; money-critical, reorg-aware Go | `chainntnfs/`, `sweep/` |
| **cometbft/cometbft** | Go | BFT consensus engine (Tendermint's successor) | `abci/types/`, `spec/` docs, then `internal/consensus/` |
| **cosmos/cosmos-sdk** | Go | app-chain framework | `x/bank/` (how a real token module is written) |
| **coinbase/mesh-sdk-go** | Go | a standard API model for exchange integrations | `types/` |
| **OpenZeppelin/openzeppelin-contracts** | Solidity | the reference token implementations your Go code talks to | `token/ERC20/ERC20.sol`, `utils/cryptography/SignatureChecker.sol` |

**How to read a big codebase** (you'll practise this in the walkthroughs): start from a test or an
RPC method you already understand, follow one call path end to end, and write down the 5–10 types that
matter. Ignore everything else on the first pass.

---

# Part 7 — Glossary

| Term | Meaning |
|---|---|
| **ABI** | contract calling convention: 4-byte selector + 32-byte encoded words |
| **ABCI** | interface between CometBFT consensus and your application state machine |
| **Account model** | state as `address → balance/nonce/code/storage` (Ethereum) |
| **Base fee** | protocol-set per-gas price, burned; moves ±12.5% per block |
| **BIP-32/39/44** | HD key derivation / mnemonics / derivation path convention |
| **Block tag** | `latest`, `safe`, `finalized`, `pending`, `earliest` |
| **Confirmations** | blocks on top of (and including) the one containing your tx |
| **EOA** | externally owned account, controlled by a private key |
| **EIP-55** | mixed-case checksum for addresses |
| **EIP-155** | chain ID in signatures (cross-chain replay protection) |
| **EIP-191** | `personal_sign` message prefix |
| **EIP-712** | typed structured data signing with a domain separator |
| **EIP-1271** | signature validation for contract wallets |
| **EIP-1559** | base fee + priority fee transaction pricing |
| **EIP-4361** | Sign-In with Ethereum message format |
| **Finality** | the point after which a block can't be reverted (probabilistic in PoW, economic in PoS, immediate in BFT) |
| **Fork choice** | the rule for picking the canonical chain among competing tips |
| **Gas** | unit of EVM computation |
| **GossipSub** | libp2p's pub/sub protocol for spreading messages across peers |
| **Hot / cold wallet** | online keys with a small float / offline keys with the bulk of funds |
| **Indexer** | service turning chain data (blocks, logs) into queryable databases |
| **Log / event** | data emitted by a contract, stored in receipts, filterable by topics |
| **Mempool / txpool** | a node's local set of valid, not-yet-mined transactions |
| **Merkle proof** | the sibling hashes proving a leaf belongs to a Merkle root |
| **Nonce (account)** | per-account tx counter; enforces order and prevents replay |
| **Nonce (PoW)** | the number miners vary to find a hash below the target |
| **Orphan / uncle** | a valid block that lost fork choice |
| **Receipt** | the result of a mined tx: status, gas used, logs |
| **Reorg** | switching the canonical chain to a different branch, undoing blocks |
| **RLP** | Recursive Length Prefix — Ethereum's execution-layer serialisation |
| **secp256k1** | the elliptic curve used by Bitcoin and Ethereum |
| **Selector** | first 4 bytes of `keccak256(functionSignature)` |
| **SIWE** | Sign-In with Ethereum |
| **Slashing** | destroying a PoS validator's stake for provable misbehaviour |
| **Sweeping** | consolidating funds from many deposit addresses into a treasury |
| **Topic** | an indexed log field; `topics[0]` is the event signature hash |
| **UTXO** | unspent transaction output (Bitcoin's coin model) |
| **Wei / gwei** | 10⁻¹⁸ ETH / 10⁻⁹ ETH |
| **xpub / xprv** | extended public / private key (BIP-32) |
