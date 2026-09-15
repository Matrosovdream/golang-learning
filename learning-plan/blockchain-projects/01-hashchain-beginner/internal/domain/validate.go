package domain

import "fmt"

// Rule names reported by InvalidBlockError, one per validation check.
const (
	RuleHeight     = "height"
	RuleDataHash   = "data_hash"
	RuleStaleHash  = "stale_hash"
	RulePrevHash   = "prev_hash"
	RuleTimestamp  = "timestamp"
	RuleDifficulty = "difficulty"
	RuleWork       = "work"
)

// InvalidBlockError describes the first rule a block breaks.
type InvalidBlockError struct {
	Height uint64
	Rule   string // one of the Rule* constants
	Reason string // human-readable detail
}

func (e *InvalidBlockError) Error() string {
	return fmt.Sprintf("block %d is invalid (%s): %s", e.Height, e.Rule, e.Reason)
}

// ValidateChain checks every block from genesis to tip and returns the first
// problem as *InvalidBlockError, or nil if the whole chain is valid.
//
// It trusts nothing that was stored: hashes are recomputed from headers, links
// are checked against those recomputed hashes, and the expected difficulty is
// recomputed from history. This is what a node does with blocks it receives.
func ValidateChain(p Params, blocks []Block) error {
	headerAt := func(h uint64) Header { return blocks[h].Header }
	for i := range blocks {
		var prev *Block
		if i > 0 {
			prev = &blocks[i-1]
		}
		if err := ValidateBlock(p, blocks[i], prev, uint64(i), headerAt); err != nil {
			return err
		}
	}
	return nil
}

// ValidateBlock checks one block against its parent (nil for genesis).
// wantHeight is the position the block should occupy; headerAt must return
// headers for every height below it (NextDifficulty reads them).
func ValidateBlock(p Params, b Block, prev *Block, wantHeight uint64, headerAt func(uint64) Header) error {
	fail := func(rule, format string, args ...any) error {
		return &InvalidBlockError{Height: wantHeight, Rule: rule, Reason: fmt.Sprintf(format, args...)}
	}

	// 1. It sits where it claims to.
	if b.Height != wantHeight {
		return fail(RuleHeight, "header says height %d", b.Height)
	}

	// 2. The body is the one the header committed to.
	if HashData(b.Data) != b.DataHash {
		return fail(RuleDataHash, "data does not match the header's data_hash (the body was edited)")
	}

	// 3. The cached hash is the real hash of this header.
	hash := b.ComputeHash()
	if hash != b.Hash {
		return fail(RuleStaleHash, "stored hash %s… but the header hashes to %s… (the header was edited)",
			b.Hash.String()[:12], hash.String()[:12])
	}

	// 4. The hash pointer links to the parent — compared with the parent's
	//    RECOMPUTED hash, so a tampered parent is caught here too.
	if prev == nil {
		if !b.PrevHash.IsZero() {
			return fail(RulePrevHash, "genesis must have an all-zero prev_hash")
		}
	} else {
		if parent := prev.ComputeHash(); b.PrevHash != parent {
			return fail(RulePrevHash, "prev_hash %s… does not match block %d's hash %s…",
				b.PrevHash.String()[:12], prev.Height, parent.String()[:12])
		}
		// 5. Time does not run backwards. (Bitcoin uses the median of the last
		//    11 blocks and rejects blocks >2h in the future; this is the simple version.)
		if b.Timestamp < prev.Timestamp {
			return fail(RuleTimestamp, "timestamp %d is earlier than its parent's %d", b.Timestamp, prev.Timestamp)
		}
	}

	// 6. It claims the difficulty the rules require at this height.
	if want := NextDifficulty(p, wantHeight, headerAt); b.Difficulty != want {
		return fail(RuleDifficulty, "header claims difficulty %d, the rules require %d", b.Difficulty, want)
	}

	// 7. The work was actually done.
	if zeros := LeadingZeroBits(hash); zeros < int(b.Difficulty) {
		return fail(RuleWork, "hash has %d leading zero bits, difficulty requires %d", zeros, b.Difficulty)
	}
	return nil
}
