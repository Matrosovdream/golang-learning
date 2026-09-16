package handler

import (
	"context"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"coinledger/internal/domain"
	"coinledger/internal/mempool"
	"coinledger/internal/service"
)

// NodeHandler turns HTTP requests into Node calls.
type NodeHandler struct {
	node        *service.Node
	mineTimeout time.Duration
}

func NewNodeHandler(node *service.Node, mineTimeout time.Duration) *NodeHandler {
	return &NodeHandler{node: node, mineTimeout: mineTimeout}
}

// --- DTOs ---------------------------------------------------------------------
//
// Coin amounts are JSON *strings*. JavaScript (and many JSON parsers) store
// numbers as float64, which silently loses precision above 2^53. A uint64
// balance can exceed that. Strings survive every parser intact.

type submitTxRequest struct {
	Raw string `json:"raw"` // 0x-prefixed hex of the RLP-encoded signed tx
}

type submitTxResponse struct {
	Hash   string `json:"hash"`
	From   string `json:"from"`
	Status string `json:"status"`
}

type txResponse struct {
	Hash          string  `json:"hash"`
	Status        string  `json:"status"`
	From          string  `json:"from"`
	To            string  `json:"to"`
	Amount        string  `json:"amount"`
	Fee           string  `json:"fee"`
	Nonce         uint64  `json:"nonce"`
	ChainID       uint64  `json:"chain_id"`
	BlockHeight   *uint64 `json:"block_height,omitempty"`
	Index         *int    `json:"index,omitempty"`
	Confirmations uint64  `json:"confirmations"`
	Raw           string  `json:"raw"`
}

type accountResponse struct {
	Address      string `json:"address"`
	Balance      string `json:"balance"`
	Nonce        uint64 `json:"nonce"`
	PendingNonce uint64 `json:"pending_nonce"`
	AtHeight     uint64 `json:"at_height"`
}

type blockResponse struct {
	Height     uint64   `json:"height"`
	Hash       string   `json:"hash"`
	PrevHash   string   `json:"prev_hash"`
	Timestamp  string   `json:"timestamp"`
	TxRoot     string   `json:"tx_root"`
	StateRoot  string   `json:"state_root"`
	Miner      string   `json:"miner"`
	Difficulty uint8    `json:"difficulty"`
	Nonce      uint64   `json:"nonce"`
	TxCount    int      `json:"tx_count"`
	TxHashes   []string `json:"tx_hashes"`
}

type mempoolTx struct {
	Hash   string `json:"hash"`
	From   string `json:"from"`
	To     string `json:"to"`
	Nonce  uint64 `json:"nonce"`
	Amount string `json:"amount"`
	Fee    string `json:"fee"`
}

type mempoolResponse struct {
	Pending []mempoolTx `json:"pending"`
	Queued  []mempoolTx `json:"queued"`
}

type proofStep struct {
	Sibling  string `json:"sibling"`
	Position string `json:"position"` // "left" or "right" — where the sibling sits
}

type proofResponse struct {
	TxHash    string      `json:"tx_hash"`
	Height    uint64      `json:"block_height"`
	Index     int         `json:"index"`
	HeaderRaw string      `json:"header_raw"` // 141 bytes: hash it yourself, don't trust a "hash" field
	TxRoot    string      `json:"tx_root"`
	Steps     []proofStep `json:"steps"`
}

type validationResponse struct {
	Valid          bool    `json:"valid"`
	Blocks         uint64  `json:"blocks"`
	InvalidHeight  *uint64 `json:"first_invalid_height,omitempty"`
	Rule           string  `json:"rule,omitempty"`
	Reason         string  `json:"reason,omitempty"`
	StateRoot      string  `json:"replayed_state_root,omitempty"`
	StateMatches   bool    `json:"state_matches_live"`
	TotalSupply    string  `json:"total_supply"`
	ExpectedSupply string  `json:"expected_supply"`
	TookMS         float64 `json:"took_ms"`
}

type statsResponse struct {
	Height         uint64 `json:"height"`
	TipHash        string `json:"tip_hash"`
	Accounts       int    `json:"accounts"`
	TotalSupply    string `json:"total_supply"`
	ExpectedSupply string `json:"expected_supply"`
	MempoolPending int    `json:"mempool_pending"`
	MempoolQueued  int    `json:"mempool_queued"`
	Miner          string `json:"miner"`
	ChainID        uint64 `json:"chain_id"`
	Difficulty     uint8  `json:"difficulty"`
	BlockReward    string `json:"block_reward"`
	MinFee         string `json:"min_fee"`
	MaxBlockTxs    int    `json:"max_block_txs"`
}

// --- handlers -----------------------------------------------------------------

// SubmitTx accepts a raw signed transaction into the mempool.
func (h *NodeHandler) SubmitTx(w http.ResponseWriter, r *http.Request) {
	var req submitTxRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(req.Raw, "0x"))
	if err != nil || len(raw) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_input", "raw must be a 0x-prefixed hex string")
		return
	}
	res, err := h.node.SubmitTx(raw)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	// 202 Accepted: the node took it, but nothing has happened on-chain yet.
	writeJSON(w, http.StatusAccepted, submitTxResponse{
		Hash: res.Hash.Hex(), From: res.Sender.Hex(), Status: string(res.Status),
	})
}

// GetTx returns a transaction from the chain or the mempool.
func (h *NodeHandler) GetTx(w http.ResponseWriter, r *http.Request) {
	hash, err := domain.ParseHash(r.PathValue("hash"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	v, err := h.node.Tx(hash)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp := txResponse{
		Hash: v.Hash.Hex(), Status: string(v.Status), From: v.Sender.Hex(), To: v.Tx.To.Hex(),
		Amount: u(v.Tx.Amount), Fee: u(v.Tx.Fee), Nonce: v.Tx.Nonce, ChainID: v.Tx.ChainID,
		Raw: "0x" + hex.EncodeToString(v.Tx.Encode()),
	}
	if v.Status == service.TxIncluded {
		resp.BlockHeight, resp.Index, resp.Confirmations = &v.BlockHeight, &v.Index, v.Confirmations
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetAccount returns balance, nonce, and the next nonce to use.
func (h *NodeHandler) GetAccount(w http.ResponseWriter, r *http.Request) {
	addr, err := domain.ParseAddress(r.PathValue("address"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	v := h.node.Account(addr)
	writeJSON(w, http.StatusOK, accountResponse{
		Address: v.Address.Hex(), Balance: u(v.Account.Balance), Nonce: v.Account.Nonce,
		PendingNonce: v.PendingNonce, AtHeight: v.TipHeight,
	})
}

// Mempool lists pooled transactions.
func (h *NodeHandler) Mempool(w http.ResponseWriter, r *http.Request) {
	pending, queued := h.node.Mempool()
	writeJSON(w, http.StatusOK, mempoolResponse{Pending: toMempoolTxs(pending), Queued: toMempoolTxs(queued)})
}

// Mine produces a block now.
func (h *NodeHandler) Mine(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.mineTimeout)
	defer cancel()
	b, err := h.node.Mine(ctx)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toBlockResponse(b))
}

// ListBlocks returns blocks from ?from= (default 0), at most ?limit= (default 20, max 100).
func (h *NodeHandler) ListBlocks(w http.ResponseWriter, r *http.Request) {
	from, err := queryUint(r, "from", 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", "from must be a non-negative integer")
		return
	}
	limit, err := queryUint(r, "limit", 20)
	if err != nil || limit == 0 || limit > 100 {
		writeError(w, http.StatusBadRequest, "invalid_input", "limit must be between 1 and 100")
		return
	}
	blocks, err := h.node.Blocks(from, int(limit))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp := make([]blockResponse, len(blocks))
	for i, b := range blocks {
		resp[i] = toBlockResponse(b)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *NodeHandler) LatestBlock(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, toBlockResponse(h.node.Latest()))
}

func (h *NodeHandler) GetBlock(w http.ResponseWriter, r *http.Request) {
	height, ok := parseHeight(w, r)
	if !ok {
		return
	}
	b, err := h.node.Block(height)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toBlockResponse(b))
}

// Proof returns a Merkle inclusion proof for a transaction in a block.
func (h *NodeHandler) Proof(w http.ResponseWriter, r *http.Request) {
	height, ok := parseHeight(w, r)
	if !ok {
		return
	}
	hash, err := domain.ParseHash(r.PathValue("hash"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	p, err := h.node.Proof(height, hash)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	header, _ := domain.DecodeHeader(p.HeaderRaw)
	resp := proofResponse{
		TxHash: p.TxHash.Hex(), Height: p.Height, Index: p.Index,
		HeaderRaw: "0x" + hex.EncodeToString(p.HeaderRaw), TxRoot: header.TxRoot.Hex(),
		Steps: make([]proofStep, len(p.Steps)),
	}
	for i, s := range p.Steps {
		pos := "right"
		if s.Left {
			pos = "left"
		}
		resp.Steps[i] = proofStep{Sibling: s.Sibling.Hex(), Position: pos}
	}
	writeJSON(w, http.StatusOK, resp)
}

// Validate replays the whole chain.
func (h *NodeHandler) Validate(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.mineTimeout)
	defer cancel()
	rep, err := h.node.Validate(ctx)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp := validationResponse{
		Valid: rep.Valid, Blocks: rep.Blocks, StateMatches: rep.StateMatches,
		TotalSupply: u(rep.TotalSupply), ExpectedSupply: u(rep.ExpectedSupply),
		TookMS: float64(rep.Took.Microseconds()) / 1000,
	}
	if !rep.StateRoot.IsZero() {
		resp.StateRoot = rep.StateRoot.Hex()
	}
	if rep.Invalid != nil {
		resp.InvalidHeight, resp.Rule, resp.Reason = &rep.Invalid.Height, rep.Invalid.Rule, rep.Invalid.Reason
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *NodeHandler) Stats(w http.ResponseWriter, r *http.Request) {
	s := h.node.Stats()
	writeJSON(w, http.StatusOK, statsResponse{
		Height: s.Height, TipHash: s.TipHash.Hex(), Accounts: s.Accounts,
		TotalSupply: u(s.TotalSupply), ExpectedSupply: u(s.ExpectedSupply),
		MempoolPending: s.Pending, MempoolQueued: s.Queued, Miner: s.Miner.Hex(),
		ChainID: s.Params.ChainID, Difficulty: s.Params.Difficulty, BlockReward: u(s.Params.BlockReward),
		MinFee: u(s.Params.MinFee), MaxBlockTxs: s.Params.MaxBlockTxs,
	})
}

func (h *NodeHandler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- helpers ------------------------------------------------------------------

func u(n uint64) string { return strconv.FormatUint(n, 10) }

func parseHeight(w http.ResponseWriter, r *http.Request) (uint64, bool) {
	height, err := strconv.ParseUint(r.PathValue("height"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", "height must be a non-negative integer")
		return 0, false
	}
	return height, true
}

func queryUint(r *http.Request, key string, fallback uint64) (uint64, error) {
	v := r.URL.Query().Get(key)
	if v == "" {
		return fallback, nil
	}
	return strconv.ParseUint(v, 10, 64)
}

func toBlockResponse(b domain.Block) blockResponse {
	hashes := make([]string, len(b.Txs))
	for i, t := range b.Txs {
		hashes[i] = t.Hash().Hex()
	}
	return blockResponse{
		Height: b.Height, Hash: b.Hash.Hex(), PrevHash: b.PrevHash.Hex(),
		Timestamp: time.UnixMilli(b.Timestamp).UTC().Format(time.RFC3339Nano),
		TxRoot:    b.TxRoot.Hex(), StateRoot: b.StateRoot.Hex(), Miner: b.Miner.Hex(),
		Difficulty: b.Difficulty, Nonce: b.Nonce, TxCount: len(b.Txs), TxHashes: hashes,
	}
}

func toMempoolTxs(entries []mempool.Entry) []mempoolTx {
	out := make([]mempoolTx, len(entries))
	for i, e := range entries {
		out[i] = mempoolTx{
			Hash: e.Hash.Hex(), From: e.Sender.Hex(), To: e.Tx.To.Hex(),
			Nonce: e.Tx.Nonce, Amount: u(e.Tx.Amount), Fee: u(e.Tx.Fee),
		}
	}
	return out
}
