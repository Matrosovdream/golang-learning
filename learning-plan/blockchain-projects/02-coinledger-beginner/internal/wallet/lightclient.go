package wallet

import (
	"encoding/hex"
	"fmt"
	"strings"

	"coinledger/internal/domain"
)

// Verification is what a light client could establish on its own.
type Verification struct {
	Header       domain.Header
	BlockHash    domain.Hash // computed here from HeaderRaw, not taken from the node
	LeadingZeros int
	WorkOK       bool // the hash meets the header's difficulty, and that difficulty ≥ the minimum we accept
	HeightOK     bool // the header is at the height the node claimed
	ProofOK      bool // the Merkle path leads from the tx to the header's TxRoot
}

// Valid reports whether every check passed.
func (v Verification) Valid() bool { return v.WorkOK && v.HeightOK && v.ProofOK }

// VerifyInclusion checks a proof the way a light client would: using only the
// 141-byte header and the sibling hashes — never the block's other
// transactions, and never believing a hash or root the node merely states.
//
// What it proves: SOME header with real proof-of-work commits to this tx.
// What it doesn't: that this header is part of the chain everyone else
// follows. A real light client downloads the whole header chain (80 bytes per
// block in Bitcoin) and follows the one with the most work. At this project's
// low difficulty, forging a single header takes a laptop milliseconds.
func VerifyInclusion(p Proof, txHash domain.Hash, minDifficulty uint8) (Verification, error) {
	raw, err := hex.DecodeString(strings.TrimPrefix(p.HeaderRaw, "0x"))
	if err != nil {
		return Verification{}, fmt.Errorf("header_raw: %w", err)
	}
	header, err := domain.DecodeHeader(raw)
	if err != nil {
		return Verification{}, err
	}

	steps := make([]domain.ProofStep, len(p.Steps))
	for i, s := range p.Steps {
		sibling, err := domain.ParseHash(s.Sibling)
		if err != nil {
			return Verification{}, err
		}
		if s.Position != "left" && s.Position != "right" {
			return Verification{}, fmt.Errorf("step %d: position %q", i, s.Position)
		}
		steps[i] = domain.ProofStep{Sibling: sibling, Left: s.Position == "left"}
	}

	hash := header.ComputeHash()
	return Verification{
		Header:       header,
		BlockHash:    hash,
		LeadingZeros: domain.LeadingZeroBits(hash),
		WorkOK:       header.Difficulty >= minDifficulty && domain.MeetsDifficulty(hash, header.Difficulty),
		HeightOK:     header.Height == p.Height,
		ProofOK:      domain.VerifyProof(txHash, steps, header.TxRoot),
	}, nil
}
