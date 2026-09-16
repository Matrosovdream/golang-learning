package mempool

import (
	"crypto/ecdsa"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"

	"coinledger/internal/domain"
)

func key(t *testing.T, n byte) (*ecdsa.PrivateKey, domain.Address) {
	t.Helper()
	b := make([]byte, 32)
	b[31] = n
	k, err := crypto.ToECDSA(b)
	if err != nil {
		t.Fatal(err)
	}
	return k, crypto.PubkeyToAddress(k.PublicKey)
}

func tx(t *testing.T, k *ecdsa.PrivateKey, nonce, amount, fee uint64) domain.Tx {
	t.Helper()
	_, to := key(t, 200)
	s, err := domain.SignTx(domain.Tx{ChainID: 1337, Nonce: nonce, To: to, Amount: amount, Fee: fee}, k)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

var rich = domain.Account{Balance: 1_000}

func newPool() *Pool { return New(Config{MaxSize: 100, MaxNonceGap: 4}) }

func TestPendingAndQueued(t *testing.T) {
	p := newPool()
	alice, a := key(t, 1)

	if st, err := p.Add(tx(t, alice, 2, 1, 1), a, rich); err != nil || st != StatusQueued {
		t.Fatalf("nonce 2 with nonces 0,1 missing: %s %v, want queued", st, err)
	}
	if st, err := p.Add(tx(t, alice, 0, 1, 1), a, rich); err != nil || st != StatusPending {
		t.Fatalf("nonce 0: %s %v, want pending", st, err)
	}
	if got := p.PendingNonce(a, 0); got != 1 {
		t.Fatalf("pending nonce = %d, want 1 (the gap)", got)
	}
	if _, err := p.Add(tx(t, alice, 1, 1, 1), a, rich); err != nil {
		t.Fatal(err)
	}
	pending, queued := p.Snapshot(func(domain.Address) uint64 { return 0 })
	if len(pending) != 3 || len(queued) != 0 {
		t.Fatalf("after filling the gap: %d pending, %d queued, want 3/0", len(pending), len(queued))
	}
}

func TestAdmissionRules(t *testing.T) {
	alice, a := key(t, 1)
	tests := []struct {
		name    string
		acct    domain.Account
		tx      domain.Tx
		wantErr error
	}{
		{"nonce already used", domain.Account{Balance: 100, Nonce: 5}, tx(t, alice, 4, 1, 1), domain.ErrNonceTooLow},
		{"nonce too far ahead", domain.Account{Balance: 100, Nonce: 5}, tx(t, alice, 10, 1, 1), ErrNonceGap},
		{"cannot afford", domain.Account{Balance: 10}, tx(t, alice, 0, 10, 1), domain.ErrInsufficientFunds},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := newPool().Add(tt.tx, a, tt.acct); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestDuplicateIsRejected(t *testing.T) {
	p := newPool()
	alice, a := key(t, 1)
	t0 := tx(t, alice, 0, 1, 1)
	p.Add(t0, a, rich)
	if _, err := p.Add(t0, a, rich); !errors.Is(err, ErrAlreadyKnown) {
		t.Fatalf("err = %v, want ErrAlreadyKnown", err)
	}
}

func TestReplaceByFee(t *testing.T) {
	p := newPool()
	alice, a := key(t, 1)
	original := tx(t, alice, 0, 5, 20)
	if _, err := p.Add(original, a, rich); err != nil {
		t.Fatal(err)
	}

	// Same nonce, +5% fee: not enough.
	if _, err := p.Add(tx(t, alice, 0, 5, 21), a, rich); !errors.Is(err, ErrReplacementUnderpriced) {
		t.Fatalf("+5%%: err = %v, want ErrReplacementUnderpriced", err)
	}
	// Same nonce, +10% fee, different amount: replaces.
	speedUp := tx(t, alice, 0, 7, 22)
	if _, err := p.Add(speedUp, a, rich); err != nil {
		t.Fatalf("+10%%: %v", err)
	}
	if _, _, ok := p.Get(original.Hash(), func(domain.Address) uint64 { return 0 }); ok {
		t.Fatal("the replaced tx is still pooled")
	}
	if p.Len() != 1 {
		t.Fatalf("pool has %d txs, want 1", p.Len())
	}
}

func TestFullPoolRejectsNewButAllowsReplacement(t *testing.T) {
	p := New(Config{MaxSize: 1, MaxNonceGap: 4})
	alice, a := key(t, 1)
	p.Add(tx(t, alice, 0, 1, 10), a, rich)
	if _, err := p.Add(tx(t, alice, 1, 1, 10), a, rich); !errors.Is(err, ErrPoolFull) {
		t.Fatalf("err = %v, want ErrPoolFull", err)
	}
	if _, err := p.Add(tx(t, alice, 0, 1, 11), a, rich); err != nil {
		t.Fatalf("replacement in a full pool: %v", err)
	}
}

// Alice's nonce 1 pays 100, but it can't go before her nonce 0 (fee 1).
// Bob's single tx (fee 50) therefore goes first.
func TestSelectRespectsNonceOrderThenFee(t *testing.T) {
	p := newPool()
	alice, a := key(t, 1)
	bob, b := key(t, 2)
	a0, a1, b0 := tx(t, alice, 0, 1, 1), tx(t, alice, 1, 1, 100), tx(t, bob, 0, 1, 50)
	p.Add(a1, a, rich)
	p.Add(a0, a, rich)
	p.Add(b0, b, rich)

	got := p.Select(10, func(domain.Address) uint64 { return 0 })
	want := []domain.Hash{b0.Hash(), a0.Hash(), a1.Hash()}
	if len(got) != len(want) {
		t.Fatalf("selected %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Hash != want[i] {
			t.Fatalf("position %d: got nonce %d fee %d", i, got[i].Tx.Nonce, got[i].Tx.Fee)
		}
	}
	if len(p.Select(2, func(domain.Address) uint64 { return 0 })) != 2 {
		t.Fatal("max not respected")
	}
}

func TestSelectSkipsQueued(t *testing.T) {
	p := newPool()
	alice, a := key(t, 1)
	p.Add(tx(t, alice, 3, 1, 99), a, rich)
	if got := p.Select(10, func(domain.Address) uint64 { return 0 }); len(got) != 0 {
		t.Fatalf("a queued tx (gap at nonces 0–2) was selected")
	}
}

func TestPruneAfterABlock(t *testing.T) {
	p := newPool()
	alice, a := key(t, 1)
	p.Add(tx(t, alice, 0, 1, 1), a, rich)
	p.Add(tx(t, alice, 1, 1, 1), a, rich)
	p.Add(tx(t, alice, 2, 1, 1), a, rich)

	// A block mined nonces 0 and 1: alice's account nonce is now 2.
	if removed := p.Prune(func(domain.Address) uint64 { return 2 }); removed != 2 {
		t.Fatalf("pruned %d, want 2", removed)
	}
	if p.Len() != 1 {
		t.Fatalf("pool has %d, want 1", p.Len())
	}
}
