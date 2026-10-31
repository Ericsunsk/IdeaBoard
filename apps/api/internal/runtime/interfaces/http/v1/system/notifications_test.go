package system

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"ideaboard/api/internal/notifications"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"
	tenancyevents "ideaboard/api/internal/tenancy/events"
	"ideaboard/api/internal/testsupport"
)

func TestUserNotifications_Unauthenticated(t *testing.T) {
	memStore := notifications.NewMemoryStore()
	engine := newV1MediaTestEngine(
		tenancy.NewInMemoryStoreForTests(),
		newStaticUserDirectory(),
		ContractV1Deps{Notifications: memStore},
	)

	req := httptest.NewRequest(http.MethodGet, runtimehttp.ContractV1GroupPath+"/user/notifications", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUserNotifications_MissingStoreFailsClosed(t *testing.T) {
	engine := newV1MediaTestEngine(
		tenancy.NewInMemoryStoreForTests(),
		newStaticUserDirectory(),
		ContractV1Deps{},
	)

	for _, call := range []v1Call{
		{method: http.MethodGet, path: "/user/notifications"},
		{method: http.MethodGet, path: "/user/notifications/unread-count"},
		{method: http.MethodPost, path: "/user/notifications/notif_missing/read"},
		{method: http.MethodPost, path: "/user/notifications/read-all"},
		{method: http.MethodPost, path: "/user/notifications/notif_missing/archive"},
	} {
		t.Run(call.method+" "+call.path, func(t *testing.T) {
			call.path = runtimehttp.ContractV1GroupPath + call.path
			call.userID = 100
			status, _, payload := doV1(t, engine, call)
			if status != http.StatusServiceUnavailable || v1ErrorCode(t, payload) != string(contract.CodeServiceUnavailable) {
				t.Fatalf("missing store response = %d, %v", status, payload)
			}
		})
	}
}

func TestUserNotifications_Lifecycle(t *testing.T) {
	ctx := context.Background()
	memStore := notifications.NewMemoryStore()
	dir := newStaticUserDirectory()
	engine := newV1MediaTestEngine(
		tenancy.NewInMemoryStoreForTests(),
		dir,
		ContractV1Deps{Notifications: memStore},
	)

	user1ID := int64(100)
	user2ID := int64(200)

	// Seed notifications for user1
	n1, err := memStore.CreateNotification(ctx, notifications.CreateNotificationInput{
		UserID: user1ID,
		Type:   notifications.TypeWorkspaceInvite,
		Title:  "Workspace Invitation",
		Body:   "You have been invited to join Workspace Alpha",
		ActionPayload: notifications.InviteActionPayload{
			InviteID:      "wsi_001",
			WorkspaceID:   "ws_alpha",
			WorkspaceName: "Alpha",
			Role:          "member",
		}.ToActionPayload(),
	})
	if err != nil {
		t.Fatalf("CreateNotification n1 failed: %v", err)
	}

	n2, err := memStore.CreateNotification(ctx, notifications.CreateNotificationInput{
		UserID: user1ID,
		Type:   notifications.TypeStudioHandover,
		Title:  "Studio Handover",
		Body:   "Handover invitation for Studio Beta",
		ActionPayload: notifications.HandoverActionPayload{
			TransferInviteID: "bti_001",
		}.ToActionPayload(),
	})
	if err != nil {
		t.Fatalf("CreateNotification n2 failed: %v", err)
	}

	// 1. GET unread-count
	req := httptest.NewRequest(http.MethodGet, runtimehttp.ContractV1GroupPath+"/user/notifications/unread-count", nil)
	req.Header.Set(testsupport.HeaderTestUser, fmt.Sprintf("%d", user1ID))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unread-count status = %d: %s", rec.Code, rec.Body.String())
	}
	var countResp contract.SuccessEnvelope[v1UnreadCountDTO]
	if err := json.Unmarshal(rec.Body.Bytes(), &countResp); err != nil {
		t.Fatalf("unmarshal count response: %v", err)
	}
	if countResp.Data.Count != 2 {
		t.Fatalf("expected count 2, got %d", countResp.Data.Count)
	}

	// 2. GET list
	req = httptest.NewRequest(http.MethodGet, runtimehttp.ContractV1GroupPath+"/user/notifications?limit=10", nil)
	req.Header.Set(testsupport.HeaderTestUser, fmt.Sprintf("%d", user1ID))
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("list notifications status = %d: %s", rec.Code, rec.Body.String())
	}
	var listResp contract.SuccessEnvelope[v1NotificationListDTO]
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	if listResp.Data.Total != 2 || len(listResp.Data.Items) != 2 {
		t.Fatalf("expected 2 items, got total=%d, len=%d", listResp.Data.Total, len(listResp.Data.Items))
	}

	// 3. POST mark read for n1
	req = httptest.NewRequest(http.MethodPost, runtimehttp.ContractV1GroupPath+"/user/notifications/"+n1.PublicID+"/read", nil)
	req.Header.Set(testsupport.HeaderTestUser, fmt.Sprintf("%d", user1ID))
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("mark read status = %d: %s", rec.Code, rec.Body.String())
	}
	var readResp contract.SuccessEnvelope[v1NotificationDTO]
	if err := json.Unmarshal(rec.Body.Bytes(), &readResp); err != nil {
		t.Fatalf("unmarshal mark read response: %v", err)
	}
	if readResp.Data.Status != "read" || readResp.Data.ReadAt == nil {
		t.Fatalf("expected read status and non-nil ReadAt: %+v", readResp.Data)
	}

	// 4. GET unread-count after marking one read
	req = httptest.NewRequest(http.MethodGet, runtimehttp.ContractV1GroupPath+"/user/notifications/unread-count", nil)
	req.Header.Set(testsupport.HeaderTestUser, fmt.Sprintf("%d", user1ID))
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if err := json.Unmarshal(rec.Body.Bytes(), &countResp); err != nil {
		t.Fatalf("unmarshal count response: %v", err)
	}
	if countResp.Data.Count != 1 {
		t.Fatalf("expected count 1, got %d", countResp.Data.Count)
	}

	// 5. User2 cannot mark User1's notification as read (tenant isolation -> 404)
	req = httptest.NewRequest(http.MethodPost, runtimehttp.ContractV1GroupPath+"/user/notifications/"+n2.PublicID+"/read", nil)
	req.Header.Set(testsupport.HeaderTestUser, fmt.Sprintf("%d", user2ID))
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-user mark read, got %d: %s", rec.Code, rec.Body.String())
	}

	// 6. POST read-all
	req = httptest.NewRequest(http.MethodPost, runtimehttp.ContractV1GroupPath+"/user/notifications/read-all", nil)
	req.Header.Set(testsupport.HeaderTestUser, fmt.Sprintf("%d", user1ID))
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("read-all status = %d: %s", rec.Code, rec.Body.String())
	}
	var readAllResp contract.SuccessEnvelope[v1MarkAllReadDTO]
	if err := json.Unmarshal(rec.Body.Bytes(), &readAllResp); err != nil {
		t.Fatalf("unmarshal read-all response: %v", err)
	}
	if readAllResp.Data.Updated != 1 {
		t.Fatalf("expected 1 updated, got %d", readAllResp.Data.Updated)
	}

	// 7. POST archive
	req = httptest.NewRequest(http.MethodPost, runtimehttp.ContractV1GroupPath+"/user/notifications/"+n1.PublicID+"/archive", nil)
	req.Header.Set(testsupport.HeaderTestUser, fmt.Sprintf("%d", user1ID))
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("archive status = %d: %s", rec.Code, rec.Body.String())
	}
	var archiveResp contract.SuccessEnvelope[v1NotificationDTO]
	if err := json.Unmarshal(rec.Body.Bytes(), &archiveResp); err != nil {
		t.Fatalf("unmarshal archive response: %v", err)
	}
	if archiveResp.Data.Status != "archived" {
		t.Fatalf("expected archived status, got %s", archiveResp.Data.Status)
	}

	// 8. Filter by archived
	req = httptest.NewRequest(http.MethodGet, runtimehttp.ContractV1GroupPath+"/user/notifications?status=archived", nil)
	req.Header.Set(testsupport.HeaderTestUser, fmt.Sprintf("%d", user1ID))
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("list archived status = %d: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	if listResp.Data.Total != 1 || len(listResp.Data.Items) != 1 || listResp.Data.Items[0].ID != n1.PublicID {
		t.Fatalf("expected 1 archived item (%s), got total=%d items=%+v", n1.PublicID, listResp.Data.Total, listResp.Data.Items)
	}
}

func TestUserNotifications_InvalidStatus(t *testing.T) {
	memStore := notifications.NewMemoryStore()
	engine := newV1MediaTestEngine(
		tenancy.NewInMemoryStoreForTests(),
		newStaticUserDirectory(),
		ContractV1Deps{Notifications: memStore},
	)

	req := httptest.NewRequest(http.MethodGet, runtimehttp.ContractV1GroupPath+"/user/notifications?status=bogus", nil)
	req.Header.Set(testsupport.HeaderTestUser, "100")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for invalid status, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUserNotifications_WorkspaceInviteTriggers(t *testing.T) {
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	notifStore := notifications.NewMemoryStore()

	inviterID := int64(10)
	inviteeID := int64(20)
	dir.ensure(inviterID)
	dir.ensure(inviteeID)

	engine := newV1MediaTestEngine(
		store,
		dir,
		ContractV1Deps{
			Store:         store,
			Users:         dir,
			Notifications: notifStore,
			Invitations:   testInvitations(store, nil, tenancyevents.NewCollaborationEventDispatcher(notifStore, nil), NewInvitationIdentityReader(nil, nil, dir)),
		},
	)

	// Create a workspace for inviter
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{
		Name:        "Team Hub",
		OwnerUserID: inviterID,
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	// 1. Create an invite for invitee
	invite, rawToken, err := store.CreateWorkspaceMemberInvite(ctx, ws.PublicID, inviterID, "invitee@example.com", tenancy.RoleMember, inviteeID)
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}

	// Manually trigger the helper logic or call the endpoint
	// First let's test creating invite via HTTP with InviteeUserID set
	// In InMemoryStore, let's verify accept trigger:
	// Invitee accepts the invite
	req := httptest.NewRequest(http.MethodPost, runtimehttp.ContractV1GroupPath+"/invites/"+rawToken+"/accept", nil)
	req.Header.Set(testsupport.HeaderTestUser, fmt.Sprintf("%d", inviteeID))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("accept invite status = %d: %s", rec.Code, rec.Body.String())
	}

	// Inviter should have received workspace_member_joined notification!
	notifs, total, err := notifStore.ListNotifications(ctx, inviterID, notifications.ListFilter{}, 10, 0)
	if err != nil {
		t.Fatalf("list notifications for inviter: %v", err)
	}
	if total != 1 || len(notifs) != 1 {
		t.Fatalf("expected 1 notification for inviter, got %d", total)
	}
	if notifs[0].Type != notifications.TypeWorkspaceMemberJoined {
		t.Fatalf("expected TypeWorkspaceMemberJoined, got %s", notifs[0].Type)
	}

	// 2. Decline trigger: create another invite
	invitee2ID := int64(30)
	_, rawToken2, err := store.CreateWorkspaceMemberInvite(ctx, ws.PublicID, inviterID, "invitee2@example.com", tenancy.RoleMember, invitee2ID)
	if err != nil {
		t.Fatalf("create invite 2: %v", err)
	}

	dir.ensure(invitee2ID)
	req = httptest.NewRequest(http.MethodPost, runtimehttp.ContractV1GroupPath+"/invites/"+rawToken2+"/decline", nil)
	req.Header.Set(testsupport.HeaderTestUser, fmt.Sprintf("%d", invitee2ID))
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("decline invite status = %d: %s", rec.Code, rec.Body.String())
	}

	// Inviter should now have 2 notifications, the latest being workspace_invite_declined
	notifs, total, err = notifStore.ListNotifications(ctx, inviterID, notifications.ListFilter{}, 10, 0)
	if err != nil || total != 2 {
		t.Fatalf("expected 2 notifications for inviter, got %d, err: %v", total, err)
	}
	if notifs[0].Type != notifications.TypeWorkspaceInviteDeclined {
		t.Fatalf("expected TypeWorkspaceInviteDeclined, got %s", notifs[0].Type)
	}
	_ = invite
}

func TestUserNotifications_CategoryValidation(t *testing.T) {
	engine := newV1MediaTestEngine(
		tenancy.NewInMemoryStoreForTests(), newStaticUserDirectory(),
		ContractV1Deps{Notifications: notifications.NewMemoryStore()},
	)
	for _, tc := range []struct {
		category string
		status   int
	}{
		{"all", http.StatusOK}, {"invites", http.StatusOK}, {"billing", http.StatusOK}, {"other", http.StatusBadRequest},
	} {
		t.Run(tc.category, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, runtimehttp.ContractV1GroupPath+"/user/notifications?category="+tc.category, nil)
			req.Header.Set(testsupport.HeaderTestUser, "100")
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status=%d want=%d: %s", rec.Code, tc.status, rec.Body.String())
			}
		})
	}
}
