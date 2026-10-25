package events

import (
	"context"
	"testing"
	"time"

	"ideaboard/api/internal/notifications"
	"ideaboard/api/internal/tenancy"
)

type mockEmailSender struct {
	lastInvite *tenancy.WorkspaceMemberInvite
	lastToken  string
	called     bool
}

func (m *mockEmailSender) SendWorkspaceInviteEmail(ctx context.Context, invite *tenancy.WorkspaceMemberInvite, rawToken string) error {
	m.called = true
	m.lastInvite = invite
	m.lastToken = rawToken
	return nil
}

func TestCollaborationEventDispatcher_WorkspaceMemberInvited(t *testing.T) {
	ctx := context.Background()
	notifStore := notifications.NewMemoryStore()
	emailSender := &mockEmailSender{}
	dispatcher := NewCollaborationEventDispatcher(notifStore, emailSender)

	inviteeID := int64(42)
	invite := &tenancy.WorkspaceMemberInvite{
		PublicID:          "wsi_test1",
		WorkspacePublicID: "ws_test1",
		WorkspaceName:     "Test Space",
		InviterUserID:     10,
		InviteeEmail:      "invitee@example.com",
		InviteeUserID:     &inviteeID,
		Role:              tenancy.RoleMember,
		CreatedAt:         time.Now(),
	}

	err := dispatcher.OnWorkspaceMemberInvited(ctx, invite, "raw-token-123")
	if err != nil {
		t.Fatalf("OnWorkspaceMemberInvited error: %v", err)
	}

	if !emailSender.called {
		t.Fatal("expected emailSender to be called")
	}
	if emailSender.lastToken != "raw-token-123" {
		t.Fatalf("expected raw token 'raw-token-123', got %s", emailSender.lastToken)
	}
	if emailSender.lastInvite.PublicID != "wsi_test1" {
		t.Fatalf("expected invite wsi_test1, got %s", emailSender.lastInvite.PublicID)
	}

	// Invitee notification check
	notifs, count, err := notifStore.ListNotifications(ctx, inviteeID, notifications.ListFilter{}, 10, 0)
	if err != nil {
		t.Fatalf("list notifications failed: %v", err)
	}
	if count != 1 || len(notifs) != 1 {
		t.Fatalf("expected 1 notification for invitee, got %d", count)
	}
	if notifs[0].Type != notifications.TypeWorkspaceInvite {
		t.Fatalf("expected TypeWorkspaceInvite, got %s", notifs[0].Type)
	}
	if notifs[0].ActionPayload[notifications.PayloadKeyInviteID] != "wsi_test1" {
		t.Fatalf("expected invite_id wsi_test1, got %v", notifs[0].ActionPayload[notifications.PayloadKeyInviteID])
	}
	if notifs[0].ActionPayload[notifications.PayloadKeyInviteStatus] != "pending" {
		t.Fatalf("expected pending invite_status, got %v", notifs[0].ActionPayload[notifications.PayloadKeyInviteStatus])
	}

	// Without invitee user ID (not registered yet), email is sent but no notification
	emailSender.called = false
	inviteNoUser := &tenancy.WorkspaceMemberInvite{
		PublicID:          "wsi_test2",
		WorkspacePublicID: "ws_test1",
		WorkspaceName:     "Test Space",
		InviterUserID:     10,
		InviteeEmail:      "unregistered@example.com",
		InviteeUserID:     nil,
		Role:              tenancy.RoleAdmin,
	}
	err = dispatcher.OnWorkspaceMemberInvited(ctx, inviteNoUser, "token-2")
	if err != nil {
		t.Fatalf("OnWorkspaceMemberInvited error: %v", err)
	}
	if !emailSender.called {
		t.Fatal("expected emailSender to be called for unregistered user")
	}
}

func TestCollaborationEventDispatcher_WorkspaceMemberInviteAccepted(t *testing.T) {
	ctx := context.Background()
	notifStore := notifications.NewMemoryStore()
	dispatcher := NewCollaborationEventDispatcher(notifStore, nil)

	inviterID := int64(10)
	invite := &tenancy.WorkspaceMemberInvite{
		PublicID:          "wsi_acc1",
		WorkspacePublicID: "ws_acc1",
		WorkspaceName:     "Acme Workspace",
		InviterUserID:     inviterID,
		InviteeEmail:      "joiner@example.com",
		Role:              tenancy.RoleMember,
	}
	member := &tenancy.WorkspaceMember{
		UserID:         20,
		UserExternalID: "usr_member20",
		Role:           tenancy.RoleMember,
	}

	err := dispatcher.OnWorkspaceMemberInviteAccepted(ctx, invite, member, "joiner@example.com")
	if err != nil {
		t.Fatalf("OnWorkspaceMemberInviteAccepted error: %v", err)
	}

	notifs, count, err := notifStore.ListNotifications(ctx, inviterID, notifications.ListFilter{}, 10, 0)
	if err != nil {
		t.Fatalf("list notifications error: %v", err)
	}
	if count != 1 || len(notifs) != 1 {
		t.Fatalf("expected 1 notification for inviter, got %d", count)
	}
	n := notifs[0]
	if n.Type != notifications.TypeWorkspaceMemberJoined {
		t.Fatalf("expected TypeWorkspaceMemberJoined, got %s", n.Type)
	}
	if n.ActionPayload[notifications.PayloadKeyMemberUserID] != "usr_member20" {
		t.Fatalf("expected member_user_id usr_member20, got %v", n.ActionPayload[notifications.PayloadKeyMemberUserID])
	}
	if n.Body != "joiner@example.com joined Acme Workspace" {
		t.Fatalf("unexpected body: %s", n.Body)
	}
}

func TestCollaborationEventDispatcher_WorkspaceMemberInviteDeclined(t *testing.T) {
	ctx := context.Background()
	notifStore := notifications.NewMemoryStore()
	dispatcher := NewCollaborationEventDispatcher(notifStore, nil)

	inviterID := int64(10)
	invite := &tenancy.WorkspaceMemberInvite{
		PublicID:          "wsi_dec1",
		WorkspacePublicID: "ws_dec1",
		WorkspaceName:     "Declined Workspace",
		InviterUserID:     inviterID,
		InviteeEmail:      "decliner@example.com",
		Role:              tenancy.RoleMember,
	}

	err := dispatcher.OnWorkspaceMemberInviteDeclined(ctx, invite)
	if err != nil {
		t.Fatalf("OnWorkspaceMemberInviteDeclined error: %v", err)
	}

	notifs, count, err := notifStore.ListNotifications(ctx, inviterID, notifications.ListFilter{}, 10, 0)
	if err != nil {
		t.Fatalf("list notifications error: %v", err)
	}
	if count != 1 || len(notifs) != 1 {
		t.Fatalf("expected 1 notification for inviter, got %d", count)
	}
	n := notifs[0]
	if n.Type != notifications.TypeWorkspaceInviteDeclined {
		t.Fatalf("expected TypeWorkspaceInviteDeclined, got %s", n.Type)
	}
	if n.Body != "decliner@example.com declined the invitation to join Declined Workspace" {
		t.Fatalf("unexpected body: %s", n.Body)
	}
	if n.ActionPayload[notifications.PayloadKeyInviteeEmail] != "decliner@example.com" {
		t.Fatalf("expected invitee_email in payload, got %v", n.ActionPayload[notifications.PayloadKeyInviteeEmail])
	}
}

func TestCollaborationEventDispatcher_InviteStatusResolved(t *testing.T) {
	ctx := context.Background()
	notifStore := notifications.NewMemoryStore()
	dispatcher := NewCollaborationEventDispatcher(notifStore, nil)

	inviteeID := int64(77)
	invite := &tenancy.WorkspaceMemberInvite{
		PublicID:          "wsi_status9",
		WorkspacePublicID: "ws_status9",
		WorkspaceName:     "Status Space",
		InviterUserID:     10,
		InviteeEmail:      "status@example.com",
		InviteeUserID:     &inviteeID,
		Role:              tenancy.RoleMember,
		CreatedAt:         time.Now(),
	}
	if err := dispatcher.OnWorkspaceMemberInvited(ctx, invite, "tok"); err != nil {
		t.Fatalf("OnWorkspaceMemberInvited error: %v", err)
	}
	if err := dispatcher.OnWorkspaceMemberInviteAccepted(ctx, invite, &tenancy.WorkspaceMember{UserID: 77, UserExternalID: "usr_77"}, "status@example.com"); err != nil {
		t.Fatalf("OnWorkspaceMemberInviteAccepted error: %v", err)
	}
	notifs, _, err := notifStore.ListNotifications(ctx, inviteeID, notifications.ListFilter{}, 10, 0)
	if err != nil || len(notifs) != 1 {
		t.Fatalf("expected 1 invitee notification, got %d, err: %v", len(notifs), err)
	}
	if notifs[0].ActionPayload[notifications.PayloadKeyInviteStatus] != "accepted" {
		t.Fatalf("expected accepted invite_status, got %v", notifs[0].ActionPayload[notifications.PayloadKeyInviteStatus])
	}

	inviteeID2 := int64(78)
	invite2 := &tenancy.WorkspaceMemberInvite{
		PublicID:          "wsi_status10",
		WorkspacePublicID: "ws_status10",
		WorkspaceName:     "Status Space",
		InviterUserID:     10,
		InviteeEmail:      "status2@example.com",
		InviteeUserID:     &inviteeID2,
		Role:              tenancy.RoleMember,
		CreatedAt:         time.Now(),
	}
	if err := dispatcher.OnWorkspaceMemberInvited(ctx, invite2, "tok"); err != nil {
		t.Fatalf("OnWorkspaceMemberInvited error: %v", err)
	}
	if err := dispatcher.OnWorkspaceMemberInviteDeclined(ctx, invite2); err != nil {
		t.Fatalf("OnWorkspaceMemberInviteDeclined error: %v", err)
	}
	notifs2, _, err := notifStore.ListNotifications(ctx, inviteeID2, notifications.ListFilter{}, 10, 0)
	if err != nil || len(notifs2) != 1 {
		t.Fatalf("expected 1 invitee notification, got %d, err: %v", len(notifs2), err)
	}
	if notifs2[0].ActionPayload[notifications.PayloadKeyInviteStatus] != "declined" {
		t.Fatalf("expected declined invite_status, got %v", notifs2[0].ActionPayload[notifications.PayloadKeyInviteStatus])
	}
}

func TestCollaborationEventDispatcher_BillingTransferInitiated(t *testing.T) {
	ctx := context.Background()
	notifStore := notifications.NewMemoryStore()
	dispatcher := NewCollaborationEventDispatcher(notifStore, nil)

	recipientID := int64(30)
	transfer := &tenancy.BillingTransferInvite{
		PublicID:         "bti_trans1",
		BillingAccountID: 100,
		FromUserID:       10,
		ToUserID:         recipientID,
		Status:           tenancy.BillingTransferPending,
	}

	err := dispatcher.OnBillingTransferInitiated(ctx, transfer, tenancy.BillingTransferInitiatedParams{
		FromUserPublicID:       "usr_owner10",
		WorkspacePublicID:      "ws_main",
		WorkspaceName:          "Main Studio",
		BillingAccountPublicID: "bac_100",
	})
	if err != nil {
		t.Fatalf("OnBillingTransferInitiated error: %v", err)
	}

	notifs, count, err := notifStore.ListNotifications(ctx, recipientID, notifications.ListFilter{}, 10, 0)
	if err != nil {
		t.Fatalf("list notifications error: %v", err)
	}
	if count != 1 || len(notifs) != 1 {
		t.Fatalf("expected 1 notification for recipient, got %d", count)
	}
	n := notifs[0]
	if n.Type != notifications.TypeStudioHandover {
		t.Fatalf("expected TypeStudioHandover, got %s", n.Type)
	}
	if n.ActionPayload[notifications.PayloadKeyTransferInviteID] != "bti_trans1" {
		t.Fatalf("expected transfer_invite_id bti_trans1, got %v", n.ActionPayload[notifications.PayloadKeyTransferInviteID])
	}
	if n.ActionPayload[notifications.PayloadKeyBillingAccountID] != "bac_100" {
		t.Fatalf("expected billing_account_id bac_100, got %v", n.ActionPayload[notifications.PayloadKeyBillingAccountID])
	}
	if n.ActionPayload[notifications.PayloadKeyFromUserID] != "usr_owner10" {
		t.Fatalf("expected from_user_id usr_owner10, got %v", n.ActionPayload[notifications.PayloadKeyFromUserID])
	}
}
