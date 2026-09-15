package domain

import (
	"context"
	"time"
)

// Genesis block contents. Fixed values make genesis deterministic: every node
// started with the same Params mines the exact same genesis block and hash.
// (Real networks go further and hard-code the genesis hash into the software.)
var (
	GenesisTimestamp = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	GenesisData      = []byte("hashchain genesis block")
)

// NewGenesis mines block 0: no parent (zero PrevHash), fixed data and time.
func NewGenesis(ctx context.Context, p Params) (Block, error) {
	return MineBlock(ctx, Header{
		Height:     0,
		Timestamp:  GenesisTimestamp,
		Difficulty: p.InitialDifficulty,
	}, GenesisData)
}
