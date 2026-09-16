package domain

import (
	"crypto/ecdsa"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// Private keys 1, 2, 3: valid, deterministic, and PUBLICLY KNOWN — test only.
func testKey(t *testing.T, n byte) (*ecdsa.PrivateKey, Address) {
	t.Helper()
	b := make([]byte, 32)
	b[31] = n
	key, err := crypto.ToECDSA(b)
	if err != nil {
		t.Fatal(err)
	}
	return key, crypto.PubkeyToAddress(key.PublicKey)
}

var testParams = Params{ChainID: 1337, Difficulty: 8, BlockReward: 50, MinFee: 1, MaxBlockTxs: 10}

func signed(t *testing.T, key *ecdsa.PrivateKey, tx Tx) Tx {
	t.Helper()
	if tx.ChainID == 0 {
		tx.ChainID = testParams.ChainID
	}
	s, err := SignTx(tx, key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
