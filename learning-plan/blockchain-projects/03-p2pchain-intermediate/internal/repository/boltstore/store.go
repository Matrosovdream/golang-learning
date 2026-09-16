// Package boltstore persists the block tree in a bbolt file.
package boltstore

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"time"

	bolt "go.etcd.io/bbolt"

	"p2pchain/internal/domain"
)

// Buckets. Keys are big-endian, so byte order equals numeric order.
var (
	bucketBlocks    = []byte("blocks")    // block hash (32) → header ‖ RLP(txs)
	bucketMeta      = []byte("meta")      // block hash (32) → height (8) ‖ parent (32) ‖ work
	bucketTips      = []byte("tips")      // block hash (32) → "" (the tree's leaves)
	bucketCanonical = []byte("canonical") // height (8)      → block hash (32)
	bucketTxIndex   = []byte("txindex")   // tx hash (32)    → block hash (32) ‖ index (4)
	bucketUndo      = []byte("undo")      // block hash (32) → account diff (restores the pre-block state)
	bucketState     = []byte("state")     // address (20)    → balance (8) ‖ nonce (8)
	bucketChain     = []byte("chain")     // "tip" → block hash; "params" → fingerprint
	keyTip          = []byte("tip")
	keyParams       = []byte("params")
)

var allBuckets = [][]byte{bucketBlocks, bucketMeta, bucketTips, bucketCanonical, bucketTxIndex, bucketUndo, bucketState, bucketChain}

// Store is the bbolt-backed domain.ChainStore.
type Store struct{ db *bolt.DB }

var _ domain.ChainStore = (*Store)(nil)

// Open opens (or creates) the database. bbolt takes an exclusive file lock, so
// two nodes can never share a data directory.
func Open(path string) (*Store, error) {
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: time.Second})
	if errors.Is(err, bolt.ErrTimeout) {
		return nil, fmt.Errorf("open %s: locked by another process — is another node using this data dir? (%w)", path, err)
	}
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		for _, name := range allBuckets {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) IsEmpty() (bool, error) {
	empty := true
	err := s.db.View(func(tx *bolt.Tx) error {
		empty = tx.Bucket(bucketChain).Get(keyTip) == nil
		return nil
	})
	return empty, err
}

func (s *Store) SetParamsFingerprint(fp domain.Hash) error {
	return s.db.Update(func(tx *bolt.Tx) error { return tx.Bucket(bucketChain).Put(keyParams, fp[:]) })
}

func (s *Store) ParamsFingerprint() (domain.Hash, error) {
	var fp domain.Hash
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketChain).Get(keyParams)
		if len(v) != len(fp) {
			return domain.ErrNotFound
		}
		copy(fp[:], v)
		return nil
	})
	return fp, err
}

// PutBlock adds a block to the tree and maintains the set of tips: the new
// block is a leaf, and its parent stops being one.
func (s *Store) PutBlock(b domain.Block, meta domain.BlockMeta) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(bucketBlocks).Put(b.Hash[:], domain.EncodeBlock(b)); err != nil {
			return err
		}
		if err := tx.Bucket(bucketMeta).Put(b.Hash[:], encodeMeta(meta)); err != nil {
			return err
		}
		tips := tx.Bucket(bucketTips)
		if err := tips.Delete(meta.Parent[:]); err != nil {
			return err
		}
		return tips.Put(b.Hash[:], nil)
	})
}

func (s *Store) HasBlock(h domain.Hash) (bool, error) {
	var found bool
	err := s.db.View(func(tx *bolt.Tx) error {
		found = tx.Bucket(bucketBlocks).Get(h[:]) != nil
		return nil
	})
	return found, err
}

func (s *Store) BlockByHash(h domain.Hash) (domain.Block, error) {
	var b domain.Block
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketBlocks).Get(h[:])
		if v == nil {
			return domain.ErrNotFound
		}
		var err error
		b, err = domain.DecodeBlock(v) // decoding copies: bbolt's bytes die with the transaction
		return err
	})
	return b, err
}

func (s *Store) Meta(h domain.Hash) (domain.BlockMeta, error) {
	var m domain.BlockMeta
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketMeta).Get(h[:])
		if v == nil {
			return domain.ErrNotFound
		}
		var err error
		m, err = decodeMeta(v)
		return err
	})
	return m, err
}

func (s *Store) Tips() ([]domain.Hash, error) {
	var out []domain.Hash
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketTips).ForEach(func(k, _ []byte) error {
			var h domain.Hash
			copy(h[:], k)
			out = append(out, h)
			return nil
		})
	})
	return out, err
}

// Switch performs a chain reorganisation in ONE bbolt transaction: either the
// node ends up fully on the new branch, or fully on the old one. A crash can
// never leave half a reorg behind.
func (s *Store) Switch(disconnect, connect []domain.BlockUpdate) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		state, canonical, txIndex, undo := tx.Bucket(bucketState), tx.Bucket(bucketCanonical), tx.Bucket(bucketTxIndex), tx.Bucket(bucketUndo)

		// Roll back, tip first.
		for _, u := range disconnect {
			if err := applyDiff(state, u.Backward); err != nil {
				return err
			}
			if err := canonical.Delete(u64(u.Meta.Height)); err != nil {
				return err
			}
			for _, t := range u.Block.Txs {
				h := t.Hash()
				if err := txIndex.Delete(h[:]); err != nil {
					return err
				}
			}
		}

		// Roll forward, oldest first.
		for _, u := range connect {
			if err := applyDiff(state, u.Forward); err != nil {
				return err
			}
			if err := canonical.Put(u64(u.Meta.Height), u.Block.Hash[:]); err != nil {
				return err
			}
			if err := undo.Put(u.Block.Hash[:], encodeDiff(u.Backward)); err != nil {
				return err
			}
			for i, t := range u.Block.Txs {
				h := t.Hash()
				loc := binary.BigEndian.AppendUint32(append([]byte{}, u.Block.Hash[:]...), uint32(i))
				if err := txIndex.Put(h[:], loc); err != nil {
					return err
				}
			}
		}

		// The new tip: the last connected block, or the ancestor we rolled back to.
		var tip domain.Hash
		switch {
		case len(connect) > 0:
			tip = connect[len(connect)-1].Block.Hash
		case len(disconnect) > 0:
			tip = disconnect[len(disconnect)-1].Meta.Parent
		default:
			return nil
		}
		return tx.Bucket(bucketChain).Put(keyTip, tip[:])
	})
}

func (s *Store) TipHash() (domain.Hash, error) {
	var h domain.Hash
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketChain).Get(keyTip)
		if len(v) != len(h) {
			return domain.ErrNotFound
		}
		copy(h[:], v)
		return nil
	})
	return h, err
}

func (s *Store) CanonicalHash(height uint64) (domain.Hash, error) {
	var h domain.Hash
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketCanonical).Get(u64(height))
		if v == nil {
			return domain.ErrNotFound
		}
		copy(h[:], v)
		return nil
	})
	return h, err
}

func (s *Store) TxLocation(hash domain.Hash) (domain.Hash, int, error) {
	var blockHash domain.Hash
	var index int
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketTxIndex).Get(hash[:])
		if v == nil {
			return domain.ErrNotFound
		}
		copy(blockHash[:], v[:32])
		index = int(binary.BigEndian.Uint32(v[32:36]))
		return nil
	})
	return blockHash, index, err
}

func (s *Store) UndoDiff(h domain.Hash) (map[domain.Address]domain.Account, error) {
	var diff map[domain.Address]domain.Account
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketUndo).Get(h[:])
		if v == nil {
			return domain.ErrNotFound
		}
		var err error
		diff, err = decodeDiff(v)
		return err
	})
	return diff, err
}

func (s *Store) LoadState() (domain.State, error) {
	st := domain.State{}
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketState).ForEach(func(k, v []byte) error {
			if len(k) != 20 || len(v) != 16 {
				return errors.New("corrupt state entry")
			}
			var addr domain.Address
			copy(addr[:], k)
			st[addr] = domain.Account{Balance: binary.BigEndian.Uint64(v[:8]), Nonce: binary.BigEndian.Uint64(v[8:16])}
			return nil
		})
	})
	return st, err
}

func (s *Store) ReplaceState(st domain.State) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket(bucketState); err != nil {
			return err
		}
		b, err := tx.CreateBucket(bucketState)
		if err != nil {
			return err
		}
		for addr, acct := range st {
			if err := b.Put(addr[:], encodeAccount(acct)); err != nil {
				return err
			}
		}
		return nil
	})
}

// --- encoding helpers ----------------------------------------------------------

func u64(n uint64) []byte { return binary.BigEndian.AppendUint64(nil, n) }

func encodeAccount(a domain.Account) []byte {
	return binary.BigEndian.AppendUint64(u64(a.Balance), a.Nonce)
}

func encodeMeta(m domain.BlockMeta) []byte {
	buf := u64(m.Height)
	buf = append(buf, m.Parent[:]...)
	return append(buf, m.Work.Bytes()...) // big-endian, variable length
}

func decodeMeta(v []byte) (domain.BlockMeta, error) {
	if len(v) < 40 {
		return domain.BlockMeta{}, errors.New("corrupt block meta")
	}
	m := domain.BlockMeta{Height: binary.BigEndian.Uint64(v[:8])}
	copy(m.Parent[:], v[8:40])
	m.Work = new(big.Int).SetBytes(v[40:])
	return m, nil
}

// A diff is a flat list of address ‖ balance ‖ nonce entries. An all-zero
// account means "no account": applying it deletes the entry.
func encodeDiff(diff map[domain.Address]domain.Account) []byte {
	buf := make([]byte, 0, len(diff)*36)
	for addr, acct := range diff {
		buf = append(buf, addr[:]...)
		buf = append(buf, encodeAccount(acct)...)
	}
	return buf
}

func decodeDiff(v []byte) (map[domain.Address]domain.Account, error) {
	if len(v)%36 != 0 {
		return nil, errors.New("corrupt state diff")
	}
	diff := make(map[domain.Address]domain.Account, len(v)/36)
	for i := 0; i < len(v); i += 36 {
		var addr domain.Address
		copy(addr[:], v[i:i+20])
		diff[addr] = domain.Account{
			Balance: binary.BigEndian.Uint64(v[i+20 : i+28]),
			Nonce:   binary.BigEndian.Uint64(v[i+28 : i+36]),
		}
	}
	return diff, nil
}

func applyDiff(bucket *bolt.Bucket, diff map[domain.Address]domain.Account) error {
	for addr, acct := range diff {
		var err error
		if acct.IsEmpty() {
			err = bucket.Delete(addr[:])
		} else {
			err = bucket.Put(addr[:], encodeAccount(acct))
		}
		if err != nil {
			return err
		}
	}
	return nil
}
