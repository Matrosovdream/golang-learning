package p2p

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	"p2pchain/internal/domain"
)

// syncProtocol is a request/response protocol on its own libp2p stream.
// Gossip spreads NEW blocks; this is how a node fetches blocks it MISSED.
const syncProtocol = protocol.ID("/p2pchain/sync/1.0.0")

const (
	maxBlocksPerRequest = 64
	maxRequestBytes     = 1 << 20
	syncTimeout         = 20 * time.Second
)

// Status is a node's view of its own chain, exchanged when peers meet.
type Status struct {
	Genesis string `json:"genesis"`
	Tip     string `json:"tip"`
	Height  uint64 `json:"height"`
	Work    string `json:"work"` // decimal string: cumulative work can exceed uint64
}

type request struct {
	Type    string   `json:"type"`              // "status" | "blocks" | "block"
	Locator []string `json:"locator,omitempty"` // for "blocks": our chain's fingerprints, newest first
	Max     int      `json:"max,omitempty"`
	Hash    string   `json:"hash,omitempty"` // for "block"
}

type response struct {
	Status *Status  `json:"status,omitempty"`
	Blocks []string `json:"blocks,omitempty"` // hex-encoded blocks
	Error  string   `json:"error,omitempty"`
}

// handleSyncStream serves one request per stream.
func (n *Node) handleSyncStream(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(syncTimeout))

	if n.isBlocked(s.Conn().RemotePeer()) {
		return // partitioned: stay silent, as a broken link would
	}

	var req request
	if err := json.NewDecoder(io.LimitReader(s, maxRequestBytes)).Decode(&req); err != nil {
		return
	}

	var resp response
	switch req.Type {
	case "status":
		st := n.h.Status()
		resp.Status = &st
	case "blocks":
		locator := make([]domain.Hash, 0, len(req.Locator))
		for _, h := range req.Locator {
			if hash, err := domain.ParseHash(h); err == nil {
				locator = append(locator, hash)
			}
		}
		blocks, err := n.h.BlocksAfter(locator, min(max(req.Max, 1), maxBlocksPerRequest))
		if err != nil {
			resp.Error = err.Error()
		} else {
			resp.Blocks = encodeBlocks(blocks)
		}
	case "block":
		hash, err := domain.ParseHash(req.Hash)
		if err != nil {
			resp.Error = "bad hash"
			break
		}
		if b, ok := n.h.BlockByHash(hash); ok {
			resp.Blocks = encodeBlocks([]domain.Block{b})
		}
	default:
		resp.Error = "unknown request type"
	}
	_ = json.NewEncoder(s).Encode(resp)
}

func encodeBlocks(blocks []domain.Block) []string {
	out := make([]string, len(blocks))
	for i, b := range blocks {
		out[i] = hex.EncodeToString(domain.EncodeBlock(b))
	}
	return out
}

func (n *Node) ask(ctx context.Context, p peer.ID, req request) (response, error) {
	ctx, cancel := context.WithTimeout(ctx, syncTimeout)
	defer cancel()

	s, err := n.host.NewStream(ctx, p, syncProtocol)
	if err != nil {
		return response{}, err
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(syncTimeout))

	if err := json.NewEncoder(s).Encode(req); err != nil {
		return response{}, err
	}
	if err := s.CloseWrite(); err != nil {
		return response{}, err
	}
	var resp response
	if err := json.NewDecoder(io.LimitReader(s, maxRequestBytes)).Decode(&resp); err != nil {
		return response{}, err
	}
	if resp.Error != "" {
		return resp, fmt.Errorf("peer error: %s", resp.Error)
	}
	return resp, nil
}

// RequestStatus asks a peer where its chain is.
func (n *Node) RequestStatus(ctx context.Context, p peer.ID) (Status, error) {
	resp, err := n.ask(ctx, p, request{Type: "status"})
	if err != nil {
		return Status{}, err
	}
	if resp.Status == nil {
		return Status{}, fmt.Errorf("peer sent no status")
	}
	return *resp.Status, nil
}

// RequestBlocks sends our locator and receives the blocks that follow the
// newest block we both have.
func (n *Node) RequestBlocks(ctx context.Context, p peer.ID, locator []domain.Hash, max int) ([]domain.Block, error) {
	hexes := make([]string, len(locator))
	for i, h := range locator {
		hexes[i] = h.Hex()
	}
	resp, err := n.ask(ctx, p, request{Type: "blocks", Locator: hexes, Max: max})
	if err != nil {
		return nil, err
	}
	return decodeBlocks(resp.Blocks)
}

// RequestBlock fetches a single block by hash — used to chase down the missing
// parent of an orphan.
func (n *Node) RequestBlock(ctx context.Context, p peer.ID, h domain.Hash) (domain.Block, error) {
	resp, err := n.ask(ctx, p, request{Type: "block", Hash: h.Hex()})
	if err != nil {
		return domain.Block{}, err
	}
	blocks, err := decodeBlocks(resp.Blocks)
	if err != nil {
		return domain.Block{}, err
	}
	if len(blocks) == 0 {
		return domain.Block{}, fmt.Errorf("peer does not have block %s", h.Hex()[:12])
	}
	return blocks[0], nil
}

func decodeBlocks(hexes []string) ([]domain.Block, error) {
	out := make([]domain.Block, 0, len(hexes))
	for _, h := range hexes {
		raw, err := hex.DecodeString(h)
		if err != nil {
			return nil, err
		}
		b, err := domain.DecodeBlock(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}
