package service

import (
	"context"
	"slices"

	"midigator/internal/domain"
)

// NotificationService exposes a user's in-app notifications and their per-user
// delivery preferences. Unlike the case services it scopes by the authenticated
// user (via actor) rather than the tenant, so a platform admin with no tenant
// can still read their own notifications.
type NotificationService struct {
	repo domain.NotificationRepository
}

// NewNotificationService wires the notification service.
func NewNotificationService(repo domain.NotificationRepository) *NotificationService {
	return &NotificationService{repo: repo}
}

// NotificationList is a page of notifications plus the unread badge count.
type NotificationList struct {
	Notifications []domain.Notification
	UnreadCount   int
}

// List returns the current user's notifications and unread count.
func (s *NotificationService) List(ctx context.Context, unreadOnly bool, limit int) (*NotificationList, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListByUser(ctx, a.UserID, unreadOnly, limit)
	if err != nil {
		return nil, err
	}
	unread, err := s.repo.UnreadCount(ctx, a.UserID)
	if err != nil {
		return nil, err
	}
	return &NotificationList{Notifications: items, UnreadCount: unread}, nil
}

// MarkRead marks one of the current user's notifications as read.
func (s *NotificationService) MarkRead(ctx context.Context, id string) error {
	a, err := actor(ctx)
	if err != nil {
		return err
	}
	return s.repo.MarkRead(ctx, a.UserID, id)
}

// MarkAllRead marks every unread notification for the current user as read.
func (s *NotificationService) MarkAllRead(ctx context.Context) error {
	a, err := actor(ctx)
	if err != nil {
		return err
	}
	return s.repo.MarkAllRead(ctx, a.UserID)
}

// GetSettings returns the current user's notification preferences (defaults if
// none are saved).
func (s *NotificationService) GetSettings(ctx context.Context) (*domain.NotificationSettings, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	return s.repo.GetSettings(ctx, a.UserID)
}

var notificationChannels = []string{"email", "in_app", "both"}

// UpdateSettings validates and persists the current user's preferences.
func (s *NotificationService) UpdateSettings(ctx context.Context, in *domain.NotificationSettings) (*domain.NotificationSettings, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(notificationChannels, in.Channel) {
		return nil, domain.Invalid("channel", "must be one of email, in_app, both")
	}
	in.UserID = a.UserID
	return s.repo.UpsertSettings(ctx, in)
}
