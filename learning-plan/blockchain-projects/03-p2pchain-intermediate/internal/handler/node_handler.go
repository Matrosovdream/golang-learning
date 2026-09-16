package handler

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"p2pchain/internal/domain"
	"p2pchain/internal/events"
	"p2pchain/internal/mempool"
	"p2pchain/internal/service"
)

// NodeHandler turns HTTP requests into node calls.
type NodeHandler struct {
	node        *service.Node
	bus         *events.Bus
	mineTimeout time.Duration
}

func NewNodeHandler(node *service.Node, bus *events.Bus, mineTimeout time.Duration) *NodeHandler {
	return &NodeHandler{node: node, bus: bus, mineTimeout: mineTimeout}
}

// --- DTOs (coin amounts are strings: see coinledger's README) ---------------------

type submitTxRequest struct {
	Raw string `json:"raw"`
}

type txResponse struct {
	Hash          string  `json:"hash"`
	Status        string  `json:"status"`
	From          string  `json:"from"`
	To            string  `json:"to"`
	Amount        string  `json:"amount"`
	Fee           string  `json:"fee"`
	Nonce         uint64  `json:"nonce"`
	BlockHash     string  `json:"block_hash,omitempty"`
	BlockHeight   *uint64 `json:"block_height,omitempty"`
	Confirmations uint64  `json:"confirmations"`
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

type statusResponse struct {
	PeerID      string      `json:"peer_id"`
	Addresses   []string    `json:"addresses"`
	ChainID     uint64      `json:"chain_id"`
	Height      uint64      `json:"height"`
	TipHash     string      `json:"tip_hash"`
	TotalWork   string      `json:"total_work"`
	PeerCount   int         `json:"peer_count"`
	GossipPeers int         `json:"gossip_peers"`
	Syncing     bool        `json:"syncing"`
	Orphans     int         `json:"orphans"`
	Pending     int         `json:"mempool_pending"`
	Queued      int         `json:"mempool_queued"`
	Threads     int         `json:"miner_threads"`
	Mined       uint64      `json:"blocks_mined"`
	Hashes      uint64      `json:"hashes_tried"`
	Miner       string      `json:"miner_address"`
	Peers       []peerEntry `json:"peers"`
}

type peerEntry struct {
	ID        string `json:"id"`
	Address   string `json:"address"`
	Direction string `json:"direction"`
	Blocked   bool   `json:"blocked"`
}

type forkEntry struct {
	Hash      string `json:"hash"`
	Height    uint64 `json:"height"`
	Work      string `json:"work"`
	Canonical bool   `json:"canonical"`
}

// --- handlers -------------------------------------------------------------------

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
	writeJSON(w, http.StatusAccepted, map[string]string{
		"hash": res.Hash.Hex(), "from": res.Sender.Hex(), "status": string(res.Status),
	})
}

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
		Amount: u(v.Tx.Amount), Fee: u(v.Tx.Fee), Nonce: v.Tx.Nonce,
	}
	if v.Status == service.TxIncluded {
		resp.BlockHash, resp.BlockHeight, resp.Confirmations = v.BlockHash.Hex(), &v.BlockHeight, v.Confirmations
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *NodeHandler) GetAccount(w http.ResponseWriter, r *http.Request) {
	addr, err := domain.ParseAddress(r.PathValue("address"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	v := h.node.Account(addr)
	writeJSON(w, http.StatusOK, map[string]any{
		"address": v.Address.Hex(), "balance": u(v.Account.Balance), "nonce": v.Account.Nonce,
		"pending_nonce": v.PendingNonce, "at_height": v.TipHeight,
	})
}

func (h *NodeHandler) Mempool(w http.ResponseWriter, r *http.Request) {
	pending, queued := h.node.Mempool()
	writeJSON(w, http.StatusOK, map[string]any{"pending": toMempoolTxs(pending), "queued": toMempoolTxs(queued)})
}

func (h *NodeHandler) Mine(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.mineTimeout)
	defer cancel()
	b, err := h.node.MineOne(ctx)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toBlockResponse(b))
}

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
	ref := r.PathValue("ref")
	if hash, err := domain.ParseHash(ref); err == nil { // a block hash: may be off the canonical chain
		b, err := h.node.Chain().BlockByHash(hash)
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, toBlockResponse(b))
		return
	}
	height, err := strconv.ParseUint(ref, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", "block reference must be a height or a hash")
		return
	}
	b, err := h.node.Block(height)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toBlockResponse(b))
}

func (h *NodeHandler) Status(w http.ResponseWriter, r *http.Request) {
	s := h.node.NodeStatus()
	resp := statusResponse{
		PeerID: s.PeerID, Addresses: s.Addresses, ChainID: s.ChainID, Height: s.Height,
		TipHash: s.TipHash.Hex(), TotalWork: s.Work.String(), PeerCount: len(s.Peers),
		GossipPeers: s.Gossiping, Syncing: s.Syncing, Orphans: s.Orphans,
		Pending: s.Pending, Queued: s.Queued, Threads: s.Threads, Mined: s.Mined,
		Hashes: s.Hashes, Miner: s.Miner.Hex(),
	}
	for _, p := range s.Peers {
		resp.Peers = append(resp.Peers, peerEntry(p))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *NodeHandler) Peers(w http.ResponseWriter, r *http.Request) {
	s := h.node.NodeStatus()
	out := make([]peerEntry, 0, len(s.Peers))
	for _, p := range s.Peers {
		out = append(out, peerEntry(p))
	}
	writeJSON(w, http.StatusOK, out)
}

// Forks lists every branch tip this node knows: the forks it has seen.
func (h *NodeHandler) Forks(w http.ResponseWriter, r *http.Request) {
	tips, err := h.node.Forks()
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	out := make([]forkEntry, len(tips))
	for i, t := range tips {
		out[i] = forkEntry{Hash: t.Hash.Hex(), Height: t.Height, Work: t.Work.String(), Canonical: t.Canonical}
	}
	writeJSON(w, http.StatusOK, out)
}

// Events streams chain events as server-sent events (lesson 58).
func (h *NodeHandler) Events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal", "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, unsubscribe := h.bus.Subscribe(64)
	defer unsubscribe()

	ticker := time.NewTicker(15 * time.Second) // keep-alive through proxies
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			payload, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Kind, payload)
			flusher.Flush()
		}
	}
}

// Partition blocks or unblocks peers, simulating a network split.
func (h *NodeHandler) Partition(w http.ResponseWriter, r *http.Request) {
	net := h.node.Network()
	if net == nil {
		writeError(w, http.StatusServiceUnavailable, "no_network", "networking is disabled")
		return
	}
	var req struct {
		Peers []string `json:"peers"` // empty = every currently connected peer
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}
	var blocked []string
	if len(req.Peers) == 0 {
		for _, id := range net.BlockAllPeers(true) {
			blocked = append(blocked, id.String())
		}
	} else {
		ids := make([]peer.ID, 0, len(req.Peers))
		for _, s := range req.Peers {
			id, err := peer.Decode(s)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_input", "bad peer id: "+s)
				return
			}
			ids = append(ids, id)
			blocked = append(blocked, s)
		}
		net.SetBlocked(ids, true)
	}
	writeJSON(w, http.StatusOK, map[string]any{"blocked": blocked})
}

// Heal removes every simulated partition.
func (h *NodeHandler) Heal(w http.ResponseWriter, r *http.Request) {
	net := h.node.Network()
	if net == nil {
		writeError(w, http.StatusServiceUnavailable, "no_network", "networking is disabled")
		return
	}
	net.ClearBlocked()
	writeJSON(w, http.StatusOK, map[string]any{"blocked": []string{}})
}

// SetMiner changes this node's hashpower at runtime.
func (h *NodeHandler) SetMiner(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Threads *int `json:"threads"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Threads == nil {
		writeError(w, http.StatusBadRequest, "invalid_input", `body must be {"threads": n}`)
		return
	}
	m := h.node.Miner()
	if m == nil {
		writeError(w, http.StatusServiceUnavailable, "no_miner", "miner is not running")
		return
	}
	m.SetThreads(*req.Threads)
	writeJSON(w, http.StatusOK, map[string]any{"threads": m.Threads()})
}

func (h *NodeHandler) Validate(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.mineTimeout)
	defer cancel()
	rep, err := h.node.Validate(ctx)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp := map[string]any{
		"valid": rep.Valid, "blocks": rep.Blocks, "state_matches_live": rep.StateMatches,
		"total_supply": u(rep.TotalSupply), "expected_supply": u(rep.ExpectedSupply),
		"took_ms": float64(rep.Took.Microseconds()) / 1000,
	}
	if !rep.StateRoot.IsZero() {
		resp["replayed_state_root"] = rep.StateRoot.Hex()
	}
	if rep.Invalid != nil {
		resp["first_invalid_height"], resp["rule"], resp["reason"] = rep.Invalid.Height, rep.Invalid.Rule, rep.Invalid.Reason
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *NodeHandler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- helpers ---------------------------------------------------------------------

func u(n uint64) string { return strconv.FormatUint(n, 10) }

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

func toMempoolTxs(entries []mempool.Entry) []map[string]any {
	out := make([]map[string]any, len(entries))
	for i, e := range entries {
		out[i] = map[string]any{
			"hash": e.Hash.Hex(), "from": e.Sender.Hex(), "to": e.Tx.To.Hex(),
			"nonce": e.Tx.Nonce, "amount": u(e.Tx.Amount), "fee": u(e.Tx.Fee),
		}
	}
	return out
}
