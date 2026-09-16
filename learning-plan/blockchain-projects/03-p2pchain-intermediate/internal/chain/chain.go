// Package chain owns the block tree: which blocks exist, which branch is
// canonical, and what happens when that changes.
package chain

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"p2pchain/internal/domain"
	"p2pchain/internal/events"
	"p2pchain/internal/mempool"
)

// maxFutureDrift is how far ahead of our clock a block's timestamp may be.
// Without a limit, a miner could claim a far-future timestamp; with one that is
// too tight, honest nodes reject each other over clock skew.
const maxFutureDrift = 2 * time.Minute

// Status is what happened to a block we were offered.
type Status string

const (
	StatusKnown    Status = "known"    // already in the tree
	StatusExtended Status = "extended" // new tip, directly on top of the old one
	StatusReorg    Status = "reorg"    // new tip on another branch: the chain switched
	StatusSide     Status = "side"     // valid, stored, but not enough work to win
	StatusOrphan   Status = "orphan"   // parent unknown — we must fetch it first
)

// AddResult reports what a block did to the chain.
type AddResult struct {
	Status      Status
	Hash        domain.Hash
	Height      uint64
	ReorgDepth  int         // blocks disconnected (0 unless Status is reorg)
	OldTip      domain.Hash //
	ReturnedTxs int         // transactions pushed back into the mempool by a reorg
}

// Chain is the block tree plus the canonical branch's state.
type Chain struct {
	params domain.Params
	store  domain.ChainStore
	pool   *mempool.Pool
	bus    *events.Bus

	mu      sync.RWMutex
	tip     domain.Block
	tipMeta domain.BlockMeta
	state   domain.State
}

// Options are startup choices.
type Options struct {
	RebuildState bool
}

// New opens the chain: creates genesis on an empty store, or loads the tip.
func New(ctx context.Context, p domain.Params, store domain.ChainStore, pool *mempool.Pool, bus *events.Bus, opts Options) (*Chain, error) {
	c := &Chain{params: p, store: store, pool: pool, bus: bus}

	genesis, genesisState, err := domain.NewGenesis(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("genesis: %w", err)
	}
	empty, err := store.IsEmpty()
	if err != nil {
		return nil, err
	}

	if empty {
		if err := store.SetParamsFingerprint(p.Fingerprint()); err != nil {
			return nil, err
		}
		meta := domain.BlockMeta{Height: 0, Work: domain.BlockWork(p.Difficulty)}
		if err := store.PutBlock(genesis, meta); err != nil {
			return nil, err
		}
		update := domain.BlockUpdate{
			Block: genesis, Meta: meta,
			Forward:  domain.Diff(domain.State{}, genesisState),
			Backward: domain.Diff(genesisState, domain.State{}),
		}
		if err := store.Switch(nil, []domain.BlockUpdate{update}); err != nil {
			return nil, err
		}
		c.tip, c.tipMeta, c.state = genesis, meta, genesisState
		return c, nil
	}

	if fp, err := store.ParamsFingerprint(); err != nil || fp != p.Fingerprint() {
		return nil, fmt.Errorf("%w (chain id, difficulty, reward, min fee, max block txs, or genesis alloc changed); use a new DATA_DIR",
			domain.ErrParamsMismatch)
	}
	stored, err := store.CanonicalHash(0)
	if err != nil {
		return nil, err
	}
	if stored != genesis.Hash {
		return nil, fmt.Errorf("%w: stored %s, configured %s", domain.ErrGenesisMismatch, stored.Hex(), genesis.Hash.Hex())
	}

	tipHash, err := store.TipHash()
	if err != nil {
		return nil, err
	}
	if c.tip, err = store.BlockByHash(tipHash); err != nil {
		return nil, err
	}
	if c.tipMeta, err = store.Meta(tipHash); err != nil {
		return nil, err
	}

	if opts.RebuildState {
		start := time.Now()
		replayed, err := c.Replay()
		if err != nil {
			return nil, fmt.Errorf("rebuild state: %w", err)
		}
		if err := store.ReplaceState(replayed); err != nil {
			return nil, err
		}
		log.Printf("state rebuilt by replaying %d blocks in %s", c.tip.Height+1, time.Since(start).Round(time.Millisecond))
	}
	if c.state, err = store.LoadState(); err != nil {
		return nil, err
	}
	if root := c.state.Root(); root != c.tip.StateRoot {
		return nil, fmt.Errorf("stored state root %s does not match tip state root %s (restart with REBUILD_STATE=true)",
			root.Hex(), c.tip.StateRoot.Hex())
	}
	return c, nil
}

// Replay re-executes the canonical chain from genesis and returns the final
// state. The account state is derived data: the blocks alone rebuild it.
func (c *Chain) Replay() (domain.State, error) {
	var parent *domain.Block
	var state domain.State
	for height := uint64(0); ; height++ {
		hash, err := c.store.CanonicalHash(height)
		if errors.Is(err, domain.ErrNotFound) {
			return state, nil
		} else if err != nil {
			return nil, err
		}
		b, err := c.store.BlockByHash(hash)
		if err != nil {
			return nil, err
		}
		next, err := domain.ValidateBlock(c.params, parent, state, b)
		if err != nil {
			return nil, err
		}
		parent, state = &b, next
	}
}

// --- reads --------------------------------------------------------------------

// Tip returns the canonical tip and its cumulative work.
func (c *Chain) Tip() (domain.Block, domain.BlockMeta) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tip, c.tipMeta
}

// State returns the state at the tip. The map is never mutated after being
// installed, so the caller may keep using it without holding the lock.
func (c *Chain) State() domain.State {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *Chain) Params() domain.Params { return c.params }

func (c *Chain) BlockByHash(h domain.Hash) (domain.Block, error) { return c.store.BlockByHash(h) }

func (c *Chain) Meta(h domain.Hash) (domain.BlockMeta, error) { return c.store.Meta(h) }

// BlockByHeight returns the canonical block at a height.
func (c *Chain) BlockByHeight(height uint64) (domain.Block, error) {
	hash, err := c.store.CanonicalHash(height)
	if err != nil {
		return domain.Block{}, err
	}
	return c.store.BlockByHash(hash)
}

// TxLocation finds a mined transaction on the canonical chain.
func (c *Chain) TxLocation(hash domain.Hash) (domain.Hash, int, error) {
	return c.store.TxLocation(hash)
}

func (c *Chain) HasBlock(h domain.Hash) (bool, error) { return c.store.HasBlock(h) }

// TipInfo describes one leaf of the block tree.
type TipInfo struct {
	Hash      domain.Hash
	Height    uint64
	Work      domain.Work
	Canonical bool
}

// Tips lists every branch tip the node knows — the forks it has seen.
func (c *Chain) Tips() ([]TipInfo, error) {
	hashes, err := c.store.Tips()
	if err != nil {
		return nil, err
	}
	best, _ := c.Tip()
	out := make([]TipInfo, 0, len(hashes))
	for _, h := range hashes {
		m, err := c.store.Meta(h)
		if err != nil {
			return nil, err
		}
		out = append(out, TipInfo{Hash: h, Height: m.Height, Work: m.Work, Canonical: h == best.Hash})
	}
	return out, nil
}

// Locator is a compact description of "where I am" for a peer: canonical block
// hashes at exponentially increasing distance from the tip, ending at genesis.
//
// It costs ~log₂(height) hashes and lets a peer find our last common block even
// if we are on different branches. Bitcoin's block locator, same idea.
func (c *Chain) Locator() []domain.Hash {
	tip, _ := c.Tip()
	var out []domain.Hash
	step := uint64(1)
	for height, added := int64(tip.Height), 0; height >= 0; added++ {
		hash, err := c.store.CanonicalHash(uint64(height))
		if err != nil {
			break
		}
		out = append(out, hash)
		if added >= 10 { // the first 10 are dense, then double the gap
			step *= 2
		}
		height -= int64(step)
	}
	genesis := mustGenesis(c)
	if len(out) == 0 || out[len(out)-1] != genesis {
		out = append(out, genesis) // always end at genesis: the one block everyone shares
	}
	return out
}

func mustGenesis(c *Chain) domain.Hash {
	h, _ := c.store.CanonicalHash(0)
	return h
}

// BlocksAfter answers a peer's locator: it finds the newest block we share and
// returns the canonical blocks after it, so the peer can catch up.
func (c *Chain) BlocksAfter(locator []domain.Hash, max int) ([]domain.Block, error) {
	start := uint64(0)
	for _, h := range locator { // the locator is ordered newest first
		meta, err := c.store.Meta(h)
		if err != nil {
			continue // we don't know this block at all
		}
		canonical, err := c.store.CanonicalHash(meta.Height)
		if err == nil && canonical == h {
			start = meta.Height + 1
			break
		}
	}
	out := make([]domain.Block, 0, max)
	for height := start; len(out) < max; height++ {
		b, err := c.BlockByHeight(height)
		if errors.Is(err, domain.ErrNotFound) {
			break
		} else if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}
