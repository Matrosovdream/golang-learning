# Blockchain Projects Track · Progress

Plan & theory: [README.md](README.md). Each project gets its own `progress.md` (phases tagged
📋 Copy · 🔁 Repeat · 🧠 Rebuild) once Claude has built and verified the reference copy.
Reference copies live here (`NN-name/`); your rebuilds live at `projects/<name>/` (repo root).

> ▶ **Resume here:** **01-hashchain-beginner** reference built & verified (2026-09-14).
> Walkthrough **in progress** — current: Phase 2 (`Block` + canonical encoding) → see [01-hashchain-beginner/progress.md](01-hashchain-beginner/progress.md).

| # | Project | Level | Reference built | Walkthrough | Notes |
|---|---|---|---|---|---|
| 1 | [hashchain](01-hashchain-beginner/README.md) | 🟢 | done | in progress | stdlib only; `go test -race` green, live-run verified; Docker files not built (no Docker on build machine) |
| 2 | coinledger | 🟢+ | not started | not started | go-ethereum `crypto`/`rlp`, bbolt |
| 3 | p2pchain | 🟡 | not started | not started | go-libp2p + GossipSub, 5-node compose |
| 4 | eth-gateway | 🟡 | not started | not started | ethclient, anvil, Sepolia |
| 5 | web3-auth | 🟡 | not started | not started | SIWE, EIP-712, EIP-1271 |
| 6 | token-indexer | 🔴 | not started | not started | Foundry, abigen v2, Postgres, reorgs |
| 7 | crypto-payments | 🟣 | not started | not started | HD wallets, ledger, txmgr, signer, webhooks |

**Status legend:** `not started` · `in progress` · `done`

## 🧠 Rebuild-from-memory scoreboard

Tick when you've written it from memory and it matched the reference (small diffs are fine).

- [ ] P1 `Header.Encode` + `ComputeHash` (canonical encoding)
- [ ] P1 `LeadingZeroBits` + `Mine` (proof-of-work)
- [ ] P2 `Tx.Sender` (signature recovery)
- [ ] P2 `ApplyTx` (state transition)
- [ ] P2 `VerifyProof` (Merkle)
- [ ] P3 fork choice + reorg
- [ ] P4 `FormatUnits` / `ParseUnits`
- [ ] P5 SIWE verify use case
- [ ] P6 block processor (one DB tx)
- [ ] P6 reorg detection & rollback
- [ ] P7 xpub address derivation
- [ ] P7 ledger invariants
- [ ] P7 tx manager (nonce + bump)

## Log

- 2026-09-14 — track planned: 7 projects + 3 bonus, theory (Parts 1–3), library catalogue verified on the Go proxy, all doc snippets compiled against go-ethereum v1.17.5.
- 2026-09-14 — built 01-hashchain-beginner: PoW chain, 7-rule validation, 4 tamper modes, retargeting (measured 16→24 bits, oscillates 24↔25 at 20 MH/s), auto-miner, graceful shutdown. Found & fixed a uint64 underflow in /stats during the live run.
- 2026-09-15 — hashchain rewrite started into `projects/hashchain/`: Phase 1 (scaffold + `Hash`), files handed over with blockchain comments inside.

---
*Global progress: [../PROGRESS.md](../PROGRESS.md).*
