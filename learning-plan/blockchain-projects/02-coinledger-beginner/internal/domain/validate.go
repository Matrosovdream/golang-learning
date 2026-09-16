package domain

import (
	"fmt"
)

// Rule names reported by InvalidBlockError.
const (
	RuleHeight     = "height"
	RuleStaleHash  = "stale_hash"
	RulePrevHash   = "prev_hash"
	RuleTimestamp  = "timestamp"
	RuleDifficulty = "difficulty"
	RuleWork       = "work"
	RuleTxRoot     = "tx_root"
	RuleTxs        = "txs"
	RuleStateRoot  = "state_root"
)

// InvalidBlockError describes the first rule a block breaks.
type InvalidBlockError struct {
	Height uint64
	Rule   string
	Reason string
	Err    error // the underlying transaction error for RuleTxs
}

func (e *InvalidBlockError) Error() string {
	return fmt.Sprintf("block %d is invalid (%s): %s", e.Height, e.Rule, e.Reason)
}

func (e *InvalidBlockError) Unwrap() error { return e.Err }

// ValidateBlock checks b as the child of parent and executes it on parentState.
// On success it returns the state after b. parent is nil for genesis.
//
// Header rules first (cheap), then the expensive part: re-running every
// transaction. The block producer's claims — "these are the txs" (TxRoot) and
// "this is the resulting state" (StateRoot) — are recomputed, not believed.
func ValidateBlock(p Params, parent *Block, parentState State, b Block) (State, error) {
	fail := func(rule, format string, args ...any) error {
		return &InvalidBlockError{Height: b.Height, Rule: rule, Reason: fmt.Sprintf(format, args...)}
	}

	wantHeight := uint64(0)
	if parent != nil {
		wantHeight = parent.Height + 1
	}
	if b.Height != wantHeight {
		return nil, fail(RuleHeight, "header says height %d, want %d", b.Height, wantHeight)
	}

	hash := b.ComputeHash()
	if hash != b.Hash {
		return nil, fail(RuleStaleHash, "stored hash does not match the header")
	}

	if parent == nil {
		if !b.PrevHash.IsZero() {
			return nil, fail(RulePrevHash, "genesis must have an all-zero prev_hash")
		}
		if len(b.Txs) != 0 {
			return nil, fail(RuleTxs, "genesis must not contain transactions")
		}
	} else {
		if b.PrevHash != parent.ComputeHash() {
			return nil, fail(RulePrevHash, "prev_hash does not match block %d", parent.Height)
		}
		if b.Timestamp < parent.Timestamp {
			return nil, fail(RuleTimestamp, "timestamp is earlier than the parent's")
		}
	}

	if b.Difficulty != p.Difficulty {
		return nil, fail(RuleDifficulty, "header claims difficulty %d, the chain requires %d", b.Difficulty, p.Difficulty)
	}
	if zeros := LeadingZeroBits(hash); zeros < int(p.Difficulty) {
		return nil, fail(RuleWork, "hash has %d leading zero bits, need %d", zeros, p.Difficulty)
	}

	if root := ComputeTxRoot(b.Txs); root != b.TxRoot {
		return nil, fail(RuleTxRoot, "tx_root %s… does not match the transactions (%s…)", b.TxRoot.Hex()[:12], root.Hex()[:12])
	}

	// Execute. Genesis has no transactions and no reward: its state is the allocation.
	var next State
	if parent == nil {
		gs, err := GenesisState(p)
		if err != nil {
			return nil, fail(RuleTxs, "%v", err)
		}
		next = gs
	} else {
		s, err := ApplyBlock(p, parentState, b)
		if err != nil {
			return nil, &InvalidBlockError{Height: b.Height, Rule: RuleTxs, Reason: err.Error(), Err: err}
		}
		next = s
	}

	if root := next.Root(); root != b.StateRoot {
		return nil, fail(RuleStateRoot, "state_root %s… does not match the executed state (%s…)", b.StateRoot.Hex()[:12], root.Hex()[:12])
	}
	return next, nil
}
