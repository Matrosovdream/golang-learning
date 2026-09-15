package domain

import (
	"context"
	"errors"
	"testing"
	"time"
)

var testParams = Params{InitialDifficulty: 12, MinDifficulty: 8, MaxDifficulty: 16, TargetBlockTime: time.Second}

// buildChain mines a small valid chain with fixed timestamps, so every run
// produces the same blocks and the same nonces.
func buildChain(t *testing.T, n int) []Block {
	t.Helper()
	ctx := context.Background()
	genesis, err := NewGenesis(ctx, testParams)
	if err != nil {
		t.Fatal(err)
	}
	chain := []Block{genesis}
	for i := 1; i < n; i++ {
		prev := chain[i-1]
		b, err := MineBlock(ctx, Header{
			Height:     uint64(i),
			Timestamp:  GenesisTimestamp + int64(i)*1000,
			PrevHash:   prev.Hash,
			Difficulty: testParams.InitialDifficulty,
		}, []byte("block data"))
		if err != nil {
			t.Fatal(err)
		}
		chain = append(chain, b)
	}
	return chain
}

// remine redoes the proof-of-work of chain[i] after its header was changed.
func remine(t *testing.T, chain []Block, i int) {
	t.Helper()
	b, err := MineBlock(context.Background(), chain[i].Header, chain[i].Data)
	if err != nil {
		t.Fatal(err)
	}
	chain[i] = b
}

func TestValidChainPasses(t *testing.T) {
	if err := ValidateChain(testParams, buildChain(t, 6)); err != nil {
		t.Fatalf("valid chain rejected: %v", err)
	}
}

func TestValidateCatchesTampering(t *testing.T) {
	tests := []struct {
		name       string
		tamper     func(t *testing.T, c []Block)
		wantHeight uint64
		wantRule   string
	}{
		{
			name:       "body edited, header untouched",
			tamper:     func(t *testing.T, c []Block) { c[3].Data = []byte("evil") },
			wantHeight: 3, wantRule: RuleDataHash,
		},
		{
			name: "header updated, cached hash not",
			tamper: func(t *testing.T, c []Block) {
				c[3].Data = []byte("evil")
				c[3].DataHash = HashData(c[3].Data)
			},
			wantHeight: 3, wantRule: RuleStaleHash,
		},
		{
			name: "header and hash updated, no new proof-of-work",
			tamper: func(t *testing.T, c []Block) {
				c[3].Data = []byte("evil")
				c[3].DataHash = HashData(c[3].Data)
				c[3].Hash = c[3].ComputeHash()
			},
			wantHeight: 3, wantRule: RuleWork,
		},
		{
			name: "block re-mined: the next block's link breaks",
			tamper: func(t *testing.T, c []Block) {
				c[3].Data = []byte("evil")
				remine(t, c, 3)
			},
			wantHeight: 4, wantRule: RulePrevHash,
		},
		{
			name: "difficulty lowered and re-mined",
			tamper: func(t *testing.T, c []Block) {
				c[5].Difficulty = 1
				remine(t, c, 5)
			},
			wantHeight: 5, wantRule: RuleDifficulty,
		},
		{
			name: "timestamp before parent",
			tamper: func(t *testing.T, c []Block) {
				c[5].Timestamp = c[4].Timestamp - 1
				remine(t, c, 5)
			},
			wantHeight: 5, wantRule: RuleTimestamp,
		},
		{
			name:       "height skipped",
			tamper:     func(t *testing.T, c []Block) { c[5].Height = 9; remine(t, c, 5) },
			wantHeight: 5, wantRule: RuleHeight,
		},
		{
			name: "genesis with a parent",
			tamper: func(t *testing.T, c []Block) {
				c[0].PrevHash[0] = 1
				remine(t, c, 0)
			},
			wantHeight: 0, wantRule: RulePrevHash,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := buildChain(t, 6)
			tt.tamper(t, chain)

			err := ValidateChain(testParams, chain)
			var invalid *InvalidBlockError
			if !errors.As(err, &invalid) {
				t.Fatalf("err = %v, want *InvalidBlockError", err)
			}
			if invalid.Height != tt.wantHeight || invalid.Rule != tt.wantRule {
				t.Fatalf("got height %d rule %q (%s), want height %d rule %q",
					invalid.Height, invalid.Rule, invalid.Reason, tt.wantHeight, tt.wantRule)
			}
		})
	}
}

func TestRewritingEverythingAfterTheEditIsValid(t *testing.T) {
	chain := buildChain(t, 6)
	chain[3].Data = []byte("evil")
	remine(t, chain, 3)
	for i := 4; i < len(chain); i++ {
		chain[i].PrevHash = chain[i-1].Hash
		remine(t, chain, i)
	}
	if err := ValidateChain(testParams, chain); err != nil {
		t.Fatalf("fully re-mined chain should be valid, got %v", err)
	}
}
