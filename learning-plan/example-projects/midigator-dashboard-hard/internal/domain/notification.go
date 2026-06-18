package domain

import (
	"context"
	"time"
)

// Notification is an in-app message for a user.
type Notification struct {
	ID             string // uuid
	UserID         int64
	TenantID       int64
	Type           string
	Title          string
	Body           string
	NotifiableType string
	NotifiableID   *int64
	ReadAt         *time.Time
	CreatedAt      time.Time
}

// NotificationSettings are a user's per-channel / per-event preferences.
type NotificationSettings struct {
	UserID           int64
	Channel          string
	ChargebackNew    bool
	ChargebackResult bool
	PreventionNew    bool
	DailyDigest      bool
	WeeklyReport     bool
}

// NotificationRepository stores notifications and per-user settings.
type NotificationRepository interface {
	Create(ctx context.Context, n *Notification) (*Notification, error)
	ListByUser(ctx context.Context, userID int64, unreadOnly bool, limit int) ([]Notification, error)
	UnreadCount(ctx context.Context, userID int64) (int, error)
	MarkRead(ctx context.Context, userID int64, id string) error
	MarkAllRead(ctx context.Context, userID int64) error

	GetSettings(ctx context.Context, userID int64) (*NotificationSettings, error)
	UpsertSettings(ctx context.Context, s *NotificationSettings) (*NotificationSettings, error)
}
