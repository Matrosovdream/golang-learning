package domain

import (
	"context"
	"errors"
	"testing"
)

func TestHeaderEncodeDecodeRoundTrip(t *testing.T) {
	_, miner := testKey(t, 9)
	h := Header{
		Height: 42, Timestamp: GenesisTimestamp + 1234, PrevHash: SHA256([]byte("p")),
		TxRoot: SHA256([]byte("t")), StateRoot: SHA256([]byte("s")), Miner: miner, Difficulty: 18, Nonce: 99,
	}
	b := h.Encode()
	if len(b) != HeaderSize {
		t.Fatalf("encoded %d bytes, want %d", len(b), HeaderSize)
	}
	got, err := DecodeHeader(b)
	if err != nil || got != h {
		t.Fatalf("round trip: %+v, %v", got, err)
	}
}

func TestBlockEncodeDecodeRoundTrip(t *testing.T) {
	alice, _ := testKey(t, 1)
	_, bob := testKey(t, 2)
	txs := []Tx{signed(t, alice, Tx{To: bob, Amount: 1, Fee: 1}), signed(t, alice, Tx{Nonce: 1, To: bob, Amount: 2, Fee: 1})}
	b := Block{Header: Header{Height: 1, TxRoot: ComputeTxRoot(txs)}, Txs: txs}
	b.Hash = b.ComputeHash()

	got, err := DecodeBlock(EncodeBlock(b))
	if err != nil {
		t.Fatal(err)
	}
	if got.Header != b.Header || got.Hash != b.Hash || len(got.Txs) != 2 || got.Txs[1] != txs[1] {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

// chain builds genesis (alice holds 100) and block 1 with the given txs,
// with correct roots and real proof-of-work.
func chain(t *testing.T, txs ...Tx) (Block, State, Block) {
	t.Helper()
	_, aliceAddr := testKey(t, 1)
	_, miner := testKey(t, 9)
	p := testParams
	p.GenesisAlloc = map[Address]uint64{aliceAddr: 100}

	genesis, gstate, err := NewGenesis(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	post, err := ApplyBlock(p, gstate, Block{Header: Header{Miner: miner}, Txs: txs})
	if err != nil {
		t.Fatal(err)
	}
	b1 := seal(t, Header{
		Height: 1, Timestamp: GenesisTimestamp + 1000, PrevHash: genesis.Hash,
		TxRoot: ComputeTxRoot(txs), StateRoot: post.Root(), Miner: miner, Difficulty: p.Difficulty,
	}, txs)
	return genesis, gstate, b1
}

func seal(t *testing.T, h Header, txs []Tx) Block {
	t.Helper()
	b, err := Seal(context.Background(), h, txs)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestValidateBlock(t *testing.T) {
	alice, aliceAddr := testKey(t, 1)
	_, bob := testKey(t, 2)
	_, mallory := testKey(t, 3)
	p := testParams
	p.GenesisAlloc = map[Address]uint64{aliceAddr: 100}

	pay := signed(t, alice, Tx{Nonce: 0, To: bob, Amount: 10, Fee: 1})
	genesis, gstate, b1 := chain(t, pay)

	if _, err := ValidateBlock(p, nil, nil, genesis); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	post, err := ValidateBlock(p, &genesis, gstate, b1)
	if err != nil {
		t.Fatalf("block 1: %v", err)
	}
	if post.Get(bob).Balance != 10 {
		t.Fatal("validation did not return the executed state")
	}

	tests := []struct {
		name     string
		tamper   func(b Block) Block
		wantRule string
	}{
		{"cached hash edited", func(b Block) Block { b.Hash[0] ^= 1; return b }, RuleStaleHash},
		{"prev hash points elsewhere", func(b Block) Block {
			b.PrevHash[0] ^= 1
			return seal(t, b.Header, b.Txs)
		}, RulePrevHash},
		{"lower difficulty claimed", func(b Block) Block {
			b.Difficulty = 1
			return seal(t, b.Header, b.Txs)
		}, RuleDifficulty},
		{"no proof-of-work", func(b Block) Block {
			b.Nonce++ // almost certainly no longer meets difficulty 8…
			for MeetsDifficulty(b.ComputeHash(), p.Difficulty) {
				b.Nonce++ // …and if it does by luck, keep going
			}
			b.Hash = b.ComputeHash()
			return b
		}, RuleWork},
		{"transaction swapped after mining", func(b Block) Block {
			b.Txs = []Tx{signed(t, alice, Tx{Nonce: 0, To: mallory, Amount: 10, Fee: 1})}
			return b // header (and its TxRoot) unchanged
		}, RuleTxRoot},
		{"double spend inside the block", func(b Block) Block {
			txs := []Tx{pay, signed(t, alice, Tx{Nonce: 0, To: mallory, Amount: 10, Fee: 1})}
			b.TxRoot = ComputeTxRoot(txs)
			return seal(t, b.Header, txs)
		}, RuleTxs},
		{"miner paid itself extra", func(b Block) Block {
			inflated := post.Clone()
			inflated[b.Miner] = Account{Balance: inflated[b.Miner].Balance + 1_000_000}
			b.StateRoot = inflated.Root()
			return seal(t, b.Header, b.Txs)
		}, RuleStateRoot},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateBlock(p, &genesis, gstate, tt.tamper(b1))
			var invalid *InvalidBlockError
			if !errors.As(err, &invalid) || invalid.Rule != tt.wantRule {
				t.Fatalf("err = %v, want rule %q", err, tt.wantRule)
			}
		})
	}
}

func TestGenesisIsDeterministicAndDependsOnParams(t *testing.T) {
	_, a := testKey(t, 1)
	p := testParams
	p.GenesisAlloc = map[Address]uint64{a: 100}
	g1, _, _ := NewGenesis(context.Background(), p)
	g2, _, _ := NewGenesis(context.Background(), p)
	if g1.Hash != g2.Hash {
		t.Fatal("same params, different genesis")
	}
	p.GenesisAlloc = map[Address]uint64{a: 101}
	g3, _, _ := NewGenesis(context.Background(), p)
	if g3.Hash == g1.Hash {
		t.Fatal("different allocation, same genesis")
	}
}
