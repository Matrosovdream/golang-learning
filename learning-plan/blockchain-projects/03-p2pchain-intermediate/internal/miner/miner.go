// Package miner turns mempool transactions into blocks by doing proof-of-work.
package miner

import (
	"context"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"p2pchain/internal/chain"
	"p2pchain/internal/domain"
	"p2pchain/internal/events"
	"p2pchain/internal/mempool"
)

// Miner mines on top of the current tip and restarts whenever the tip changes.
//
// Restarting matters: the moment a peer's block arrives, everything we were
// mining is worthless — it points at the wrong parent. Real miners switch the
// same way, and the wasted work between "a block was found" and "we heard about
// it" is what the network's propagation delay costs miners.
type Miner struct {
	chain    *chain.Chain
	pool     *mempool.Pool
	bus      *events.Bus
	coinbase domain.Address
	onBlock  func(domain.Block) // announce a block we mined

	threads atomic.Int64
	mined   atomic.Uint64
	hashes  atomic.Uint64
	now     func() time.Time

	mu sync.Mutex // one mining round at a time
}

func New(c *chain.Chain, pool *mempool.Pool, bus *events.Bus, coinbase domain.Address, threads int, onBlock func(domain.Block)) *Miner {
	m := &Miner{chain: c, pool: pool, bus: bus, coinbase: coinbase, onBlock: onBlock, now: time.Now}
	m.threads.Store(int64(threads))
	return m
}

func (m *Miner) Threads() int             { return int(m.threads.Load()) }
func (m *Miner) SetThreads(n int)         { m.threads.Store(int64(max(n, 0))) }
func (m *Miner) Mined() uint64            { return m.mined.Load() }
func (m *Miner) Hashes() uint64           { return m.hashes.Load() }
func (m *Miner) Coinbase() domain.Address { return m.coinbase }

// Run mines continuously until ctx ends. With 0 threads it idles, so mining can
// be turned on and off at runtime (POST /debug/miner).
func (m *Miner) Run(ctx context.Context) {
	sub, unsubscribe := m.bus.Subscribe(64)
	defer unsubscribe()

	for ctx.Err() == nil {
		threads := m.Threads()
		if threads == 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(250 * time.Millisecond):
			}
			continue
		}

		// Drop events from before this round: only a NEW tip should interrupt us.
		drain(sub)

		roundCtx, cancelRound := context.WithCancel(ctx)
		done := make(chan struct{})
		// Watchdog: a new tip makes this round pointless. It only READS `done`;
		// the mining loop below is the single owner that closes it.
		go func() {
			for {
				select {
				case <-done:
					return
				case <-roundCtx.Done():
					return
				case ev := <-sub:
					if ev.Kind == events.BlockAdded || ev.Kind == events.Reorg {
						cancelRound()
						return
					}
				}
			}
		}()

		b, err := m.mineRound(roundCtx, threads)
		close(done)
		cancelRound()

		switch {
		case err == nil:
			m.submit(b)
		case errors.Is(err, context.Canceled) && ctx.Err() == nil:
			// Someone else won the race; build a new candidate on the new tip.
		case ctx.Err() != nil:
			return
		default:
			log.Printf("miner: %v", err)
			time.Sleep(200 * time.Millisecond)
		}
	}
}

// MineOne mines a single block on demand (POST /mine and tests), even when
// continuous mining is off.
func (m *Miner) MineOne(ctx context.Context) (domain.Block, error) {
	b, err := m.mineRound(ctx, max(m.Threads(), 1))
	if err != nil {
		return domain.Block{}, err
	}
	m.submit(b)
	return b, nil
}

// submit adds our own block to our own chain first (it must pass the same
// checks as anyone else's), and only then announces it.
func (m *Miner) submit(b domain.Block) {
	res, err := m.chain.AddBlock(b)
	if err != nil {
		log.Printf("miner: own block rejected: %v", err) // a bug in our block building
		return
	}
	m.mined.Add(1)
	if res.Status == chain.StatusExtended || res.Status == chain.StatusReorg {
		m.onBlock(b)
	}
}

// mineRound builds a candidate block from the tip and searches for a nonce.
func (m *Miner) mineRound(ctx context.Context, threads int) (domain.Block, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	params := m.chain.Params()
	parent, _ := m.chain.Tip()
	parentState := m.chain.State()

	// Pick transactions, trial-applying each: the pool checked them one by one,
	// but together they may not fit (see coinledger).
	scratch := parentState.Clone()
	var included []domain.Tx
	var unaffordable []domain.Hash
	for _, e := range m.pool.Select(params.MaxBlockTxs, func(a domain.Address) uint64 { return parentState.Get(a).Nonce }) {
		if _, err := domain.ApplyTx(params, scratch, e.Tx, m.coinbase); err != nil {
			if errors.Is(err, domain.ErrInsufficientFunds) {
				unaffordable = append(unaffordable, e.Hash)
			}
			continue
		}
		included = append(included, e.Tx)
	}
	m.pool.Remove(unaffordable...)

	// Execute with the same function every validator will run, then commit to
	// the result in the header.
	draft := domain.Block{Header: domain.Header{Miner: m.coinbase}, Txs: included}
	post, err := domain.ApplyBlock(params, parentState, draft)
	if err != nil {
		return domain.Block{}, err
	}
	header := domain.Header{
		Height:     parent.Height + 1,
		Timestamp:  max(m.now().UnixMilli(), parent.Timestamp),
		PrevHash:   parent.Hash,
		TxRoot:     domain.ComputeTxRoot(included),
		StateRoot:  post.Root(),
		Miner:      m.coinbase,
		Difficulty: params.Difficulty,
	}

	return m.search(ctx, header, included, threads)
}

// search runs `threads` goroutines over disjoint nonce ranges. The first one to
// find a solution cancels the others — a miniature mining pool.
func (m *Miner) search(ctx context.Context, header domain.Header, txs []domain.Tx, threads int) (domain.Block, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		header domain.Header
		err    error
	}
	results := make(chan result, threads)
	start := time.Now()

	var wg sync.WaitGroup
	for i := range threads {
		wg.Go(func() {
			// Thread i tries nonces i, i+threads, i+2·threads, … — no overlap,
			// no coordination needed.
			mined, tried, err := domain.MineFrom(ctx, header, uint64(i), uint64(threads))
			m.hashes.Add(tried)
			results <- result{mined, err}
		})
	}
	go func() { wg.Wait(); close(results) }()

	for res := range results {
		if res.err == nil {
			cancel() // stop the other threads
			return domain.Block{
				Header: res.header, Txs: txs, Hash: res.header.ComputeHash(),
				MinedIn: time.Since(start),
			}, nil
		}
	}
	return domain.Block{}, ctx.Err()
}

func drain[T any](ch <-chan T) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}
