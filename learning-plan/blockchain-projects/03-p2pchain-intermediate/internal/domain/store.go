package domain

// ChainStore persists the block TREE, not just one chain.
//
// A node may know several branches at once. Every block it accepts is stored;
// exactly one branch is "canonical" (the one with the most work), and that is
// what the canonical index, the transaction index, and the account state reflect.
type ChainStore interface {
	// IsEmpty reports whether no block has been stored yet.
	IsEmpty() (bool, error)

	// SetParamsFingerprint records the consensus params the chain was created with.
	SetParamsFingerprint(fp Hash) error
	// ParamsFingerprint returns the recorded fingerprint, or ErrNotFound.
	ParamsFingerprint() (Hash, error)

	// PutBlock stores a block and its tree metadata. The block is NOT made
	// canonical and its transactions are not executed: a side branch may never
	// be executed at all.
	PutBlock(b Block, meta BlockMeta) error
	// HasBlock reports whether the block is already in the tree.
	HasBlock(h Hash) (bool, error)
	// BlockByHash returns a stored block, or ErrNotFound.
	BlockByHash(h Hash) (Block, error)
	// Meta returns a block's tree metadata, or ErrNotFound.
	Meta(h Hash) (BlockMeta, error)
	// Tips returns every leaf of the tree (blocks with no stored children).
	Tips() ([]Hash, error)

	// Switch atomically moves the canonical chain: it undoes `disconnect`
	// (tip first) and applies `connect` (oldest first), updating the account
	// state, the canonical height index, the transaction index, and the tip.
	Switch(disconnect []BlockUpdate, connect []BlockUpdate) error

	// TipHash returns the canonical tip.
	TipHash() (Hash, error)
	// CanonicalHash returns the canonical block hash at a height, or ErrNotFound.
	CanonicalHash(height uint64) (Hash, error)
	// UndoDiff returns the state values a canonical block replaced, so the
	// block can be rolled back. Only set for blocks that were connected.
	UndoDiff(h Hash) (map[Address]Account, error)
	// TxLocation finds a transaction in the canonical chain.
	TxLocation(hash Hash) (blockHash Hash, index int, err error)

	// LoadState reads every stored account.
	LoadState() (State, error)
	// ReplaceState overwrites all stored accounts.
	ReplaceState(s State) error

	Close() error
}

// BlockMeta is what the tree knows about a block without executing it.
type BlockMeta struct {
	Height uint64
	Parent Hash
	Work   Work // cumulative work of the chain ending at this block
}

// BlockUpdate is a block plus the state changes connecting it causes.
// Forward is applied when the block becomes canonical; Backward restores the
// previous values when it is disconnected during a reorg.
type BlockUpdate struct {
	Block    Block
	Meta     BlockMeta
	Forward  map[Address]Account
	Backward map[Address]Account
}
