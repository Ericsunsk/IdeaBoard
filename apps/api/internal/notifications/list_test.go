package notifications_test

import (
	"context"
	"fmt"
	"testing"

	"ideaboard/api/internal/notifications"
	"ideaboard/api/internal/testdb"
)

// Both repositories filter before applying offset/limit, including rows that
// would otherwise be hidden behind the first page of a different category.
func testNotificationFeeds(t *testing.T, store notifications.Store, userID int64) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < 25; i++ {
		if _, err := store.CreateNotification(ctx, notifications.CreateNotificationInput{
			UserID: userID, Type: notifications.TypeWorkspaceInvite,
			Title: fmt.Sprintf("invite %d", i), Body: "invite",
		}); err != nil {
			t.Fatal(err)
		}
	}
	bill, err := store.CreateNotification(ctx, notifications.CreateNotificationInput{
		UserID: userID, Type: notifications.TypeBillingCreditLow, Title: "billing", Body: "billing",
	})
	if err != nil {
		t.Fatal(err)
	}
	archived, err := store.CreateNotification(ctx, notifications.CreateNotificationInput{
		UserID: userID, Type: notifications.TypeWorkspaceInvite, Title: "archived", Body: "archived",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Archive(ctx, userID, archived.PublicID); err != nil {
		t.Fatal(err)
	}

	page, total, err := store.ListNotifications(ctx, userID, notifications.ListFilter{}, 20, 0)
	if err != nil || len(page) != 20 || total != 26 {
		t.Fatalf("first page: len=%d total=%d err=%v", len(page), total, err)
	}
	page, total, err = store.ListNotifications(ctx, userID, notifications.ListFilter{}, 20, 20)
	if err != nil || len(page) != 6 || total != 26 {
		t.Fatalf("second page: len=%d total=%d err=%v", len(page), total, err)
	}
	page, total, err = store.ListNotifications(ctx, userID, notifications.ListFilter{Category: "billing"}, 20, 0)
	if err != nil || len(page) != 1 || total != 1 || page[0].PublicID != bill.PublicID {
		t.Fatalf("billing: len=%d total=%d err=%v", len(page), total, err)
	}
	page, total, err = store.ListNotifications(ctx, userID, notifications.ListFilter{Category: "invites"}, 20, 20)
	if err != nil || len(page) != 5 || total != 25 {
		t.Fatalf("invite page: len=%d total=%d err=%v", len(page), total, err)
	}
	page, total, err = store.ListNotifications(ctx, userID, notifications.ListFilter{Status: "archived", Category: "invites"}, 20, 0)
	if err != nil || len(page) != 1 || total != 1 || page[0].PublicID != archived.PublicID {
		t.Fatalf("archive: len=%d total=%d err=%v", len(page), total, err)
	}
	page, total, err = store.ListNotifications(ctx, userID+1, notifications.ListFilter{}, 20, 0)
	if err != nil || len(page) != 0 || total != 0 {
		t.Fatalf("other user: len=%d total=%d err=%v", len(page), total, err)
	}
}

func TestMemoryNotificationFeeds(t *testing.T) {
	testNotificationFeeds(t, notifications.NewMemoryStore(), 200)
}

func TestPostgresNotificationFeeds(t *testing.T) {
	db := testdb.Open(t)
	var userID int64
	if err := db.QueryRow(t.Context(), `INSERT INTO users (display_name, storage_uid, role, status) VALUES ('notifications-test', 'NTF001', 'user', 1) RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	testNotificationFeeds(t, notifications.NewPostgresStore(db), userID)
}
