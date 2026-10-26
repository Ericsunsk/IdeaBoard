package notifications

import (
	"context"
	"encoding/json"
	"fmt"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5/pgxpool"
)

const mintAttempts = 5

// PostgresStore implements Store against PostgreSQL/sqlc.
type PostgresStore struct {
	db *pgxpool.Pool
	q  *dbsqlc.Queries
}

var _ Store = (*PostgresStore)(nil)

// NewPostgresStore creates a new PostgreSQL notifications store.
func NewPostgresStore(db *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{
		db: db,
		q:  dbsqlc.New(db),
	}
}

type repositoryQueryer struct {
	ctx context.Context
	db  *pgxpool.Pool
	q   *dbsqlc.Queries
}

func (s *PostgresStore) dbWithContext(ctx context.Context) repositoryQueryer {
	return newRepositoryQueryer(ctx, s.db, s.q)
}

func newRepositoryQueryer(ctx context.Context, pool *pgxpool.Pool, q *dbsqlc.Queries) repositoryQueryer {
	return repositoryQueryer{ctx: ctx, db: pool, q: dbutil.BorrowedQueries(ctx, q)}
}

func (s *PostgresStore) CreateNotification(ctx context.Context, input CreateNotificationInput) (*Notification, error) {
	db := s.dbWithContext(ctx)

	payload := input.ActionPayload
	if payload == nil {
		payload = make(ActionPayload)
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("notifications: marshal action payload: %w", err)
	}

	for attempt := 0; attempt < mintAttempts; attempt++ {
		publicID, err := contract.MintPublicID(contract.PrefixNotification, 12)
		if err != nil {
			return nil, fmt.Errorf("notifications: mint public id: %w", err)
		}

		row, err := db.q.CreateNotification(ctx, dbsqlc.CreateNotificationParams{
			PublicID:         publicID,
			DeduplicationKey: input.DeduplicationKey,
			UserID:           input.UserID,
			Type:             string(input.Type),
			Title:            input.Title,
			Body:             input.Body,
			ActionPayload:    payloadBytes,
		})
		if err != nil {
			if database.IsDuplicateConstraintError(err) {
				continue
			}
			return nil, err
		}

		return toDomainNotification(row), nil
	}

	return nil, fmt.Errorf("notifications: failed to mint unique public id after %d attempts", mintAttempts)
}

func (s *PostgresStore) ListNotifications(ctx context.Context, userID int64, filter ListFilter, limit, offset int32) ([]*Notification, int64, error) {
	db := s.dbWithContext(ctx)

	total, err := db.q.CountNotificationsByUserID(ctx, dbsqlc.CountNotificationsByUserIDParams{
		UserID:   userID,
		Status:   filter.Status,
		Category: filter.Category,
	})
	if err != nil {
		return nil, 0, err
	}

	rows, err := db.q.ListNotificationsByUserID(ctx, dbsqlc.ListNotificationsByUserIDParams{
		UserID:     userID,
		Status:     filter.Status,
		Category:   filter.Category,
		PageOffset: offset,
		PageLimit:  limit,
	})
	if err != nil {
		return nil, 0, err
	}

	items := make([]*Notification, 0, len(rows))
	for _, row := range rows {
		items = append(items, toDomainNotification(row))
	}

	return items, total, nil
}

func (s *PostgresStore) CountUnread(ctx context.Context, userID int64) (int64, error) {
	db := s.dbWithContext(ctx)
	return db.q.CountUnreadNotifications(ctx, userID)
}

func (s *PostgresStore) MarkRead(ctx context.Context, userID int64, publicID string) (*Notification, error) {
	db := s.dbWithContext(ctx)

	row, err := db.q.MarkNotificationAsRead(ctx, dbsqlc.MarkNotificationAsReadParams{
		PublicID: publicID,
		UserID:   userID,
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return toDomainNotification(row), nil
}

func (s *PostgresStore) MarkAllRead(ctx context.Context, userID int64) (int64, error) {
	db := s.dbWithContext(ctx)
	return db.q.MarkAllNotificationsAsRead(ctx, userID)
}

func (s *PostgresStore) Archive(ctx context.Context, userID int64, publicID string) (*Notification, error) {
	db := s.dbWithContext(ctx)

	row, err := db.q.ArchiveNotification(ctx, dbsqlc.ArchiveNotificationParams{
		PublicID: publicID,
		UserID:   userID,
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return toDomainNotification(row), nil
}

func (s *PostgresStore) MarkInviteResolved(ctx context.Context, inviteID string, status string) (int64, error) {
	db := s.dbWithContext(ctx)

	return db.q.MarkInviteNotificationResolved(ctx, dbsqlc.MarkInviteNotificationResolvedParams{
		InviteStatus: status,
		InviteID:     inviteID,
	})
}

func toDomainNotification(row dbsqlc.UserNotification) *Notification {
	notif := &Notification{
		InternalID:    row.InternalID,
		PublicID:      row.PublicID,
		UserID:        row.UserID,
		Type:          NotificationType(row.Type),
		Title:         row.Title,
		Body:          row.Body,
		ActionPayload: make(ActionPayload),
		Status:        NotificationStatus(row.Status),
		CreatedAt:     row.CreatedAt.Time,
		UpdatedAt:     row.UpdatedAt.Time,
	}

	if row.ReadAt.Valid {
		t := row.ReadAt.Time
		notif.ReadAt = &t
	}

	if len(row.ActionPayload) > 0 {
		_ = json.Unmarshal(row.ActionPayload, &notif.ActionPayload)
	}

	return notif
}
