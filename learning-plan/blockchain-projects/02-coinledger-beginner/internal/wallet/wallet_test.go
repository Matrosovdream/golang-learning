package wallet_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/keystore"

	"coinledger/internal/domain"
	"coinledger/internal/handler"
	"coinledger/internal/mempool"
	"coinledger/internal/repository/boltstore"
	"coinledger/internal/router"
	"coinledger/internal/service"
	"coinledger/internal/wallet"
)

// The full path a real user takes: a key in an encrypted keystore, a node
// behind HTTP, sign locally, broadcast, mine, check status, verify the proof.
func TestWalletAgainstARealNode(t *testing.T) {
	dir := t.TempDir()
	ks := wallet.OpenKeystore(filepath.Join(dir, "keystore"))
	alice, err := ks.NewAccount("alice-pw")
	if err != nil {
		t.Fatal(err)
	}
	bob, _ := ks.NewAccount("bob-pw")

	params := domain.Params{
		ChainID: 1337, Difficulty: 8, BlockReward: 50, MinFee: 1, MaxBlockTxs: 10,
		GenesisAlloc: map[domain.Address]uint64{alice.Address: 1_000},
	}
	store, err := boltstore.Open(filepath.Join(dir, "chain.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	node, err := service.NewNode(context.Background(), params, store,
		mempool.New(mempool.Config{MaxSize: 10, MaxNonceGap: 4}), service.Options{})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(router.New(handler.NewNodeHandler(node, 10*time.Second)))
	defer srv.Close()

	ctx := context.Background()
	client := wallet.NewClient(srv.URL)

	acct, err := client.Account(ctx, alice.Address.Hex())
	if err != nil || acct.Balance != "1000" || acct.PendingNonce != 0 {
		t.Fatalf("alice before: %+v %v", acct, err)
	}

	// Wrong password: the keystore refuses to decrypt the key.
	if _, err := wallet.SignTx(ks, alice, "wrong", domain.Tx{ChainID: 1337}); !errors.Is(err, keystore.ErrDecrypt) {
		t.Fatalf("wrong password: err = %v", err)
	}

	tx, err := wallet.SignTx(ks, alice, "alice-pw", domain.Tx{
		ChainID: 1337, Nonce: acct.PendingNonce, To: bob.Address, Amount: 250, Fee: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.Submit(ctx, tx.Encode())
	if err != nil || res.Status != "pending" || res.From != alice.Address.Hex() {
		t.Fatalf("submit: %+v %v", res, err)
	}

	// Submitting again: a machine-readable error code, not just a message.
	var apiErr *wallet.APIError
	if _, err := client.Submit(ctx, tx.Encode()); !errors.As(err, &apiErr) || apiErr.Code != "already_known" || apiErr.Status != http.StatusConflict {
		t.Fatalf("duplicate submit: %v", err)
	}

	if resp, err := http.Post(srv.URL+"/mine", "application/json", nil); err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("mine: %v %v", resp, err)
	}

	info, err := client.Tx(ctx, res.Hash)
	if err != nil || info.Status != "included" || info.BlockHeight == nil {
		t.Fatalf("tx after mining: %+v %v", info, err)
	}
	if b, _ := client.Account(ctx, bob.Address.Hex()); b.Balance != "250" {
		t.Fatalf("bob = %s", b.Balance)
	}

	proof, err := client.Proof(ctx, *info.BlockHeight, res.Hash)
	if err != nil {
		t.Fatal(err)
	}
	v, err := wallet.VerifyInclusion(proof, tx.Hash(), params.Difficulty)
	if err != nil || !v.Valid() {
		t.Fatalf("light client: %+v %v", v, err)
	}

	// A light client must not accept a header that claims less work than it expects.
	if v, _ := wallet.VerifyInclusion(proof, tx.Hash(), 40); v.WorkOK {
		t.Fatal("accepted a header below the minimum difficulty")
	}
	// Nor a proof for a different transaction.
	if v, _ := wallet.VerifyInclusion(proof, domain.Hash{7}, params.Difficulty); v.ProofOK {
		t.Fatal("proof verified an unrelated tx")
	}
}
