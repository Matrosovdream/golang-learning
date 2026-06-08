package domain

import (
	"context"
	"errors"
	"time"
)

// Domain errors. The repository and service translate storage/validation
// failures into these so the handler never sees Postgres-specific details.
var (
	ErrNotFound      = errors.New("link not found")
	ErrDuplicateCode = errors.New("link code already exists")
)

// Link is the core entity: a shortened URL and its click stats.
type Link struct {
	ID        int64
	Code      string
	LongURL   string
	Clicks    int64
	CreatedAt time.Time
}

// LinkRepository is the storage contract the domain depends on.
// The concrete implementation lives in the repository layer and is
// injected at startup, keeping the domain free of any database imports.
type LinkRepository interface {
	Create(ctx context.Context, link *Link) error
	GetByCode(ctx context.Context, code string) (*Link, error)
	IncrementClicks(ctx context.Context, code string) error
}
