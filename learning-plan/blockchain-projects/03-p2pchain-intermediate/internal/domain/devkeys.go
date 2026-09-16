package domain

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"fmt"

	"github.com/ethereum/go-ethereum/crypto"
)

// Dev accounts: deterministic keys so every node in a test network starts from
// the same genesis allocation and tools can sign without a keystore.
//
// ⚠️ These private keys are derived from a published formula: anyone can
// compute them. They exist for local demos. Never send real funds to them.
// (Anvil and Hardhat ship the same idea with their published mnemonics.)

// DevKey returns the i-th development private key.
func DevKey(i int) (*ecdsa.PrivateKey, error) {
	seed := sha256.Sum256(fmt.Appendf(nil, "p2pchain dev key %d", i))
	return crypto.ToECDSA(seed[:])
}

// DevAddress returns the address of the i-th development key.
func DevAddress(i int) (Address, error) {
	key, err := DevKey(i)
	if err != nil {
		return Address{}, err
	}
	return crypto.PubkeyToAddress(key.PublicKey), nil
}

// DevAlloc builds a genesis allocation giving each of the first n dev accounts
// the same balance. It is part of the consensus params, so every node must use
// the same numbers.
func DevAlloc(n int, balance uint64) (map[Address]uint64, error) {
	alloc := make(map[Address]uint64, n)
	for i := range n {
		addr, err := DevAddress(i)
		if err != nil {
			return nil, err
		}
		alloc[addr] = balance
	}
	return alloc, nil
}
