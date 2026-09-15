package handler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strconv"
	"time"

	"hashchain/internal/config"
	"hashchain/internal/domain"
	"hashchain/internal/service"
)

// ChainHandler turns HTTP requests into ChainService calls and writes responses.
type ChainHandler struct {
	svc *service.ChainService
	cfg config.Config
}

func NewChainHandler(svc *service.ChainService, cfg config.Config) *ChainHandler {
	return &ChainHandler{svc: svc, cfg: cfg}
}

// --- DTOs ---------------------------------------------------------------------

type mineRequest struct {
	Data string `json:"data"`
}

type tamperRequest struct {
	Data string `json:"data"`
	Mode string `json:"mode"` // body | header (default) | remine | rewrite
}

type minerStartRequest struct {
	MaxBlocks uint64 `json:"max_blocks"` // 0 = until stopped
}

type blockResponse struct {
	Height          uint64  `json:"height"`
	Hash            string  `json:"hash"`
	PrevHash        string  `json:"prev_hash"`
	Timestamp       string  `json:"timestamp"`
	Data            string  `json:"data"`
	DataHash        string  `json:"data_hash"`
	Difficulty      uint8   `json:"difficulty"`
	Nonce           uint64  `json:"nonce"`
	LeadingZeroBits int     `json:"leading_zero_bits"`
	Attempts        uint64  `json:"attempts"`
	MinedInMS       float64 `json:"mined_in_ms"`
}

type validationResponse struct {
	Valid              bool    `json:"valid"`
	Blocks             int     `json:"blocks"`
	FirstInvalidHeight *uint64 `json:"first_invalid_height,omitempty"`
	Rule               string  `json:"rule,omitempty"`
	Reason             string  `json:"reason,omitempty"`
	TookMS             float64 `json:"took_ms"`
}

type tamperResponse struct {
	Mode       string             `json:"mode"`
	Height     uint64             `json:"height"`
	Remined    int                `json:"remined_blocks"`
	Attempts   uint64             `json:"attempts"`
	TookMS     float64            `json:"took_ms"`
	OriginalMS float64            `json:"originally_took_ms"`
	Validation validationResponse `json:"validation_after"`
}

type statsResponse struct {
	Height           uint64  `json:"height"`
	TipHash          string  `json:"tip_hash"`
	NextDifficulty   uint8   `json:"next_difficulty"`
	NextTarget       string  `json:"next_target"`
	ExpectedAttempts string  `json:"expected_attempts"` // 2^difficulty, as a string: it can exceed 2^53
	AvgBlockTimeMS   float64 `json:"avg_block_time_ms"`
	AvgWindow        int     `json:"avg_window_blocks"`
	TotalHashes      uint64  `json:"total_hashes"`
	AutoMinerRunning bool    `json:"auto_miner_running"`
	AutoMined        uint64  `json:"auto_mined_blocks"`
	Params           params  `json:"params"`
}

type params struct {
	InitialDifficulty uint8  `json:"initial_difficulty"`
	MinDifficulty     uint8  `json:"min_difficulty"`
	MaxDifficulty     uint8  `json:"max_difficulty"`
	RetargetInterval  uint64 `json:"retarget_interval"`
	TargetBlockTimeMS int64  `json:"target_block_time_ms"`
}

// --- handlers -----------------------------------------------------------------

// List returns blocks from ?from= (default 0), at most ?limit= (default 20, max 100).
func (h *ChainHandler) List(w http.ResponseWriter, r *http.Request) {
	from, err := queryUint(r, "from", 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, "from must be a non-negative integer")
		return
	}
	limit, err := queryUint(r, "limit", 20)
	if err != nil || limit == 0 || limit > 100 {
		writeError(w, http.StatusBadRequest, "limit must be between 1 and 100")
		return
	}
	blocks := h.svc.Blocks(from, int(limit))
	resp := make([]blockResponse, len(blocks))
	for i, b := range blocks {
		resp[i] = toBlockResponse(b)
	}
	writeJSON(w, http.StatusOK, resp)
}

// Latest returns the tip.
func (h *ChainHandler) Latest(w http.ResponseWriter, r *http.Request) {
	b, err := h.svc.Latest()
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toBlockResponse(b))
}

// Get returns one block by height.
func (h *ChainHandler) Get(w http.ResponseWriter, r *http.Request) {
	height, ok := parseHeight(w, r)
	if !ok {
		return
	}
	b, err := h.svc.Block(height)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toBlockResponse(b))
}

// Mine mines a block with the given data and appends it (synchronously).
func (h *ChainHandler) Mine(w http.ResponseWriter, r *http.Request) {
	var req mineRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if len(req.Data) > h.cfg.MaxDataBytes {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("data is larger than %d bytes", h.cfg.MaxDataBytes))
		return
	}

	// r.Context() ends if the client disconnects (or the server shuts down);
	// the timeout bounds how long a hard block may pin a CPU core.
	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.MineTimeout)
	defer cancel()

	b, err := h.svc.MineBlock(ctx, []byte(req.Data))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toBlockResponse(b))
}

// Validate re-checks the whole chain.
func (h *ChainHandler) Validate(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, toValidationResponse(h.svc.Validate()))
}

// Tamper edits a block's data (debug endpoint) and reports the validation result afterwards.
func (h *ChainHandler) Tamper(w http.ResponseWriter, r *http.Request) {
	height, ok := parseHeight(w, r)
	if !ok {
		return
	}
	req := tamperRequest{Mode: string(service.TamperHeader)}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if len(req.Data) > h.cfg.MaxDataBytes {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("data is larger than %d bytes", h.cfg.MaxDataBytes))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.MineTimeout)
	defer cancel()

	res, err := h.svc.Tamper(ctx, height, []byte(req.Data), service.TamperMode(req.Mode))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, tamperResponse{
		Mode:       string(res.Mode),
		Height:     res.Height,
		Remined:    res.Remined,
		Attempts:   res.Attempts,
		TookMS:     ms(res.Took),
		OriginalMS: ms(res.Originally),
		Validation: toValidationResponse(h.svc.Validate()),
	})
}

// MinerStart launches the background auto-miner.
func (h *ChainHandler) MinerStart(w http.ResponseWriter, r *http.Request) {
	var req minerStartRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if err := h.svc.StartAutoMiner(req.MaxBlocks); err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"running": true, "max_blocks": req.MaxBlocks})
}

// MinerStop stops the auto-miner and waits for it to exit.
func (h *ChainHandler) MinerStop(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"stopped": h.svc.StopAutoMiner()})
}

// Stats returns a snapshot of the node.
func (h *ChainHandler) Stats(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Stats()
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	p := h.svc.Params()
	writeJSON(w, http.StatusOK, statsResponse{
		Height:           st.Height,
		TipHash:          st.TipHash.String(),
		NextDifficulty:   st.NextDifficulty,
		NextTarget:       fmt.Sprintf("%064x", domain.Target(st.NextDifficulty)),
		ExpectedAttempts: new(big.Int).Lsh(big.NewInt(1), uint(st.NextDifficulty)).String(),
		AvgBlockTimeMS:   ms(st.AvgBlockTime),
		AvgWindow:        st.AvgWindow,
		TotalHashes:      st.TotalHashes,
		AutoMinerRunning: st.AutoMinerRunning,
		AutoMined:        st.AutoMined,
		Params: params{
			InitialDifficulty: p.InitialDifficulty,
			MinDifficulty:     p.MinDifficulty,
			MaxDifficulty:     p.MaxDifficulty,
			RetargetInterval:  p.RetargetInterval,
			TargetBlockTimeMS: p.TargetBlockTime.Milliseconds(),
		},
	})
}

// Health is a trivial liveness probe.
func (h *ChainHandler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- helpers ------------------------------------------------------------------

func parseHeight(w http.ResponseWriter, r *http.Request) (uint64, bool) {
	height, err := strconv.ParseUint(r.PathValue("height"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "height must be a non-negative integer")
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

// ms converts a duration to milliseconds with two decimals.
func ms(d time.Duration) float64 {
	return float64(d.Microseconds()/10) / 100
}

func toBlockResponse(b domain.Block) blockResponse {
	return blockResponse{
		Height:          b.Height,
		Hash:            b.Hash.String(),
		PrevHash:        b.PrevHash.String(),
		Timestamp:       time.UnixMilli(b.Timestamp).UTC().Format(time.RFC3339Nano),
		Data:            string(b.Data),
		DataHash:        b.DataHash.String(),
		Difficulty:      b.Difficulty,
		Nonce:           b.Nonce,
		LeadingZeroBits: domain.LeadingZeroBits(b.Hash),
		Attempts:        b.Attempts,
		MinedInMS:       ms(b.MinedIn),
	}
}

func toValidationResponse(rep service.ValidationReport) validationResponse {
	resp := validationResponse{Valid: rep.Valid, Blocks: rep.Blocks, TookMS: ms(rep.Took)}
	if rep.Invalid != nil {
		resp.FirstInvalidHeight = &rep.Invalid.Height
		resp.Rule = rep.Invalid.Rule
		resp.Reason = rep.Invalid.Reason
	}
	return resp
}

// writeServiceError maps domain and service errors to HTTP status codes.
func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var ie domain.InputError
	switch {
	case errors.As(err, &ie):
		writeError(w, http.StatusBadRequest, ie.Error())
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "block not found")
	case errors.Is(err, service.ErrMinerRunning):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "mining took longer than the timeout; lower DIFFICULTY or raise MINE_TIMEOUT_S")
	case errors.Is(err, context.Canceled):
		// The client went away or the server is shutting down; nobody may be listening.
		log.Printf("%s %s: cancelled", r.Method, r.URL.Path)
		writeError(w, http.StatusServiceUnavailable, "request cancelled")
	default:
		log.Printf("%s %s: %v", r.Method, r.URL.Path, err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
