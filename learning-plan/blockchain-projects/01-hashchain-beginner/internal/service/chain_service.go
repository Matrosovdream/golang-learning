package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"hashchain/internal/domain"
)

// ErrMinerRunning is returned when the auto-miner is started twice.
var ErrMinerRunning = errors.New("auto-miner is already running")

// avgWindow is how many recent blocks Stats averages the block time over.
const avgWindow = 10

// ChainService is the node: it mines blocks, serves the chain, validates it,
// and (for the lesson) lets you tamper with it.
type ChainService struct {
	repo   domain.ChainRepository
	params domain.Params
	now    func() time.Time

	// miningSlot is a context-aware mutex: a channel with room for one token.
	// Whoever puts the token in may mine; taking it out releases the slot.
	// Unlike sync.Mutex, waiting for it can be abandoned when ctx is cancelled
	// (see acquireSlot). Only one miner may extend the tip at a time, otherwise
	// two blocks would be mined on the same parent and one would be wasted.
	miningSlot chan struct{}

	totalHashes atomic.Uint64 // every hash tried by this node, from many goroutines

	autoMu     sync.Mutex         // guards autoCancel and autoDone
	autoCancel context.CancelFunc // non-nil while the auto-miner runs
	autoDone   chan struct{}      // closed when the auto-miner goroutine exits
	autoMined  atomic.Uint64      // blocks mined by the auto-miner since startup
}

// NewChainService mines the genesis block and returns a ready node.
func NewChainService(ctx context.Context, repo domain.ChainRepository, params domain.Params) (*ChainService, error) {
	s := &ChainService{
		repo:       repo,
		params:     params,
		now:        time.Now,
		miningSlot: make(chan struct{}, 1),
	}
	genesis, err := domain.NewGenesis(ctx, params)
	s.totalHashes.Add(genesis.Attempts)
	if err != nil {
		return nil, fmt.Errorf("mine genesis: %w", err)
	}
	if err := repo.Append(genesis); err != nil {
		return nil, fmt.Errorf("store genesis: %w", err)
	}
	return s, nil
}

// Params returns the consensus parameters this node runs with.
func (s *ChainService) Params() domain.Params { return s.params }

// MineBlock mines one block containing data on top of the current tip.
// Cancelling ctx (client disconnect, timeout, shutdown) stops both the wait for
// the mining slot and the nonce search itself.
func (s *ChainService) MineBlock(ctx context.Context, data []byte) (domain.Block, error) {
	if err := s.acquireSlot(ctx); err != nil {
		return domain.Block{}, err
	}
	defer s.releaseSlot()
	return s.mineOnTip(ctx, data)
}

// mineOnTip builds the next header from the tip and mines it.
// The caller must hold the mining slot.
func (s *ChainService) mineOnTip(ctx context.Context, data []byte) (domain.Block, error) {
	tip, err := s.repo.Tip()
	if err != nil {
		return domain.Block{}, err
	}
	height := tip.Height + 1
	header := domain.Header{
		Height: height,
		// Never earlier than the parent, even if the clock stepped backwards.
		Timestamp:  max(s.now().UnixMilli(), tip.Timestamp),
		PrevHash:   tip.Hash,
		Difficulty: domain.NextDifficulty(s.params, height, s.headerAt),
	}

	b, err := domain.MineBlock(ctx, header, data)
	s.totalHashes.Add(b.Attempts)
	if err != nil {
		return domain.Block{}, err
	}
	if err := s.repo.Append(b); err != nil {
		return domain.Block{}, err
	}
	return b, nil
}

// headerAt reads a header from the repository for NextDifficulty.
func (s *ChainService) headerAt(height uint64) domain.Header {
	b, _ := s.repo.Get(height) // heights below the tip always exist
	return b.Header
}

// acquireSlot waits for the mining slot or for ctx to end, whichever is first.
func (s *ChainService) acquireSlot(ctx context.Context) error {
	select {
	case s.miningSlot <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *ChainService) releaseSlot() { <-s.miningSlot }

// --- auto-miner ---------------------------------------------------------------

// StartAutoMiner launches a background goroutine that mines blocks one after
// another — maxBlocks of them, or until StopAutoMiner if maxBlocks is 0.
// Continuous mining is what gives retargeting real block times to react to.
func (s *ChainService) StartAutoMiner(maxBlocks uint64) error {
	s.autoMu.Lock()
	defer s.autoMu.Unlock()
	if s.autoCancel != nil {
		return ErrMinerRunning
	}

	// The miner's context is NOT derived from any request: it must outlive the
	// HTTP call that started it. StopAutoMiner cancels it.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.autoCancel, s.autoDone = cancel, done

	go s.runAutoMiner(ctx, maxBlocks, done)
	return nil
}

func (s *ChainService) runAutoMiner(ctx context.Context, maxBlocks uint64, done chan struct{}) {
	defer close(done)
	defer s.clearAutoMiner(done) // runs first: forget ourselves if we stopped on our own

	for mined := uint64(0); maxBlocks == 0 || mined < maxBlocks; mined++ {
		if err := s.acquireSlot(ctx); err != nil {
			return // stopped while waiting for the slot
		}
		b, err := s.mineOnTip(ctx, fmt.Appendf(nil, "auto-mined block #%d", s.autoMined.Load()+1))
		s.releaseSlot()
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("auto-miner: %v", err)
			}
			return
		}
		s.autoMined.Add(1)
		log.Printf("auto-miner: block %d difficulty=%d attempts=%d in %s",
			b.Height, b.Difficulty, b.Attempts, b.MinedIn.Round(time.Millisecond))
	}
}

// clearAutoMiner resets the running state, but only if it still belongs to the
// goroutine that owns done (StopAutoMiner may already have cleared it).
func (s *ChainService) clearAutoMiner(done chan struct{}) {
	s.autoMu.Lock()
	defer s.autoMu.Unlock()
	if s.autoDone == done {
		s.autoCancel()
		s.autoCancel, s.autoDone = nil, nil
	}
}

// StopAutoMiner cancels the auto-miner and waits until its goroutine has
// exited, so no half-finished block is appended after it returns. It reports
// whether a miner was running.
func (s *ChainService) StopAutoMiner() bool {
	s.autoMu.Lock()
	cancel, done := s.autoCancel, s.autoDone
	s.autoCancel, s.autoDone = nil, nil
	s.autoMu.Unlock() // unlock BEFORE waiting: the goroutine's clearAutoMiner needs this lock

	if cancel == nil {
		return false
	}
	cancel()
	<-done
	return true
}

// AutoMinerRunning reports whether the auto-miner goroutine is active.
func (s *ChainService) AutoMinerRunning() bool {
	s.autoMu.Lock()
	defer s.autoMu.Unlock()
	return s.autoCancel != nil
}

// --- reads ----------------------------------------------------------------------

func (s *ChainService) Blocks(from uint64, limit int) []domain.Block {
	return s.repo.Range(from, limit)
}

func (s *ChainService) Block(height uint64) (domain.Block, error) { return s.repo.Get(height) }

func (s *ChainService) Latest() (domain.Block, error) { return s.repo.Tip() }

// ValidationReport is the result of validating the whole chain.
type ValidationReport struct {
	Valid   bool
	Blocks  int
	Invalid *domain.InvalidBlockError // nil when Valid
	Took    time.Duration
}

// Validate re-checks the entire chain from genesis, like a node that has just
// downloaded it from a stranger would.
func (s *ChainService) Validate() ValidationReport {
	start := time.Now()
	blocks := s.repo.All()
	report := ValidationReport{Valid: true, Blocks: len(blocks)}

	if err := domain.ValidateChain(s.params, blocks); err != nil {
		report.Valid = false
		if !errors.As(err, &report.Invalid) {
			report.Invalid = &domain.InvalidBlockError{Rule: "unknown", Reason: err.Error()}
		}
	}
	report.Took = time.Since(start)
	return report
}

// Stats is a snapshot of the node.
type Stats struct {
	Height           uint64
	TipHash          domain.Hash
	NextDifficulty   uint8
	AvgBlockTime     time.Duration // over the last AvgWindow blocks (0 until there are 2 non-genesis blocks)
	AvgWindow        int
	TotalHashes      uint64
	AutoMinerRunning bool
	AutoMined        uint64
}

func (s *ChainService) Stats() (Stats, error) {
	tip, err := s.repo.Tip()
	if err != nil {
		return Stats{}, err
	}
	st := Stats{
		Height:           tip.Height,
		TipHash:          tip.Hash,
		NextDifficulty:   domain.NextDifficulty(s.params, tip.Height+1, s.headerAt),
		TotalHashes:      s.totalHashes.Load(),
		AutoMinerRunning: s.AutoMinerRunning(),
		AutoMined:        s.autoMined.Load(),
	}
	// Average over real mining intervals only — genesis has a fixed old timestamp.
	if tip.Height >= 2 {
		from := uint64(1)
		if tip.Height > avgWindow+1 { // careful: tip.Height-avgWindow would wrap around below zero
			from = tip.Height - avgWindow
		}
		first, _ := s.repo.Get(from)
		st.AvgWindow = int(tip.Height - from)
		st.AvgBlockTime = time.Duration(tip.Timestamp-first.Timestamp) * time.Millisecond / time.Duration(st.AvgWindow)
	}
	return st, nil
}
