package service

import (
	"context"
	"errors"
	"math/big"
	"time"

	"p2pchain/internal/chain"
	"p2pchain/internal/domain"
	"p2pchain/internal/mempool"
	"p2pchain/internal/p2p"
)

// AccountView is an account plus the nonce a wallet should use next.
type AccountView struct {
	Address      domain.Address
	Account      domain.Account
	PendingNonce uint64
	TipHeight    uint64
}

func (n *Node) Account(addr domain.Address) AccountView {
	tip, _ := n.chain.Tip()
	state := n.chain.State()
	acct := state.Get(addr)
	return AccountView{Address: addr, Account: acct, PendingNonce: n.pool.PendingNonce(addr, acct.Nonce), TipHeight: tip.Height}
}

// TxStatus is where a transaction is in its life.
type TxStatus string

const (
	TxPending  TxStatus = "pending"
	TxQueued   TxStatus = "queued"
	TxIncluded TxStatus = "included"
)

// TxView is a transaction and what this node knows about it.
type TxView struct {
	Tx            domain.Tx
	Hash          domain.Hash
	Sender        domain.Address
	Status        TxStatus
	BlockHash     domain.Hash
	BlockHeight   uint64
	Index         int
	Confirmations uint64
}

// Tx finds a transaction on the canonical chain or in the mempool.
//
// "Included" is always relative to THIS node's canonical chain: a reorg can
// move a transaction back to pending. That is why confirmations matter.
func (n *Node) Tx(hash domain.Hash) (TxView, error) {
	tip, _ := n.chain.Tip()
	if blockHash, index, err := n.chain.TxLocation(hash); err == nil {
		b, err := n.chain.BlockByHash(blockHash)
		if err != nil {
			return TxView{}, err
		}
		tx := b.Txs[index]
		sender, _ := tx.Sender()
		return TxView{
			Tx: tx, Hash: hash, Sender: sender, Status: TxIncluded, BlockHash: blockHash,
			BlockHeight: b.Height, Index: index, Confirmations: tip.Height - b.Height + 1,
		}, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return TxView{}, err
	}

	state := n.chain.State()
	if e, status, ok := n.pool.Get(hash, func(a domain.Address) uint64 { return state.Get(a).Nonce }); ok {
		return TxView{Tx: e.Tx, Hash: hash, Sender: e.Sender, Status: TxStatus(status)}, nil
	}
	return TxView{}, domain.ErrNotFound
}

func (n *Node) Latest() domain.Block { b, _ := n.chain.Tip(); return b }

func (n *Node) Block(height uint64) (domain.Block, error) { return n.chain.BlockByHeight(height) }

func (n *Node) Blocks(from uint64, limit int) ([]domain.Block, error) {
	out := make([]domain.Block, 0, limit)
	for height := from; len(out) < limit; height++ {
		b, err := n.chain.BlockByHeight(height)
		if errors.Is(err, domain.ErrNotFound) {
			break
		} else if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

func (n *Node) Mempool() (pending, queued []mempool.Entry) {
	state := n.chain.State()
	return n.pool.Snapshot(func(a domain.Address) uint64 { return state.Get(a).Nonce })
}

func (n *Node) Forks() ([]chain.TipInfo, error) { return n.chain.Tips() }

// NodeStatus is the "where am I" view: chain, peers, miner, mempool.
type NodeStatus struct {
	PeerID    string
	Addresses []string
	Height    uint64
	TipHash   domain.Hash
	Work      *big.Int
	Peers     []p2p.PeerInfo
	Gossiping int
	Syncing   bool
	Orphans   int
	Pending   int
	Queued    int
	Threads   int
	Mined     uint64
	Hashes    uint64
	Miner     domain.Address
	ChainID   uint64
}

func (n *Node) NodeStatus() NodeStatus {
	tip, meta := n.chain.Tip()
	pending, queued := n.Mempool()
	st := NodeStatus{
		Height: tip.Height, TipHash: tip.Hash, Work: meta.Work, Syncing: n.syncing.Load(),
		Orphans: n.orphans.Len(), Pending: len(pending), Queued: len(queued), ChainID: n.params.ChainID,
	}
	if net := n.net.Load(); net != nil {
		st.PeerID, st.Addresses, st.Peers, st.Gossiping = net.ID().String(), net.Addrs(), net.Peers(), net.GossipPeers()
	}
	if m := n.miner.Load(); m != nil {
		st.Threads, st.Mined, st.Hashes, st.Miner = m.Threads(), m.Mined(), m.Hashes(), m.Coinbase()
	}
	return st
}

// ValidationReport is the result of replaying the canonical chain.
type ValidationReport struct {
	Valid          bool
	Blocks         uint64
	Invalid        *domain.InvalidBlockError
	StateRoot      domain.Hash
	StateMatches   bool
	TotalSupply    uint64
	ExpectedSupply uint64
	Took           time.Duration
}

// Validate replays every canonical block from genesis, as a fresh node would.
func (n *Node) Validate(ctx context.Context) (ValidationReport, error) {
	start := time.Now()
	tip, _ := n.chain.Tip()
	live := n.chain.State()
	rep := ValidationReport{Blocks: tip.Height + 1}

	replayed, err := n.chain.Replay()
	if err != nil {
		if !errors.As(err, &rep.Invalid) {
			return rep, err
		}
		rep.Took = time.Since(start)
		return rep, nil
	}
	rep.StateRoot = replayed.Root()
	rep.StateMatches = rep.StateRoot == live.Root()
	rep.TotalSupply, _ = replayed.TotalSupply()
	rep.ExpectedSupply, _ = domain.ExpectedSupply(n.params, tip.Height)
	rep.Valid = rep.StateMatches && rep.TotalSupply == rep.ExpectedSupply
	rep.Took = time.Since(start)
	return rep, nil
}

// MineOne mines a single block on demand.
func (n *Node) MineOne(ctx context.Context) (domain.Block, error) {
	m := n.miner.Load()
	if m == nil {
		return domain.Block{}, errors.New("miner is not running")
	}
	return m.MineOne(ctx)
}
