package chain_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"p2pchain/internal/chain"
	"p2pchain/internal/domain"
	"p2pchain/internal/events"
	"p2pchain/internal/mempool"
	"p2pchain/internal/miner"
	"p2pchain/internal/repository/boltstore"
)

// Two independent nodes on the same rules: exactly the situation that creates
// forks in a real network.
type node struct {
	t     *testing.T
	chain *chain.Chain
	pool  *mempool.Pool
	miner *miner.Miner
}

var testParams = domain.Params{ChainID: 1337, Difficulty: 6, BlockReward: 50, MinFee: 1, MaxBlockTxs: 10}

func newNode(t *testing.T, coinbaseIdx int) *node {
	t.Helper()
	params := testParams
	alloc, err := domain.DevAlloc(3, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	params.GenesisAlloc = alloc

	store, err := boltstore.Open(filepath.Join(t.TempDir(), "chain.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	bus := events.NewBus()
	pool := mempool.New(mempool.Config{MaxSize: 100, MaxNonceGap: 8})
	c, err := chain.New(context.Background(), params, store, pool, bus, chain.Options{})
	if err != nil {
		t.Fatal(err)
	}
	coinbase, err := domain.DevAddress(coinbaseIdx)
	if err != nil {
		t.Fatal(err)
	}
	return &node{t: t, chain: c, pool: pool, miner: miner.New(c, pool, bus, coinbase, 1, func(domain.Block) {})}
}

func (n *node) mine(count int) []domain.Block {
	n.t.Helper()
	var blocks []domain.Block
	for range count {
		b, err := n.miner.MineOne(context.Background())
		if err != nil {
			n.t.Fatal(err)
		}
		blocks = append(blocks, b)
	}
	return blocks
}

func (n *node) send(from, to int, amount, fee, nonce uint64) domain.Tx {
	n.t.Helper()
	key, _ := domain.DevKey(from)
	toAddr, _ := domain.DevAddress(to)
	tx, err := domain.SignTx(domain.Tx{ChainID: testParams.ChainID, Nonce: nonce, To: toAddr, Amount: amount, Fee: fee}, key)
	if err != nil {
		n.t.Fatal(err)
	}
	sender, _ := tx.Sender()
	if _, err := n.pool.Add(tx, sender, n.chain.State().Get(sender)); err != nil {
		n.t.Fatal(err)
	}
	return tx
}

func (n *node) feed(blocks []domain.Block) []chain.AddResult {
	n.t.Helper()
	var out []chain.AddResult
	for _, b := range blocks {
		res, err := n.chain.AddBlock(b)
		if err != nil {
			n.t.Fatalf("block %d: %v", b.Height, err)
		}
		out = append(out, res)
	}
	return out
}

func (n *node) height() uint64   { b, _ := n.chain.Tip(); return b.Height }
func (n *node) tip() domain.Hash { b, _ := n.chain.Tip(); return b.Hash }

func TestExtendingTheTip(t *testing.T) {
	a := newNode(t, 0)
	blocks := a.mine(3)
	if a.height() != 3 || a.tip() != blocks[2].Hash {
		t.Fatalf("height %d", a.height())
	}
	if res, _ := a.chain.AddBlock(blocks[2]); res.Status != chain.StatusKnown {
		t.Fatalf("re-adding a known block: %s", res.Status)
	}
}

// A block of equal work does NOT replace our tip: ties go to what we saw first,
// so the network doesn't flap between two equally good branches.
func TestEqualWorkStaysASideBranch(t *testing.T) {
	a, b := newNode(t, 0), newNode(t, 1)
	a.mine(1)
	competing := b.mine(1)

	before := a.tip()
	res := a.feed(competing)[0]
	if res.Status != chain.StatusSide {
		t.Fatalf("status = %s, want side", res.Status)
	}
	if a.tip() != before {
		t.Fatal("an equal-work block took over the tip")
	}
	tips, _ := a.chain.Tips()
	if len(tips) != 2 {
		t.Fatalf("the node should now know 2 branch tips, got %d", len(tips))
	}
}

// The heavier branch wins, even if we were several blocks into the other one.
func TestHeavierBranchCausesAReorg(t *testing.T) {
	a, b := newNode(t, 0), newNode(t, 1)
	a.mine(2)
	competing := b.mine(3)

	results := a.feed(competing)
	if results[0].Status != chain.StatusSide || results[1].Status != chain.StatusSide {
		t.Fatalf("first two blocks: %s, %s", results[0].Status, results[1].Status)
	}
	last := results[2]
	if last.Status != chain.StatusReorg || last.ReorgDepth != 2 {
		t.Fatalf("status %s, depth %d, want reorg with depth 2", last.Status, last.ReorgDepth)
	}
	if a.height() != 3 || a.tip() != b.tip() {
		t.Fatalf("a is at %d (%s), b at %d (%s)", a.height(), a.tip().Hex()[:10], b.height(), b.tip().Hex()[:10])
	}
	// Both nodes must now agree on every balance, not just the tip hash.
	if a.chain.State().Root() != b.chain.State().Root() {
		t.Fatal("same chain, different state")
	}
}

// Transactions in disconnected blocks are not lost: they go back to the mempool
// so they can be mined again on the winning branch.
func TestReorgReturnsTransactionsToTheMempool(t *testing.T) {
	a, b := newNode(t, 0), newNode(t, 1)
	// dev 2 pays dev 0. (Not dev 1: that is node B's mining address, and its
	// block rewards would hide whether the transfer was undone.)
	tx := a.send(2, 0, 500, 2, 0) // only node A knows this transaction
	mined := a.mine(1)
	if len(mined[0].Txs) != 1 {
		t.Fatal("the transaction was not mined")
	}
	if _, _, err := a.chain.TxLocation(tx.Hash()); err != nil {
		t.Fatal("transaction not indexed")
	}

	res := a.feed(b.mine(2))[1] // node B's heavier branch never saw it
	if res.Status != chain.StatusReorg || res.ReturnedTxs != 1 {
		t.Fatalf("status %s, returned %d, want reorg returning 1", res.Status, res.ReturnedTxs)
	}
	if _, _, err := a.chain.TxLocation(tx.Hash()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("a disconnected transaction is still in the index")
	}
	pending, _ := a.pool.Snapshot(func(addr domain.Address) uint64 { return a.chain.State().Get(addr).Nonce })
	if len(pending) != 1 || pending[0].Hash != tx.Hash() {
		t.Fatalf("mempool has %d pending, want the returned transaction", len(pending))
	}
	// And the money it moved is back where it started: node A's own block —
	// transfer, fee, and reward — was undone with it.
	sender, _ := tx.Sender()
	if got := a.chain.State().Get(tx.To).Balance; got != 1_000_000 {
		t.Fatalf("recipient balance %d: the disconnected transfer was not undone", got)
	}
	if got := a.chain.State().Get(sender).Balance; got != 1_000_000 {
		t.Fatalf("sender balance %d: the disconnected transfer was not undone", got)
	}
}

// A transaction mined on BOTH branches must not come back to the pool.
func TestReorgKeepsTransactionsMinedOnTheNewBranch(t *testing.T) {
	a, b := newNode(t, 0), newNode(t, 1)
	tx := a.send(2, 1, 500, 2, 0)
	a.mine(1)

	b.send(2, 1, 500, 2, 0) // the same transfer reaches node B too
	competing := b.mine(2)
	if len(competing[0].Txs) != 1 {
		t.Fatal("node B did not mine the transaction")
	}

	res := a.feed(competing)[1]
	if res.Status != chain.StatusReorg || res.ReturnedTxs != 0 {
		t.Fatalf("status %s, returned %d, want reorg returning 0", res.Status, res.ReturnedTxs)
	}
	if _, _, err := a.chain.TxLocation(tx.Hash()); err != nil {
		t.Fatal("the transaction should be indexed on the new branch")
	}
	if n := a.pool.Len(); n != 0 {
		t.Fatalf("mempool has %d txs, want 0", n)
	}
}

func TestOrphanIsNotStored(t *testing.T) {
	a, b := newNode(t, 0), newNode(t, 1)
	blocks := b.mine(3)

	res, err := a.chain.AddBlock(blocks[2]) // its parent is unknown to A
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != chain.StatusOrphan {
		t.Fatalf("status = %s, want orphan", res.Status)
	}
	if have, _ := a.chain.HasBlock(blocks[2].Hash); have {
		t.Fatal("an orphan was stored in the tree")
	}
}

func TestInvalidBlocksAreRejected(t *testing.T) {
	a, b := newNode(t, 0), newNode(t, 1)
	good := b.mine(1)[0]

	tests := map[string]func(domain.Block) domain.Block{
		"no proof-of-work": func(bl domain.Block) domain.Block {
			for {
				bl.Nonce++
				bl.Hash = bl.Header.ComputeHash()
				if !domain.MeetsDifficulty(bl.Hash, testParams.Difficulty) {
					return bl
				}
			}
		},
		"tx root does not match": func(bl domain.Block) domain.Block {
			bl.Txs = []domain.Tx{{ChainID: 1337, Amount: 1}}
			return bl
		},
		"wrong difficulty": func(bl domain.Block) domain.Block {
			bl.Difficulty = 1
			bl.Hash = bl.Header.ComputeHash()
			return bl
		},
		"hash does not match header": func(bl domain.Block) domain.Block {
			bl.Hash[0] ^= 0xff
			return bl
		},
	}
	for name, tamper := range tests {
		t.Run(name, func(t *testing.T) {
			before := a.tip()
			if _, err := a.chain.AddBlock(tamper(good)); err == nil {
				t.Fatal("invalid block accepted")
			}
			if a.tip() != before {
				t.Fatal("tip moved")
			}
		})
	}
}

// A branch that is heavier but contains an invalid transaction must leave the
// node exactly where it was: the switch is all-or-nothing.
func TestFailedSwitchLeavesTheChainUntouched(t *testing.T) {
	a, b := newNode(t, 0), newNode(t, 1)
	a.mine(1)

	// Node B mines a heavier branch, but we corrupt its last block's state root
	// and re-mine it, so it is structurally valid yet wrong on execution.
	competing := b.mine(3)
	bad := competing[2]
	bad.StateRoot[0] ^= 0xff
	sealed, err := domain.Seal(context.Background(), bad.Header, bad.Txs)
	if err != nil {
		t.Fatal(err)
	}

	a.feed(competing[:2])
	beforeTip, beforeRoot := a.tip(), a.chain.State().Root()
	if _, err := a.chain.AddBlock(sealed); err == nil {
		t.Fatal("a block with a wrong state root was accepted")
	}
	if a.tip() != beforeTip || a.chain.State().Root() != beforeRoot {
		t.Fatal("a failed switch changed the chain")
	}
}

// The locator lets a peer find the newest block we share, even across a fork.
func TestLocatorAndBlocksAfter(t *testing.T) {
	a, b := newNode(t, 0), newNode(t, 1)
	shared := a.mine(3)
	b.feed(shared) // both nodes share blocks 1..3
	a.mine(4)      // A goes to height 7
	b.mine(1)      // B forks at height 4

	blocks, err := a.chain.BlocksAfter(b.chain.Locator(), 64)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 4 || blocks[0].Height != 4 {
		t.Fatalf("got %d blocks starting at %d, want 4 starting at 4", len(blocks), blocks[0].Height)
	}
	// Feeding them back makes B switch to A's heavier chain.
	b.feed(blocks)
	if b.tip() != a.tip() {
		t.Fatal("B did not converge on A's chain")
	}
}

func TestChainSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	params := testParams
	params.GenesisAlloc, _ = domain.DevAlloc(3, 1_000_000)

	open := func() (*chain.Chain, *boltstore.Store) {
		store, err := boltstore.Open(filepath.Join(dir, "chain.db"))
		if err != nil {
			t.Fatal(err)
		}
		c, err := chain.New(context.Background(), params, store, mempool.New(mempool.Config{MaxSize: 10, MaxNonceGap: 4}), events.NewBus(), chain.Options{})
		if err != nil {
			t.Fatal(err)
		}
		return c, store
	}

	c, store := open()
	coinbase, _ := domain.DevAddress(0)
	m := miner.New(c, mempool.New(mempool.Config{MaxSize: 10, MaxNonceGap: 4}), events.NewBus(), coinbase, 1, func(domain.Block) {})
	for range 3 {
		if _, err := m.MineOne(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	tip, _ := c.Tip()
	store.Close()

	reopened, store2 := open()
	defer store2.Close()
	got, _ := reopened.Tip()
	if got.Hash != tip.Hash || got.Height != 3 {
		t.Fatalf("after restart: height %d %s, want 3 %s", got.Height, got.Hash.Hex()[:10], tip.Hash.Hex()[:10])
	}
	replayed, err := reopened.Replay()
	if err != nil || replayed.Root() != reopened.State().Root() {
		t.Fatalf("replay mismatch: %v", err)
	}
}
