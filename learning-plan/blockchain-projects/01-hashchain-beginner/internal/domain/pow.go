package domain

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
	"math/big"
	"math/bits"
	"time"
)

// ErrNonceSpaceExhausted means all 2^64 nonces failed. Unreachable at any
// sane difficulty; real miners would then change the timestamp and continue.
var ErrNonceSpaceExhausted = errors.New("nonce space exhausted")

// ctxCheckInterval is how many hashes Mine tries between cancellation checks.
// ctx.Err() takes a lock, so checking on every hash would slow mining down;
// every 16,384 hashes still reacts within a few milliseconds.
const ctxCheckInterval = 1 << 14

// Target is the proof-of-work threshold for a difficulty: 2^(256-difficulty).
// A header is valid when its hash, read as a 256-bit number, is below it.
// Used for display; Mine and validation use LeadingZeroBits (same rule, no allocations).
func Target(difficulty uint8) *big.Int {
	return new(big.Int).Lsh(big.NewInt(1), 256-uint(difficulty))
}

// LeadingZeroBits counts the zero bits at the most significant end of h.
//
// "hash < 2^(256-d)" and "the top d bits of hash are zero" are the same
// statement, so this is an exact, allocation-free way to check the target.
// (Bitcoin's target is not always a power of two, so it compares numbers.)
func LeadingZeroBits(h Hash) int {
	n := 0
	for _, b := range h {
		if b != 0 {
			return n + bits.LeadingZeros8(b)
		}
		n += 8
	}
	return n
}

// MeetsDifficulty reports whether a hash satisfies a difficulty.
func MeetsDifficulty(h Hash, difficulty uint8) bool {
	return LeadingZeroBits(h) >= int(difficulty)
}

// Mine searches for the first nonce (counting up from 0) whose header hash
// meets h.Difficulty. It returns the header with Nonce set and how many hashes
// it tried. Each extra bit of difficulty doubles the expected number of tries:
// 2^difficulty on average.
//
// The header is encoded once; only the last 8 bytes (the nonce) are rewritten
// per attempt. That is why Nonce is the last field in Encode.
func Mine(ctx context.Context, h Header) (Header, uint64, error) {
	buf := h.Encode()
	nonceAt := len(buf) - 8

	for nonce := uint64(0); ; nonce++ {
		if nonce%ctxCheckInterval == 0 {
			if err := ctx.Err(); err != nil {
				return Header{}, nonce, err // cancelled or timed out: stop burning CPU
			}
		}
		binary.BigEndian.PutUint64(buf[nonceAt:], nonce)
		if MeetsDifficulty(sha256.Sum256(buf), h.Difficulty) {
			h.Nonce = nonce
			return h, nonce + 1, nil
		}
		if nonce == math.MaxUint64 {
			return Header{}, nonce, ErrNonceSpaceExhausted
		}
	}
}

// MineBlock builds a full block: it commits data into the header, mines the
// header, and fills in the cached hash and mining stats. Any Nonce already in h
// is ignored — the search always starts from 0. On error the returned Block is
// empty except for Attempts.
func MineBlock(ctx context.Context, h Header, data []byte) (Block, error) {
	h.DataHash = HashData(data)
	start := time.Now()
	mined, attempts, err := Mine(ctx, h)
	if err != nil {
		return Block{Attempts: attempts}, err
	}
	return Block{
		Header:   mined,
		Data:     data,
		Hash:     mined.ComputeHash(),
		MinedIn:  time.Since(start),
		Attempts: attempts,
	}, nil
}
