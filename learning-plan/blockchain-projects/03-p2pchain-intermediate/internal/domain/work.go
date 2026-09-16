package domain

import "math/big"

// Work is how much proof-of-work a block or a chain represents.
//
// A block with difficulty d takes 2^d hash attempts on average, so that is its
// work. A chain's work is the sum over its blocks — this is the number the fork
// choice rule compares, NOT the number of blocks.
//
// It is a big.Int because real chains overflow uint64 quickly: Bitcoin's
// accumulated work passed 2^93 years ago. Work values are treated as immutable:
// helpers always return new values.
type Work = *big.Int

// BlockWork is the work of a single block at the given difficulty: 2^difficulty.
func BlockWork(difficulty uint8) Work {
	return new(big.Int).Lsh(big.NewInt(1), uint(difficulty))
}

// AddWork returns total + BlockWork(difficulty).
func AddWork(total Work, difficulty uint8) Work {
	return new(big.Int).Add(total, BlockWork(difficulty))
}

// ZeroWork is the work of an empty chain.
func ZeroWork() Work { return new(big.Int) }

// MoreWork reports whether a is strictly heavier than b.
//
// Strictly: on an exact tie the chain we already have wins. Every node applies
// that rule, so a tie leaves each node on whichever block it saw first — and
// the tie is broken by whoever mines the next block.
func MoreWork(a, b Work) bool { return a.Cmp(b) > 0 }
