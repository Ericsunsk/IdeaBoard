package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"ideaboard/api/internal/tenancy"
)

func TestOrganizationInviteRoleGrants(t *testing.T) {
	db := newTenancyTestDB(t)
	store := NewStore(db, nil)
	ctx := t.Context()
	owner, admin, member := createTenancyTestUser(t, db), createTenancyTestUser(t, db), createTenancyTestUser(t, db)
	org, err := store.CreateOrganization(ctx, owner, "invite roles")
	if err != nil {
		t.Fatal(err)
	}
	for userID, role := range map[int64]string{admin: tenancy.RoleAdmin, member: tenancy.RoleMember} {
		if _, err := store.AddOrganizationMember(ctx, org.PublicID, userID, role); err != nil {
			t.Fatal(err)
		}
	}
	for i, tc := range []struct {
		name    string
		actor   int64
		grant   string
		allowed bool
	}{
		{"owner invites member", owner, tenancy.RoleMember, true},
		{"owner invites admin", owner, tenancy.RoleAdmin, true},
		{"admin invites member", admin, tenancy.RoleMember, true},
		{"admin cannot invite admin", admin, tenancy.RoleAdmin, false},
		{"member cannot invite member", member, tenancy.RoleMember, false},
		{"member cannot invite admin", member, tenancy.RoleAdmin, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := store.CreateOrganizationMemberInvite(ctx, org.PublicID, tc.actor, fmt.Sprintf("invite-%d@example.com", i), tc.grant, 0)
			if tc.allowed && err != nil {
				t.Fatal(err)
			}
			if !tc.allowed && !errors.Is(err, tenancy.ErrForbidden) {
				t.Fatalf("invite error = %v, want ErrForbidden", err)
			}
		})
	}
}

func TestOrganizationMemberInvitesPostgresLifecycle(t *testing.T) {
	db := newTenancyTestDB(t)
	store := NewStore(db, nil)
	ctx := context.Background()

	ownerID := createTenancyTestUser(t, db)
	inviteeID := createTenancyTestUser(t, db)
	otherUserID := createTenancyTestUser(t, db)

	// 1. Create Organization
	org, err := store.CreateOrganization(ctx, ownerID, "Acme Corp")
	if err != nil {
		t.Fatal(err)
	}

	// 2. Non-owner non-member cannot invite
	_, _, err = store.CreateOrganizationMemberInvite(ctx, org.PublicID, otherUserID, "new@example.com", tenancy.RoleMember, 0)
	if !errors.Is(err, tenancy.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for non-member, got %v", err)
	}

	// 3. Owner creates invite
	inviteeEmail := "invitee@example.com"
	invite, rawToken, err := store.CreateOrganizationMemberInvite(ctx, org.PublicID, ownerID, inviteeEmail, tenancy.RoleMember, inviteeID)
	if err != nil {
		t.Fatalf("CreateOrganizationMemberInvite error = %v", err)
	}
	if !strings.HasPrefix(invite.PublicID, tenancy.OrganizationInvitePrefix) {
		t.Fatalf("expected osi_ prefix, got %q", invite.PublicID)
	}
	if len(rawToken) != 64 {
		t.Fatalf("expected 64 hex char raw token, got %q", rawToken)
	}
	if invite.Status != tenancy.MemberInviteStatusPending {
		t.Fatalf("expected pending status, got %q", invite.Status)
	}
	if invite.InviteeUserID == nil || *invite.InviteeUserID != inviteeID {
		t.Fatalf("expected persisted invitee_user_id %d, got %+v", inviteeID, invite.InviteeUserID)
	}

	// 4. Duplicate pending invite conflicts
	_, _, err = store.CreateOrganizationMemberInvite(ctx, org.PublicID, ownerID, inviteeEmail, tenancy.RoleMember, inviteeID)
	if !errors.Is(err, tenancy.ErrOrganizationInviteConflict) && !errors.Is(err, tenancy.ErrConflict) {
		t.Fatalf("duplicate invite error = %v, want conflict", err)
	}

	// 5. List organization invites
	invites, err := store.ListOrganizationMemberInvites(ctx, org.PublicID)
	if err != nil {
		t.Fatalf("ListOrganizationMemberInvites error = %v", err)
	}
	if len(invites) != 1 || invites[0].PublicID != invite.PublicID {
		t.Fatalf("unexpected invites list: %+v", invites)
	}

	// 5b. List pending invites for user by id alone (email unknown/changed):
	// the persisted invitee_user_id branch must hit.
	userInvitesByID, err := store.ListPendingOrganizationInvitesForUser(ctx, inviteeID, "")
	if err != nil {
		t.Fatalf("ListPendingOrganizationInvitesForUser by id error = %v", err)
	}
	if len(userInvitesByID) != 1 || userInvitesByID[0].PublicID != invite.PublicID {
		t.Fatalf("user invites by id mismatch: %+v", userInvitesByID)
	}

	// 6. Get by token
	preview, err := store.GetOrganizationInviteByToken(ctx, rawToken)
	if err != nil {
		t.Fatalf("GetOrganizationInviteByToken error = %v", err)
	}
	if preview.PublicID != invite.PublicID || preview.OrganizationName != "Acme Corp" {
		t.Fatalf("preview mismatch: %+v", preview)
	}

	// 7. Cancel invite
	err = store.CancelOrganizationMemberInvite(ctx, org.PublicID, invite.PublicID, ownerID)
	if err != nil {
		t.Fatalf("CancelOrganizationMemberInvite error = %v", err)
	}

	// 8. Re-invite after cancel
	invite2, _, err := store.CreateOrganizationMemberInvite(ctx, org.PublicID, ownerID, inviteeEmail, tenancy.RoleMember, 0)
	if err != nil {
		t.Fatalf("CreateOrganizationMemberInvite after cancel error = %v", err)
	}

	// 9. Accept invite (authoritative post-decision invite, no re-fetch)
	member, decided, err := store.AcceptOrganizationInvite(ctx, invite2.PublicID, inviteeID, inviteeEmail)
	if err != nil {
		t.Fatalf("AcceptOrganizationInvite error = %v", err)
	}
	if member.UserID != inviteeID || member.Role != tenancy.RoleMember {
		t.Fatalf("unexpected member: %+v", member)
	}
	if decided == nil || decided.PublicID != invite2.PublicID || decided.Status != tenancy.MemberInviteStatusAccepted {
		t.Fatalf("decided invite mismatch: %+v", decided)
	}

	// Verify member is in organization
	members, err := store.ListOrganizationMembers(ctx, org.PublicID)
	if err != nil {
		t.Fatalf("ListOrganizationMembers error = %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}

	// 10. Single organization limit check:
	// If invitee is invited to another organization and tries to accept, fails with ErrOrganizationLimitReached
	org2, err := store.CreateOrganization(ctx, otherUserID, "Beta Inc")
	if err != nil {
		t.Fatal(err)
	}
	invite3, _, err := store.CreateOrganizationMemberInvite(ctx, org2.PublicID, otherUserID, inviteeEmail, tenancy.RoleMember, 0)
	if err != nil {
		t.Fatalf("CreateOrganizationMemberInvite on org2 error = %v", err)
	}
	_, _, err = store.AcceptOrganizationInvite(ctx, invite3.PublicID, inviteeID, inviteeEmail)
	if !errors.Is(err, tenancy.ErrOrganizationLimitReached) {
		t.Fatalf("expected ErrOrganizationLimitReached, got %v", err)
	}
}
