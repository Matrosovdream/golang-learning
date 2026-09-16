package chain

import (
	"fmt"
	"log"
	"time"

	"p2pchain/internal/domain"
	"p2pchain/internal/events"
)

// AddBlock offers a block to the chain: from a peer, or from our own miner.
//
// This is the whole of Nakamoto consensus in one function:
//
//  1. cheap checks that need no state (hash, work, shape, timestamp)
//  2. do we know the parent? if not, it is an orphan and the caller must sync
//  3. store it in the tree — every valid block is kept, winner or not
//  4. FORK CHOICE: is this branch heavier than the one we are on?
//  5. if so, switch to it, which may mean undoing blocks we already had
func (c *Chain) AddBlock(b domain.Block) (AddResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	res := AddResult{Hash: b.Hash, Height: b.Height, OldTip: c.tip.Hash}

	if known, err := c.store.HasBlock(b.Hash); err != nil {
		return res, err
	} else if known {
		res.Status = StatusKnown
		return res, nil
	}

	if err := c.CheckStateless(b); err != nil {
		return res, err
	}

	parentMeta, err := c.store.Meta(b.PrevHash)
	if err != nil { // we don't have the parent: can't place it in the tree yet
		res.Status = StatusOrphan
		return res, nil
	}
	if b.Height != parentMeta.Height+1 {
		return res, &domain.InvalidBlockError{Height: b.Height, Rule: domain.RuleHeight,
			Reason: fmt.Sprintf("parent is at height %d", parentMeta.Height)}
	}

	meta := domain.BlockMeta{Height: b.Height, Parent: b.PrevHash, Work: domain.AddWork(parentMeta.Work, b.Difficulty)}
	if err := c.store.PutBlock(b, meta); err != nil {
		return res, err
	}

	// Fork choice: strictly more work wins. A block that only ties stays a side
	// branch, so every node keeps the tip it saw first and the network doesn't
	// flap between two equal branches.
	if !domain.MoreWork(meta.Work, c.tipMeta.Work) {
		res.Status = StatusSide
		c.bus.Publish(events.SideBlock, map[string]any{
			"hash": b.Hash.Hex(), "height": b.Height, "tip_height": c.tip.Height,
		})
		return res, nil
	}

	extends := b.PrevHash == c.tip.Hash
	if err := c.switchTo(b.Hash, meta, &res); err != nil {
		return res, err
	}
	if extends {
		res.Status = StatusExtended
		c.bus.Publish(events.BlockAdded, map[string]any{
			"hash": b.Hash.Hex(), "height": b.Height, "txs": len(b.Txs), "miner": b.Miner.Hex(),
		})
	} else {
		res.Status = StatusReorg
		// Logged here, not at the call sites: a branch switch can be triggered
		// by a gossiped block, by a sync, by connecting an orphan, or by our own
		// miner, and it must never pass unnoticed.
		log.Printf("REORG to block %d %s: %d block(s) undone, %d tx(s) back in the mempool",
			b.Height, b.Hash.Hex()[:12], res.ReorgDepth, res.ReturnedTxs)
		c.bus.Publish(events.Reorg, map[string]any{
			"old_tip": res.OldTip.Hex(), "new_tip": b.Hash.Hex(), "height": b.Height,
			"depth": res.ReorgDepth, "returned_txs": res.ReturnedTxs,
		})
	}
	return res, nil
}

// CheckStateless runs every rule that needs no account state, so junk is
// rejected before it costs us a database write or a state execution. The
// gossip validator calls it too, to decide whether to relay a block at all.
func (c *Chain) CheckStateless(b domain.Block) error {
	fail := func(rule, format string, args ...any) error {
		return &domain.InvalidBlockError{Height: b.Height, Rule: rule, Reason: fmt.Sprintf(format, args...)}
	}
	hash := b.Header.ComputeHash()
	if hash != b.Hash {
		return fail(domain.RuleStaleHash, "block hash does not match its header")
	}
	if b.Difficulty != c.params.Difficulty {
		return fail(domain.RuleDifficulty, "claims difficulty %d, the chain requires %d", b.Difficulty, c.params.Difficulty)
	}
	if zeros := domain.LeadingZeroBits(hash); zeros < int(c.params.Difficulty) {
		return fail(domain.RuleWork, "hash has %d leading zero bits, need %d", zeros, c.params.Difficulty)
	}
	if root := domain.ComputeTxRoot(b.Txs); root != b.TxRoot {
		return fail(domain.RuleTxRoot, "tx_root does not match the transactions")
	}
	if len(b.Txs) > c.params.MaxBlockTxs {
		return fail(domain.RuleTxs, "%d transactions, maximum %d", len(b.Txs), c.params.MaxBlockTxs)
	}
	// A block from the future would let a miner claim a timestamp nobody can
	// check. Some drift is allowed: peer clocks are never exactly ours.
	if drift := time.UnixMilli(b.Timestamp).Sub(time.Now()); drift > maxFutureDrift {
		return fail(domain.RuleTimestamp, "timestamp is %s in the future", drift.Round(time.Second))
	}
	return nil
}

// switchTo makes `target` the canonical tip. The caller holds c.mu.
//
// Nothing is written until every block of the new branch has been executed
// successfully, and then it is written in a single store transaction. A branch
// that turns out to be invalid halfway through leaves the node exactly where it was.
func (c *Chain) switchTo(target domain.Hash, targetMeta domain.BlockMeta, res *AddResult) error {
	ancestor, disconnectHashes, connectHashes, err := c.paths(c.tip.Hash, target)
	if err != nil {
		return err
	}

	// 1. Walk back to the common ancestor, undoing state as we go.
	state := c.state.Clone()
	var disconnect []domain.BlockUpdate
	var returned []domain.Tx
	for _, h := range disconnectHashes { // tip first
		block, err := c.store.BlockByHash(h)
		if err != nil {
			return err
		}
		meta, err := c.store.Meta(h)
		if err != nil {
			return err
		}
		undo, err := c.store.UndoDiff(h)
		if err != nil {
			return fmt.Errorf("undo diff for %s: %w", h.Hex(), err)
		}
		state.Apply(undo)
		disconnect = append(disconnect, domain.BlockUpdate{Block: block, Meta: meta, Backward: undo})
		returned = append(returned, block.Txs...) // these transactions are un-mined again
	}

	// 2. Execute the new branch on top of the ancestor's state.
	parent, err := c.blockOrNil(ancestor)
	if err != nil {
		return err
	}
	var connect []domain.BlockUpdate
	for _, h := range connectHashes { // oldest first
		block, err := c.store.BlockByHash(h)
		if err != nil {
			return err
		}
		meta, err := c.store.Meta(h)
		if err != nil {
			return err
		}
		next, err := domain.ValidateBlock(c.params, parent, state, block)
		if err != nil {
			return err // nothing has been written: the node stays on its old branch
		}
		connect = append(connect, domain.BlockUpdate{
			Block: block, Meta: meta,
			Forward:  domain.Diff(state, next),
			Backward: domain.Diff(next, state),
		})
		state = next
		b := block
		parent = &b
	}

	// 3. One atomic store transaction for the whole switch.
	if err := c.store.Switch(disconnect, connect); err != nil {
		return err
	}
	newTip, err := c.store.BlockByHash(target)
	if err != nil {
		return err
	}
	c.tip, c.tipMeta, c.state = newTip, targetMeta, state

	// 4. The mempool follows the chain: transactions from disconnected blocks
	//    go back in (unless the new branch mined them), then anything now stale
	//    is pruned.
	res.ReorgDepth = len(disconnect)
	res.ReturnedTxs = c.returnToPool(returned)
	c.pool.Prune(func(a domain.Address) uint64 { return state.Get(a).Nonce })
	return nil
}

// returnToPool re-admits transactions from disconnected blocks. Ones the new
// branch already contains fail the nonce check and are simply dropped.
func (c *Chain) returnToPool(txs []domain.Tx) int {
	returned := 0
	for _, tx := range txs {
		sender, err := tx.Sender()
		if err != nil {
			continue
		}
		if _, err := c.pool.Add(tx, sender, c.state.Get(sender)); err == nil {
			returned++
		}
	}
	return returned
}

// paths returns the blocks to disconnect (from tip, tip first) and to connect
// (up to target, oldest first) to move from `from` to `to`.
func (c *Chain) paths(from, to domain.Hash) (ancestor domain.Hash, disconnect, connect []domain.Hash, err error) {
	a, b := from, to
	aMeta, err := c.store.Meta(a)
	if err != nil {
		return ancestor, nil, nil, err
	}
	bMeta, err := c.store.Meta(b)
	if err != nil {
		return ancestor, nil, nil, err
	}

	// Bring both walkers to the same height.
	for bMeta.Height > aMeta.Height {
		connect = append(connect, b)
		b = bMeta.Parent
		if bMeta, err = c.store.Meta(b); err != nil {
			return ancestor, nil, nil, err
		}
	}
	for aMeta.Height > bMeta.Height {
		disconnect = append(disconnect, a)
		a = aMeta.Parent
		if aMeta, err = c.store.Meta(a); err != nil {
			return ancestor, nil, nil, err
		}
	}
	// Same height: step back together until the paths meet.
	for a != b {
		disconnect = append(disconnect, a)
		connect = append(connect, b)
		a, b = aMeta.Parent, bMeta.Parent
		if aMeta, err = c.store.Meta(a); err != nil {
			return ancestor, nil, nil, err
		}
		if bMeta, err = c.store.Meta(b); err != nil {
			return ancestor, nil, nil, err
		}
	}
	ancestor = a // a == b: the newest block both branches share
	// `connect` was collected newest-first; the caller applies oldest-first.
	for i, j := 0, len(connect)-1; i < j; i, j = i+1, j-1 {
		connect[i], connect[j] = connect[j], connect[i]
	}
	return ancestor, disconnect, connect, nil
}

// blockOrNil loads a block, returning nil for genesis's absent parent.
func (c *Chain) blockOrNil(h domain.Hash) (*domain.Block, error) {
	if h.IsZero() {
		return nil, nil
	}
	b, err := c.store.BlockByHash(h)
	if err != nil {
		return nil, err
	}
	return &b, nil
}
