package domain

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
	"math/bits"
	"time"
)

// Proof-of-work, carried over from Project 1 unchanged in spirit.
// Difficulty is fixed here (Params.Difficulty): retargeting was Project 1's lesson.

// ErrNonceSpaceExhausted means all 2^64 nonces failed.
var ErrNonceSpaceExhausted = errors.New("nonce space exhausted")

const ctxCheckInterval = 1 << 14

// LeadingZeroBits counts zero bits at the most significant end of h.
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

// MeetsDifficulty reports whether a hash has at least difficulty leading zero bits.
func MeetsDifficulty(h Hash, difficulty uint8) bool {
	return LeadingZeroBits(h) >= int(difficulty)
}

// Mine searches upward from nonce 0 for a header hash that meets h.Difficulty.
func Mine(ctx context.Context, h Header) (Header, uint64, error) {
	buf := h.Encode()
	nonceAt := len(buf) - 8
	for nonce := uint64(0); ; nonce++ {
		if nonce%ctxCheckInterval == 0 {
			if err := ctx.Err(); err != nil {
				return Header{}, nonce, err
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

// Seal mines a header whose roots are already filled in and returns the
// finished block. On error the Block is empty except for Attempts.
func Seal(ctx context.Context, h Header, txs []Tx) (Block, error) {
	start := time.Now()
	mined, attempts, err := Mine(ctx, h)
	if err != nil {
		return Block{Attempts: attempts}, err
	}
	return Block{
		Header:   mined,
		Txs:      txs,
		Hash:     mined.ComputeHash(),
		MinedIn:  time.Since(start),
		Attempts: attempts,
	}, nil
}

// MineFrom searches nonces start, start+stride, start+2·stride, … so several
// goroutines can mine the same header without ever trying the same nonce.
// stride must be at least 1.
func MineFrom(ctx context.Context, h Header, start, stride uint64) (Header, uint64, error) {
	buf := h.Encode()
	nonceAt := len(buf) - 8
	var tried uint64
	for nonce := start; ; nonce += stride {
		if tried%ctxCheckInterval == 0 {
			if err := ctx.Err(); err != nil {
				return Header{}, tried, err
			}
		}
		tried++
		binary.BigEndian.PutUint64(buf[nonceAt:], nonce)
		if MeetsDifficulty(sha256.Sum256(buf), h.Difficulty) {
			h.Nonce = nonce
			return h, tried, nil
		}
		if nonce > math.MaxUint64-stride {
			return Header{}, tried, ErrNonceSpaceExhausted
		}
	}
}
