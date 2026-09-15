package domain

import "encoding/hex"

// Hash is a 32-byte SHA-256 digest. A named array type (not a slice) so it is
// comparable with ==, usable as a map key, and copied by value.
type Hash [32]byte

// String renders the hash as 64 lowercase hex characters.
func (h Hash) String() string { return hex.EncodeToString(h[:]) }

// IsZero reports whether every byte is zero — the genesis block's PrevHash.
func (h Hash) IsZero() bool { return h == Hash{} }
