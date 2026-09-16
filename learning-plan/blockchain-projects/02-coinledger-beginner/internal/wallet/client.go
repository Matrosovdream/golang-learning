// Package wallet is the client side: talking to a node, signing with a
// keystore, and verifying inclusion proofs like a light client.
package wallet

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Client calls a coinledger node's HTTP API.
type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: 90 * time.Second}}
}

// APIError is a non-2xx response from the node.
type APIError struct {
	Status  int
	Code    string `json:"code"`
	Message string `json:"error"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("node returned %d %s: %s", e.Status, e.Code, e.Message)
}

type Account struct {
	Address      string `json:"address"`
	Balance      string `json:"balance"`
	Nonce        uint64 `json:"nonce"`
	PendingNonce uint64 `json:"pending_nonce"`
	AtHeight     uint64 `json:"at_height"`
}

type SubmitResult struct {
	Hash   string `json:"hash"`
	From   string `json:"from"`
	Status string `json:"status"`
}

type TxInfo struct {
	Hash          string  `json:"hash"`
	Status        string  `json:"status"`
	From          string  `json:"from"`
	To            string  `json:"to"`
	Amount        string  `json:"amount"`
	Fee           string  `json:"fee"`
	Nonce         uint64  `json:"nonce"`
	BlockHeight   *uint64 `json:"block_height"`
	Confirmations uint64  `json:"confirmations"`
}

type ProofStep struct {
	Sibling  string `json:"sibling"`
	Position string `json:"position"`
}

type Proof struct {
	TxHash    string      `json:"tx_hash"`
	Height    uint64      `json:"block_height"`
	Index     int         `json:"index"`
	HeaderRaw string      `json:"header_raw"`
	TxRoot    string      `json:"tx_root"`
	Steps     []ProofStep `json:"steps"`
}

type Stats struct {
	Height  uint64 `json:"height"`
	ChainID uint64 `json:"chain_id"`
	MinFee  string `json:"min_fee"`
}

func (c *Client) Account(ctx context.Context, address string) (Account, error) {
	var out Account
	return out, c.do(ctx, http.MethodGet, "/accounts/"+address, nil, &out)
}

// Submit broadcasts a raw signed transaction.
func (c *Client) Submit(ctx context.Context, raw []byte) (SubmitResult, error) {
	var out SubmitResult
	body := map[string]string{"raw": "0x" + hex.EncodeToString(raw)}
	return out, c.do(ctx, http.MethodPost, "/txs", body, &out)
}

func (c *Client) Tx(ctx context.Context, hash string) (TxInfo, error) {
	var out TxInfo
	return out, c.do(ctx, http.MethodGet, "/txs/"+hash, nil, &out)
}

func (c *Client) Proof(ctx context.Context, height uint64, hash string) (Proof, error) {
	var out Proof
	return out, c.do(ctx, http.MethodGet, fmt.Sprintf("/blocks/%d/txs/%s/proof", height, hash), nil, &out)
}

func (c *Client) Stats(ctx context.Context) (Stats, error) {
	var out Stats
	return out, c.do(ctx, http.MethodGet, "/stats", nil, &out)
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		apiErr := &APIError{Status: resp.StatusCode}
		_ = json.NewDecoder(resp.Body).Decode(apiErr)
		return apiErr
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
