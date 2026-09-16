package domain

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math/bits"
	"slices"
	"time"
)

// Params are the consensus rules. Every node of a network must use identical
// values; a node with a different BlockReward would compute a different state
// root for every block and reject the whole chain.
//
// Node-local policy (mempool size, how far ahead nonces may queue, the miner's
// own address) is deliberately NOT here — nodes may differ on those.
type Params struct {
	ChainID      uint64
	Difficulty   uint8              // fixed proof-of-work difficulty
	BlockReward  uint64             // new coins created per block, paid to the miner
	MinFee       uint64             // a tx paying less is invalid
	MaxBlockTxs  int                // transactions per block
	GenesisAlloc map[Address]uint64 // balances that exist before block 1
}

// Fingerprint commits to every consensus parameter in 32 bytes. A node stores
// it next to the chain and refuses to start with different params: the genesis
// hash alone only covers the allocation and difficulty, not the reward, fee, or
// chain ID. (geth does the same by storing its chain config in the database.)
func (p Params) Fingerprint() Hash {
	var buf []byte
	buf = binary.BigEndian.AppendUint64(buf, p.ChainID)
	buf = append(buf, p.Difficulty)
	buf = binary.BigEndian.AppendUint64(buf, p.BlockReward)
	buf = binary.BigEndian.AppendUint64(buf, p.MinFee)
	buf = binary.BigEndian.AppendUint64(buf, uint64(p.MaxBlockTxs))

	addrs := make([]Address, 0, len(p.GenesisAlloc))
	for addr := range p.GenesisAlloc {
		addrs = append(addrs, addr)
	}
	slices.SortFunc(addrs, func(a, b Address) int { return bytes.Compare(a[:], b[:]) })
	for _, addr := range addrs {
		buf = append(buf, addr[:]...)
		buf = binary.BigEndian.AppendUint64(buf, p.GenesisAlloc[addr])
	}
	return SHA256(buf)
}

// GenesisTimestamp is fixed so genesis is deterministic for given Params.
var GenesisTimestamp = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()

// GenesisState builds the initial state from the allocation.
func GenesisState(p Params) (State, error) {
	s := make(State, len(p.GenesisAlloc))
	var total uint64
	for addr, amount := range p.GenesisAlloc {
		var carry uint64
		if total, carry = bits.Add64(total, amount, 0); carry != 0 {
			return nil, fmt.Errorf("genesis allocation overflows uint64")
		}
		s.set(addr, Account{Balance: amount})
	}
	return s, nil
}

// NewGenesis mines block 0: no parent, no transactions, no reward — just the
// allocation, committed through StateRoot. Same Params → same genesis hash.
func NewGenesis(ctx context.Context, p Params) (Block, State, error) {
	state, err := GenesisState(p)
	if err != nil {
		return Block{}, nil, err
	}
	b, err := Seal(ctx, Header{
		Height:     0,
		Timestamp:  GenesisTimestamp,
		StateRoot:  state.Root(),
		Difficulty: p.Difficulty,
	}, nil)
	if err != nil {
		return Block{}, nil, err
	}
	return b, state, nil
}

// ExpectedSupply is how many coins must exist after the block at height:
// the genesis allocation plus one reward per mined block. Fees only move coins.
func ExpectedSupply(p Params, height uint64) (uint64, bool) {
	var alloc uint64
	for _, amount := range p.GenesisAlloc {
		var carry uint64
		if alloc, carry = bits.Add64(alloc, amount, 0); carry != 0 {
			return 0, false
		}
	}
	hi, rewards := bits.Mul64(p.BlockReward, height)
	total, carry := bits.Add64(alloc, rewards, 0)
	return total, hi == 0 && carry == 0
}
