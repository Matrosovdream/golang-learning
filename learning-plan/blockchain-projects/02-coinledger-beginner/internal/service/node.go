package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"coinledger/internal/domain"
	"coinledger/internal/mempool"
)

// Node is a single coinledger node: it accepts transactions into its mempool,
// produces blocks, stores them, and answers queries.
type Node struct {
	params domain.Params
	store  domain.ChainStore
	pool   *mempool.Pool
	miner  domain.Address // node policy: who gets this node's rewards and fees
	now    func() time.Time

	// mineSlot lets only one block be produced at a time (a ctx-aware mutex, as in Project 1).
	mineSlot chan struct{}

	// mu guards tip and state. state is never modified after being installed,
	// only replaced, so code may keep using a State it read under the lock.
	mu    sync.RWMutex
	tip   domain.Block
	state domain.State
}

// Options are the node's startup choices.
type Options struct {
	Miner        domain.Address
	RebuildState bool // replay every block to recompute (and repair) the stored state
}

// NewNode opens the chain in store: it creates genesis on an empty store, or
// checks that the stored chain matches the configured params and loads the tip.
func NewNode(ctx context.Context, p domain.Params, store domain.ChainStore, pool *mempool.Pool, opts Options) (*Node, error) {
	n := &Node{
		params:   p,
		store:    store,
		pool:     pool,
		miner:    opts.Miner,
		now:      time.Now,
		mineSlot: make(chan struct{}, 1),
	}

	genesis, genesisState, err := domain.NewGenesis(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("genesis: %w", err)
	}

	empty, err := store.IsEmpty()
	if err != nil {
		return nil, err
	}
	if empty {
		if err := store.SetParamsFingerprint(p.Fingerprint()); err != nil {
			return nil, err
		}
		if err := store.CommitBlock(genesis, domain.Diff(domain.State{}, genesisState)); err != nil {
			return nil, fmt.Errorf("store genesis: %w", err)
		}
		n.tip, n.state = genesis, genesisState
		return n, nil
	}

	// Same data directory, different consensus params (another chain ID, reward,
	// allocation…)? Then every stored block would be judged by the wrong rules.
	if fp, err := store.ParamsFingerprint(); err != nil || fp != p.Fingerprint() {
		return nil, fmt.Errorf("%w (chain id, difficulty, reward, min fee, max block txs, or genesis alloc changed); use a new DATA_DIR",
			domain.ErrParamsMismatch)
	}
	stored, err := store.BlockByHeight(0)
	if err != nil {
		return nil, fmt.Errorf("load genesis: %w", err)
	}
	if stored.Hash != genesis.Hash {
		return nil, fmt.Errorf("%w: stored %s, configured %s", domain.ErrGenesisMismatch, stored.Hash.Hex(), genesis.Hash.Hex())
	}

	height, err := store.TipHeight()
	if err != nil {
		return nil, err
	}
	if n.tip, err = store.BlockByHeight(height); err != nil {
		return nil, err
	}

	if opts.RebuildState {
		start := time.Now()
		replayed, err := n.replay()
		if err != nil {
			return nil, fmt.Errorf("rebuild state: %w", err)
		}
		if err := store.ReplaceState(replayed); err != nil {
			return nil, err
		}
		log.Printf("state rebuilt by replaying %d blocks in %s (root %s)",
			height+1, time.Since(start).Round(time.Millisecond), replayed.Root().Hex())
	}

	if n.state, err = store.LoadState(); err != nil {
		return nil, err
	}
	// Cheap integrity check on every start: the stored accounts must hash to
	// the state root the tip block committed to.
	if root := n.state.Root(); root != n.tip.StateRoot {
		return nil, fmt.Errorf("stored state root %s does not match tip state root %s (restart with REBUILD_STATE=true)",
			root.Hex(), n.tip.StateRoot.Hex())
	}
	return n, nil
}

// replay re-validates every stored block from genesis and returns the final
// state. The state is derived data: the blocks alone are enough to rebuild it.
func (n *Node) replay() (domain.State, error) {
	var parent *domain.Block
	var state domain.State
	err := n.store.ForEachBlock(func(b domain.Block) error {
		next, err := domain.ValidateBlock(n.params, parent, state, b)
		if err != nil {
			return err
		}
		parent, state = &b, next
		return nil
	})
	return state, err
}

// snapshot returns the tip and its state together.
func (n *Node) snapshot() (domain.Block, domain.State) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.tip, n.state
}

func nonceIn(s domain.State) mempool.NonceFunc {
	return func(a domain.Address) uint64 { return s.Get(a).Nonce }
}

// --- writes -------------------------------------------------------------------

// SubmitResult reports what happened to a submitted transaction.
type SubmitResult struct {
	Hash   domain.Hash
	Sender domain.Address
	Status mempool.Status
}

// SubmitTx decodes a raw transaction, checks what can be checked statelessly,
// and admits it to the mempool against the tip state.
func (n *Node) SubmitTx(raw []byte) (SubmitResult, error) {
	tx, err := domain.DecodeTx(raw)
	if err != nil {
		return SubmitResult{}, err
	}
	if tx.ChainID != n.params.ChainID {
		return SubmitResult{}, fmt.Errorf("%w: tx is for chain %d, this is chain %d", domain.ErrWrongChainID, tx.ChainID, n.params.ChainID)
	}
	sender, err := tx.Sender()
	if err != nil {
		return SubmitResult{}, err
	}
	if tx.Fee < n.params.MinFee {
		return SubmitResult{}, fmt.Errorf("%w: fee %d, minimum %d", domain.ErrFeeTooLow, tx.Fee, n.params.MinFee)
	}
	hash := tx.Hash()
	if _, _, err := n.store.TxLocation(hash); err == nil {
		return SubmitResult{}, fmt.Errorf("%w: already mined", mempool.ErrAlreadyKnown)
	}

	// Hold the read lock while adding, so a block can't be committed between
	// reading the sender's account and admitting the tx against it.
	n.mu.RLock()
	defer n.mu.RUnlock()
	status, err := n.pool.Add(tx, sender, n.state.Get(sender))
	if err != nil {
		return SubmitResult{}, err
	}
	return SubmitResult{Hash: hash, Sender: sender, Status: status}, nil
}

// Mine produces one block on top of the tip from the best mempool transactions.
func (n *Node) Mine(ctx context.Context) (domain.Block, error) {
	select {
	case n.mineSlot <- struct{}{}:
	case <-ctx.Done():
		return domain.Block{}, ctx.Err()
	}
	defer func() { <-n.mineSlot }()

	parent, parentState := n.snapshot()

	// 1. Choose transactions. Trial-apply each to a scratch state: the pool
	//    only checked each tx on its own, and together they may not fit.
	scratch := parentState.Clone()
	var included []domain.Tx
	var unaffordable []domain.Hash
	for _, e := range n.pool.Select(n.params.MaxBlockTxs, nonceIn(parentState)) {
		if _, err := domain.ApplyTx(n.params, scratch, e.Tx, n.miner); err != nil {
			if errors.Is(err, domain.ErrInsufficientFunds) {
				unaffordable = append(unaffordable, e.Hash) // earlier txs spent the money
			}
			// nonce too high: an earlier tx of this sender was skipped; it stays pooled.
			continue
		}
		included = append(included, e.Tx)
	}

	// 2. Execute the block with the SAME function every validator will run,
	//    and commit to the result in the header.
	draft := domain.Block{Header: domain.Header{Miner: n.miner}, Txs: included}
	post, err := domain.ApplyBlock(n.params, parentState, draft)
	if err != nil {
		return domain.Block{}, fmt.Errorf("build block: %w", err) // a bug: trial application just succeeded
	}
	header := domain.Header{
		Height:     parent.Height + 1,
		Timestamp:  max(n.now().UnixMilli(), parent.Timestamp),
		PrevHash:   parent.Hash,
		TxRoot:     domain.ComputeTxRoot(included),
		StateRoot:  post.Root(),
		Miner:      n.miner,
		Difficulty: n.params.Difficulty,
	}

	// 3. Do the work (no locks held — this is the slow part).
	b, err := domain.Seal(ctx, header, included)
	if err != nil {
		return domain.Block{}, err
	}

	// 4. Persist, then install as the new tip.
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.tip.Hash != parent.Hash {
		return domain.Block{}, domain.ErrTipChanged
	}
	if err := n.store.CommitBlock(b, domain.Diff(parentState, post)); err != nil {
		return domain.Block{}, fmt.Errorf("commit block: %w", err)
	}
	n.tip, n.state = b, post

	n.pool.Prune(nonceIn(post)) // mined txs, and anything that reused their nonces
	n.pool.Remove(unaffordable...)
	return b, nil
}

// RunBlockProducer mines a block every interval until ctx is cancelled.
func (n *Node) RunBlockProducer(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b, err := n.Mine(ctx)
			if err != nil {
				if ctx.Err() == nil {
					log.Printf("block producer: %v", err)
				}
				continue
			}
			log.Printf("mined block %d with %d txs (%d attempts, %s)",
				b.Height, len(b.Txs), b.Attempts, b.MinedIn.Round(time.Millisecond))
		}
	}
}

// --- reads --------------------------------------------------------------------

// AccountView is an account plus the nonce a wallet should use next.
type AccountView struct {
	Address      domain.Address
	Account      domain.Account
	PendingNonce uint64 // account nonce advanced past this sender's pooled txs
	TipHeight    uint64
}

func (n *Node) Account(addr domain.Address) AccountView {
	tip, state := n.snapshot()
	acct := state.Get(addr)
	return AccountView{
		Address:      addr,
		Account:      acct,
		PendingNonce: n.pool.PendingNonce(addr, acct.Nonce),
		TipHeight:    tip.Height,
	}
}

// TxStatus is where a transaction is in its life.
type TxStatus string

const (
	TxPending  TxStatus = "pending"
	TxQueued   TxStatus = "queued"
	TxIncluded TxStatus = "included"
)

// TxView is a transaction and what the node knows about it.
type TxView struct {
	Tx            domain.Tx
	Hash          domain.Hash
	Sender        domain.Address
	Status        TxStatus
	BlockHeight   uint64 // included only
	Index         int    // included only
	Confirmations uint64 // included only: tip height − block height + 1
}

// Tx finds a transaction in the chain or in the mempool.
func (n *Node) Tx(hash domain.Hash) (TxView, error) {
	tip, state := n.snapshot()

	if height, index, err := n.store.TxLocation(hash); err == nil {
		b, err := n.store.BlockByHeight(height)
		if err != nil {
			return TxView{}, err
		}
		tx := b.Txs[index]
		sender, _ := tx.Sender() // valid: it's in a validated block
		return TxView{
			Tx: tx, Hash: hash, Sender: sender, Status: TxIncluded,
			BlockHeight: height, Index: index, Confirmations: tip.Height - height + 1,
		}, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return TxView{}, err
	}

	if e, status, ok := n.pool.Get(hash, nonceIn(state)); ok {
		return TxView{Tx: e.Tx, Hash: hash, Sender: e.Sender, Status: TxStatus(status)}, nil
	}
	return TxView{}, domain.ErrNotFound
}

func (n *Node) Latest() domain.Block {
	tip, _ := n.snapshot()
	return tip
}

func (n *Node) Block(height uint64) (domain.Block, error) { return n.store.BlockByHeight(height) }

func (n *Node) Blocks(from uint64, limit int) ([]domain.Block, error) {
	return n.store.Blocks(from, limit)
}

// Mempool lists pooled transactions by status.
func (n *Node) Mempool() (pending, queued []mempool.Entry) {
	_, state := n.snapshot()
	return n.pool.Snapshot(nonceIn(state))
}

// ProofView is everything a light client needs to check that a transaction is
// in a block: the raw header (to hash and check its work) and the Merkle path
// from the transaction to that header's TxRoot.
type ProofView struct {
	TxHash    domain.Hash
	Height    uint64
	Index     int
	HeaderRaw []byte
	Steps     []domain.ProofStep
}

func (n *Node) Proof(height uint64, txHash domain.Hash) (ProofView, error) {
	b, err := n.store.BlockByHeight(height)
	if err != nil {
		return ProofView{}, err
	}
	hashes := domain.TxHashes(b.Txs)
	for i, h := range hashes {
		if h != txHash {
			continue
		}
		steps, err := domain.MerkleProof(hashes, i)
		if err != nil {
			return ProofView{}, err
		}
		return ProofView{TxHash: txHash, Height: height, Index: i, HeaderRaw: b.Header.Encode(), Steps: steps}, nil
	}
	return ProofView{}, domain.ErrNotFound
}

// ValidationReport is the result of re-validating the whole stored chain.
type ValidationReport struct {
	Valid          bool
	Blocks         uint64
	Invalid        *domain.InvalidBlockError
	StateRoot      domain.Hash // after replaying every block
	StateMatches   bool        // replayed root == the live node's state root
	TotalSupply    uint64
	ExpectedSupply uint64
	Took           time.Duration
}

// Validate replays every block from genesis, as a brand-new node would, and
// checks that the result matches the live state and the supply rule.
func (n *Node) Validate(ctx context.Context) (ValidationReport, error) {
	// Hold the mining slot so no block is committed while we compare.
	select {
	case n.mineSlot <- struct{}{}:
	case <-ctx.Done():
		return ValidationReport{}, ctx.Err()
	}
	defer func() { <-n.mineSlot }()

	start := time.Now()
	tip, live := n.snapshot()
	rep := ValidationReport{Blocks: tip.Height + 1}

	replayed, err := n.replay()
	if err != nil {
		if !errors.As(err, &rep.Invalid) {
			return rep, err
		}
		rep.Took = time.Since(start)
		return rep, nil
	}

	rep.StateRoot = replayed.Root()
	rep.StateMatches = rep.StateRoot == live.Root()
	rep.TotalSupply, _ = replayed.TotalSupply()
	rep.ExpectedSupply, _ = domain.ExpectedSupply(n.params, tip.Height)
	rep.Valid = rep.StateMatches && rep.TotalSupply == rep.ExpectedSupply
	rep.Took = time.Since(start)
	return rep, nil
}

// Stats is a snapshot of the node.
type Stats struct {
	Height         uint64
	TipHash        domain.Hash
	Accounts       int
	TotalSupply    uint64
	ExpectedSupply uint64
	Pending        int
	Queued         int
	Miner          domain.Address
	Params         domain.Params
}

func (n *Node) Stats() Stats {
	tip, state := n.snapshot()
	pending, queued := n.pool.Snapshot(nonceIn(state))
	supply, _ := state.TotalSupply()
	expected, _ := domain.ExpectedSupply(n.params, tip.Height)
	return Stats{
		Height: tip.Height, TipHash: tip.Hash, Accounts: len(state),
		TotalSupply: supply, ExpectedSupply: expected,
		Pending: len(pending), Queued: len(queued),
		Miner: n.miner, Params: n.params,
	}
}
