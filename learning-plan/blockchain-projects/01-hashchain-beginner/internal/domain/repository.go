package domain

import "errors"

var (
	// ErrNotFound is returned when a block height does not exist.
	ErrNotFound = errors.New("block not found")
	// ErrTipChanged is returned when an appended block does not extend the current tip.
	ErrTipChanged = errors.New("block does not extend the current tip")
)

// InputError signals invalid client input; the handler maps it to HTTP 400.
type InputError struct{ Message string }

func (e InputError) Error() string { return e.Message }

// ChainRepository stores the chain. The in-memory implementation is
// concurrency-safe: HTTP handlers read while the miner appends.
type ChainRepository interface {
	// Append adds b on top of the tip. It returns ErrTipChanged unless b's
	// height and PrevHash extend the current tip exactly.
	Append(b Block) error
	// Tip returns the last block, or ErrNotFound for an empty chain.
	Tip() (Block, error)
	// Get returns the block at a height, or ErrNotFound.
	Get(height uint64) (Block, error)
	// Range returns up to limit blocks starting at height from.
	Range(from uint64, limit int) []Block
	// All returns a copy of the whole chain, genesis first.
	All() []Block
	// ReplaceFrom overwrites blocks in place starting at height, without any
	// checks. It exists only for the tamper endpoint — a real node never edits history.
	ReplaceFrom(height uint64, blocks []Block) error
}
