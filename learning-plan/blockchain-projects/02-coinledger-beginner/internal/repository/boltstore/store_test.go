package boltstore

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"

	"coinledger/internal/domain"
)

func TestCommitReopenAndQuery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chain.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	if empty, _ := s.IsEmpty(); !empty {
		t.Fatal("new store is not empty")
	}

	k := make([]byte, 32)
	k[31] = 1
	alice, _ := crypto.ToECDSA(k)
	aliceAddr := crypto.PubkeyToAddress(alice.PublicKey)
	bob := domain.Address{0xb0}

	tx, _ := domain.SignTx(domain.Tx{ChainID: 1337, To: bob, Amount: 5, Fee: 1}, alice)
	b0 := domain.Block{Header: domain.Header{Height: 0}}
	b0.Hash = b0.ComputeHash()
	b1 := domain.Block{Header: domain.Header{Height: 1, PrevHash: b0.Hash, TxRoot: domain.ComputeTxRoot([]domain.Tx{tx})}, Txs: []domain.Tx{tx}}
	b1.Hash = b1.ComputeHash()

	if err := s.CommitBlock(b0, map[domain.Address]domain.Account{aliceAddr: {Balance: 100}}); err != nil {
		t.Fatal(err)
	}
	if err := s.CommitBlock(b1, map[domain.Address]domain.Account{
		aliceAddr: {Balance: 94, Nonce: 1},
		bob:       {Balance: 5},
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	// Everything must survive a restart.
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if h, _ := s.TipHeight(); h != 1 {
		t.Fatalf("tip = %d, want 1", h)
	}
	got, err := s.BlockByHeight(1)
	if err != nil || got.Hash != b1.Hash || len(got.Txs) != 1 {
		t.Fatalf("block 1: %+v %v", got, err)
	}
	if height, index, err := s.TxLocation(tx.Hash()); err != nil || height != 1 || index != 0 {
		t.Fatalf("tx location: %d %d %v", height, index, err)
	}
	st, _ := s.LoadState()
	if st[aliceAddr] != (domain.Account{Balance: 94, Nonce: 1}) || st[bob].Balance != 5 {
		t.Fatalf("state = %v", st)
	}
	blocks, _ := s.Blocks(1, 10)
	if len(blocks) != 1 || blocks[0].Height != 1 {
		t.Fatalf("Blocks(1, 10) = %d blocks", len(blocks))
	}
	if _, err := s.BlockByHeight(7); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing block: err = %v", err)
	}

	// An empty Account in the changes deletes the stored account.
	b2 := domain.Block{Header: domain.Header{Height: 2}}
	if err := s.CommitBlock(b2, map[domain.Address]domain.Account{bob: {}}); err != nil {
		t.Fatal(err)
	}
	st, _ = s.LoadState()
	if _, ok := st[bob]; ok {
		t.Fatal("empty account was not deleted")
	}

	if err := s.ReplaceState(domain.State{bob: {Balance: 1}}); err != nil {
		t.Fatal(err)
	}
	st, _ = s.LoadState()
	if len(st) != 1 || st[bob].Balance != 1 {
		t.Fatalf("after ReplaceState: %v", st)
	}
}

func TestSecondProcessCannotOpenTheSameFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chain.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// bbolt holds an exclusive file lock; the second Open gives up after its 1s timeout.
	if _, err := Open(path); err == nil {
		t.Fatal("opened a locked database twice")
	}
}
