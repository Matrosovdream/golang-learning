package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"hashchain/internal/domain"
	"hashchain/internal/repository/memory"
)

var fastParams = domain.Params{InitialDifficulty: 10, MinDifficulty: 8, MaxDifficulty: 14, TargetBlockTime: time.Second}

func newService(t *testing.T, p domain.Params) *ChainService {
	t.Helper()
	svc, err := NewChainService(context.Background(), memory.NewChainStore(), p)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func mustMine(t *testing.T, svc *ChainService, n int) {
	t.Helper()
	for range n {
		if _, err := svc.MineBlock(context.Background(), []byte("data")); err != nil {
			t.Fatal(err)
		}
	}
}

func height(t *testing.T, svc *ChainService) uint64 {
	t.Helper()
	tip, err := svc.Latest()
	if err != nil {
		t.Fatal(err)
	}
	return tip.Height
}

func TestMineBlockExtendsAValidChain(t *testing.T) {
	svc := newService(t, fastParams)
	mustMine(t, svc, 5)
	if h := height(t, svc); h != 5 {
		t.Fatalf("height = %d, want 5", h)
	}
	if rep := svc.Validate(); !rep.Valid {
		t.Fatalf("chain invalid: %+v", rep.Invalid)
	}
}

func TestMineBlockHonoursCancelledContext(t *testing.T) {
	svc := newService(t, fastParams)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := svc.MineBlock(ctx, []byte("x")); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if h := height(t, svc); h != 0 {
		t.Fatalf("height = %d, want 0 (nothing appended)", h)
	}
}

// Blocks mined instantly against a one-hour target are "way too fast", so
// every retarget adds 2 bits until MaxDifficulty — and the validator, running
// the same rule, agrees with the miner at every height.
func TestRetargetingRaisesDifficultyAndStaysValid(t *testing.T) {
	p := fastParams
	p.RetargetInterval = 4
	p.TargetBlockTime = time.Hour
	svc := newService(t, p)
	mustMine(t, svc, 13)

	want := map[uint64]uint8{7: 10, 8: 12, 11: 12, 12: 14}
	for h, d := range want {
		b, _ := svc.Block(h)
		if b.Difficulty != d {
			t.Errorf("block %d difficulty = %d, want %d", h, b.Difficulty, d)
		}
	}
	if rep := svc.Validate(); !rep.Valid {
		t.Fatalf("chain invalid: %+v", rep.Invalid)
	}
}

func TestStatsAverageBlockTime(t *testing.T) {
	svc := newService(t, fastParams)
	clock := time.UnixMilli(domain.GenesisTimestamp)
	svc.now = func() time.Time { clock = clock.Add(1500 * time.Millisecond); return clock }

	for _, tt := range []struct {
		mine       int
		wantWindow int
	}{
		{1, 0},   // height 1: nothing to average yet
		{4, 4},   // height 5: blocks 1..5 (fewer than avgWindow — must not wrap around)
		{10, 10}, // height 15: the last 10 intervals
	} {
		mustMine(t, svc, tt.mine)
		st, err := svc.Stats()
		if err != nil {
			t.Fatal(err)
		}
		if st.AvgWindow != tt.wantWindow {
			t.Fatalf("height %d: window = %d, want %d", st.Height, st.AvgWindow, tt.wantWindow)
		}
		if tt.wantWindow > 0 && st.AvgBlockTime != 1500*time.Millisecond {
			t.Fatalf("height %d: avg block time = %s, want 1.5s", st.Height, st.AvgBlockTime)
		}
	}
}

func TestAutoMinerStartStop(t *testing.T) {
	svc := newService(t, fastParams)
	if err := svc.StartAutoMiner(0); err != nil {
		t.Fatal(err)
	}
	if err := svc.StartAutoMiner(0); !errors.Is(err, ErrMinerRunning) {
		t.Fatalf("second start: err = %v, want ErrMinerRunning", err)
	}

	waitFor(t, func() bool { return height(t, svc) >= 3 })

	if !svc.StopAutoMiner() {
		t.Fatal("StopAutoMiner reported no running miner")
	}
	stoppedAt := height(t, svc)
	time.Sleep(20 * time.Millisecond)
	if h := height(t, svc); h != stoppedAt {
		t.Fatalf("height moved from %d to %d after StopAutoMiner returned", stoppedAt, h)
	}
	if svc.StopAutoMiner() {
		t.Fatal("second StopAutoMiner should report false")
	}
	if rep := svc.Validate(); !rep.Valid {
		t.Fatalf("chain invalid: %+v", rep.Invalid)
	}
}

func TestAutoMinerStopsByItselfAfterMaxBlocks(t *testing.T) {
	svc := newService(t, fastParams)
	if err := svc.StartAutoMiner(3); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return !svc.AutoMinerRunning() })
	if h := height(t, svc); h != 3 {
		t.Fatalf("height = %d, want 3", h)
	}
	if err := svc.StartAutoMiner(1); err != nil { // can start again
		t.Fatalf("restart: %v", err)
	}
	waitFor(t, func() bool { return !svc.AutoMinerRunning() })
}

// Manual mining and the auto-miner share the mining slot, so their blocks
// interleave without ever forking the tip.
func TestManualAndAutoMiningInterleave(t *testing.T) {
	svc := newService(t, fastParams)
	if err := svc.StartAutoMiner(0); err != nil {
		t.Fatal(err)
	}
	mustMine(t, svc, 5)
	svc.StopAutoMiner()
	if rep := svc.Validate(); !rep.Valid {
		t.Fatalf("chain invalid: %+v", rep.Invalid)
	}
}

func TestTamperModes(t *testing.T) {
	tests := []struct {
		mode       TamperMode
		wantValid  bool
		wantHeight uint64
		wantRule   string
		wantRemine int
	}{
		{TamperBody, false, 2, domain.RuleDataHash, 0},
		{TamperHeader, false, 2, domain.RuleWork, 0},
		{TamperRemine, false, 3, domain.RulePrevHash, 1},
		{TamperRewrite, true, 0, "", 4}, // blocks 2..5
	}
	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			// Difficulty 16: a header edit passes the work rule by pure luck only
			// 1 time in 65,536 (timestamps are real, so hashes differ each run).
			p := fastParams
			p.InitialDifficulty, p.MaxDifficulty = 16, 16
			svc := newService(t, p)
			mustMine(t, svc, 5)

			res, err := svc.Tamper(context.Background(), 2, []byte("evil"), tt.mode)
			if err != nil {
				t.Fatal(err)
			}
			if res.Remined != tt.wantRemine {
				t.Fatalf("remined %d blocks, want %d", res.Remined, tt.wantRemine)
			}
			rep := svc.Validate()
			if rep.Valid != tt.wantValid {
				t.Fatalf("valid = %v, want %v (%+v)", rep.Valid, tt.wantValid, rep.Invalid)
			}
			if !tt.wantValid && (rep.Invalid.Height != tt.wantHeight || rep.Invalid.Rule != tt.wantRule) {
				t.Fatalf("got height %d rule %q, want height %d rule %q",
					rep.Invalid.Height, rep.Invalid.Rule, tt.wantHeight, tt.wantRule)
			}
			if b, _ := svc.Block(2); string(b.Data) != "evil" {
				t.Fatalf("block 2 data = %q, want %q", b.Data, "evil")
			}
		})
	}
}

func TestTamperRejectsBadInput(t *testing.T) {
	svc := newService(t, fastParams)
	var ie domain.InputError
	if _, err := svc.Tamper(context.Background(), 0, nil, "sneaky"); !errors.As(err, &ie) {
		t.Fatalf("unknown mode: err = %v, want InputError", err)
	}
	if _, err := svc.Tamper(context.Background(), 99, nil, TamperBody); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing height: err = %v, want ErrNotFound", err)
	}
}

func TestCancelledRewriteLeavesChainUntouched(t *testing.T) {
	svc := newService(t, fastParams)
	mustMine(t, svc, 3)
	before, _ := svc.Block(1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := svc.Tamper(ctx, 1, []byte("evil"), TamperRewrite); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	after, _ := svc.Block(1)
	if after.Hash != before.Hash {
		t.Fatal("a cancelled rewrite changed the chain")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met within 5s")
		}
		time.Sleep(time.Millisecond)
	}
}
