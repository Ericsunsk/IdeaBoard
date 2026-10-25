package notifications

import "context"

// CreateNotificationInput contains the required parameters to insert a new user notification.
type CreateNotificationInput struct {
	// DeduplicationKey is an internal durable event identity, never exposed to recipients.
	DeduplicationKey string
	UserID           int64
	Type             NotificationType
	Title            string
	Body             string
	ActionPayload    ActionPayload
}

// Store defines persistence operations for user notifications (#958, ADR-0040).
type Store interface {
	CreateNotification(ctx context.Context, input CreateNotificationInput) (*Notification, error)
	ListNotifications(ctx context.Context, userID int64, filter ListFilter, limit, offset int32) ([]*Notification, int64, error)
	CountUnread(ctx context.Context, userID int64) (int64, error)
	MarkRead(ctx context.Context, userID int64, publicID string) (*Notification, error)
	MarkAllRead(ctx context.Context, userID int64) (int64, error)
	Archive(ctx context.Context, userID int64, publicID string) (*Notification, error)
	// MarkInviteResolved merges invite_status into the actionable invite
	// notification(s) sharing the same invite_id (ADR-0040 §4). It only
	// touches workspace_invite/organization_invite rows so the inviter-side
	// joined/declined notices sharing the same invite_id stay untouched.
	MarkInviteResolved(ctx context.Context, inviteID string, status string) (int64, error)
}
