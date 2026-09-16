package domain

import (
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

// Address is a 20-byte account address, identical to an Ethereum address:
// the last 20 bytes of Keccak-256(public key). An alias, so go-ethereum's
// crypto functions and ours share one type.
type Address = common.Address

// ParseAddress accepts 40 hex characters with or without 0x.
//
// If the input mixes upper and lower case, it must be a correct EIP-55
// checksum. All-lowercase or all-uppercase input carries no checksum and is
// accepted as is. This is exactly what wallets do: a mistyped mixed-case
// address is rejected instead of sending coins into the void.
func ParseAddress(s string) (Address, error) {
	if !common.IsHexAddress(s) {
		return Address{}, InputError{Message: fmt.Sprintf("invalid address %q: want 40 hex characters", s)}
	}
	addr := common.HexToAddress(s)

	digits := strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	mixed := strings.ToLower(digits) != digits && strings.ToUpper(digits) != digits
	if mixed && addr.Hex()[2:] != digits {
		return Address{}, InputError{Message: fmt.Sprintf("address %q has an invalid EIP-55 checksum (typo?)", s)}
	}
	return addr, nil
}
