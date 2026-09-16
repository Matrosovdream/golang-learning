// Package boltstore persists the chain in a bbolt file: an embedded, single-file
// B+tree key/value store with ACID transactions (the same family as the
// LevelDB/Pebble stores real nodes use).
package boltstore

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"

	"coinledger/internal/domain"
)

// Buckets (bbolt's tables). Keys are big-endian so byte order = numeric order.
var (
	bucketBlocks  = []byte("blocks")  // height (8)        → EncodeBlock
	bucketTxIndex = []byte("txindex") // tx hash (32)      → height (8) ‖ index (4)
	bucketState   = []byte("state")   // address (20)      → balance (8) ‖ nonce (8)
	bucketMeta    = []byte("meta")    // "tip"             → height (8); "params" → fingerprint
	keyTip        = []byte("tip")
	keyParams     = []byte("params") // consensus params fingerprint (32)
)

// Store is the bbolt-backed domain.ChainStore.
type Store struct {
	db *bolt.DB
}

var _ domain.ChainStore = (*Store)(nil)

// Open opens (or creates) the database file. bbolt takes an exclusive file
// lock, so a second node process on the same file fails after the timeout
// instead of silently corrupting it.
func Open(path string) (*Store, error) {
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: time.Second})
	if errors.Is(err, bolt.ErrTimeout) {
		return nil, fmt.Errorf("open %s: locked by another process — is another node using this data dir? (%w)", path, err)
	}
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		for _, name := range [][]byte{bucketBlocks, bucketTxIndex, bucketState, bucketMeta} {
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
		empty = tx.Bucket(bucketMeta).Get(keyTip) == nil
		return nil
	})
	return empty, err
}

func (s *Store) SetParamsFingerprint(fp domain.Hash) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketMeta).Put(keyParams, fp[:])
	})
}

func (s *Store) ParamsFingerprint() (domain.Hash, error) {
	var fp domain.Hash
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketMeta).Get(keyParams)
		if len(v) != len(fp) {
			return domain.ErrNotFound
		}
		copy(fp[:], v)
		return nil
	})
	return fp, err
}

// CommitBlock writes everything a block changes in ONE bbolt transaction.
// If the process dies halfway, bbolt rolls back: the database holds either the
// old tip with the old state, or the new tip with the new state — never a block
// without its state changes.
func (s *Store) CommitBlock(b domain.Block, changes map[domain.Address]domain.Account) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		height := u64(b.Height)

		if err := tx.Bucket(bucketBlocks).Put(height, domain.EncodeBlock(b)); err != nil {
			return err
		}
		txIndex := tx.Bucket(bucketTxIndex)
		for i, t := range b.Txs {
			h := t.Hash()
			loc := binary.BigEndian.AppendUint32(u64(b.Height), uint32(i))
			if err := txIndex.Put(h[:], loc); err != nil {
				return err
			}
		}
		state := tx.Bucket(bucketState)
		for addr, acct := range changes {
			var err error
			if acct.IsEmpty() {
				err = state.Delete(addr[:])
			} else {
				err = state.Put(addr[:], encodeAccount(acct))
			}
			if err != nil {
				return err
			}
		}
		return tx.Bucket(bucketMeta).Put(keyTip, height)
	})
}

func (s *Store) TipHeight() (uint64, error) {
	var height uint64
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketMeta).Get(keyTip)
		if v == nil {
			return domain.ErrNotFound
		}
		height = binary.BigEndian.Uint64(v)
		return nil
	})
	return height, err
}

func (s *Store) BlockByHeight(height uint64) (domain.Block, error) {
	var b domain.Block
	err := s.db.View(func(tx *bolt.Tx) error {
		// bbolt's returned slices are only valid inside the transaction.
		// DecodeBlock copies everything it needs, so b stays valid afterwards.
		v := tx.Bucket(bucketBlocks).Get(u64(height))
		if v == nil {
			return domain.ErrNotFound
		}
		var err error
		b, err = domain.DecodeBlock(v)
		return err
	})
	return b, err
}

func (s *Store) Blocks(from uint64, limit int) ([]domain.Block, error) {
	out := []domain.Block{}
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bucketBlocks).Cursor()
		for k, v := c.Seek(u64(from)); k != nil && len(out) < limit; k, v = c.Next() {
			b, err := domain.DecodeBlock(v)
			if err != nil {
				return err
			}
			out = append(out, b)
		}
		return nil
	})
	return out, err
}

func (s *Store) ForEachBlock(fn func(domain.Block) error) error {
	return s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketBlocks).ForEach(func(_, v []byte) error {
			b, err := domain.DecodeBlock(v)
			if err != nil {
				return err
			}
			return fn(b)
		})
	})
}

func (s *Store) TxLocation(hash domain.Hash) (uint64, int, error) {
	var height uint64
	var index int
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketTxIndex).Get(hash[:])
		if v == nil {
			return domain.ErrNotFound
		}
		height = binary.BigEndian.Uint64(v[:8])
		index = int(binary.BigEndian.Uint32(v[8:12]))
		return nil
	})
	return height, index, err
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
			st[addr] = decodeAccount(v)
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
		bucket, err := tx.CreateBucket(bucketState)
		if err != nil {
			return err
		}
		for addr, acct := range st {
			if err := bucket.Put(addr[:], encodeAccount(acct)); err != nil {
				return err
			}
		}
		return nil
	})
}

func u64(n uint64) []byte { return binary.BigEndian.AppendUint64(nil, n) }

func encodeAccount(a domain.Account) []byte {
	return binary.BigEndian.AppendUint64(u64(a.Balance), a.Nonce)
}

func decodeAccount(v []byte) domain.Account {
	return domain.Account{Balance: binary.BigEndian.Uint64(v[:8]), Nonce: binary.BigEndian.Uint64(v[8:16])}
}
