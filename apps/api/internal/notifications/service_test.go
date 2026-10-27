package notifications_test

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/notifications"
	"ideaboard/api/internal/tenancy/invitations"
	"ideaboard/api/internal/testdb"
)

type deliveredFact struct {
	actor events.V1Actor
	typ   string
	row   notifications.Notification
}
type factPublisher struct{ deliveries []deliveredFact }

func (p *factPublisher) Publish(ctx context.Context, actor events.V1Actor, typ string, row *notifications.Notification) {
	if ctx.Err() != nil {
		panic("delivery inherited request cancellation")
	}
	p.deliveries = append(p.deliveries, deliveredFact{actor, typ, *row})
}
func notificationInput(user int64, key string) notifications.CreateNotificationInput {
	return notifications.CreateNotificationInput{UserID: user, DeduplicationKey: key, Type: notifications.TypeWorkspaceInvite, Title: "Invite", ActionPayload: notifications.InviteActionPayload{InviteID: "wsi_fact", WorkspaceID: "ws_fact", InviteStatus: "pending"}.ToActionPayload()}
}
func TestCommandsUsePersistedFactsAndSnapshotBulkRecipients(t *testing.T) {
	ctx := t.Context()
	store := notifications.NewMemoryStore()
	pub := &factPublisher{}
	service := notifications.NewService(store, pub)
	userCtx := notifications.ContextWithActor(ctx, "usr_operator")
	first, err := service.CreateNotification(userCtx, notificationInput(7, "same"))
	if err != nil {
		t.Fatal(err)
	}
	if pub.deliveries[0].actor.ID != "usr_operator" || pub.deliveries[0].row.UserID != 7 {
		t.Fatal("actor was confused with recipient")
	}
	if _, err := service.Archive(ctx, 7, first.PublicID); err != nil {
		t.Fatal(err)
	}
	duplicate := notificationInput(7, "same")
	duplicate.ActionPayload = notifications.ActionPayload{notifications.PayloadKeyWorkspaceID: "ws_wrong"}
	row, err := service.CreateNotification(ctx, duplicate)
	if err != nil {
		t.Fatal(err)
	}
	last := pub.deliveries[len(pub.deliveries)-1]
	if row.Status != notifications.StatusArchived || last.row.ActionPayload[notifications.PayloadKeyWorkspaceID] != "ws_fact" || last.actor.Type != events.V1ActorSystem {
		t.Fatal("dedupe did not preserve persisted recipient state")
	}
	for _, user := range []int64{3, 7} {
		if _, err := service.CreateNotification(ctx, notificationInput(user, "bulk")); err != nil {
			t.Fatal(err)
		}
	}
	before := len(pub.deliveries)
	rows, err := service.MarkInviteResolved(ctx, "wsi_fact", "accepted")
	if err != nil || len(rows) != 3 {
		t.Fatalf("resolved rows %d: %v", len(rows), err)
	}
	if len(pub.deliveries)-before != 3 {
		t.Fatal("bulk resolution omitted actual recipients")
	}
	before = len(pub.deliveries)
	rows, err = service.MarkAllRead(ctx, 7)
	if err != nil || len(rows) != 1 || len(pub.deliveries)-before != 1 {
		t.Fatalf("read-all rows %d: %v", len(rows), err)
	}
	if pub.deliveries[before].row.UserID != 7 {
		t.Fatal("read-all escaped user boundary")
	}
}
func TestNotificationHintsFollowTransactionOwner(t *testing.T) {
	for _, outcome := range []string{"commit", "rollback", "commit failure"} {
		t.Run(outcome, func(t *testing.T) {
			db := testdb.Open(t)
			store := notifications.NewPostgresStore(db)
			pub := &factPublisher{}
			service := notifications.NewService(store, pub)
			failure := errors.New("rollback requested")
			err := invitations.PostgresTransaction(db)(t.Context(), func(ctx context.Context) error {
				row, err := service.CreateNotification(ctx, notificationInput(7, "tx"))
				if err != nil {
					return err
				}
				if row.PublicID == "" || len(pub.deliveries) != 0 {
					t.Fatal("hint published before commit")
				}
				if outcome == "rollback" {
					return failure
				}
				if outcome == "commit failure" {
					tx, _ := dbutil.AmbientTxFromContext(ctx)
					_, err = tx.Exec(ctx, "CREATE TEMP TABLE notification_commit_failure (id INT UNIQUE DEFERRABLE INITIALLY DEFERRED) ON COMMIT DROP; INSERT INTO notification_commit_failure VALUES (1), (1)")
					return err
				}
				return nil
			})
			if outcome == "commit" {
				if err != nil || len(pub.deliveries) != 1 {
					t.Fatalf("commit: %v deliveries=%d", err, len(pub.deliveries))
				}
			} else if err == nil || len(pub.deliveries) != 0 {
				t.Fatalf("failed transaction published: %v deliveries=%d", err, len(pub.deliveries))
			}
			count, err := store.CountUnread(t.Context(), 7)
			if err != nil {
				t.Fatal(err)
			}
			if (outcome == "commit" && count != 1) || (outcome != "commit" && count != 0) {
				t.Fatalf("persisted count %d for %s", count, outcome)
			}
		})
	}
}
func TestNotificationBorrowedTransactionDefersToOuterCommit(t *testing.T) {
	db := testdb.Open(t)
	store := notifications.NewPostgresStore(db)
	pub := &factPublisher{}
	service := notifications.NewService(store, pub)
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	ctx := dbutil.ContextWithAmbientPostCommit(dbutil.ContextWithAmbientTx(t.Context(), tx))
	if err := invitations.PostgresTransaction(db)(ctx, func(ctx context.Context) error {
		row, err := service.CreateNotification(ctx, notificationInput(7, "borrowed"))
		if err != nil {
			return err
		}
		_, err = service.MarkRead(ctx, 7, row.PublicID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(pub.deliveries) != 0 {
		t.Fatal("borrower published")
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	dbutil.RunAmbientPostCommit(ctx)
	dbutil.RunAmbientPostCommit(ctx)
	if len(pub.deliveries) != 2 || pub.deliveries[0].row.Status != notifications.StatusUnread || pub.deliveries[1].row.Status != notifications.StatusRead {
		t.Fatalf("facts did not preserve command snapshots: %+v", pub.deliveries)
	}
}
func TestNotificationMissingRegistryRefusesBeforeWrite(t *testing.T) {
	db := testdb.Open(t)
	store := notifications.NewPostgresStore(db)
	pub := &factPublisher{}
	service := notifications.NewService(store, pub)
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	ctx := dbutil.ContextWithAmbientTx(t.Context(), tx)
	if _, err := service.CreateNotification(ctx, notificationInput(7, "no-registry")); err == nil {
		t.Fatal("missing registry accepted")
	}
	if count, err := store.CountUnread(ctx, 7); err != nil || count != 0 {
		t.Fatalf("write occurred before refusal: %d %v", count, err)
	}
	dbutil.RunAmbientPostCommit(ctx)
	if len(pub.deliveries) != 0 {
		t.Fatal("uncommitted hint published")
	}
}
func TestEventPublisherUsesDistinctPersistedStateIdentities(t *testing.T) {
	log := events.NewWorkspaceEventLog()
	hub := events.NewUserEventHub()
	service := notifications.NewService(notifications.NewMemoryStore(), &notifications.EventPublisher{Log: log, Hub: hub})
	stream, unsubscribe, ok := hub.Subscribe(7)
	if !ok {
		t.Fatal("subscribe")
	}
	defer unsubscribe()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	row, err := service.CreateNotification(ctx, notificationInput(7, "state"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.MarkRead(ctx, 7, row.PublicID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Archive(ctx, 7, row.PublicID); err != nil {
		t.Fatal(err)
	}
	if log.Len("ws_fact") != 3 {
		t.Fatal("read/archive identity collided")
	}
	for range 3 {
		select {
		case event := <-stream:
			_ = event
		default:
			t.Fatal("recipient hint omitted")
		}
	}
	input := notificationInput(7, "org")
	input.Type = notifications.TypeOrganizationInvite
	delete(input.ActionPayload, notifications.PayloadKeyWorkspaceID)
	if _, err := service.CreateNotification(ctx, input); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stream:
		t.Fatal("organization without workspace emitted")
	default:
	}
}

func TestPostgresBulkCommandsPublishEveryAffectedRow(t *testing.T) {
	db := testdb.Open(t)
	store := notifications.NewPostgresStore(db)
	pub := &factPublisher{}
	service := notifications.NewService(store, pub)
	for _, user := range []int64{3, 7} {
		if _, err := service.CreateNotification(t.Context(), notificationInput(user, "bulk")); err != nil {
			t.Fatal(err)
		}
	}
	before := len(pub.deliveries)
	rows, err := service.MarkInviteResolved(t.Context(), "wsi_fact", "accepted")
	if err != nil || len(rows) != 2 || len(pub.deliveries)-before != 2 {
		t.Fatalf("resolved rows %d, err %v", len(rows), err)
	}
	seen := map[int64]bool{}
	for _, delivery := range pub.deliveries[before:] {
		seen[delivery.row.UserID] = true
		if delivery.row.ActionPayload[notifications.PayloadKeyInviteStatus] != "accepted" {
			t.Fatal("hint did not use returned resolved fact")
		}
	}
	if !seen[3] || !seen[7] {
		t.Fatal("bulk omitted persisted recipient")
	}
	before = len(pub.deliveries)
	rows, err = service.MarkAllRead(t.Context(), 7)
	if err != nil || len(rows) != 1 || rows[0].UserID != 7 || len(pub.deliveries)-before != 1 {
		t.Fatalf("read-all rows %d, err %v", len(rows), err)
	}
}
