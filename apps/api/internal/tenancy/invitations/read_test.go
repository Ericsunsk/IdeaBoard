package invitations

import (
	"context"
	"reflect"
	"testing"
	"time"

	"ideaboard/api/internal/tenancy"
)

func TestInvitationReaderProjectsBothScopesWithOneParticipantBatch(t *testing.T) {
	invitee, responder := int64(2), int64(3)
	created := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	responded := created.Add(time.Hour)
	workspace := &tenancy.WorkspaceMemberInvite{
		PublicID: "wsi_test", WorkspacePublicID: "ws_test", WorkspaceName: "Design",
		InviterUserID: 1, InviteeUserID: &invitee, InviteeEmail: "member@example.com",
		Role: tenancy.RoleMember, Status: tenancy.MemberInviteStatusAccepted,
		CreatedAt: created, ExpiresAt: created.Add(tenancy.MemberInviteTTL),
		RespondedByUserID: &responder, RespondedAt: &responded,
	}
	organization := &tenancy.OrganizationMemberInvite{
		PublicID: "osi_test", OrganizationPublicID: "org_test", OrganizationName: "Studio",
		InviterUserID: 1, InviteeUserID: &invitee, InviteeEmail: "member@example.com",
		Role: tenancy.RoleAdmin, Status: tenancy.MemberInviteStatusPending,
		CreatedAt: created, ExpiresAt: workspace.ExpiresAt,
	}
	calls := 0
	reader := Reader{People: func(ctx context.Context, ids []int64) map[int64]Person {
		calls++
		if ctx != t.Context() || !reflect.DeepEqual(ids, []int64{1, 2, 3}) {
			t.Fatalf("participant batch or context changed: %v", ids)
		}
		return map[int64]Person{
			1: {PublicID: "usr_owner", Name: "Owner", Email: "owner@example.com", Avatar: "/avatar"},
			2: {PublicID: "usr_member"}, 3: {PublicID: "usr_admin"},
		}
	}}
	views := reader.Project(t.Context(), []*tenancy.MemberInvite{
		tenancy.MemberInviteFromWorkspace(workspace), nil, tenancy.MemberInviteFromOrganization(organization),
	})
	if calls != 1 || len(views) != 2 {
		t.Fatalf("one shared enrichment expected: calls=%d views=%+v", calls, views)
	}
	for i, scope := range []struct{ id, name, role string }{{"ws_test", "Design", tenancy.RoleMember}, {"org_test", "Studio", tenancy.RoleAdmin}} {
		view := views[i]
		if view.ScopePublicID != scope.id || view.ScopeName != scope.name || view.Role != scope.role ||
			view.Inviter.Name != "Owner" || view.Inviter.PublicID != "usr_owner" || view.Inviter.Email != "owner@example.com" ||
			view.Inviter.Avatar != "/avatar" || view.InviteeUserID != "usr_member" || view.InviteeEmail != "member@example.com" ||
			!view.CreatedAt.Equal(created) || !view.ExpiresAt.Equal(workspace.ExpiresAt) {
			t.Fatalf("scope projection lost facts: %+v", view)
		}
	}
	if views[0].ID != workspace.PublicID || views[0].Status != workspace.Status || views[0].RespondedByUserID != "usr_admin" ||
		views[0].RespondedAt == workspace.RespondedAt || !views[0].RespondedAt.Equal(responded) || views[1].RespondedAt != nil {
		t.Fatalf("decision projection or time ownership changed: %+v", views)
	}
}

func TestInvitationReaderOmitsMissingPeopleWithoutDroppingInvites(t *testing.T) {
	reader := Reader{People: func(context.Context, []int64) map[int64]Person { return nil }}
	views := reader.Project(t.Context(), []*tenancy.MemberInvite{{PublicID: "wsi_test", InviterUserID: 1, InviteeEmail: "new@example.com"}})
	if len(views) != 1 || views[0].ID != "wsi_test" || views[0].InviteeEmail != "new@example.com" || views[0].Inviter != (Person{}) {
		t.Fatalf("missing optional people must not hide invitation facts: %+v", views)
	}
	if empty := (Reader{}).Project(t.Context(), []*tenancy.MemberInvite{nil}); empty == nil || len(empty) != 0 {
		t.Fatalf("empty projection must remain a list: %+v", empty)
	}
}
