package domain

// ChainStore is the persistence port. The bbolt adapter implements it; the
// node depends only on this interface.
type ChainStore interface {
	// IsEmpty reports whether no block has been committed yet.
	IsEmpty() (bool, error)
	// SetParamsFingerprint records the consensus params the chain was created with.
	SetParamsFingerprint(fp Hash) error
	// ParamsFingerprint returns the recorded fingerprint, or ErrNotFound.
	ParamsFingerprint() (Hash, error)
	// CommitBlock atomically stores b, indexes its transactions, applies the
	// account changes (empty Account = delete), and moves the tip to b.
	CommitBlock(b Block, changes map[Address]Account) error
	// TipHeight returns the height of the last committed block.
	TipHeight() (uint64, error)
	// BlockByHeight returns a stored block, or ErrNotFound.
	BlockByHeight(height uint64) (Block, error)
	// Blocks returns up to limit blocks from height from.
	Blocks(from uint64, limit int) ([]Block, error)
	// ForEachBlock calls fn for every block from genesis to tip, in order.
	ForEachBlock(fn func(Block) error) error
	// TxLocation finds a mined transaction: block height and index in the block.
	TxLocation(hash Hash) (height uint64, index int, err error)
	// LoadState reads every stored account.
	LoadState() (State, error)
	// ReplaceState overwrites all stored accounts with s.
	ReplaceState(s State) error
	// Close releases the database.
	Close() error
}
