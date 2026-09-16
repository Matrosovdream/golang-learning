// Package service wires the chain, the mempool, the miner, and the network
// into one node, and decides what happens when blocks and transactions arrive.
package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/big"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"p2pchain/internal/chain"
	"p2pchain/internal/domain"
	"p2pchain/internal/events"
	"p2pchain/internal/mempool"
	"p2pchain/internal/miner"
	"p2pchain/internal/p2p"
)

// maxAncestorFetches bounds how far back we chase an orphan's ancestors before
// giving up and doing a normal locator sync.
const maxAncestorFetches = 16

// Node is one participant in the network.
type Node struct {
	params domain.Params
	chain  *chain.Chain
	pool   *mempool.Pool
	bus    *events.Bus

	net   atomic.Pointer[p2p.Node] // set once networking is up
	miner atomic.Pointer[miner.Miner]

	orphans *orphanPool
	syncing atomic.Bool
	ctx     context.Context
}

func NewNode(ctx context.Context, params domain.Params, c *chain.Chain, pool *mempool.Pool, bus *events.Bus) *Node {
	return &Node{params: params, chain: c, pool: pool, bus: bus, orphans: newOrphanPool(64), ctx: ctx}
}

func (n *Node) AttachNetwork(net *p2p.Node) { n.net.Store(net) }
func (n *Node) AttachMiner(m *miner.Miner)  { n.miner.Store(m) }
func (n *Node) Chain() *chain.Chain         { return n.chain }
func (n *Node) Miner() *miner.Miner         { return n.miner.Load() }
func (n *Node) Network() *p2p.Node          { return n.net.Load() }

// --- transactions --------------------------------------------------------------

// SubmitResult reports what happened to a submitted transaction.
type SubmitResult struct {
	Hash   domain.Hash
	Sender domain.Address
	Status mempool.Status
}

// SubmitTx validates a raw transaction, adds it to our mempool, and gossips it.
func (n *Node) SubmitTx(raw []byte) (SubmitResult, error) {
	res, err := n.acceptTx(raw)
	if err != nil {
		return res, err
	}
	if net := n.net.Load(); net != nil {
		if err := net.BroadcastTx(n.ctx, raw); err != nil {
			log.Printf("broadcast tx: %v", err)
		}
	}
	return res, nil
}

// acceptTx runs the checks and adds to the pool, without gossiping.
func (n *Node) acceptTx(raw []byte) (SubmitResult, error) {
	tx, err := domain.DecodeTx(raw)
	if err != nil {
		return SubmitResult{}, err
	}
	if tx.ChainID != n.params.ChainID {
		return SubmitResult{}, fmt.Errorf("%w: tx is for chain %d, this is chain %d", domain.ErrWrongChainID, tx.ChainID, n.params.ChainID)
	}
	sender, err := tx.Sender()
	if err != nil {
		return SubmitResult{}, err
	}
	if tx.Fee < n.params.MinFee {
		return SubmitResult{}, fmt.Errorf("%w: fee %d, minimum %d", domain.ErrFeeTooLow, tx.Fee, n.params.MinFee)
	}
	hash := tx.Hash()
	if _, _, err := n.chain.TxLocation(hash); err == nil {
		return SubmitResult{}, fmt.Errorf("%w: already mined", mempool.ErrAlreadyKnown)
	}
	state := n.chain.State()
	status, err := n.pool.Add(tx, sender, state.Get(sender))
	if err != nil {
		return SubmitResult{}, err
	}
	n.bus.Publish(events.TxAdded, map[string]any{"hash": hash.Hex(), "from": sender.Hex(), "status": string(status)})
	return SubmitResult{Hash: hash, Sender: sender, Status: status}, nil
}

// --- p2p.Handler ---------------------------------------------------------------

// ValidateBlockForGossip decides whether a block is worth relaying. Only
// stateless rules: a block on a branch we can't execute yet is still valid to
// forward, and blaming the sender for it would punish honest peers.
func (n *Node) ValidateBlockForGossip(b domain.Block) error { return n.chain.CheckStateless(b) }

// ValidateTxForGossip does the same for transactions: signature, chain, fee.
// Nonce and balance are NOT checked here — they depend on our current state,
// which may legitimately differ from the sender's.
func (n *Node) ValidateTxForGossip(raw []byte) error {
	tx, err := domain.DecodeTx(raw)
	if err != nil {
		return err
	}
	if tx.ChainID != n.params.ChainID {
		return domain.ErrWrongChainID
	}
	if tx.Fee < n.params.MinFee {
		return domain.ErrFeeTooLow
	}
	_, err = tx.Sender()
	return err
}

// OnBlock handles a gossiped block.
func (n *Node) OnBlock(from peer.ID, b domain.Block) {
	res, err := n.chain.AddBlock(b)
	if err != nil {
		log.Printf("block %s from %s rejected: %v", b.Hash.Hex()[:12], from.ShortString(), err)
		return
	}
	switch res.Status {
	case chain.StatusOrphan:
		// We are missing its ancestors: park it and go get them.
		n.orphans.Add(b)
		go n.fetchAncestors(from, b)
	case chain.StatusExtended:
		log.Printf("block %d %s from %s (%d txs)", b.Height, b.Hash.Hex()[:12], from.ShortString(), len(b.Txs))
		n.connectOrphansOf(b.Hash)
	case chain.StatusReorg:
		n.connectOrphansOf(b.Hash) // the chain logs the switch itself
	case chain.StatusSide:
		log.Printf("side block %d %s from %s", b.Height, b.Hash.Hex()[:12], from.ShortString())
		n.connectOrphansOf(b.Hash)
	}
}

// OnTx handles a gossiped transaction. GossipSub already forwarded it to our
// peers when the validator accepted it, so we only add it to our pool.
func (n *Node) OnTx(from peer.ID, raw []byte) {
	if _, err := n.acceptTx(raw); err != nil && !errors.Is(err, mempool.ErrAlreadyKnown) {
		log.Printf("tx from %s rejected: %v", from.ShortString(), err)
	}
}

// Status describes our chain for a peer.
func (n *Node) Status() p2p.Status {
	tip, meta := n.chain.Tip()
	genesis, _ := n.chain.BlockByHeight(0)
	return p2p.Status{Genesis: genesis.Hash.Hex(), Tip: tip.Hash.Hex(), Height: tip.Height, Work: meta.Work.String()}
}

func (n *Node) BlocksAfter(locator []domain.Hash, max int) ([]domain.Block, error) {
	return n.chain.BlocksAfter(locator, max)
}

func (n *Node) BlockByHash(h domain.Hash) (domain.Block, bool) {
	b, err := n.chain.BlockByHash(h)
	return b, err == nil
}

// OnPeerStatus reacts to a peer telling us where its chain is. More work than
// ours means we are behind — pull the missing blocks.
func (n *Node) OnPeerStatus(p peer.ID, s p2p.Status) {
	genesis, _ := n.chain.BlockByHeight(0)
	if s.Genesis != genesis.Hash.Hex() {
		log.Printf("peer %s is on a different chain (genesis %s) — ignoring", p.ShortString(), short(s.Genesis))
		return
	}
	theirs, ok := new(big.Int).SetString(s.Work, 10)
	if !ok {
		return
	}
	_, meta := n.chain.Tip()
	if domain.MoreWork(theirs, meta.Work) {
		go n.SyncFrom(p)
	}
}

// --- syncing --------------------------------------------------------------------

// SyncFrom pulls blocks from a peer until we stop making progress.
//
// Each round sends a locator ("here is my chain, newest first") and receives
// the blocks after the newest block we share. That works whether we are simply
// behind or on a different branch entirely.
func (n *Node) SyncFrom(p peer.ID) {
	net := n.net.Load()
	if net == nil || !n.syncing.CompareAndSwap(false, true) {
		return // one sync at a time
	}
	defer n.syncing.Store(false)

	n.bus.Publish(events.SyncStarted, map[string]any{"peer": p.String()})
	added := 0
	for round := 0; round < 1000; round++ {
		blocks, err := net.RequestBlocks(n.ctx, p, n.chain.Locator(), 64)
		if err != nil {
			log.Printf("sync from %s: %v", p.ShortString(), err)
			break
		}
		progress := false
		for _, b := range blocks {
			res, err := n.chain.AddBlock(b)
			if err != nil {
				log.Printf("sync: peer %s sent an invalid block: %v", p.ShortString(), err)
				n.bus.Publish(events.SyncFinished, map[string]any{"peer": p.String(), "added": added, "error": err.Error()})
				return
			}
			if res.Status != chain.StatusKnown {
				progress, added = true, added+1
				n.connectOrphansOf(b.Hash)
			}
		}
		if !progress {
			break
		}
	}
	n.bus.Publish(events.SyncFinished, map[string]any{"peer": p.String(), "added": added})
	if added > 0 {
		tip, _ := n.chain.Tip()
		log.Printf("synced %d blocks from %s, now at height %d", added, p.ShortString(), tip.Height)
	}
}

// fetchAncestors walks backwards from an orphan, asking the peer for each
// missing parent, until the chain connects to something we know.
func (n *Node) fetchAncestors(from peer.ID, orphan domain.Block) {
	net := n.net.Load()
	if net == nil {
		return
	}
	missing := orphan.PrevHash
	for range maxAncestorFetches {
		if have, _ := n.chain.HasBlock(missing); have {
			n.connectOrphansOf(missing)
			return
		}
		ctx, cancel := context.WithTimeout(n.ctx, 10*time.Second)
		parent, err := net.RequestBlock(ctx, from, missing)
		cancel()
		if err != nil {
			break
		}
		res, err := n.chain.AddBlock(parent)
		if err != nil {
			log.Printf("ancestor %s invalid: %v", missing.Hex()[:12], err)
			return
		}
		if res.Status == chain.StatusOrphan {
			n.orphans.Add(parent)
			missing = parent.PrevHash
			continue
		}
		n.connectOrphansOf(parent.Hash)
		return
	}
	n.SyncFrom(from) // fall back to a normal locator sync
}

// connectOrphansOf connects blocks that were waiting for this one, and any
// blocks waiting for those, and so on.
func (n *Node) connectOrphansOf(parent domain.Hash) {
	queue := []domain.Hash{parent}
	for len(queue) > 0 {
		h := queue[0]
		queue = queue[1:]
		for _, child := range n.orphans.Take(h) {
			if _, err := n.chain.AddBlock(child); err != nil {
				continue
			}
			queue = append(queue, child.Hash)
		}
	}
}

// RunPeerSync periodically asks every peer where its chain is. Gossip can be
// missed (a partition, a restart, a dropped message); polling is the safety net
// that eventually brings every node back to the heaviest chain.
func (n *Node) RunPeerSync(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			net := n.net.Load()
			if net == nil {
				continue
			}
			for _, info := range net.Peers() {
				id, err := peer.Decode(info.ID)
				if err != nil {
					continue
				}
				askCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				st, err := net.RequestStatus(askCtx, id)
				cancel()
				if err == nil {
					n.OnPeerStatus(id, st)
				}
			}
		}
	}
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
