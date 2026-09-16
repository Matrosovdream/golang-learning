package domain

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/rlp"
)

// HeaderSize is the exact length of an encoded header.
const HeaderSize = 8 + 8 + 32 + 32 + 32 + 20 + 1 + 8 // 141 bytes

// Header is what gets hashed and mined. Compared with Project 1, DataHash is
// replaced by two commitments and a miner:
//
//   - TxRoot commits to the ORDERED list of transactions (a Merkle root, so a
//     single tx can be proven without the others).
//   - StateRoot commits to every account balance and nonce AFTER this block.
//     A node that executes the block and gets a different root rejects it.
//   - Miner receives the block reward and the fees.
type Header struct {
	Height     uint64
	Timestamp  int64 // Unix milliseconds
	PrevHash   Hash
	TxRoot     Hash
	StateRoot  Hash
	Miner      Address
	Difficulty uint8
	Nonce      uint64 // encoded last: the miner patches only these 8 bytes
}

// Block is a header, the transactions it commits to, and local bookkeeping.
type Block struct {
	Header
	Txs []Tx

	Hash     Hash          // cached Header.ComputeHash(); never trusted by validation
	MinedIn  time.Duration // local metadata, not stored or hashed
	Attempts uint64        // local metadata, not stored or hashed
}

// Encode serializes the header canonically: fixed widths, fixed order, big-endian.
func (h Header) Encode() []byte {
	buf := make([]byte, 0, HeaderSize)
	buf = binary.BigEndian.AppendUint64(buf, h.Height)
	buf = binary.BigEndian.AppendUint64(buf, uint64(h.Timestamp))
	buf = append(buf, h.PrevHash[:]...)
	buf = append(buf, h.TxRoot[:]...)
	buf = append(buf, h.StateRoot[:]...)
	buf = append(buf, h.Miner[:]...)
	buf = append(buf, h.Difficulty)
	buf = binary.BigEndian.AppendUint64(buf, h.Nonce)
	return buf
}

// DecodeHeader is the inverse of Encode.
func DecodeHeader(b []byte) (Header, error) {
	if len(b) != HeaderSize {
		return Header{}, fmt.Errorf("header is %d bytes, want %d", len(b), HeaderSize)
	}
	var h Header
	h.Height = binary.BigEndian.Uint64(b[0:8])
	h.Timestamp = int64(binary.BigEndian.Uint64(b[8:16]))
	copy(h.PrevHash[:], b[16:48])
	copy(h.TxRoot[:], b[48:80])
	copy(h.StateRoot[:], b[80:112])
	copy(h.Miner[:], b[112:132])
	h.Difficulty = b[132]
	h.Nonce = binary.BigEndian.Uint64(b[133:141])
	return h, nil
}

// ComputeHash is SHA-256 of the encoded header: the block hash.
func (h Header) ComputeHash() Hash { return SHA256(h.Encode()) }

// TxHashes returns the IDs of txs in order.
func TxHashes(txs []Tx) []Hash {
	out := make([]Hash, len(txs))
	for i, tx := range txs {
		out[i] = tx.Hash()
	}
	return out
}

// ComputeTxRoot is the Merkle root of the transaction IDs, in block order.
func ComputeTxRoot(txs []Tx) Hash { return MerkleRoot(TxHashes(txs)) }

// EncodeBlock is the storage (and, in Project 3, network) format:
// the 141-byte header followed by the RLP list of raw transactions.
func EncodeBlock(b Block) []byte {
	txs, err := rlp.EncodeToBytes(b.Txs)
	if err != nil {
		panic(err) // unreachable: see Tx.Encode
	}
	return append(b.Header.Encode(), txs...)
}

// DecodeBlock is the inverse of EncodeBlock. The cached Hash is recomputed,
// never read from the input.
func DecodeBlock(data []byte) (Block, error) {
	if len(data) < HeaderSize {
		return Block{}, fmt.Errorf("block is %d bytes, shorter than a header", len(data))
	}
	h, err := DecodeHeader(data[:HeaderSize])
	if err != nil {
		return Block{}, err
	}
	var txs []Tx
	if err := rlp.DecodeBytes(data[HeaderSize:], &txs); err != nil {
		return Block{}, fmt.Errorf("decode block transactions: %w", err)
	}
	return Block{Header: h, Txs: txs, Hash: h.ComputeHash()}, nil
}
