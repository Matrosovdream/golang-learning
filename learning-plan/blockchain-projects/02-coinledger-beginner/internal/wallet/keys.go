package wallet

import (
	"fmt"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"

	"coinledger/internal/domain"
)

// OpenKeystore opens a directory of encrypted key files — the same "Web3
// Secret Storage" JSON format geth and many Ethereum wallets use.
//
// Each file holds a private key encrypted with a key derived from your
// password by scrypt, a deliberately slow, memory-hungry function that makes
// guessing passwords expensive. LightScryptN (2^12) keeps this demo snappy;
// real wallets use StandardScryptN (2^18), about 64× more work per guess.
func OpenKeystore(dir string) *keystore.KeyStore {
	return keystore.NewKeyStore(dir, keystore.LightScryptN, keystore.LightScryptP)
}

// FindAccount looks up the key file for an address.
func FindAccount(ks *keystore.KeyStore, address domain.Address) (accounts.Account, error) {
	acct, err := ks.Find(accounts.Account{Address: address})
	if err != nil {
		return accounts.Account{}, fmt.Errorf("no key for %s in the keystore: %w", address.Hex(), err)
	}
	return acct, nil
}

// SignTx signs a transaction with a keystore key. The private key is
// decrypted in memory for this one signature and never leaves the wallet
// process — the node only ever sees the signature.
func SignTx(ks *keystore.KeyStore, from accounts.Account, password string, tx domain.Tx) (domain.Tx, error) {
	h := tx.SigningHash()
	sig, err := ks.SignHashWithPassphrase(from, password, h[:])
	if err != nil {
		return domain.Tx{}, fmt.Errorf("sign: %w", err)
	}
	return tx.WithSignature(sig)
}
