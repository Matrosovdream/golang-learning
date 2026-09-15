package domain

import (
	"testing"
	"time"
)

// headersSpaced returns a headerAt func for a chain whose blocks (after
// genesis) are spacingMS apart and all have the given difficulty.
func headersSpaced(spacingMS int64, difficulty uint8) func(uint64) Header {
	return func(h uint64) Header {
		return Header{Height: h, Timestamp: int64(h) * spacingMS, Difficulty: difficulty}
	}
}

func TestNextDifficulty(t *testing.T) {
	p := Params{
		InitialDifficulty: 20,
		MinDifficulty:     10,
		MaxDifficulty:     24,
		RetargetInterval:  10,
		TargetBlockTime:   1000 * time.Millisecond,
	}
	fixed := p
	fixed.RetargetInterval = 0

	tests := []struct {
		name      string
		params    Params
		height    uint64
		spacingMS int64
		prevDiff  uint8
		want      uint8
	}{
		{"genesis uses initial", p, 0, 1000, 0, 20},
		{"retargeting off keeps difficulty", fixed, 30, 1, 20, 20},
		{"not a retarget height", p, 25, 1, 20, 20},
		{"first window would touch genesis", p, 10, 1, 20, 20},
		{"on target: no change", p, 20, 1000, 20, 20},
		{"1.9x fast: dead band", p, 20, 526, 20, 20},
		{"2x fast: +1", p, 20, 500, 20, 21},
		{"4x fast: +2", p, 20, 250, 20, 22},
		{"100x fast: still only +2", p, 20, 10, 20, 22},
		{"2x slow: -1", p, 20, 2000, 20, 19},
		{"4x slow: -2", p, 20, 4000, 20, 18},
		{"clamped to max", p, 20, 1, 23, 24},
		{"clamped to min", p, 20, 100_000, 11, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NextDifficulty(tt.params, tt.height, headersSpaced(tt.spacingMS, tt.prevDiff))
			if got != tt.want {
				t.Fatalf("NextDifficulty = %d, want %d", got, tt.want)
			}
		})
	}
}
