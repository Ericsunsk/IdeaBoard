package notifications_test

import (
	"context"
	"testing"

	"ideaboard/api/internal/notifications"
)

func TestMemoryStore(t *testing.T) {
	ctx := context.Background()
	store := notifications.NewMemoryStore()

	userID := int64(42)

	// Initially 0 unread
	unread, err := store.CountUnread(ctx, userID)
	if err != nil {
		t.Fatalf("CountUnread failed: %v", err)
	}
	if unread != 0 {
		t.Fatalf("expected 0 unread, got %d", unread)
	}

	// Create notification 1
	n1, err := store.CreateNotification(ctx, notifications.CreateNotificationInput{
		UserID: userID,
		Type:   notifications.TypeWorkspaceInvite,
		Title:  "Workspace Invitation",
		Body:   "You have been invited to join Workspace A",
		ActionPayload: notifications.InviteActionPayload{
			WorkspaceID: "ws_123",
		}.ToActionPayload(),
	})
	if err != nil {
		t.Fatalf("CreateNotification failed: %v", err)
	}
	if n1.PublicID == "" || n1.Status != notifications.StatusUnread {
		t.Fatalf("unexpected notification: %+v", n1)
	}

	// Create notification 2
	n2, err := store.CreateNotification(ctx, notifications.CreateNotificationInput{
		UserID: userID,
		Type:   notifications.TypeBillingCreditLow,
		Title:  "Credits Low",
		Body:   "Your credits are running low",
		ActionPayload: notifications.BillingActionPayload{
			BillingAccountID: "ba_123",
		}.ToActionPayload(),
	})
	if err != nil {
		t.Fatalf("CreateNotification failed: %v", err)
	}

	// Unread count should be 2
	unread, err = store.CountUnread(ctx, userID)
	if err != nil || unread != 2 {
		t.Fatalf("expected 2 unread, got %d, err: %v", unread, err)
	}

	// List notifications
	list, total, err := store.ListNotifications(ctx, userID, notifications.ListFilter{}, 10, 0)
	if err != nil {
		t.Fatalf("ListNotifications failed: %v", err)
	}
	if total != 2 || len(list) != 2 {
		t.Fatalf("expected 2 items, got %d (total %d)", len(list), total)
	}

	// Filter by status unread
	statusUnread := string(notifications.StatusUnread)
	list, total, err = store.ListNotifications(ctx, userID, notifications.ListFilter{Status: statusUnread}, 10, 0)
	if err != nil || len(list) != 2 || total != 2 {
		t.Fatalf("expected 2 unread items, got %d, total %d", len(list), total)
	}

	// MarkRead n1
	readN1, err := store.MarkRead(ctx, userID, n1.PublicID)
	if err != nil {
		t.Fatalf("MarkRead failed: %v", err)
	}
	if readN1.Status != notifications.StatusRead || readN1.ReadAt == nil {
		t.Fatalf("expected read status and non-nil ReadAt, got %+v", readN1)
	}

	// Unread count should now be 1
	unread, err = store.CountUnread(ctx, userID)
	if err != nil || unread != 1 {
		t.Fatalf("expected 1 unread, got %d", unread)
	}

	// Archive n2
	archivedN2, err := store.Archive(ctx, userID, n2.PublicID)
	if err != nil {
		t.Fatalf("Archive failed: %v", err)
	}
	if archivedN2.Status != notifications.StatusArchived {
		t.Fatalf("expected archived status, got %+v", archivedN2)
	}

	// Create another unread
	_, err = store.CreateNotification(ctx, notifications.CreateNotificationInput{
		UserID: userID,
		Type:   notifications.TypeWorkspaceMemberJoined,
		Title:  "Member joined",
		Body:   "Alice joined the workspace",
	})
	if err != nil {
		t.Fatalf("CreateNotification failed: %v", err)
	}

	// MarkAllRead
	marked, err := store.MarkAllRead(ctx, userID)
	if err != nil {
		t.Fatalf("MarkAllRead failed: %v", err)
	}
	if len(marked) != 1 {
		t.Fatalf("expected 1 marked read, got %d", len(marked))
	}

	unread, err = store.CountUnread(ctx, userID)
	if err != nil || unread != 0 {
		t.Fatalf("expected 0 unread, got %d", unread)
	}

	// MarkRead non-existent should return ErrNotFound
	_, err = store.MarkRead(ctx, userID, "notif_nonexistent")
	if err != notifications.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// MarkInviteResolved merges invite_status into actionable invite rows only
	invNotif, err := store.CreateNotification(ctx, notifications.CreateNotificationInput{
		UserID: userID,
		Type:   notifications.TypeWorkspaceInvite,
		Title:  "Workspace Invitation",
		Body:   "Join",
		ActionPayload: notifications.InviteActionPayload{
			InviteID:     "wsi_status1",
			WorkspaceID:  "ws_1",
			InviteStatus: "pending",
		}.ToActionPayload(),
	})
	if err != nil {
		t.Fatalf("CreateNotification failed: %v", err)
	}
	if invNotif.ActionPayload[notifications.PayloadKeyInviteStatus] != "pending" {
		t.Fatalf("expected pending invite_status, got %v", invNotif.ActionPayload[notifications.PayloadKeyInviteStatus])
	}
	affected, err := store.MarkInviteResolved(ctx, "wsi_status1", "accepted")
	if err != nil || len(affected) != 1 {
		t.Fatalf("expected 1 row resolved, got %d, err: %v", len(affected), err)
	}
}
