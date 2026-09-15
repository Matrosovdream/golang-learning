package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"time"
)

// HeaderSize is the exact length of an encoded header. Every field is fixed
// width, so every header encodes to the same number of bytes.
const HeaderSize = 8 + 8 + 32 + 32 + 1 + 8 // height, timestamp, prev, data hash, difficulty, nonce

// Header is the part of a block that gets hashed and mined.
//
// The block's variable-length Data is NOT in the header — only its SHA-256
// (DataHash) is. That keeps the header small and fixed-size, and it still
// commits to the data: change one byte of Data and DataHash no longer matches.
// Real chains do the same with a Merkle root of the transactions (Project 2).
type Header struct {
	Height     uint64 // position in the chain; genesis = 0
	Timestamp  int64  // Unix milliseconds, set when mining starts
	PrevHash   Hash   // hash pointer: the previous block's header hash
	DataHash   Hash   // SHA-256 of Data — commits the body into the header
	Difficulty uint8  // required number of leading zero bits in this header's hash
	Nonce      uint64 // the number the miner varies; encoded LAST (see Mine)
}

// Block is a header, its body, and some local bookkeeping.
type Block struct {
	Header

	// Data is the body: arbitrary bytes. In Project 2 it becomes a list of
	// signed transactions. Treat it as immutable — replace the slice, never
	// write into it, because copies of a Block share the same backing array.
	Data []byte

	// Hash caches Header.ComputeHash() for lookups and display. It is never
	// trusted: validation always recomputes it from the header.
	Hash Hash

	// MinedIn and Attempts are local metadata about how THIS node mined the
	// block. They are not part of the header, not hashed, and another node
	// would have different numbers.
	MinedIn  time.Duration
	Attempts uint64
}

// Encode serializes the header into its canonical byte form: fixed-width
// big-endian integers in a fixed order, no field names, no separators.
//
// "Canonical" is the whole point. Two nodes hashing the same block must feed
// SHA-256 byte-for-byte identical input, or they compute different hashes and
// disagree about the chain. That is why this is hand-written instead of
// json.Marshal: JSON's field order, spacing, and escaping are not a contract.
func (h Header) Encode() []byte {
	buf := make([]byte, 0, HeaderSize)
	buf = binary.BigEndian.AppendUint64(buf, h.Height)
	buf = binary.BigEndian.AppendUint64(buf, uint64(h.Timestamp))
	buf = append(buf, h.PrevHash[:]...)
	buf = append(buf, h.DataHash[:]...)
	buf = append(buf, h.Difficulty)
	buf = binary.BigEndian.AppendUint64(buf, h.Nonce)
	return buf
}

// ComputeHash returns SHA-256 of the canonical header encoding: the block hash.
func (h Header) ComputeHash() Hash { return sha256.Sum256(h.Encode()) }

// HashData returns the SHA-256 of a block body, for Header.DataHash.
func HashData(data []byte) Hash { return sha256.Sum256(data) }
