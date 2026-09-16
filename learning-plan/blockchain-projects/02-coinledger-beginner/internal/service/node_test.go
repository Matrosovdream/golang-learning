package service

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"

	"coinledger/internal/domain"
	"coinledger/internal/mempool"
	"coinledger/internal/repository/boltstore"
)

type actor struct {
	key  *ecdsa.PrivateKey
	addr domain.Address
}

func newActor(t *testing.T, n byte) actor {
	t.Helper()
	b := make([]byte, 32)
	b[31] = n
	k, err := crypto.ToECDSA(b)
	if err != nil {
		t.Fatal(err)
	}
	return actor{k, crypto.PubkeyToAddress(k.PublicKey)}
}

type fixture struct {
	t                        *testing.T
	path                     string
	params                   domain.Params
	alice, bob, carol, miner actor
	node                     *Node
	store                    *boltstore.Store
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{
		t:     t,
		path:  filepath.Join(t.TempDir(), "chain.db"),
		alice: newActor(t, 1), bob: newActor(t, 2), carol: newActor(t, 3), miner: newActor(t, 9),
	}
	f.params = domain.Params{
		ChainID: 1337, Difficulty: 8, BlockReward: 50, MinFee: 1, MaxBlockTxs: 10,
		GenesisAlloc: map[domain.Address]uint64{f.alice.addr: 100},
	}
	f.start(false)
	t.Cleanup(func() { f.store.Close() })
	return f
}

// start opens the store and a node on it. Calling it again simulates a restart.
func (f *fixture) start(rebuild bool) error {
	f.t.Helper()
	if f.store != nil {
		f.store.Close()
	}
	store, err := boltstore.Open(f.path)
	if err != nil {
		f.t.Fatal(err)
	}
	f.store = store
	pool := mempool.New(mempool.Config{MaxSize: 100, MaxNonceGap: 8})
	node, err := NewNode(context.Background(), f.params, store, pool, Options{Miner: f.miner.addr, RebuildState: rebuild})
	f.node = node
	return err
}

func (f *fixture) send(from actor, nonce uint64, to domain.Address, amount, fee uint64) (SubmitResult, error) {
	f.t.Helper()
	tx, err := domain.SignTx(domain.Tx{ChainID: f.params.ChainID, Nonce: nonce, To: to, Amount: amount, Fee: fee}, from.key)
	if err != nil {
		f.t.Fatal(err)
	}
	return f.node.SubmitTx(tx.Encode())
}

func (f *fixture) mine() domain.Block {
	f.t.Helper()
	b, err := f.node.Mine(context.Background())
	if err != nil {
		f.t.Fatal(err)
	}
	return b
}

func (f *fixture) balance(a actor) uint64 { return f.node.Account(a.addr).Account.Balance }

func (f *fixture) mustValidate() {
	f.t.Helper()
	rep, err := f.node.Validate(context.Background())
	if err != nil || !rep.Valid {
		f.t.Fatalf("chain invalid: %+v (invalid %+v) %v", rep, rep.Invalid, err)
	}
}

func TestTransferEndToEnd(t *testing.T) {
	f := newFixture(t)

	res, err := f.send(f.alice, 0, f.bob.addr, 30, 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != mempool.StatusPending || res.Sender != f.alice.addr {
		t.Fatalf("submit: %+v", res)
	}
	if v, _ := f.node.Tx(res.Hash); v.Status != TxPending {
		t.Fatalf("before mining: %s", v.Status)
	}

	b := f.mine()
	if len(b.Txs) != 1 {
		t.Fatalf("block has %d txs", len(b.Txs))
	}
	if f.balance(f.alice) != 68 || f.balance(f.bob) != 30 || f.balance(f.miner) != 52 {
		t.Fatalf("balances alice %d bob %d miner %d", f.balance(f.alice), f.balance(f.bob), f.balance(f.miner))
	}
	v, _ := f.node.Tx(res.Hash)
	if v.Status != TxIncluded || v.BlockHeight != 1 || v.Confirmations != 1 {
		t.Fatalf("after mining: %+v", v)
	}
	f.mine()
	if v, _ := f.node.Tx(res.Hash); v.Confirmations != 2 {
		t.Fatalf("confirmations = %d after another block", v.Confirmations)
	}
	f.mustValidate()
}

func TestReplayOfAMinedTxIsRejected(t *testing.T) {
	f := newFixture(t)
	tx, _ := domain.SignTx(domain.Tx{ChainID: 1337, To: f.bob.addr, Amount: 10, Fee: 1}, f.alice.key)
	if _, err := f.node.SubmitTx(tx.Encode()); err != nil {
		t.Fatal(err)
	}
	f.mine()
	if _, err := f.node.SubmitTx(tx.Encode()); !errors.Is(err, mempool.ErrAlreadyKnown) {
		t.Fatalf("replay: err = %v, want ErrAlreadyKnown", err)
	}
	// Re-signed with the same nonce (so a different hash): the nonce rule stops it.
	if _, err := f.send(f.alice, 0, f.bob.addr, 11, 1); !errors.Is(err, domain.ErrNonceTooLow) {
		t.Fatalf("same nonce, new tx: err = %v, want ErrNonceTooLow", err)
	}
	if f.balance(f.bob) != 10 {
		t.Fatalf("bob = %d", f.balance(f.bob))
	}
}

// Alice tries to pay bob AND carol with nonce 0. Only one can win.
func TestDoubleSpendWithTheSameNonce(t *testing.T) {
	f := newFixture(t)
	if _, err := f.send(f.alice, 0, f.bob.addr, 90, 5); err != nil {
		t.Fatal(err)
	}
	if _, err := f.send(f.alice, 0, f.carol.addr, 90, 5); !errors.Is(err, mempool.ErrReplacementUnderpriced) {
		t.Fatalf("second spend at the same fee: err = %v", err)
	}
	if _, err := f.send(f.alice, 0, f.carol.addr, 90, 6); err != nil { // +20%: replaces the payment to bob
		t.Fatal(err)
	}
	f.mine()
	if f.balance(f.bob) != 0 || f.balance(f.carol) != 90 {
		t.Fatalf("bob %d carol %d: exactly one payment must happen", f.balance(f.bob), f.balance(f.carol))
	}
	f.mustValidate()
}

func TestQueuedTxIsMinedOnceTheGapIsFilled(t *testing.T) {
	f := newFixture(t)
	res, err := f.send(f.alice, 1, f.bob.addr, 5, 1)
	if err != nil || res.Status != mempool.StatusQueued {
		t.Fatalf("nonce 1 first: %+v %v", res, err)
	}
	if b := f.mine(); len(b.Txs) != 0 {
		t.Fatal("a queued tx was mined")
	}
	if got := f.node.Account(f.alice.addr).PendingNonce; got != 0 {
		t.Fatalf("pending nonce = %d, want 0", got)
	}
	f.send(f.alice, 0, f.bob.addr, 5, 1)
	if b := f.mine(); len(b.Txs) != 2 {
		t.Fatalf("block has %d txs, want both", len(b.Txs))
	}
}

// Each tx is affordable on its own; together they aren't. The block producer
// includes the first and drops the second.
func TestTxsThatDontFitTogether(t *testing.T) {
	f := newFixture(t)
	f.send(f.alice, 0, f.bob.addr, 60, 1)
	f.send(f.alice, 1, f.carol.addr, 60, 1)
	b := f.mine()
	if len(b.Txs) != 1 || f.balance(f.bob) != 60 || f.balance(f.carol) != 0 {
		t.Fatalf("txs %d bob %d carol %d", len(b.Txs), f.balance(f.bob), f.balance(f.carol))
	}
	if f.node.pool.Len() != 0 {
		t.Fatal("the unaffordable tx is still pooled")
	}
	f.mustValidate()
}

func TestStateSurvivesRestart(t *testing.T) {
	f := newFixture(t)
	f.send(f.alice, 0, f.bob.addr, 25, 1)
	f.mine()
	tip := f.node.Latest()

	if err := f.start(false); err != nil {
		t.Fatal(err)
	}
	if f.node.Latest().Hash != tip.Hash || f.balance(f.bob) != 25 || f.node.Account(f.alice.addr).Account.Nonce != 1 {
		t.Fatal("restart lost data")
	}
	f.mustValidate()
}

// The stored accounts are a cache of "replay all blocks". Corrupt them, and a
// normal start refuses; REBUILD_STATE recomputes them from the blocks.
func TestCorruptStateIsDetectedAndRebuilt(t *testing.T) {
	f := newFixture(t)
	f.send(f.alice, 0, f.bob.addr, 25, 1)
	f.mine()

	if err := f.store.ReplaceState(domain.State{f.bob.addr: {Balance: 1_000_000}}); err != nil {
		t.Fatal(err)
	}
	if err := f.start(false); err == nil {
		t.Fatal("started on a state that doesn't match the tip's state root")
	}
	if err := f.start(true); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if f.balance(f.bob) != 25 {
		t.Fatalf("bob = %d after rebuild, want 25", f.balance(f.bob))
	}
}

// Genesis covers only the allocation and difficulty; the fingerprint covers
// every consensus param. Each change must be refused on an existing data dir.
func TestDifferentParamsCannotReuseTheDataDir(t *testing.T) {
	changes := map[string]func(*domain.Params){
		"block reward (not in genesis)": func(p *domain.Params) { p.BlockReward = 1_000 },
		"chain id (not in genesis)":     func(p *domain.Params) { p.ChainID = 1 },
		"min fee (not in genesis)":      func(p *domain.Params) { p.MinFee = 0 },
		"genesis allocation":            func(p *domain.Params) { p.GenesisAlloc = map[domain.Address]uint64{{0x01}: 1} },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			change(&f.params)
			if err := f.start(false); !errors.Is(err, domain.ErrParamsMismatch) {
				t.Fatalf("err = %v, want ErrParamsMismatch", err)
			}
		})
	}
}

func TestProofFromTheNodeVerifies(t *testing.T) {
	f := newFixture(t)
	var last SubmitResult
	for i := range uint64(5) {
		last, _ = f.send(f.alice, i, f.bob.addr, 1, 1)
	}
	b := f.mine()
	p, err := f.node.Proof(b.Height, last.Hash)
	if err != nil {
		t.Fatal(err)
	}
	header, _ := domain.DecodeHeader(p.HeaderRaw)
	if !domain.VerifyProof(last.Hash, p.Steps, header.TxRoot) || header.ComputeHash() != b.Hash {
		t.Fatal("proof does not verify against the block header")
	}
	if _, err := f.node.Proof(b.Height, domain.Hash{1}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown tx: err = %v", err)
	}
}

func TestConcurrentSubmitsWhileMining(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Go(func() {
		for ctx.Err() == nil {
			f.node.Mine(ctx)
		}
	})
	for i := range uint64(20) {
		wg.Go(func() { f.send(f.alice, i, f.bob.addr, 1, 1) })
		wg.Go(func() { f.node.Account(f.bob.addr); f.node.Mempool(); f.node.Stats() })
	}
	cancel()
	wg.Wait()

	// Some submits may have been rejected (nonce gap, raced with a block), and a
	// tx above a gap stays queued forever — so drain only what is minable.
	for range 25 {
		if pending, _ := f.node.Mempool(); len(pending) == 0 {
			break
		}
		f.mine()
	}
	if pending, _ := f.node.Mempool(); len(pending) != 0 || f.balance(f.bob) == 0 {
		t.Fatalf("%d txs still pending, bob has %d", len(pending), f.balance(f.bob))
	}
	f.mustValidate()
}
