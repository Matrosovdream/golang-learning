package domain

import (
	"errors"
	"fmt"
)

// Transaction validity errors — returned by ApplyTx (consensus) and reused by
// the mempool (admission), so both layers speak the same language.
var (
	ErrWrongChainID      = errors.New("wrong chain id")
	ErrInvalidSignature  = errors.New("invalid signature")
	ErrNonceTooLow       = errors.New("nonce too low")
	ErrNonceTooHigh      = errors.New("nonce too high")
	ErrFeeTooLow         = errors.New("fee below minimum")
	ErrInsufficientFunds = errors.New("insufficient funds for amount + fee")
	ErrOverflow          = errors.New("amount overflow")
)

var (
	// ErrNotFound is returned when a block, transaction, or account lookup finds nothing.
	ErrNotFound = errors.New("not found")
	// ErrTipChanged is returned when a block does not extend the current tip.
	ErrTipChanged = errors.New("block does not extend the current tip")
	// ErrParamsMismatch means the data directory was created with different consensus params.
	ErrParamsMismatch = errors.New("data dir was created with different consensus params")
	// ErrGenesisMismatch means the stored genesis block is not the one these params produce.
	ErrGenesisMismatch = errors.New("stored genesis does not match the configured genesis")
)

// InputError signals malformed client input; the handler maps it to HTTP 400.
type InputError struct{ Message string }

func (e InputError) Error() string { return e.Message }

// TxError says which transaction of a block failed and why.
type TxError struct {
	Index int
	Hash  Hash
	Err   error
}

func (e *TxError) Error() string {
	return fmt.Sprintf("tx %d (%s…): %v", e.Index, e.Hash.Hex()[:12], e.Err)
}

func (e *TxError) Unwrap() error { return e.Err }
