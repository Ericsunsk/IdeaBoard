package notifications

import (
	"context"
	"errors"
	"time"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/events"
)

// Publisher delivers committed facts. It must never read using the completed transaction.
type Publisher interface {
	Publish(context.Context, events.V1Actor, string, *Notification)
}

type actorKey struct{}

// ContextWithActor carries event attribution only; it grants no permissions.
func ContextWithActor(ctx context.Context, userPublicID string) context.Context {
	return context.WithValue(ctx, actorKey{}, events.V1Actor{Type: events.V1ActorUser, ID: userPublicID})
}

// Service is the notification command boundary. Store owns persistence, while
// this boundary derives hints exclusively from the returned notification facts.
type Service struct {
	Reader
	store     Store
	publisher Publisher
}

func NewService(store Store, publisher Publisher) *Service {
	return &Service{Reader: store, store: store, publisher: publisher}
}

func (s *Service) write(ctx context.Context, typ string, command func() ([]*Notification, error)) ([]*Notification, error) {
	actor, ok := ctx.Value(actorKey{}).(events.V1Actor)
	if !ok {
		actor = events.V1Actor{Type: events.V1ActorSystem, ID: "notifications"}
	}
	var facts []*Notification
	publish := func(context.Context) {
		if s.publisher == nil || len(facts) == 0 {
			return
		}
		deliveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		for _, fact := range facts {
			s.publisher.Publish(deliveryCtx, actor, typ, fact)
		}
	}
	_, inTx := dbutil.AmbientTxFromContext(ctx)
	// Reserve before writing: a transaction without a post-commit owner must
	// never write facts whose hints could leak before commit.
	if inTx && !dbutil.DeferAmbientPostCommit(ctx, publish) {
		return nil, errors.New("notifications: ambient transaction requires an active post-commit registry")
	}
	rows, err := command()
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row != nil {
			facts = append(facts, cloneNotification(row))
		}
	}
	if !inTx {
		publish(ctx)
	}
	return rows, nil
}

func (s *Service) CreateNotification(ctx context.Context, input CreateNotificationInput) (*Notification, error) {
	rows, err := s.write(ctx, events.EventV1NotificationCreated, func() ([]*Notification, error) {
		row, err := s.store.CreateNotification(ctx, input)
		return []*Notification{row}, err
	})
	if err != nil {
		return nil, err
	}
	return rows[0], nil
}
func (s *Service) MarkRead(ctx context.Context, userID int64, id string) (*Notification, error) {
	rows, err := s.write(ctx, events.EventV1NotificationUpdated, func() ([]*Notification, error) {
		row, err := s.store.MarkRead(ctx, userID, id)
		return []*Notification{row}, err
	})
	if err != nil {
		return nil, err
	}
	return rows[0], nil
}
func (s *Service) Archive(ctx context.Context, userID int64, id string) (*Notification, error) {
	rows, err := s.write(ctx, events.EventV1NotificationUpdated, func() ([]*Notification, error) {
		row, err := s.store.Archive(ctx, userID, id)
		return []*Notification{row}, err
	})
	if err != nil {
		return nil, err
	}
	return rows[0], nil
}
func (s *Service) MarkAllRead(ctx context.Context, userID int64) ([]*Notification, error) {
	return s.write(ctx, events.EventV1NotificationUpdated, func() ([]*Notification, error) { return s.store.MarkAllRead(ctx, userID) })
}
func (s *Service) MarkInviteResolved(ctx context.Context, id, status string) ([]*Notification, error) {
	return s.write(ctx, events.EventV1NotificationUpdated, func() ([]*Notification, error) { return s.store.MarkInviteResolved(ctx, id, status) })
}

var _ Store = (*Service)(nil)
