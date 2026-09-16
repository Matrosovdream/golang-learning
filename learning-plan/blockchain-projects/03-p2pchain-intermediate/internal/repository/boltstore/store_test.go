package boltstore

import (
	"errors"
	"math/big"
	"path/filepath"
	"testing"

	"p2pchain/internal/domain"
)

// block builds a distinct block at a height with a given parent.
func block(height uint64, parent domain.Hash, nonce uint64) domain.Block {
	b := domain.Block{Header: domain.Header{Height: height, PrevHash: parent, Nonce: nonce}}
	b.Hash = b.Header.ComputeHash()
	return b
}

func open(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := Open(filepath.Join(dir, "chain.db"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// The store keeps a tree: two children of the same parent both exist, and the
// set of tips reflects the branches.
func TestTreeAndTips(t *testing.T) {
	s := open(t, t.TempDir())
	defer s.Close()

	genesis := block(0, domain.Hash{}, 0)
	childA := block(1, genesis.Hash, 1)
	childB := block(1, genesis.Hash, 2) // a competing block at the same height

	for _, b := range []domain.Block{genesis, childA, childB} {
		meta := domain.BlockMeta{Height: b.Height, Parent: b.PrevHash, Work: big.NewInt(int64(b.Height + 1))}
		if err := s.PutBlock(b, meta); err != nil {
			t.Fatal(err)
		}
	}

	tips, err := s.Tips()
	if err != nil {
		t.Fatal(err)
	}
	if len(tips) != 2 {
		t.Fatalf("got %d tips, want 2 (genesis stopped being a tip when it got children)", len(tips))
	}
	if have, _ := s.HasBlock(childB.Hash); !have {
		t.Fatal("the losing branch was not stored")
	}
	got, err := s.BlockByHash(childA.Hash)
	if err != nil || got.Hash != childA.Hash {
		t.Fatalf("BlockByHash: %v", err)
	}
	if _, err := s.Meta(domain.Hash{9}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown block: %v", err)
	}
}

// Switch moves the canonical chain: state, height index, transaction index and
// tip move together, and a rollback restores exactly what was there before.
func TestSwitchConnectsAndDisconnects(t *testing.T) {
	s := open(t, t.TempDir())
	defer s.Close()

	alice := domain.Address{0xa1}
	bob := domain.Address{0xb0}

	key, _ := domain.DevKey(0)
	toAddr, _ := domain.DevAddress(1)
	tx, err := domain.SignTx(domain.Tx{ChainID: 1, To: toAddr, Amount: 5, Fee: 1}, key)
	if err != nil {
		t.Fatal(err)
	}

	genesis := block(0, domain.Hash{}, 0)
	b1 := domain.Block{Header: domain.Header{Height: 1, PrevHash: genesis.Hash}, Txs: []domain.Tx{tx}}
	b1.Hash = b1.Header.ComputeHash()

	meta0 := domain.BlockMeta{Work: big.NewInt(1)}
	meta1 := domain.BlockMeta{Height: 1, Parent: genesis.Hash, Work: big.NewInt(2)}
	if err := s.PutBlock(genesis, meta0); err != nil {
		t.Fatal(err)
	}
	if err := s.PutBlock(b1, meta1); err != nil {
		t.Fatal(err)
	}

	connect := []domain.BlockUpdate{
		{Block: genesis, Meta: meta0,
			Forward:  map[domain.Address]domain.Account{alice: {Balance: 100}},
			Backward: map[domain.Address]domain.Account{alice: {}}},
		{Block: b1, Meta: meta1,
			Forward:  map[domain.Address]domain.Account{alice: {Balance: 94, Nonce: 1}, bob: {Balance: 5}},
			Backward: map[domain.Address]domain.Account{alice: {Balance: 100}, bob: {}}},
	}
	if err := s.Switch(nil, connect); err != nil {
		t.Fatal(err)
	}

	if tip, _ := s.TipHash(); tip != b1.Hash {
		t.Fatal("tip did not move")
	}
	if h, _ := s.CanonicalHash(1); h != b1.Hash {
		t.Fatal("height index did not move")
	}
	state, _ := s.LoadState()
	if state[alice] != (domain.Account{Balance: 94, Nonce: 1}) || state[bob].Balance != 5 {
		t.Fatalf("state = %v", state)
	}
	if bh, idx, err := s.TxLocation(tx.Hash()); err != nil || bh != b1.Hash || idx != 0 {
		t.Fatalf("tx index: %v %d %v", bh, idx, err)
	}

	// Now roll block 1 back, as a reorg would.
	undo, err := s.UndoDiff(b1.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Switch([]domain.BlockUpdate{{Block: b1, Meta: meta1, Backward: undo}}, nil); err != nil {
		t.Fatal(err)
	}
	if tip, _ := s.TipHash(); tip != genesis.Hash {
		t.Fatal("tip did not roll back to the parent")
	}
	state, _ = s.LoadState()
	if state[alice] != (domain.Account{Balance: 100}) {
		t.Fatalf("alice = %+v, want her pre-block balance", state[alice])
	}
	if _, ok := state[bob]; ok {
		t.Fatal("bob's account should be gone again")
	}
	if _, _, err := s.TxLocation(tx.Hash()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("the transaction is still indexed after a rollback")
	}
	if _, err := s.CanonicalHash(1); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("height 1 is still canonical after a rollback")
	}
	// The block itself stays in the tree: it may win again later.
	if have, _ := s.HasBlock(b1.Hash); !have {
		t.Fatal("a disconnected block was deleted from the tree")
	}
}

func TestSecondProcessCannotOpenTheSameFile(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	defer s.Close()
	if _, err := Open(filepath.Join(dir, "chain.db")); err == nil {
		t.Fatal("opened a locked database twice")
	}
}
