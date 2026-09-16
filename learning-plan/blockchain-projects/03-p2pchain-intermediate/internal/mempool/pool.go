// Package mempool holds valid transactions that are waiting to be mined.
//
// Every node has its own pool; "pending" is a node-local opinion, not a
// global fact. The pool is policy, not consensus: two nodes may keep different
// transactions, and a block is valid or not regardless of what's in any pool.
package mempool

import (
	"cmp"
	"container/heap"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"slices"
	"sync"
	"time"

	"p2pchain/internal/domain"
)

var (
	ErrAlreadyKnown           = errors.New("transaction already known")
	ErrNonceGap               = errors.New("nonce too far ahead of the account nonce")
	ErrReplacementUnderpriced = errors.New("replacement transaction underpriced")
	ErrPoolFull               = errors.New("mempool is full")
)

// Status says whether a pooled transaction could go into the next block.
type Status string

const (
	// StatusPending: every lower nonce of the sender is on-chain or pooled,
	// so this tx can be included as soon as those are.
	StatusPending Status = "pending"
	// StatusQueued: a lower nonce is missing (a gap). It waits until the gap is filled.
	StatusQueued Status = "queued"
)

// Config is node-local pool policy.
type Config struct {
	MaxSize     int    // maximum transactions held
	MaxNonceGap uint64 // how far above the account nonce a tx may be (queued)
}

// Entry is one pooled transaction plus what the pool learned about it.
type Entry struct {
	Tx     domain.Tx
	Hash   domain.Hash
	Sender domain.Address
	Added  time.Time
	seq    uint64 // arrival order, the tie-breaker between equal fees
}

// NonceFunc returns an account's nonce in the state the pool should assume (usually the tip).
type NonceFunc func(domain.Address) uint64

// Pool is a concurrency-safe mempool.
type Pool struct {
	cfg Config

	mu       sync.Mutex
	byHash   map[domain.Hash]*Entry
	bySender map[domain.Address]map[uint64]*Entry // sender → nonce → entry
	seq      uint64
	now      func() time.Time
}

func New(cfg Config) *Pool {
	return &Pool{
		cfg:      cfg,
		byHash:   make(map[domain.Hash]*Entry),
		bySender: make(map[domain.Address]map[uint64]*Entry),
		now:      time.Now,
	}
}

// Add admits a transaction whose chain ID, signature, and minimum fee the
// caller has already checked. acct is the sender's account at the tip.
//
// Replace-by-fee: a tx with the same sender and nonce as a pooled one replaces
// it only if it pays at least 10% more. That's how wallets "speed up" or
// "cancel" a stuck transaction — and the bump stops spam replacements that
// cost nothing.
func (p *Pool) Add(tx domain.Tx, sender domain.Address, acct domain.Account) (Status, error) {
	hash := tx.Hash()

	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := p.byHash[hash]; ok {
		return "", ErrAlreadyKnown
	}
	if tx.Nonce < acct.Nonce {
		return "", fmt.Errorf("%w: tx nonce %d, account nonce %d", domain.ErrNonceTooLow, tx.Nonce, acct.Nonce)
	}
	if tx.Nonce-acct.Nonce > p.cfg.MaxNonceGap {
		return "", fmt.Errorf("%w: tx nonce %d, account nonce %d, max gap %d", ErrNonceGap, tx.Nonce, acct.Nonce, p.cfg.MaxNonceGap)
	}
	// Only this tx's own cost is checked. Two pooled txs may together exceed the
	// balance; the block producer then includes the first and drops the other.
	cost, ok := tx.Cost()
	if !ok || cost > acct.Balance {
		return "", fmt.Errorf("%w: balance %d, cost %d", domain.ErrInsufficientFunds, acct.Balance, cost)
	}

	queue := p.bySender[sender]
	if old, ok := queue[tx.Nonce]; ok {
		if tx.Fee < bumped(old.Tx.Fee) {
			return "", fmt.Errorf("%w: fee %d, need at least %d to replace", ErrReplacementUnderpriced, tx.Fee, bumped(old.Tx.Fee))
		}
		delete(p.byHash, old.Hash) // replaced: the old tx is gone
	} else if len(p.byHash) >= p.cfg.MaxSize {
		return "", ErrPoolFull
	}

	if queue == nil {
		queue = make(map[uint64]*Entry)
		p.bySender[sender] = queue
	}
	p.seq++
	e := &Entry{Tx: tx, Hash: hash, Sender: sender, Added: p.now(), seq: p.seq}
	queue[tx.Nonce] = e
	p.byHash[hash] = e
	return p.statusLocked(e, acct.Nonce), nil
}

// bumped is the minimum fee that may replace a tx paying fee: +10%, at least +1.
func bumped(fee uint64) uint64 {
	sum, carry := bits.Add64(fee, max(fee/10, 1), 0)
	if carry != 0 {
		return math.MaxUint64
	}
	return sum
}

// statusLocked walks up from the account nonce: pending if every nonce below
// the tx's is pooled. The caller holds p.mu.
func (p *Pool) statusLocked(e *Entry, accountNonce uint64) Status {
	queue := p.bySender[e.Sender]
	for n := accountNonce; n < e.Tx.Nonce; n++ {
		if _, ok := queue[n]; !ok {
			return StatusQueued
		}
	}
	return StatusPending
}

// Get returns a pooled transaction and its status.
func (p *Pool) Get(hash domain.Hash, nonceOf NonceFunc) (Entry, Status, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.byHash[hash]
	if !ok {
		return Entry{}, "", false
	}
	return *e, p.statusLocked(e, nonceOf(e.Sender)), true
}

// PendingNonce is the nonce a wallet should use for its next transaction:
// the account nonce, advanced past every consecutive nonce already pooled.
func (p *Pool) PendingNonce(sender domain.Address, accountNonce uint64) uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := accountNonce
	for {
		if _, ok := p.bySender[sender][n]; !ok {
			return n
		}
		n++
	}
}

// Select picks up to max transactions for the next block: highest fee first,
// but never a sender's nonce N+1 before their nonce N.
//
// A max-heap holds only each sender's NEXT executable tx. Pop the best, and
// push that sender's following nonce if it's pooled. Every choice is valid
// ordering-wise, and the fee order is as good as the nonce rule allows.
func (p *Pool) Select(max int, nonceOf NonceFunc) []Entry {
	p.mu.Lock()
	defer p.mu.Unlock()

	h := &byFee{}
	for sender, queue := range p.bySender {
		if e, ok := queue[nonceOf(sender)]; ok {
			heap.Push(h, e)
		}
	}

	var out []Entry
	for len(out) < max && h.Len() > 0 {
		e := heap.Pop(h).(*Entry)
		out = append(out, *e)
		if next, ok := p.bySender[e.Sender][e.Tx.Nonce+1]; ok {
			heap.Push(h, next)
		}
	}
	return out
}

// Prune drops transactions whose nonce is now below the account nonce: those
// just mined, and any competitor that used the same nonce. Call after a block.
func (p *Pool) Prune(nonceOf NonceFunc) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	removed := 0
	for sender, queue := range p.bySender {
		accountNonce := nonceOf(sender)
		for nonce, e := range queue {
			if nonce < accountNonce {
				p.deleteLocked(e)
				removed++
			}
		}
	}
	return removed
}

// Remove drops specific transactions (e.g. ones that turned out unaffordable).
func (p *Pool) Remove(hashes ...domain.Hash) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, h := range hashes {
		if e, ok := p.byHash[h]; ok {
			p.deleteLocked(e)
		}
	}
}

func (p *Pool) deleteLocked(e *Entry) {
	delete(p.byHash, e.Hash)
	delete(p.bySender[e.Sender], e.Tx.Nonce)
	if len(p.bySender[e.Sender]) == 0 {
		delete(p.bySender, e.Sender)
	}
}

// Snapshot lists every pooled tx split by status, ordered by sender then nonce.
func (p *Pool) Snapshot(nonceOf NonceFunc) (pending, queued []Entry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range p.byHash {
		if p.statusLocked(e, nonceOf(e.Sender)) == StatusPending {
			pending = append(pending, *e)
		} else {
			queued = append(queued, *e)
		}
	}
	order := func(a, b Entry) int {
		if c := slices.Compare(a.Sender[:], b.Sender[:]); c != 0 {
			return c
		}
		return cmp.Compare(a.Tx.Nonce, b.Tx.Nonce)
	}
	slices.SortFunc(pending, order)
	slices.SortFunc(queued, order)
	return pending, queued
}

// Len returns how many transactions are pooled.
func (p *Pool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.byHash)
}

// byFee is a max-heap of entries: higher fee first, earlier arrival on ties.
type byFee []*Entry

func (h byFee) Len() int { return len(h) }
func (h byFee) Less(i, j int) bool {
	if h[i].Tx.Fee != h[j].Tx.Fee {
		return h[i].Tx.Fee > h[j].Tx.Fee
	}
	return h[i].seq < h[j].seq
}
func (h byFee) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *byFee) Push(x any)   { *h = append(*h, x.(*Entry)) }
func (h *byFee) Pop() any {
	old := *h
	e := old[len(old)-1]
	*h = old[:len(old)-1]
	return e
}
