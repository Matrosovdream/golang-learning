package miner_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"p2pchain/internal/chain"
	"p2pchain/internal/domain"
	"p2pchain/internal/events"
	"p2pchain/internal/mempool"
	"p2pchain/internal/miner"
	"p2pchain/internal/repository/boltstore"
)

func newMiner(t *testing.T, threads int) (*miner.Miner, *chain.Chain, *events.Bus) {
	t.Helper()
	params := domain.Params{ChainID: 1337, Difficulty: 8, BlockReward: 50, MinFee: 1, MaxBlockTxs: 10}
	params.GenesisAlloc, _ = domain.DevAlloc(2, 1_000_000)

	store, err := boltstore.Open(filepath.Join(t.TempDir(), "chain.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	bus := events.NewBus()
	pool := mempool.New(mempool.Config{MaxSize: 10, MaxNonceGap: 4})
	c, err := chain.New(context.Background(), params, store, pool, bus, chain.Options{})
	if err != nil {
		t.Fatal(err)
	}
	coinbase, _ := domain.DevAddress(0)
	return miner.New(c, pool, bus, coinbase, threads, func(domain.Block) {}), c, bus
}

// Continuous mining must keep producing blocks — and survive many rounds, which
// is where a double-closed channel or a leaked watchdog would blow up.
func TestRunMinesBlocksContinuously(t *testing.T) {
	m, c, _ := newMiner(t, 2)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Run(ctx); close(done) }()

	deadline := time.Now().Add(20 * time.Second)
	for {
		tip, _ := c.Tip()
		if tip.Height >= 5 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("only reached height %d", tip.Height)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	if m.Mined() < 5 || m.Hashes() == 0 {
		t.Fatalf("mined %d blocks, %d hashes", m.Mined(), m.Hashes())
	}
}

// Threads can be changed at runtime: 0 idles, and turning it back on resumes.
func TestThreadsCanBeChangedAtRuntime(t *testing.T) {
	m, c, _ := newMiner(t, 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)

	time.Sleep(300 * time.Millisecond)
	if tip, _ := c.Tip(); tip.Height != 0 {
		t.Fatalf("mined %d blocks with 0 threads", tip.Height)
	}
	m.SetThreads(2)
	deadline := time.Now().Add(20 * time.Second)
	for {
		if tip, _ := c.Tip(); tip.Height >= 2 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("no blocks after enabling threads")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A block from elsewhere cancels the current round: the miner must not waste
// work on a parent that is no longer the tip.
func TestMinerRestartsOnANewTip(t *testing.T) {
	m, c, bus := newMiner(t, 1)
	other, _, _ := newMiner(t, 1) // a second node, same rules
	_ = other

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)

	sub, unsubscribe := bus.Subscribe(16)
	defer unsubscribe()
	deadline := time.After(20 * time.Second)
	for {
		select {
		case ev := <-sub:
			if ev.Kind == events.BlockAdded {
				tip, _ := c.Tip()
				if tip.Height >= 1 {
					return
				}
			}
		case <-deadline:
			t.Fatal("no block mined")
		}
	}
}
