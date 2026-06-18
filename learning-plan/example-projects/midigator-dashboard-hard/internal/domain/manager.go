package domain

import (
	"context"
	"encoding/json"
	"time"
)

// ManagerProfile holds a user's performance score and assignment metadata.
type ManagerProfile struct {
	UserID       int64
	UserName     string // populated on tenant listings (join users)
	UserEmail    string
	Score        *float64
	Notes        string
	AssignedMids json.RawMessage
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ManagerProfileRepository stores manager profiles (1:1 with users).
type ManagerProfileRepository interface {
	Get(ctx context.Context, userID int64) (*ManagerProfile, error)
	// SetScore upserts a profile's score and notes.
	SetScore(ctx context.Context, userID int64, score *float64, notes string) (*ManagerProfile, error)
	// ListByTenant returns profiles for every user in a tenant (join users).
	ListByTenant(ctx context.Context, tenantID int64) ([]ManagerProfile, error)
}
