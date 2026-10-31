package tenancy

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"ideaboard/api/internal/identity/authidentity"
	"ideaboard/api/internal/identity/users/domain"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/tenancy/invitations"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type invitationUsers struct {
	*stubUserRepo
	calls [][]int64
}

func (r *invitationUsers) GetByIDs(ctx context.Context, ids []int64) (map[int64]*domain.User, error) {
	r.calls = append(r.calls, append([]int64(nil), ids...))
	return r.stubUserRepo.GetByIDs(ctx, ids)
}

func TestInvitationReadAdapterKeepsLocalFactsWhenAuthFails(t *testing.T) {
	publicID := uuid.New()
	for _, authUnavailable := range []bool{false, true} {
		t.Run(map[bool]string{false: "display available", true: "Auth unavailable"}[authUnavailable], func(t *testing.T) {
			users := &invitationUsers{stubUserRepo: &stubUserRepo{users: map[int64]*domain.User{
				1: {ID: 1, PublicID: publicID, DisplayName: "Local Owner", AuthUserID: strPtr("auth-owner")},
				2: nil,
			}}}
			emails := &stubEmailReader{identities: map[string]authidentity.UserIdentity{"auth-owner": {Email: "owner@example.com"}}}
			if authUnavailable {
				emails.err = errors.New("Auth unavailable")
			}
			reader := NewInvitationReader(users, emails)
			views := reader.Project(t.Context(), []*tenancy.MemberInvite{
				{PublicID: "wsi_test", InviterUserID: 1}, {PublicID: "osi_test", InviterUserID: 1}, {PublicID: "wsi_missing", InviterUserID: 2},
			})
			if len(users.calls) != 1 || !reflect.DeepEqual(users.calls[0], []int64{1, 2}) || len(emails.calls) != 1 {
				t.Fatalf("enrichment must batch local and Auth reads: users=%v Auth=%v", users.calls, emails.calls)
			}
			wantEmail := "owner@example.com"
			if authUnavailable {
				wantEmail = ""
			}
			if len(views) != 3 || views[0].Inviter.PublicID != tenancy.FormatUserPublicID(publicID) ||
				views[0].Inviter.Name != "Local Owner" || views[0].Inviter.Email != wantEmail || views[2].Inviter != (invitations.Person{}) {
				t.Fatalf("optional Auth failure discarded local facts: %+v", views)
			}
			users.err = errors.New("local read unavailable")
			if got := reader.Project(t.Context(), []*tenancy.MemberInvite{{PublicID: "wsi_test", InviterUserID: 1}}); len(got) != 1 || got[0].Inviter != (invitations.Person{}) {
				t.Fatalf("invitation display failure must omit only optional facts: %+v", got)
			}
		})
	}
}

func TestInvitationPublicPreviewUsesSharedProjectionWithoutRecipientPII(t *testing.T) {
	store := tenancy.NewInMemoryStoreForTests()
	workspace, err := store.CreateWorkspace(t.Context(), tenancy.CreateWorkspaceParams{Name: "Design", OwnerUserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := store.CreateWorkspaceMemberInvite(t.Context(), workspace.PublicID, 1, "private@example.com", tenancy.RoleMember, 0)
	if err != nil {
		t.Fatal(err)
	}
	reader := invitations.Reader{People: func(context.Context, []int64) map[int64]invitations.Person {
		return map[int64]invitations.Person{1: {PublicID: "usr_owner", Name: "Owner", Email: "owner@example.com", Avatar: "/avatar"}}
	}}
	h := contractV1Handler{deps: ContractV1Deps{Store: store, InvitationReader: reader}}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(runtimehttp.ContractRequestIDMiddleware())
	engine.GET(runtimehttp.ContractV1GroupPath+"/invites/:token", h.getWorkspaceInviteByToken)
	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: runtimehttp.ContractV1GroupPath + "/invites/" + token})
	if status != http.StatusOK {
		t.Fatalf("preview failed: %d %+v", status, payload)
	}
	preview := payload["data"].(map[string]any)
	if preview["workspace_id"] != workspace.PublicID || preview["inviter_name"] != "Owner" || preview["inviter_avatar"] != "/avatar" {
		t.Fatalf("preview lost optional display: %+v", preview)
	}
	for _, key := range []string{"invitee_email", "invitee_user_id", "inviter_email", "token"} {
		if _, exists := preview[key]; exists {
			t.Fatalf("public preview exposed private fact %q", key)
		}
	}
}
