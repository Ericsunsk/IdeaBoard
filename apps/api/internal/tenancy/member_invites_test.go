package tenancy

import (
	"context"
	"errors"
	"testing"
)

func TestMemberInviteRecipientRequiresTrustedAttribution(t *testing.T) {
	bound := int64(7)
	for _, tc := range []struct {
		name    string
		bound   *int64
		userID  int64
		email   string
		allowed bool
	}{
		{"missing both", nil, 8, "", false},
		{"email matches", nil, 8, " Person@Example.com ", true},
		{"email differs", nil, 8, "other@example.com", false},
		{"bound id matches without email", &bound, 7, "", true},
		{"bound id differs without email", &bound, 8, "", false},
		{"matching email cannot override bound id", &bound, 8, "person@example.com", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckMemberInviteRecipient("person@example.com", tc.bound, tc.userID, tc.email)
			if tc.allowed && err != nil {
				t.Fatal(err)
			}
			if !tc.allowed && !errors.Is(err, ErrNotFound) {
				t.Fatalf("got %v, want ErrNotFound", err)
			}
		})
	}
}

func TestWorkspaceInviteAcceptRequiresCurrentOrganizationMembership(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStoreForTests()
	org, err := s.CreateOrganization(ctx, 1, "organization")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := s.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "team", OwnerUserID: 1, OrganizationID: &org.PublicID})
	if err != nil {
		t.Fatal(err)
	}
	inv, _, err := s.CreateWorkspaceMemberInvite(ctx, ws.PublicID, 1, "person@example.com", RoleMember, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AcceptWorkspaceInvite(ctx, inv.PublicID, 2, "person@example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("external acceptance = %v", err)
	}
	current, err := s.GetWorkspaceMemberInvite(ctx, inv.PublicID)
	if err != nil || current.Status != MemberInviteStatusPending {
		t.Fatalf("failed accept consumed invitation: %+v %v", current, err)
	}
	if _, err := s.AddOrganizationMember(ctx, org.PublicID, 2, RoleMember); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AcceptWorkspaceInvite(ctx, inv.PublicID, 2, "person@example.com"); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceGrantReservesPendingInvitationCapacity(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStoreForTests()
	org, err := s.CreateOrganization(ctx, 1, "organization")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := s.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "team", OwnerUserID: 1, OrganizationID: &org.PublicID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddOrganizationMember(ctx, org.PublicID, 2, RoleMember); err != nil {
		t.Fatal(err)
	}
	for _, email := range []string{"a@example.com", "b@example.com", "c@example.com", "d@example.com"} {
		if _, _, err := s.CreateWorkspaceMemberInvite(ctx, ws.PublicID, 1, email, RoleMember, 0); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.AddWorkspaceMember(ctx, ws.PublicID, 2, RoleMember); !errors.Is(err, ErrWorkspaceMemberLimitReached) {
		t.Fatalf("grant = %v", err)
	}
}
