package service

import (
	"context"
	"fmt"
	"time"

	"hashchain/internal/domain"
)

// TamperMode is how thoroughly an "attacker" covers their tracks after editing
// a block's data. Each mode is caught by a different validation rule — or,
// for rewrite, not caught at all.
type TamperMode string

const (
	// TamperBody edits Data only. Caught by the data_hash rule at that block.
	TamperBody TamperMode = "body"
	// TamperHeader also updates DataHash and the cached hash, but doesn't redo
	// the proof-of-work. Caught by the work rule at that block.
	TamperHeader TamperMode = "header"
	// TamperRemine re-mines the edited block, so it is valid on its own — but
	// its hash changed, so the NEXT block's prev_hash no longer matches.
	TamperRemine TamperMode = "remine"
	// TamperRewrite re-mines the edited block and every block after it. The
	// chain is valid again: the only defence is the cost of all that work.
	TamperRewrite TamperMode = "rewrite"
)

// TamperResult reports what a tamper did and what it cost.
type TamperResult struct {
	Mode       TamperMode
	Height     uint64
	Remined    int           // blocks re-mined (remine: 1, rewrite: tip-height+1)
	Attempts   uint64        // hashes spent re-mining
	Took       time.Duration // time spent re-mining
	Originally time.Duration // how long this node originally took to mine those blocks
}

// Tamper edits the data of the block at height, then covers its tracks according to mode.
func (s *ChainService) Tamper(ctx context.Context, height uint64, data []byte, mode TamperMode) (TamperResult, error) {
	switch mode {
	case TamperBody, TamperHeader, TamperRemine, TamperRewrite:
	default:
		return TamperResult{}, domain.InputError{Message: fmt.Sprintf(
			"mode must be one of %q, %q, %q, %q", TamperBody, TamperHeader, TamperRemine, TamperRewrite)}
	}

	// Hold the mining slot for every mode, so no new block is appended on top of
	// a tip we are in the middle of rewriting.
	if err := s.acquireSlot(ctx); err != nil {
		return TamperResult{}, err
	}
	defer s.releaseSlot()

	blocks := s.repo.All()
	if height >= uint64(len(blocks)) {
		return TamperResult{}, domain.ErrNotFound
	}
	res := TamperResult{Mode: mode, Height: height}

	b := blocks[height]
	switch mode {
	case TamperBody:
		b.Data = data
		return res, s.repo.ReplaceFrom(height, []domain.Block{b})

	case TamperHeader:
		b.Data = data
		b.DataHash = domain.HashData(data)
		b.Hash = b.ComputeHash()
		return res, s.repo.ReplaceFrom(height, []domain.Block{b})
	}

	// remine / rewrite: redo the proof-of-work.
	end := height + 1
	if mode == TamperRewrite {
		end = uint64(len(blocks))
	}

	start := time.Now()
	rewritten := make([]domain.Block, 0, end-height)
	prevHash := b.PrevHash // the block before `height` is untouched
	for i := height; i < end; i++ {
		orig := blocks[i]
		payload := orig.Data
		if i == height {
			payload = data
		}
		// Keep height, timestamp, and difficulty so the difficulty rule still
		// holds; re-link to the (possibly re-mined) parent and search a new nonce.
		header := orig.Header
		header.PrevHash = prevHash

		nb, err := domain.MineBlock(ctx, header, payload)
		s.totalHashes.Add(nb.Attempts)
		res.Attempts += nb.Attempts
		if err != nil {
			// Nothing has been written yet: a cancelled rewrite leaves the chain as it was.
			return res, err
		}
		res.Originally += orig.MinedIn
		rewritten = append(rewritten, nb)
		prevHash = nb.Hash
	}
	res.Remined = len(rewritten)
	res.Took = time.Since(start)
	return res, s.repo.ReplaceFrom(height, rewritten)
}
