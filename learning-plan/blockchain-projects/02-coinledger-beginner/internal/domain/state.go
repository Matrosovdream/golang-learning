package domain

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/bits"
	"slices"
)

// Account is everything the chain knows about an address.
type Account struct {
	Balance uint64 // coins
	Nonce   uint64 // how many transactions this account has sent = the nonce its next tx must use
}

// IsEmpty reports whether the account is indistinguishable from one that never existed.
func (a Account) IsEmpty() bool { return a == Account{} }

// State is the world state after some block: every non-empty account.
// A missing address means a zero account.
//
// Treat a State as immutable once a node has installed it as "the state at
// the tip": the node replaces it with a new map after each block and never
// writes into the old one, so readers can use it without locks. Work on a Clone.
type State map[Address]Account

// Get returns the account at addr (zero if absent).
func (s State) Get(addr Address) Account { return s[addr] }

// set stores acct, deleting empty accounts so that "sent 0 coins to a new
// address" and "never touched it" produce the same state (and state root).
// Ethereum has the same rule (EIP-161).
func (s State) set(addr Address, acct Account) {
	if acct.IsEmpty() {
		delete(s, addr)
		return
	}
	s[addr] = acct
}

// Clone returns an independent copy.
func (s State) Clone() State {
	out := make(State, len(s))
	for addr, acct := range s {
		out[addr] = acct
	}
	return out
}

// Root commits to the entire state in 32 bytes: a Merkle root over accounts
// sorted by address, each leaf SHA-256(address ‖ balance ‖ nonce).
//
// Sorting matters: Go map iteration order is random, so without it two nodes
// with identical state would compute different roots.
func (s State) Root() Hash {
	addrs := make([]Address, 0, len(s))
	for addr := range s {
		addrs = append(addrs, addr)
	}
	slices.SortFunc(addrs, func(a, b Address) int { return bytes.Compare(a[:], b[:]) })

	items := make([]Hash, len(addrs))
	var buf [20 + 8 + 8]byte
	for i, addr := range addrs {
		acct := s[addr]
		copy(buf[:20], addr[:])
		binary.BigEndian.PutUint64(buf[20:28], acct.Balance)
		binary.BigEndian.PutUint64(buf[28:36], acct.Nonce)
		items[i] = SHA256(buf[:])
	}
	return MerkleRoot(items)
}

// TotalSupply sums every balance. ok is false on overflow.
func (s State) TotalSupply() (total uint64, ok bool) {
	for _, acct := range s {
		var carry uint64
		total, carry = bits.Add64(total, acct.Balance, 0)
		if carry != 0 {
			return 0, false
		}
	}
	return total, true
}

// Diff returns the accounts that differ between old and next — what a node
// must write to disk after a block. Deleted accounts appear as empty Accounts.
func Diff(old, next State) map[Address]Account {
	changes := make(map[Address]Account)
	for addr, acct := range next {
		if old[addr] != acct {
			changes[addr] = acct
		}
	}
	for addr := range old {
		if _, ok := next[addr]; !ok {
			changes[addr] = Account{}
		}
	}
	return changes
}

// ApplyTx is the state transition function: the single definition of what a
// valid transaction is and what it does. Every node runs exactly this code on
// exactly these inputs, so every node reaches exactly the same state.
//
// It modifies s in place and returns the sender. On error, s may be partially
// modified — callers apply to a Clone and throw it away on failure (see ApplyBlock).
func ApplyTx(p Params, s State, tx Tx, miner Address) (Address, error) {
	// 1. Meant for this chain? (A tx signed for a testnet must not replay here.)
	if tx.ChainID != p.ChainID {
		return Address{}, fmt.Errorf("%w: tx is for chain %d, this is chain %d", ErrWrongChainID, tx.ChainID, p.ChainID)
	}

	// 2. Who sent it? The signature, not a field, decides.
	sender, err := tx.Sender()
	if err != nil {
		return Address{}, err
	}
	from := s.Get(sender)

	// 3. Right place in the sender's sequence? Replays (same nonce again) and
	//    skips (a future nonce) both fail here. This is double-spend protection.
	switch {
	case tx.Nonce < from.Nonce:
		return sender, fmt.Errorf("%w: tx nonce %d, account nonce %d", ErrNonceTooLow, tx.Nonce, from.Nonce)
	case tx.Nonce > from.Nonce:
		return sender, fmt.Errorf("%w: tx nonce %d, account nonce %d", ErrNonceTooHigh, tx.Nonce, from.Nonce)
	}

	// 4. Pays at least the minimum fee?
	if tx.Fee < p.MinFee {
		return sender, fmt.Errorf("%w: fee %d, minimum %d", ErrFeeTooLow, tx.Fee, p.MinFee)
	}

	// 5. Can the sender afford amount + fee?
	cost, ok := tx.Cost()
	if !ok {
		return sender, ErrOverflow
	}
	if from.Balance < cost {
		return sender, fmt.Errorf("%w: balance %d, cost %d", ErrInsufficientFunds, from.Balance, cost)
	}

	// 6. Move the coins. Read-modify-write one account at a time, so it is
	//    correct even when sender, recipient, and miner are the same address.
	from.Balance -= cost
	from.Nonce++
	s.set(sender, from)

	if err := credit(s, tx.To, tx.Amount); err != nil {
		return sender, err
	}
	if err := credit(s, miner, tx.Fee); err != nil {
		return sender, err
	}
	return sender, nil
}

// credit adds amount to addr. A credit only moves existing coins, so it can't
// overflow while the total supply fits in uint64 — checked anyway, because a
// silent wrap-around would create or destroy money.
func credit(s State, addr Address, amount uint64) error {
	acct := s.Get(addr)
	sum, carry := bits.Add64(acct.Balance, amount, 0)
	if carry != 0 {
		return ErrOverflow
	}
	acct.Balance = sum
	s.set(addr, acct)
	return nil
}

// ApplyBlock executes a block on top of parent and returns the new state:
// every transaction in order, then the block reward to the miner.
//
// All or nothing: it works on a clone, so if any transaction is invalid the
// whole block is rejected and parent is untouched. A block is never "partly" valid.
func ApplyBlock(p Params, parent State, b Block) (State, error) {
	if len(b.Txs) > p.MaxBlockTxs {
		return nil, fmt.Errorf("block has %d transactions, maximum %d", len(b.Txs), p.MaxBlockTxs)
	}
	s := parent.Clone()
	for i, tx := range b.Txs {
		if _, err := ApplyTx(p, s, tx, b.Miner); err != nil {
			return nil, &TxError{Index: i, Hash: tx.Hash(), Err: err}
		}
	}
	// New coins enter circulation only here: the reward for doing the work.
	if err := credit(s, b.Miner, p.BlockReward); err != nil {
		return nil, err
	}
	return s, nil
}
