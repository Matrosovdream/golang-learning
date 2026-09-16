package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
)

// Hash is a 32-byte digest — the same type as in Project 1.
type Hash [32]byte

// String renders the hash as 64 lowercase hex characters (no 0x prefix).
func (h Hash) String() string { return hex.EncodeToString(h[:]) }

// Hex renders the hash with a 0x prefix, the way chain tools display it.
func (h Hash) Hex() string { return "0x" + h.String() }

// IsZero reports whether every byte is zero.
func (h Hash) IsZero() bool { return h == Hash{} }

// ParseHash accepts 64 hex characters, with or without a 0x prefix.
func ParseHash(s string) (Hash, error) {
	var h Hash
	b, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	if err != nil || len(b) != len(h) {
		return h, InputError{Message: fmt.Sprintf("invalid hash %q: want 64 hex characters", s)}
	}
	copy(h[:], b)
	return h, nil
}

// This project uses two hash functions, each for one family of data:
//
//   - Keccak-256 for everything inherited from Ethereum: addresses, transaction
//     signing hashes, and transaction IDs. Keys and addresses are real
//     Ethereum ones, so they must use Ethereum's hash.
//   - SHA-256 for everything inherited from Project 1: block headers, Merkle
//     trees, and the state root.

// Keccak256 hashes data with Ethereum's Keccak-256.
func Keccak256(data ...[]byte) Hash { return Hash(crypto.Keccak256Hash(data...)) }

// SHA256 hashes data with SHA-256.
func SHA256(data []byte) Hash { return sha256.Sum256(data) }
