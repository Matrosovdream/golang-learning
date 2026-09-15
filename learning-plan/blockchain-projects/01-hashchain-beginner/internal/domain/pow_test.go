package domain

import (
	"context"
	"errors"
	"math/big"
	"testing"
)

func TestMineFindsTheFirstValidNonce(t *testing.T) {
	for _, difficulty := range []uint8{0, 1, 8, 12} {
		h := sampleHeader()
		h.Difficulty = difficulty

		mined, attempts, err := Mine(context.Background(), h)
		if err != nil {
			t.Fatal(err)
		}
		if !MeetsDifficulty(mined.ComputeHash(), difficulty) {
			t.Fatalf("difficulty %d: mined hash %s does not meet it", difficulty, mined.ComputeHash())
		}
		if attempts != mined.Nonce+1 {
			t.Fatalf("difficulty %d: attempts = %d, want nonce+1 = %d", difficulty, attempts, mined.Nonce+1)
		}
		// Every smaller nonce must fail — Mine searches upward from 0.
		for n := uint64(0); n < mined.Nonce; n++ {
			h.Nonce = n
			if MeetsDifficulty(h.ComputeHash(), difficulty) {
				t.Fatalf("difficulty %d: nonce %d already worked, Mine returned %d", difficulty, n, mined.Nonce)
			}
		}
	}
}

func TestMineStopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	h := sampleHeader()
	h.Difficulty = 255 // would never finish
	_, _, err := Mine(ctx, h)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// LeadingZeroBits(h) >= d must mean exactly the same as hash < 2^(256-d).
// Build hashes with exactly k leading zero bits and compare both rules for every d.
func TestLeadingZeroBitsAgreesWithTarget(t *testing.T) {
	for k := 0; k <= 256; k++ {
		var h Hash
		if k < 256 {
			h[k/8] = 0x80 >> (k % 8) // first 1-bit at position k
			h[31] |= 1               // some low bits set too
		}
		if got := LeadingZeroBits(h); got != k {
			t.Fatalf("LeadingZeroBits = %d, want %d for %s", got, k, h)
		}
		n := new(big.Int).SetBytes(h[:])
		for d := 0; d <= 255; d++ {
			byBits := MeetsDifficulty(h, uint8(d))
			byTarget := n.Cmp(Target(uint8(d))) < 0
			if byBits != byTarget {
				t.Fatalf("k=%d d=%d: zero-bits rule says %v, target rule says %v", k, d, byBits, byTarget)
			}
		}
	}
}

func BenchmarkMineDifficulty16(b *testing.B) {
	h := sampleHeader()
	h.Difficulty = 16
	for b.Loop() {
		if _, _, err := Mine(context.Background(), h); err != nil {
			b.Fatal(err)
		}
	}
}
