package service_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"p2pchain/internal/chain"
	"p2pchain/internal/domain"
	"p2pchain/internal/events"
	"p2pchain/internal/mempool"
	"p2pchain/internal/miner"
	"p2pchain/internal/p2p"
	"p2pchain/internal/repository/boltstore"
	"p2pchain/internal/service"
)

// These tests run several REAL libp2p nodes in one process: real gossip, real
// streams, real sync — only the machine is shared.

var netParams = domain.Params{ChainID: 4242, Difficulty: 6, BlockReward: 50, MinFee: 1, MaxBlockTxs: 10}

type harness struct {
	t     *testing.T
	node  *service.Node
	chain *chain.Chain
	pool  *mempool.Pool
	net   *p2p.Node
	miner *miner.Miner
}

func start(t *testing.T, seed string, coinbaseIdx int, bootstrap ...string) *harness {
	t.Helper()
	params := netParams
	alloc, err := domain.DevAlloc(3, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	params.GenesisAlloc = alloc

	store, err := boltstore.Open(filepath.Join(t.TempDir(), seed+".db"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); store.Close() })

	bus := events.NewBus()
	pool := mempool.New(mempool.Config{MaxSize: 100, MaxNonceGap: 8})
	c, err := chain.New(ctx, params, store, pool, bus, chain.Options{})
	if err != nil {
		t.Fatal(err)
	}
	node := service.NewNode(ctx, params, c, pool, bus)

	network, err := p2p.New(ctx, p2p.Config{ListenPort: 0, KeySeed: seed, Bootstrap: bootstrap, ChainID: params.ChainID}, node, bus)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { network.Close() })
	node.AttachNetwork(network)

	coinbase, _ := domain.DevAddress(coinbaseIdx)
	m := miner.New(c, pool, bus, coinbase, 1, func(b domain.Block) {
		if err := network.BroadcastBlock(ctx, b); err != nil {
			t.Logf("broadcast: %v", err)
		}
	})
	node.AttachMiner(m)

	// A fast peer-sync poll stands in for the 10s default, so tests don't crawl.
	go node.RunPeerSync(ctx, 300*time.Millisecond)

	return &harness{t: t, node: node, chain: c, pool: pool, net: network, miner: m}
}

// localAddr is the 127.0.0.1 multiaddr other test nodes can dial.
func (h *harness) localAddr() string {
	h.t.Helper()
	for _, a := range h.net.Addrs() {
		if strings.Contains(a, "/ip4/127.0.0.1/") {
			return a
		}
	}
	h.t.Fatalf("no loopback address in %v", h.net.Addrs())
	return ""
}

func (h *harness) mine(count int) []domain.Block {
	h.t.Helper()
	var out []domain.Block
	for range count {
		b, err := h.miner.MineOne(context.Background())
		if err != nil {
			h.t.Fatal(err)
		}
		out = append(out, b)
	}
	return out
}

func (h *harness) height() uint64   { b, _ := h.chain.Tip(); return b.Height }
func (h *harness) tip() domain.Hash { b, _ := h.chain.Tip(); return b.Hash }

func (h *harness) submit(from, to int, amount, fee uint64) domain.Tx {
	h.t.Helper()
	key, _ := domain.DevKey(from)
	toAddr, _ := domain.DevAddress(to)
	addr, _ := domain.DevAddress(from)
	nonce := h.node.Account(addr).PendingNonce
	tx, err := domain.SignTx(domain.Tx{ChainID: netParams.ChainID, Nonce: nonce, To: toAddr, Amount: amount, Fee: fee}, key)
	if err != nil {
		h.t.Fatal(err)
	}
	if _, err := h.node.SubmitTx(tx.Encode()); err != nil {
		h.t.Fatal(err)
	}
	return tx
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func connect(t *testing.T, nodes ...*harness) {
	t.Helper()
	for _, n := range nodes {
		waitFor(t, "peers to connect", func() bool { return len(n.net.Peers()) >= len(nodes)-1 })
	}
}

func TestGossipSpreadsBlocksAndTransactions(t *testing.T) {
	a := start(t, "gossip-a", 0)
	b := start(t, "gossip-b", 1, a.localAddr())
	connect(t, a, b)

	mined := a.mine(1)[0]
	waitFor(t, "B to receive the block", func() bool { return b.tip() == mined.Hash })

	// A transaction submitted to A must reach B's mempool the same way.
	tx := a.submit(2, 1, 100, 2)
	waitFor(t, "B to receive the transaction", func() bool {
		v, err := b.node.Tx(tx.Hash())
		return err == nil && v.Status == service.TxPending
	})

	// …and when B mines it, A learns the transaction is confirmed.
	b.mine(1)
	waitFor(t, "A to accept B's block", func() bool {
		v, err := a.node.Tx(tx.Hash())
		return err == nil && v.Status == service.TxIncluded
	})
}

// A partition stops messages without closing connections — the nodes keep
// mining, and their chains drift apart. Healing must reconcile them.
func TestPartitionAndHeal(t *testing.T) {
	a := start(t, "part-a", 0)
	b := start(t, "part-b", 1, a.localAddr())
	connect(t, a, b)
	a.mine(1)
	waitFor(t, "initial agreement", func() bool { return a.tip() == b.tip() })

	a.net.BlockAllPeers(true)
	b.net.BlockAllPeers(true)

	a.mine(1) // one block on A's side
	b.mine(3) // three on B's side: heavier
	if a.tip() == b.tip() {
		t.Fatal("the partition did not split the chains")
	}

	a.net.ClearBlocked()
	b.net.ClearBlocked()

	// Nobody re-sends the old blocks: the periodic status poll notices B is
	// heavier, and A pulls the missing blocks with a locator sync.
	waitFor(t, "the network to converge", func() bool { return a.tip() == b.tip() })
	if a.height() != 4 {
		t.Fatalf("converged at height %d, want 4 (the heavier branch)", a.height())
	}
	if a.chain.State().Root() != b.chain.State().Root() {
		t.Fatal("same tip, different state")
	}
}

// The classic attack: while the network is split, the same coins are spent on
// both sides. When the split heals, only the heavier branch's spend survives.
func TestDoubleSpendAcrossAPartition(t *testing.T) {
	a := start(t, "ds-a", 0)
	b := start(t, "ds-b", 1, a.localAddr())
	connect(t, a, b)
	a.mine(1)
	waitFor(t, "initial agreement", func() bool { return a.tip() == b.tip() })

	a.net.BlockAllPeers(true)
	b.net.BlockAllPeers(true)

	// dev 2 pays dev 0 on side A, and the same nonce pays dev 1 on side B.
	key, _ := domain.DevKey(2)
	toA, _ := domain.DevAddress(0)
	toB, _ := domain.DevAddress(1)
	sender, _ := domain.DevAddress(2)
	nonce := a.node.Account(sender).PendingNonce

	txA, _ := domain.SignTx(domain.Tx{ChainID: netParams.ChainID, Nonce: nonce, To: toA, Amount: 900_000, Fee: 1}, key)
	txB, _ := domain.SignTx(domain.Tx{ChainID: netParams.ChainID, Nonce: nonce, To: toB, Amount: 900_000, Fee: 1}, key)
	if _, err := a.node.SubmitTx(txA.Encode()); err != nil {
		t.Fatal(err)
	}
	if _, err := b.node.SubmitTx(txB.Encode()); err != nil {
		t.Fatal(err)
	}

	blockA := a.mine(1)[0] // A mines its version: "payment confirmed!"
	b.mine(2)              // B mines a heavier branch with the other version
	if len(blockA.Txs) != 1 {
		t.Fatal("side A did not mine its transaction")
	}

	a.net.ClearBlocked()
	b.net.ClearBlocked()
	waitFor(t, "the network to converge", func() bool { return a.tip() == b.tip() })

	// Side B's branch won, so its payment is the real one.
	state := a.chain.State()
	if got := state.Get(toB).Balance; got < 1_900_000 {
		t.Fatalf("recipient on the winning branch has %d, expected the payment", got)
	}
	if got := state.Get(toA).Balance; got >= 1_900_000 {
		t.Fatalf("recipient on the losing branch has %d: the payment should have been undone", got)
	}
	// The losing transaction can never be mined again: its nonce is used.
	v, err := a.node.Tx(txA.Hash())
	if err == nil && v.Status == service.TxIncluded {
		t.Fatal("the losing transaction is still included")
	}
	waitFor(t, "the losing transaction to be dropped from the mempool", func() bool {
		return a.pool.Len() == 0
	})
}

// A node with an empty database catches up from its peers.
func TestNewNodeSyncsFromScratch(t *testing.T) {
	a := start(t, "sync-a", 0)
	a.mine(6)

	c := start(t, "sync-c", 2, a.localAddr())
	waitFor(t, "the new node to catch up", func() bool { return c.tip() == a.tip() })
	if c.height() != 6 {
		t.Fatalf("height %d, want 6", c.height())
	}
	if c.chain.State().Root() != a.chain.State().Root() {
		t.Fatal("synced chain, different state")
	}
	// It validated everything it received, rather than trusting the peer.
	replayed, err := c.chain.Replay()
	if err != nil || replayed.Root() != c.chain.State().Root() {
		t.Fatalf("replay mismatch: %v", err)
	}
}

// An out-of-order block is parked and connected once its parent arrives.
func TestOrphanBlockIsConnectedAfterItsParent(t *testing.T) {
	a := start(t, "orphan-a", 0)
	b := start(t, "orphan-b", 1, a.localAddr())
	connect(t, a, b)

	a.net.BlockAllPeers(true) // A mines in private
	b.net.BlockAllPeers(true)
	blocks := a.mine(3)
	a.net.ClearBlocked()
	b.net.ClearBlocked()

	// B hears about the newest block first: an orphan, two ancestors missing.
	b.node.OnBlock(a.net.ID(), blocks[2])
	waitFor(t, "B to fetch the ancestors and connect", func() bool { return b.tip() == blocks[2].Hash })
	if b.height() != 3 {
		t.Fatalf("height %d, want 3", b.height())
	}
}
