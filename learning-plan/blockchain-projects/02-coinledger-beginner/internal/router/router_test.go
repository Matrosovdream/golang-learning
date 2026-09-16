package router_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"

	"coinledger/internal/domain"
	"coinledger/internal/handler"
	"coinledger/internal/mempool"
	"coinledger/internal/repository/boltstore"
	"coinledger/internal/router"
	"coinledger/internal/service"
)

func TestErrorStatusesAndCodes(t *testing.T) {
	k := make([]byte, 32)
	k[31] = 1
	alice, _ := crypto.ToECDSA(k)
	aliceAddr := crypto.PubkeyToAddress(alice.PublicKey)

	store, err := boltstore.Open(filepath.Join(t.TempDir(), "chain.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	params := domain.Params{ChainID: 1337, Difficulty: 8, BlockReward: 50, MinFee: 1, MaxBlockTxs: 10,
		GenesisAlloc: map[domain.Address]uint64{aliceAddr: 10}}
	node, err := service.NewNode(context.Background(), params, store,
		mempool.New(mempool.Config{MaxSize: 10, MaxNonceGap: 2}), service.Options{})
	if err != nil {
		t.Fatal(err)
	}
	srv := router.New(handler.NewNodeHandler(node, 5*time.Second))

	raw := func(tx domain.Tx) string {
		s, _ := domain.SignTx(tx, alice)
		return `{"raw":"0x` + hex.EncodeToString(s.Encode()) + `"}`
	}
	to := domain.Address{0xb0}

	tests := []struct {
		name, method, path, body string
		wantStatus               int
		wantCode                 string
	}{
		{"not hex", "POST", "/txs", `{"raw":"zz"}`, 400, "invalid_input"},
		{"not a tx", "POST", "/txs", `{"raw":"0x01"}`, 400, "invalid_input"},
		{"wrong chain", "POST", "/txs", raw(domain.Tx{ChainID: 1, To: to, Amount: 1, Fee: 1}), 400, "wrong_chain_id"},
		{"fee too low", "POST", "/txs", raw(domain.Tx{ChainID: 1337, To: to, Amount: 1, Fee: 0}), 422, "fee_too_low"},
		{"too expensive", "POST", "/txs", raw(domain.Tx{ChainID: 1337, To: to, Amount: 10, Fee: 1}), 422, "insufficient_funds"},
		{"nonce gap", "POST", "/txs", raw(domain.Tx{ChainID: 1337, Nonce: 9, To: to, Amount: 1, Fee: 1}), 422, "nonce_gap"},
		{"accepted", "POST", "/txs", raw(domain.Tx{ChainID: 1337, To: to, Amount: 1, Fee: 1}), 202, ""},
		{"duplicate", "POST", "/txs", raw(domain.Tx{ChainID: 1337, To: to, Amount: 1, Fee: 1}), 409, "already_known"},
		{"bad address", "GET", "/accounts/0x1234", "", 400, "invalid_input"},
		{"bad checksum", "GET", "/accounts/0x7E5F4552091A69125d5DfCb7b8C2659029395BDf", "", 400, "invalid_input"},
		{"unknown tx", "GET", "/txs/0x" + strings.Repeat("ab", 32), "", 404, "not_found"},
		{"unknown block", "GET", "/blocks/99", "", 404, "not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body)))
			var body struct{ Code string }
			_ = json.Unmarshal(rec.Body.Bytes(), &body)
			if rec.Code != tt.wantStatus || body.Code != tt.wantCode {
				t.Fatalf("got %d %q, want %d %q: %s", rec.Code, body.Code, tt.wantStatus, tt.wantCode, rec.Body)
			}
		})
	}

	// Mine, then validate over HTTP.
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest("POST", "/mine", nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("mine: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest("GET", "/chain/validate", nil))
	var v struct {
		Valid       bool
		TotalSupply string `json:"total_supply"`
	}
	json.Unmarshal(rec.Body.Bytes(), &v)
	if !v.Valid || v.TotalSupply != "60" {
		t.Fatalf("validate: %s", rec.Body)
	}
}
