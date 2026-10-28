package invitations

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/notifications"
	"ideaboard/api/internal/tenancy"
	tenancyevents "ideaboard/api/internal/tenancy/events"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

type notificationFailure struct {
	notifications.Store
	createErr, errorResolve error
	beforeCreate            func(notifications.CreateNotificationInput)
}

func (s notificationFailure) CreateNotification(ctx context.Context, input notifications.CreateNotificationInput) (*notifications.Notification, error) {
	if s.beforeCreate != nil {
		s.beforeCreate(input)
	}
	if s.createErr != nil {
		return nil, s.createErr
	}
	return s.Store.CreateNotification(ctx, input)
}
func (s notificationFailure) MarkInviteResolved(ctx context.Context, id, status string) ([]*notifications.Notification, error) {
	if s.errorResolve != nil {
		return nil, s.errorResolve
	}
	return s.Store.MarkInviteResolved(ctx, id, status)
}

type testMailSender struct{ err error }

func (s testMailSender) SendWorkspaceInviteEmail(context.Context, *tenancy.WorkspaceMemberInvite, string) error {
	return s.err
}

type fixture struct {
	db            *pgxpool.Pool
	store         *tenancypg.Store
	notifications *notifications.PostgresStore
	service       *Service
	workspace     *tenancy.Workspace
	owner, target int64
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	db := testdb.Open(t)
	store := tenancypg.NewStore(db, nil)
	owner, target := int64(3), int64(7)
	workspace, err := store.CreateWorkspace(t.Context(), tenancy.CreateWorkspaceParams{Name: "Atomic invites", OwnerUserID: owner})
	if err != nil {
		t.Fatal(err)
	}
	notices := notifications.NewPostgresStore(db)
	service := &Service{Store: store, Identities: &testIdentityReader{emails: map[int64]string{target: "member@example.com"}, users: map[string]int64{"member@example.com": target}}, Events: tenancyevents.NewCollaborationEventDispatcher(notices, testMailSender{}), Transaction: PostgresTransaction(db)}
	return fixture{db: db, store: store, notifications: notices, service: service, workspace: workspace, owner: owner, target: target}
}
func (f fixture) create(t *testing.T) *tenancy.WorkspaceMemberInvite {
	t.Helper()
	invite, _, err := f.service.CreateWorkspaceMemberInvite(t.Context(), f.workspace.PublicID, f.owner, "member@example.com", tenancy.RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	return invite
}
func (f fixture) requirePending(t *testing.T, invite *tenancy.WorkspaceMemberInvite) {
	t.Helper()
	actual, err := f.store.GetWorkspaceMemberInvite(t.Context(), invite.PublicID)
	if err != nil || actual.Status != tenancy.MemberInviteStatusPending {
		t.Fatalf("invite not pending: %+v %v", actual, err)
	}
	items, total, err := f.notifications.ListNotifications(t.Context(), f.target, notifications.ListFilter{}, 20, 0)
	if err != nil || total != 1 || items[0].ActionPayload[notifications.PayloadKeyInviteStatus] != tenancy.MemberInviteStatusPending {
		t.Fatalf("notification not pending: %+v %d %v", items, total, err)
	}
}
func TestInvitationCreateRollsBackWhenNotificationOrMailFails(t *testing.T) {
	failure := errors.New("injected delivery failure")
	for _, part := range []string{"notification", "mail"} {
		t.Run(part, func(t *testing.T) {
			f := newFixture(t)
			if part == "notification" {
				f.service.Events = tenancyevents.NewCollaborationEventDispatcher(notificationFailure{Store: f.notifications, createErr: failure}, testMailSender{})
			} else {
				f.service.Events = tenancyevents.NewCollaborationEventDispatcher(f.notifications, testMailSender{err: failure})
			}
			invite, token, err := f.service.CreateWorkspaceMemberInvite(t.Context(), f.workspace.PublicID, f.owner, "member@example.com", tenancy.RoleMember)
			if !errors.Is(err, failure) || invite != nil || token != "" {
				t.Fatalf("error exposed partial result: %+v %q %v", invite, token, err)
			}
			var count int
			if err := f.db.QueryRow(t.Context(), "SELECT count(*) FROM workspace_member_invites").Scan(&count); err != nil || count != 0 {
				t.Fatalf("invite survived rollback: %d %v", count, err)
			}
			if count, err := f.notifications.CountUnread(t.Context(), f.target); err != nil || count != 0 {
				t.Fatalf("notification survived rollback: %d %v", count, err)
			}
		})
	}
}
func TestInvitationDecisionAndNotificationRollbackTogether(t *testing.T) {
	failure := errors.New("injected notification failure")
	for _, decision := range []string{"accept", "decline", "cancel"} {
		t.Run(decision, func(t *testing.T) {
			f := newFixture(t)
			invite := f.create(t)
			f.service.Events = tenancyevents.NewCollaborationEventDispatcher(notificationFailure{Store: f.notifications, errorResolve: failure}, testMailSender{})
			var err error
			switch decision {
			case "accept":
				_, _, err = f.service.AcceptWorkspaceInvite(t.Context(), invite.PublicID, f.target)
			case "decline":
				_, err = f.service.DeclineWorkspaceInvite(t.Context(), invite.PublicID, f.target)
			case "cancel":
				err = f.service.CancelWorkspaceMemberInvite(t.Context(), f.workspace.PublicID, invite.PublicID, f.owner)
			}
			if !errors.Is(err, failure) {
				t.Fatalf("lost notification failure: %v", err)
			}
			f.requirePending(t, invite)
			if _, member, err := f.store.GetWorkspaceRole(t.Context(), f.workspace.PublicID, f.target); err != nil || member {
				t.Fatalf("membership survived rollback: %v %v", member, err)
			}
		})
	}
}
func TestCreateAndCancelCannotLeaveLatePendingNotification(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	reached := make(chan string, 1)
	release := make(chan struct{})
	var once sync.Once
	f.service.Events = tenancyevents.NewCollaborationEventDispatcher(notificationFailure{Store: f.notifications, beforeCreate: func(input notifications.CreateNotificationInput) {
		once.Do(func() {
			reached <- input.ActionPayload[notifications.PayloadKeyInviteID].(string)
			select {
			case <-release:
			case <-ctx.Done():
			}
		})
	}}, testMailSender{})
	created := make(chan error, 1)
	go func() {
		_, _, err := f.service.CreateWorkspaceMemberInvite(ctx, f.workspace.PublicID, f.owner, "member@example.com", tenancy.RoleMember)
		created <- err
	}()
	var inviteID string
	select {
	case inviteID = <-reached:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	cancelled := make(chan error, 1)
	go func() {
		cancelled <- f.service.CancelWorkspaceMemberInvite(ctx, f.workspace.PublicID, inviteID, f.owner)
	}()
	// The cancel cannot finish while create still owns the scope transaction.
	select {
	case err := <-cancelled:
		t.Fatalf("cancel bypassed create transaction: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	for name, ch := range map[string]<-chan error{"create": created, "cancel": cancelled} {
		select {
		case err := <-ch:
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	items, total, err := f.notifications.ListNotifications(ctx, f.target, notifications.ListFilter{}, 20, 0)
	if err != nil || total != 1 || items[0].ActionPayload[notifications.PayloadKeyInviteStatus] != tenancy.MemberInviteStatusCancelled {
		t.Fatalf("late pending notification: %+v total=%d err=%v", items, total, err)
	}
}
func TestNotificationDurableDedupePreservesRecipientState(t *testing.T) {
	f := newFixture(t)
	input := notifications.CreateNotificationInput{UserID: f.target, Type: notifications.TypeStudioHandover, Title: "Handover", Body: "Body", ActionPayload: notifications.ActionPayload{}, DeduplicationKey: "handover:stable-request"}
	original, err := f.notifications.CreateNotification(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.notifications.Archive(t.Context(), f.target, original.PublicID); err != nil {
		t.Fatal(err)
	}
	input.Body = "replayed body"
	replay, err := f.notifications.CreateNotification(t.Context(), input)
	if err != nil || replay.PublicID != original.PublicID || replay.Status != notifications.StatusArchived || replay.Body != "Body" {
		t.Fatalf("replay replaced notification facts: %+v %v", replay, err)
	}
	var count int
	if err := f.db.QueryRow(t.Context(), "SELECT count(*) FROM user_notifications WHERE user_id=$1", f.target).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate replay: %d %v", count, err)
	}
}

func TestOrganizationInvitationsAndNotificationsRollbackTogether(t *testing.T) {
	failure := errors.New("injected notification failure")
	for _, decision := range []string{"create", "accept", "decline", "cancel"} {
		t.Run(decision, func(t *testing.T) {
			f := newFixture(t)
			org, err := f.store.CreateOrganization(t.Context(), f.owner, "Atomic organization")
			if err != nil {
				t.Fatal(err)
			}
			var invite *tenancy.OrganizationMemberInvite
			if decision != "create" {
				invite, _, err = f.service.CreateOrganizationMemberInvite(t.Context(), org.PublicID, f.owner, "member@example.com", tenancy.RoleMember)
				if err != nil {
					t.Fatal(err)
				}
			}
			failing := notificationFailure{Store: f.notifications}
			if decision == "create" {
				failing.createErr = failure
			} else {
				failing.errorResolve = failure
			}
			f.service.Events = tenancyevents.NewCollaborationEventDispatcher(failing, testMailSender{})
			switch decision {
			case "create":
				_, _, err = f.service.CreateOrganizationMemberInvite(t.Context(), org.PublicID, f.owner, "member@example.com", tenancy.RoleMember)
			case "accept":
				_, _, err = f.service.AcceptOrganizationInvite(t.Context(), invite.PublicID, f.target)
			case "decline":
				_, err = f.service.DeclineOrganizationInvite(t.Context(), invite.PublicID, f.target)
			case "cancel":
				err = f.service.CancelOrganizationMemberInvite(t.Context(), org.PublicID, invite.PublicID, f.owner)
			}
			if !errors.Is(err, failure) {
				t.Fatalf("lost transaction failure: %v", err)
			}
			if decision == "create" {
				var count int
				if err := f.db.QueryRow(t.Context(), "SELECT count(*) FROM organization_member_invites").Scan(&count); err != nil || count != 0 {
					t.Fatalf("organization invite survived rollback: %d %v", count, err)
				}
			} else {
				pending, err := f.store.GetOrganizationMemberInvite(t.Context(), invite.PublicID)
				if err != nil || pending.Status != tenancy.MemberInviteStatusPending {
					t.Fatalf("organization decision escaped rollback: %+v %v", pending, err)
				}
				if _, member, err := f.store.GetOrganizationRole(t.Context(), org.PublicID, f.target); err != nil || member {
					t.Fatalf("organization membership escaped rollback: %v %v", member, err)
				}
			}
		})
	}
}

func TestHandoverNotificationIsAtomicAndDurablyDeduplicated(t *testing.T) {
	f := newFixture(t)
	if _, err := f.store.AddWorkspaceMember(t.Context(), f.workspace.PublicID, f.target, tenancy.RoleMember); err != nil {
		t.Fatal(err)
	}
	accounts := tenancypg.NewBillingAccountStore(f.db)
	account, err := accounts.EnsureBillingAccountForWorkspace(t.Context(), f.workspace.InternalID, f.workspace.PublicID)
	if err != nil {
		t.Fatal(err)
	}
	f.service.Transfers = accounts
	params := tenancy.BillingTransferInitiatedParams{WorkspacePublicID: f.workspace.PublicID, WorkspaceName: f.workspace.Name}
	failure := errors.New("notification unavailable")
	f.service.Events = tenancyevents.NewCollaborationEventDispatcher(notificationFailure{Store: f.notifications, createErr: failure}, testMailSender{})
	if _, err := f.service.InitiateTransferInvite(t.Context(), account.InternalID, f.owner, f.target, "stable-key", params); !errors.Is(err, failure) {
		t.Fatalf("handover did not roll back: %v", err)
	}
	var count int
	if err := f.db.QueryRow(t.Context(), "SELECT count(*) FROM billing_account_transfer_invites").Scan(&count); err != nil || count != 0 {
		t.Fatalf("handover invite survived notification failure: %d %v", count, err)
	}
	f.service.Events = tenancyevents.NewCollaborationEventDispatcher(f.notifications, testMailSender{})
	invite, err := f.service.InitiateTransferInvite(t.Context(), account.InternalID, f.owner, f.target, "stable-key", params)
	if err != nil {
		t.Fatal(err)
	}
	// No HTTP cache participates: replay the persisted key through a fresh service.
	restarted := *f.service
	if _, err := restarted.InitiateTransferInvite(t.Context(), account.InternalID, f.owner, f.target, "stable-key", params); err != nil {
		t.Fatal(err)
	}
	// The persistence layer also replays the same live invite for a new HTTP
	// key targeting the same recipient; that is still one invitation lifecycle.
	if _, err := restarted.InitiateTransferInvite(t.Context(), account.InternalID, f.owner, f.target, "different-key-same-pending", params); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(t.Context(), "SELECT count(*) FROM user_notifications WHERE user_id=$1", f.target).Scan(&count); err != nil || count != 1 {
		t.Fatalf("handover replay duplicated notice: %d %v", count, err)
	}
	if _, err := f.db.Exec(t.Context(), "UPDATE billing_account_transfer_invites SET expires_at=now()-interval '1 second' WHERE public_id=$1", invite.PublicID); err != nil {
		t.Fatal(err)
	}
	// Reissue is an in-place invitation update with a new request identity;
	// the recipient still needs a fresh notification for this new lifecycle.
	if _, err := restarted.InitiateTransferInvite(t.Context(), account.InternalID, f.owner, f.target, "reissued-key", params); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(t.Context(), "SELECT count(*) FROM user_notifications WHERE user_id=$1", f.target).Scan(&count); err != nil || count != 2 {
		t.Fatalf("reissue lost notification: %d %v", count, err)
	}
}
