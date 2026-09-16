package domain

import (
	"errors"
	"testing"
)

func TestApplyTx(t *testing.T) {
	alice, aliceAddr := testKey(t, 1)
	_, bob := testKey(t, 2)
	mallory, _ := testKey(t, 3)
	_, miner := testKey(t, 9)

	fresh := func() State { return State{aliceAddr: {Balance: 100, Nonce: 2}} }

	tests := []struct {
		name    string
		tx      Tx
		wantErr error
	}{
		{"valid transfer", signed(t, alice, Tx{Nonce: 2, To: bob, Amount: 90, Fee: 10}), nil},
		{"wrong chain", signed(t, alice, Tx{ChainID: 1, Nonce: 2, To: bob, Amount: 1, Fee: 1}), ErrWrongChainID},
		{"unsigned", Tx{ChainID: 1337, Nonce: 2, To: bob, Amount: 1, Fee: 1}, ErrInvalidSignature},
		{"replay of an old nonce", signed(t, alice, Tx{Nonce: 1, To: bob, Amount: 1, Fee: 1}), ErrNonceTooLow},
		{"skipped nonce", signed(t, alice, Tx{Nonce: 3, To: bob, Amount: 1, Fee: 1}), ErrNonceTooHigh},
		{"fee below minimum", signed(t, alice, Tx{Nonce: 2, To: bob, Amount: 1, Fee: 0}), ErrFeeTooLow},
		{"amount + fee > balance", signed(t, alice, Tx{Nonce: 2, To: bob, Amount: 95, Fee: 10}), ErrInsufficientFunds},
		{"amount + fee overflows", signed(t, alice, Tx{Nonce: 2, To: bob, Amount: ^uint64(0), Fee: 1}), ErrOverflow},
		{"someone else's money", signed(t, mallory, Tx{Nonce: 0, To: bob, Amount: 1, Fee: 1}), ErrInsufficientFunds},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := fresh()
			_, err := ApplyTx(testParams, s, tt.tx, miner)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			want := State{aliceAddr: {Balance: 0, Nonce: 3}, bob: {Balance: 90}, miner: {Balance: 10}}
			if s.Root() != want.Root() {
				t.Fatalf("state = %v, want %v", s, want)
			}
		})
	}
}

func TestSelfTransferOnlyCostsTheFee(t *testing.T) {
	alice, aliceAddr := testKey(t, 1)
	_, miner := testKey(t, 9)
	s := State{aliceAddr: {Balance: 100}}
	if _, err := ApplyTx(testParams, s, signed(t, alice, Tx{To: aliceAddr, Amount: 60, Fee: 5}), miner); err != nil {
		t.Fatal(err)
	}
	if got := s.Get(aliceAddr); got != (Account{Balance: 95, Nonce: 1}) {
		t.Fatalf("alice = %+v", got)
	}
}

func TestApplyBlockIsAllOrNothing(t *testing.T) {
	alice, aliceAddr := testKey(t, 1)
	_, bob := testKey(t, 2)
	_, miner := testKey(t, 9)
	parent := State{aliceAddr: {Balance: 100}}

	good := signed(t, alice, Tx{Nonce: 0, To: bob, Amount: 10, Fee: 1})
	doubleSpend := signed(t, alice, Tx{Nonce: 0, To: bob, Amount: 20, Fee: 1}) // same nonce again

	b := Block{Header: Header{Miner: miner}, Txs: []Tx{good, doubleSpend}}
	_, err := ApplyBlock(testParams, parent, b)
	var txErr *TxError
	if !errors.As(err, &txErr) || txErr.Index != 1 || !errors.Is(err, ErrNonceTooLow) {
		t.Fatalf("err = %v, want tx 1 nonce too low", err)
	}
	if parent.Get(aliceAddr).Balance != 100 || parent.Get(bob).Balance != 0 {
		t.Fatal("a rejected block modified the parent state")
	}
}

func TestBlockRewardAndSupply(t *testing.T) {
	alice, aliceAddr := testKey(t, 1)
	_, bob := testKey(t, 2)
	_, miner := testKey(t, 9)
	parent := State{aliceAddr: {Balance: 100}}

	b := Block{Header: Header{Miner: miner}, Txs: []Tx{signed(t, alice, Tx{To: bob, Amount: 10, Fee: 3})}}
	next, err := ApplyBlock(testParams, parent, b)
	if err != nil {
		t.Fatal(err)
	}
	if got := next.Get(miner).Balance; got != testParams.BlockReward+3 {
		t.Fatalf("miner = %d, want reward + fee", got)
	}
	supply, _ := next.TotalSupply()
	if supply != 100+testParams.BlockReward {
		t.Fatalf("supply = %d: fees must move coins, only the reward may create them", supply)
	}
}

func TestEmptyAccountsDisappear(t *testing.T) {
	alice, aliceAddr := testKey(t, 1)
	_, bob := testKey(t, 2)
	_, miner := testKey(t, 9)
	s := State{aliceAddr: {Balance: 5}}
	withZeroTransfer := s.Clone()
	if _, err := ApplyTx(testParams, withZeroTransfer, signed(t, alice, Tx{To: bob, Amount: 0, Fee: 1}), miner); err != nil {
		t.Fatal(err)
	}
	if _, ok := withZeroTransfer[bob]; ok {
		t.Fatal("receiving 0 coins created an empty account")
	}
}

func TestRootIgnoresInsertionOrder(t *testing.T) {
	_, a := testKey(t, 1)
	_, b := testKey(t, 2)
	_, c := testKey(t, 3)
	s1, s2 := State{}, State{}
	for _, addr := range []Address{a, b, c} {
		s1[addr] = Account{Balance: uint64(addr[19])}
	}
	for _, addr := range []Address{c, a, b} {
		s2[addr] = Account{Balance: uint64(addr[19])}
	}
	if s1.Root() != s2.Root() {
		t.Fatal("same accounts, different roots")
	}
	s2[b] = Account{Balance: s2[b].Balance + 1}
	if s1.Root() == s2.Root() {
		t.Fatal("different balances, same root")
	}
}

func TestDiff(t *testing.T) {
	_, a := testKey(t, 1)
	_, b := testKey(t, 2)
	_, c := testKey(t, 3)
	old := State{a: {Balance: 1}, b: {Balance: 2}}
	next := State{a: {Balance: 1}, c: {Balance: 3}}
	got := Diff(old, next)
	if len(got) != 2 || got[b] != (Account{}) || got[c] != (Account{Balance: 3}) {
		t.Fatalf("diff = %v", got)
	}
}
