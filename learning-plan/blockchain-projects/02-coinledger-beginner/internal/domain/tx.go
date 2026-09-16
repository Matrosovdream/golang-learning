package domain

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"math/bits"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

// SignatureSize is r (32) ‖ s (32) ‖ v (1).
const SignatureSize = 65

// Tx is a signed transfer of coins.
//
// Notice what is NOT here: a From field. The sender is recovered from the
// signature (see Sender). A client can't claim to be someone else, because
// there is no field to lie in.
type Tx struct {
	ChainID uint64              // which chain this tx is for — replay protection across chains
	Nonce   uint64              // the sender's tx counter: must equal their account nonce
	To      Address             // recipient
	Amount  uint64              // coins to transfer
	Fee     uint64              // coins paid to the miner who includes the tx
	Sig     [SignatureSize]byte // secp256k1 signature over SigningHash, v ∈ {0,1}
}

// unsignedTx is exactly the data the signature covers: every field except the
// signature itself (a signature can't sign itself).
type unsignedTx struct {
	ChainID uint64
	Nonce   uint64
	To      Address
	Amount  uint64
	Fee     uint64
}

// SigningHash is Keccak-256 of the RLP encoding of the unsigned fields.
//
// Every field is in it. Change the amount, the recipient, the nonce, or the
// chain ID after signing, and the hash changes, so the signature recovers a
// different (random) address and the tx no longer comes from the real sender.
func (tx Tx) SigningHash() Hash {
	b, err := rlp.EncodeToBytes(unsignedTx{
		ChainID: tx.ChainID,
		Nonce:   tx.Nonce,
		To:      tx.To,
		Amount:  tx.Amount,
		Fee:     tx.Fee,
	})
	if err != nil {
		panic(err) // unreachable: fixed-size integers and byte arrays always encode
	}
	return Keccak256(b)
}

// SignTx signs tx with a private key and returns the signed copy.
func SignTx(tx Tx, key *ecdsa.PrivateKey) (Tx, error) {
	h := tx.SigningHash()
	sig, err := crypto.Sign(h[:], key)
	if err != nil {
		return Tx{}, err
	}
	return tx.WithSignature(sig)
}

// WithSignature attaches a signature produced elsewhere (e.g. by a keystore
// that never exposes the private key). It accepts wallet-style v = 27/28.
func (tx Tx) WithSignature(sig []byte) (Tx, error) {
	if len(sig) != SignatureSize {
		return Tx{}, fmt.Errorf("%w: %d bytes, want %d", ErrInvalidSignature, len(sig), SignatureSize)
	}
	copy(tx.Sig[:], sig)
	if tx.Sig[64] >= 27 {
		tx.Sig[64] -= 27
	}
	return tx, nil
}

// Sender recovers the address that signed the transaction.
//
// ECDSA public-key recovery: from (hash, r, s, v) the math yields exactly one
// public key, and the address is Keccak-256(pubkey)[12:]. Anyone can do this;
// nobody can produce a signature that recovers to YOUR address without your key.
func (tx Tx) Sender() (Address, error) {
	r := new(big.Int).SetBytes(tx.Sig[0:32])
	s := new(big.Int).SetBytes(tx.Sig[32:64])
	v := tx.Sig[64]

	// Rejects r or s out of range, v not 0/1, and "high-s" signatures (EIP-2).
	// Every signature has a twin with s' = n − s that is equally valid
	// mathematically; accepting both would give one tx two different hashes.
	if !crypto.ValidateSignatureValues(v, r, s, true) {
		return Address{}, ErrInvalidSignature
	}
	h := tx.SigningHash()
	pub, err := crypto.SigToPub(h[:], tx.Sig[:])
	if err != nil {
		return Address{}, fmt.Errorf("%w: %v", ErrInvalidSignature, err)
	}
	return crypto.PubkeyToAddress(*pub), nil
}

// Encode returns the raw transaction: RLP of all fields, signature included.
// This is what a wallet sends to a node and what blocks store.
func (tx Tx) Encode() []byte {
	b, err := rlp.EncodeToBytes(tx)
	if err != nil {
		panic(err) // unreachable, as in SigningHash
	}
	return b
}

// DecodeTx parses a raw transaction. The RLP decoder rejects non-canonical
// encodings (e.g. integers with leading zero bytes), so a tx has exactly one
// valid byte form — and therefore exactly one hash.
func DecodeTx(raw []byte) (Tx, error) {
	var tx Tx
	if err := rlp.DecodeBytes(raw, &tx); err != nil {
		return Tx{}, InputError{Message: "invalid raw transaction: " + err.Error()}
	}
	return tx, nil
}

// Hash is the transaction ID: Keccak-256 of the raw (signed) encoding.
func (tx Tx) Hash() Hash { return Keccak256(tx.Encode()) }

// Cost is what the sender pays: amount + fee. ok is false if that overflows
// uint64 — otherwise a huge amount plus a fee could wrap around to a tiny
// number and pass the balance check.
func (tx Tx) Cost() (cost uint64, ok bool) {
	sum, carry := bits.Add64(tx.Amount, tx.Fee, 0)
	return sum, carry == 0
}
