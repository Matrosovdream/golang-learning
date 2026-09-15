package domain

import "time"

// Params are the consensus rules of the chain. Every node must use the same
// values: a node with a different TargetBlockTime would expect different
// difficulties and reject everyone else's blocks.
type Params struct {
	InitialDifficulty uint8         // difficulty of genesis (and of every block if retargeting is off)
	MinDifficulty     uint8         // retargeting never goes below this
	MaxDifficulty     uint8         // ...or above this
	RetargetInterval  uint64        // retarget every N blocks; 0 = fixed difficulty. Must be 0 or >= 2
	TargetBlockTime   time.Duration // the block time retargeting aims for
}

// NextDifficulty returns the difficulty the block at `height` must have.
//
// It is a pure function of the chain's history (read through headerAt), so the
// miner and every validator compute the same answer. The rule, Bitcoin-style:
//
//   - Only at heights that are multiples of RetargetInterval (N); otherwise the
//     difficulty stays the same as the previous block's.
//   - Look at the last N block intervals and compare how long they actually took
//     with how long they should have taken (N × TargetBlockTime).
//   - One bit doubles the work, so: 2× too fast → +1 bit, 4× too fast → +2 bits,
//     2× too slow → −1, 4× too slow → −2. Anything in between → no change.
//     This dead band keeps random block times from making difficulty wobble.
//   - Clamp to [MinDifficulty, MaxDifficulty].
//
// Integers only. Consensus code avoids floating point so that no rounding
// difference between machines can ever split the network.
func NextDifficulty(p Params, height uint64, headerAt func(height uint64) Header) uint8 {
	if height == 0 {
		return p.InitialDifficulty
	}
	prev := headerAt(height - 1)

	n := p.RetargetInterval
	// The window needs N intervals = N+1 blocks, and must not start at genesis
	// (its timestamp is a fixed date, not a real mining time). The first height
	// that satisfies both is 2N.
	if n == 0 || height%n != 0 || height < 2*n {
		return prev.Difficulty
	}

	first := headerAt(height - 1 - n)
	actual := prev.Timestamp - first.Timestamp              // milliseconds over N intervals
	expected := int64(n) * p.TargetBlockTime.Milliseconds() // what N intervals should take

	next := int(prev.Difficulty)
	switch {
	case actual*4 <= expected:
		next += 2
	case actual*2 <= expected:
		next++
	case actual >= expected*4:
		next -= 2
	case actual >= expected*2:
		next--
	}
	return uint8(min(max(next, int(p.MinDifficulty)), int(p.MaxDifficulty)))
}
