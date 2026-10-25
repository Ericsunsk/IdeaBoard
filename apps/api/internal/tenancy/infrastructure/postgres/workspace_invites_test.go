package postgres

import (
	"context"
	"errors"
	"fmt"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/tenancy"
)

func TestWorkspaceMemberInvitesPostgresLifecycle(t *testing.T) {
	db := newTenancyTestDB(t)
	store := NewStore(db, nil)
	ctx := context.Background()

	ownerID := createTenancyTestUser(t, db)
	inviteeID := createTenancyTestUser(t, db)
	invitee2ID := createTenancyTestUser(t, db)

	// 1. Default Workspace Protection: Personal default cannot create invites
	defWs, err := store.EnsureDefaultWorkspace(ctx, ownerID)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.CreateWorkspaceMemberInvite(ctx, defWs.PublicID, ownerID, "friend@example.com", tenancy.RoleMember, 0)
	if !errors.Is(err, tenancy.ErrDefaultWorkspaceProtected) {
		t.Fatalf("default workspace invite error = %v, want ErrDefaultWorkspaceProtected", err)
	}

	// 2. Plain workspace invite lifecycle
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{
		Name:        "Invite Test Space",
		OwnerUserID: ownerID,
	})
	if err != nil {
		t.Fatal(err)
	}

	inviteeEmail := "test-invitee@example.com"
	invite, rawToken, err := store.CreateWorkspaceMemberInvite(ctx, ws.PublicID, ownerID, inviteeEmail, tenancy.RoleMember, inviteeID)
	if err != nil {
		t.Fatalf("CreateWorkspaceMemberInvite error = %v", err)
	}
	if !strings.HasPrefix(invite.PublicID, tenancy.WorkspaceInvitePrefix) {
		t.Fatalf("expected wsi_ prefix, got %q", invite.PublicID)
	}
	if len(rawToken) != 64 {
		t.Fatalf("expected 64 hex char raw token, got %q", rawToken)
	}
	if invite.Status != tenancy.MemberInviteStatusPending {
		t.Fatalf("expected pending status, got %q", invite.Status)
	}
	if invite.WorkspacePublicID != ws.PublicID || invite.WorkspaceName != "Invite Test Space" {
		t.Fatalf("workspace info mismatch: public_id=%q name=%q", invite.WorkspacePublicID, invite.WorkspaceName)
	}
	if invite.InviteeUserID == nil || *invite.InviteeUserID != inviteeID {
		t.Fatalf("expected persisted invitee_user_id %d, got %+v", inviteeID, invite.InviteeUserID)
	}

	// Duplicate pending invite for same email should conflict
	_, _, err = store.CreateWorkspaceMemberInvite(ctx, ws.PublicID, ownerID, inviteeEmail, tenancy.RoleMember, inviteeID)
	if !errors.Is(err, tenancy.ErrWorkspaceInviteConflict) && !errors.Is(err, tenancy.ErrConflict) {
		t.Fatalf("duplicate invite error = %v, want conflict", err)
	}

	// 3. List workspace member invites
	wsInvites, err := store.ListWorkspaceMemberInvites(ctx, ws.PublicID)
	if err != nil {
		t.Fatalf("ListWorkspaceMemberInvites error = %v", err)
	}
	if len(wsInvites) != 1 || wsInvites[0].PublicID != invite.PublicID {
		t.Fatalf("unexpected workspace invites list: %+v", wsInvites)
	}

	// 4. Get invite by token preview
	preview, err := store.GetWorkspaceInviteByToken(ctx, rawToken)
	if err != nil {
		t.Fatalf("GetWorkspaceInviteByToken error = %v", err)
	}
	if preview.PublicID != invite.PublicID || preview.WorkspaceName != "Invite Test Space" {
		t.Fatalf("preview mismatch: %+v", preview)
	}

	// Unknown token returns not found
	_, err = store.GetWorkspaceInviteByToken(ctx, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if !errors.Is(err, tenancy.ErrNotFound) {
		t.Fatalf("unknown token error = %v, want ErrNotFound", err)
	}

	// 5. List pending invites for user by email
	userInvites, err := store.ListPendingWorkspaceInvitesForUser(ctx, inviteeID, inviteeEmail)
	if err != nil {
		t.Fatalf("ListPendingWorkspaceInvitesForUser error = %v", err)
	}
	if len(userInvites) != 1 || userInvites[0].PublicID != invite.PublicID {
		t.Fatalf("user invites mismatch: %+v", userInvites)
	}

	// 5b. List pending invites for user by id alone (email unknown/changed):
	// the persisted invitee_user_id branch must hit.
	userInvitesByID, err := store.ListPendingWorkspaceInvitesForUser(ctx, inviteeID, "")
	if err != nil {
		t.Fatalf("ListPendingWorkspaceInvitesForUser by id error = %v", err)
	}
	if len(userInvitesByID) != 1 || userInvitesByID[0].PublicID != invite.PublicID {
		t.Fatalf("user invites by id mismatch: %+v", userInvitesByID)
	}

	// 6. Accept invite
	// Accepting with mismatching email should fail with 404 anti-enumeration
	// (not 403), so existence is not leaked.
	_, _, err = store.AcceptWorkspaceInvite(ctx, invite.PublicID, inviteeID, "wrong@example.com")
	if !errors.Is(err, tenancy.ErrNotFound) {
		t.Fatalf("accept with wrong email error = %v, want ErrNotFound", err)
	}

	// Accepting with matching email succeeds and returns the authoritative
	// post-decision invite (no caller re-fetch).
	member, decided, err := store.AcceptWorkspaceInvite(ctx, rawToken, inviteeID, inviteeEmail)
	if err != nil {
		t.Fatalf("AcceptWorkspaceInvite error = %v", err)
	}
	if member.UserID != inviteeID || member.Role != tenancy.RoleMember || member.WorkspacePublicID != ws.PublicID {
		t.Fatalf("member mismatch: %+v", member)
	}
	if decided == nil || decided.PublicID != invite.PublicID || decided.Status != tenancy.MemberInviteStatusAccepted {
		t.Fatalf("decided invite mismatch: %+v", decided)
	}
	if decided.RespondedByUserID == nil || *decided.RespondedByUserID != inviteeID {
		t.Fatalf("decided invite missing responder: %+v", decided)
	}

	// Verify member is actually in workspace
	role, isMember, err := store.GetWorkspaceRole(ctx, ws.PublicID, inviteeID)
	if err != nil || !isMember || role != tenancy.RoleMember {
		t.Fatalf("expected member role, got member=%v role=%v err=%v", isMember, role, err)
	}

	// Accepting already accepted invite returns conflict
	_, _, err = store.AcceptWorkspaceInvite(ctx, rawToken, inviteeID, inviteeEmail)
	if !errors.Is(err, tenancy.ErrConflict) {
		t.Fatalf("re-accept invite error = %v, want ErrConflict", err)
	}

	// 7. Decline invite lifecycle
	invite2, rawToken2, err := store.CreateWorkspaceMemberInvite(ctx, ws.PublicID, ownerID, "invitee2@example.com", tenancy.RoleAdmin, invitee2ID)
	if err != nil {
		t.Fatalf("CreateWorkspaceMemberInvite 2 error = %v", err)
	}
	declined, err := store.DeclineWorkspaceInvite(ctx, rawToken2, invitee2ID, "invitee2@example.com")
	if err != nil {
		t.Fatalf("DeclineWorkspaceInvite error = %v", err)
	}
	if declined == nil || declined.PublicID != invite2.PublicID || declined.Status != tenancy.MemberInviteStatusDeclined {
		t.Fatalf("declined invite mismatch: %+v", declined)
	}

	// 8. Cancel invite lifecycle
	invite3, _, err := store.CreateWorkspaceMemberInvite(ctx, ws.PublicID, ownerID, "invitee3@example.com", tenancy.RoleMember, 0)
	if err != nil {
		t.Fatalf("CreateWorkspaceMemberInvite 3 error = %v", err)
	}
	err = store.CancelWorkspaceMemberInvite(ctx, ws.PublicID, invite3.PublicID, ownerID)
	if err != nil {
		t.Fatalf("CancelWorkspaceMemberInvite error = %v", err)
	}

	// Cancelled invite cannot be accepted
	_, _, err = store.AcceptWorkspaceInvite(ctx, invite3.PublicID, invitee2ID, "invitee3@example.com")
	if !errors.Is(err, tenancy.ErrConflict) {
		t.Fatalf("accept cancelled invite error = %v, want ErrConflict", err)
	}
	_ = invite2
}

func TestWorkspaceMemberInvitesLimitInvariant(t *testing.T) {
	db := newTenancyTestDB(t)
	store := NewStore(db, nil)
	ctx := context.Background()

	ownerID := createTenancyTestUser(t, db)
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{
		Name:        "Capacity Space",
		OwnerUserID: ownerID,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Currently 1 member (owner). Max is 5.
	// We can create 4 pending invites:
	for i := 1; i <= 4; i++ {
		email := fmt.Sprintf("cap%d@example.com", i)
		_, _, err := store.CreateWorkspaceMemberInvite(ctx, ws.PublicID, ownerID, email, tenancy.RoleMember, 0)
		if err != nil {
			t.Fatalf("invite %d failed: %v", i, err)
		}
	}

	// 5th invite exceeds (active + pending = 1 + 4 = 5 >= 5)
	_, _, err = store.CreateWorkspaceMemberInvite(ctx, ws.PublicID, ownerID, "cap5@example.com", tenancy.RoleMember, 0)
	if !errors.Is(err, tenancy.ErrWorkspaceMemberLimitReached) {
		t.Fatalf("5th invite error = %v, want ErrWorkspaceMemberLimitReached", err)
	}

	// Direct Organization authorization uses this store primitive too; it
	// must not steal a place already reserved by a pending invitation.
	targetID := createTenancyTestUser(t, db)
	if _, err := store.AddWorkspaceMember(ctx, ws.PublicID, targetID, tenancy.RoleMember); !errors.Is(err, tenancy.ErrWorkspaceMemberLimitReached) {
		t.Fatalf("grant beyond reserved capacity = %v", err)
	}

	// If one invite is cancelled, we can create another
	invites, err := store.ListWorkspaceMemberInvites(ctx, ws.PublicID)
	if err != nil || len(invites) < 1 {
		t.Fatal(err)
	}
	if err := store.CancelWorkspaceMemberInvite(ctx, ws.PublicID, invites[0].PublicID, ownerID); err != nil {
		t.Fatal(err)
	}

	_, _, err = store.CreateWorkspaceMemberInvite(ctx, ws.PublicID, ownerID, "cap5@example.com", tenancy.RoleMember, 0)
	if err != nil {
		t.Fatalf("invite after cancel failed: %v", err)
	}
}

func TestWorkspaceInviteAcceptanceChecksOrganizationMembership(t *testing.T) {
	db := newTenancyTestDB(t)
	store := NewStore(db, nil)
	ctx := t.Context()
	owner, target := createTenancyTestUser(t, db), createTenancyTestUser(t, db)
	org, err := store.CreateOrganization(ctx, owner, "organization")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "team", OwnerUserID: owner, OrganizationID: &org.PublicID})
	if err != nil {
		t.Fatal(err)
	}
	inv, _, err := store.CreateWorkspaceMemberInvite(ctx, ws.PublicID, owner, "person@example.com", tenancy.RoleMember, target)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AcceptWorkspaceInvite(ctx, inv.PublicID, target, "person@example.com"); !errors.Is(err, tenancy.ErrNotFound) {
		t.Fatalf("external acceptance = %v", err)
	}
	pending, err := store.GetWorkspaceMemberInvite(ctx, inv.PublicID)
	if err != nil || pending.Status != tenancy.MemberInviteStatusPending {
		t.Fatalf("failed acceptance consumed invite: %+v %v", pending, err)
	}
	if _, err := store.AddOrganizationMember(ctx, org.PublicID, target, tenancy.RoleMember); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AcceptWorkspaceInvite(ctx, inv.PublicID, target, "person@example.com"); err != nil {
		t.Fatal(err)
	}
}

// Hold the scope lock while accept reaches it, then cancel using that same
// transaction. An invite-first accept would prevent cancellation from finishing.
func TestMemberInviteDecisionLocksScopeBeforeInvite(t *testing.T) {
	for _, kind := range []string{"workspace", "organization"} {
		t.Run(kind, func(t *testing.T) {
			db := newTenancyTestDB(t)
			store := NewStore(db, nil)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			owner, target := createTenancyTestUser(t, db), createTenancyTestUser(t, db)
			var scopeID, inviteID string
			var scope postgresInviteTx
			if kind == "workspace" {
				ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "space", OwnerUserID: owner})
				if err != nil {
					t.Fatal(err)
				}
				inv, _, err := store.CreateWorkspaceMemberInvite(ctx, ws.PublicID, owner, "person@example.com", tenancy.RoleMember, target)
				if err != nil {
					t.Fatal(err)
				}
				scopeID, inviteID, scope = ws.PublicID, inv.PublicID, workspaceInviteTx()
			} else {
				org, err := store.CreateOrganization(ctx, owner, "organization")
				if err != nil {
					t.Fatal(err)
				}
				inv, _, err := store.CreateOrganizationMemberInvite(ctx, org.PublicID, owner, "person@example.com", tenancy.RoleMember, target)
				if err != nil {
					t.Fatal(err)
				}
				scopeID, inviteID, scope = org.PublicID, inv.PublicID, organizationInviteTx(store.OrganizationStore)
			}
			blocker, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer blocker.Rollback(context.Background())
			q := dbsqlc.New(blocker)
			if _, err := scope.lockScopeForCancel(ctx, q, scopeID); err != nil {
				t.Fatal(err)
			}
			reachedScope := make(chan struct{})
			checkScope := scope.checkAcceptScope
			scope.checkAcceptScope = func(ctx context.Context, q *dbsqlc.Queries, inv *tenancy.MemberInvite) error {
				close(reachedScope)
				return checkScope(ctx, q, inv)
			}
			done := make(chan error, 1)
			go func() {
				done <- store.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
					_, _, err := acceptMemberInviteTx(ctx, q, scope, inviteID, target, "person@example.com")
					return err
				})
			}()
			select {
			case <-reachedScope:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if err := cancelMemberInviteTx(ctx, q, scope, scopeID, inviteID, owner); err != nil {
				t.Fatalf("cancel while accept waits on scope: %v", err)
			}
			if err := blocker.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if !errors.Is(err, tenancy.ErrConflict) {
					t.Fatalf("accept after cancellation = %v", err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		})
	}
}
