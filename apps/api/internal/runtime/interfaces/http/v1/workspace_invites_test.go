package v1

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

type staticMemberDisplayProvider struct {
	emails  map[int64]string
	avatars map[int64]string
}

func (p *staticMemberDisplayProvider) GetMemberDisplay(ctx context.Context, userIDs []int64) (map[int64]V1MemberDisplay, error) {
	out := make(map[int64]V1MemberDisplay)
	for _, id := range userIDs {
		out[id] = V1MemberDisplay{
			Email:  p.emails[id],
			Avatar: p.avatars[id],
		}
	}
	return out, nil
}

func newV1InvitesEngine(store tenancy.Store, dir *staticUserDirectory, display *staticMemberDisplayProvider) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(runtimehttp.ContractRequestIDMiddleware())
	router.Use(func(c *gin.Context) {
		raw := strings.TrimSpace(c.GetHeader(testsupport.HeaderTestUser))
		if raw == "" {
			c.Next()
			return
		}
		userID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || userID <= 0 {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		role := strings.TrimSpace(c.GetHeader(testsupport.HeaderTestRole))
		if role == "" {
			role = "user"
		}
		id := dir.ensure(userID)
		identityPrincipal, err := principal.New(userID, id,
			fmt.Sprintf("auth-%d", userID), fmt.Sprintf("test-user-%d", userID), role)
		if err != nil || principal.Set(c, identityPrincipal) != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Next()
	})
	v1 := router.Group(runtimehttp.ContractV1GroupPath)
	RegisterContractV1Routes(v1, runtimehttp.AuthDeps{}, ContractV1Deps{
		Store:            store,
		Invitations:      testInvitations(store, nil, nil, NewInvitationIdentityReader(display, nil, dir)),
		Users:            dir,
		MemberDisplay:    display,
		InvitationReader: testInvitationReader(dir, display),
	})
	return router
}

func TestV1WorkspaceMemberInvitesFlow(t *testing.T) {
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()

	const ownerID, adminID, memberID, inviteeID, strangerID int64 = 1, 2, 3, 4, 5
	dir.ensure(ownerID)
	dir.ensure(adminID)
	dir.ensure(memberID)
	dir.ensure(inviteeID)
	dir.ensure(strangerID)

	display := &staticMemberDisplayProvider{
		emails: map[int64]string{
			ownerID:    "owner@example.com",
			adminID:    "admin@example.com",
			memberID:   "member@example.com",
			inviteeID:  "invitee@example.com",
			strangerID: "stranger@example.com",
		},
	}

	engine := newV1InvitesEngine(store, dir, display)

	// Create workspace with owner, admin, member
	ws := v1WorkspaceFixture(t, ctx, store, "collab-space", ownerID)
	if _, err := store.AddWorkspaceMember(ctx, ws.PublicID, adminID, tenancy.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddWorkspaceMember(ctx, ws.PublicID, memberID, tenancy.RoleMember); err != nil {
		t.Fatal(err)
	}

	// 1. Create Workspace Invite
	createInviteCall := func(actorID int64, wsID string, email, role, key string) (int, map[string]any) {
		status, _, payload := doV1(t, engine, v1Call{
			method: http.MethodPost,
			path:   runtimehttp.ContractV1GroupPath + "/workspaces/" + wsID + "/invites",
			userID: actorID,
			body:   fmt.Sprintf(`{"email":%q,"role":%q}`, email, role),
			headers: map[string]string{
				contract.IdempotencyKeyHeader: key,
			},
		})
		return status, payload
	}

	// Member cannot invite (requires Owner or Admin)
	status, payload := createInviteCall(memberID, ws.PublicID, "newbie@example.com", "member", "key-member-1")
	if status != http.StatusForbidden {
		t.Fatalf("member create invite = %d %v, want 403", status, payload)
	}

	// Stranger cannot invite (404 workspace_not_found)
	status, payload = createInviteCall(strangerID, ws.PublicID, "newbie@example.com", "member", "key-stranger-1")
	if status != http.StatusNotFound {
		t.Fatalf("stranger create invite = %d %v, want 404", status, payload)
	}

	// Admin invites invitee@example.com
	status, payload = createInviteCall(adminID, ws.PublicID, "invitee@example.com", "member", "key-admin-1")
	if status != http.StatusCreated {
		t.Fatalf("admin create invite = %d %v, want 201", status, payload)
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object in payload: %v", payload)
	}
	inviteID, _ := data["id"].(string)
	rawToken, _ := data["token"].(string)
	if !strings.HasPrefix(inviteID, tenancy.WorkspaceInvitePrefix) {
		t.Fatalf("invalid invite id: %q", inviteID)
	}
	if len(rawToken) != 64 {
		t.Fatalf("expected 64-char hex raw token, got: %q", rawToken)
	}

	// Idempotency: replay same key+body returns the same invite
	status, payload2 := createInviteCall(adminID, ws.PublicID, "invitee@example.com", "member", "key-admin-1")
	if status != http.StatusCreated {
		t.Fatalf("idempotent replay = %d %v, want 201", status, payload2)
	}
	data2 := payload2["data"].(map[string]any)
	if data2["id"] != inviteID {
		t.Fatalf("idempotent replay returned different invite id: %v vs %v", data2["id"], inviteID)
	}

	// 2. List Workspace Member Invites
	listInvitesCall := func(actorID int64, wsID string) (int, map[string]any) {
		status, _, payload := doV1(t, engine, v1Call{
			method: http.MethodGet,
			path:   runtimehttp.ContractV1GroupPath + "/workspaces/" + wsID + "/invites",
			userID: actorID,
		})
		return status, payload
	}

	status, _ = listInvitesCall(memberID, ws.PublicID)
	if status != http.StatusForbidden {
		t.Fatalf("member list invites = %d, want 403", status)
	}

	status, payload = listInvitesCall(ownerID, ws.PublicID)
	if status != http.StatusOK {
		t.Fatalf("owner list invites = %d %v, want 200", status, payload)
	}
	items := payload["data"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 invite in list, got %d", len(items))
	}
	item := items[0].(map[string]any)
	if item["token"] != nil && item["token"] != "" {
		t.Fatalf("list invites must not expose raw token: %v", item["token"])
	}

	// 3. Public Token Preview (no auth required)
	previewCall := func(token string) (int, map[string]any) {
		status, _, payload := doV1(t, engine, v1Call{
			method: http.MethodGet,
			path:   runtimehttp.ContractV1GroupPath + "/invites/" + token,
			userID: 0,
		})
		return status, payload
	}

	status, _ = previewCall("unknown-token")
	if status != http.StatusNotFound {
		t.Fatalf("unknown token preview = %d, want 404", status)
	}

	status, payload = previewCall(rawToken)
	if status != http.StatusOK {
		t.Fatalf("valid token preview = %d %v, want 200", status, payload)
	}
	previewData := payload["data"].(map[string]any)
	if previewData["workspace_id"] != ws.PublicID || previewData["workspace_name"] != "collab-space" {
		t.Fatalf("preview mismatch: %v", previewData)
	}
	// PII minimization: public preview must not expose the invitee email.
	if _, present := previewData["invitee_email"]; present {
		t.Fatalf("preview must not expose invitee_email: %v", previewData)
	}

	// 4. In-App User Invites List
	userInvitesCall := func(actorID int64) (int, map[string]any) {
		status, _, payload := doV1(t, engine, v1Call{
			method: http.MethodGet,
			path:   runtimehttp.ContractV1GroupPath + "/user/invites",
			userID: actorID,
		})
		return status, payload
	}

	status, payload = userInvitesCall(inviteeID)
	if status != http.StatusOK {
		t.Fatalf("user invites = %d %v, want 200", status, payload)
	}
	userItems := payload["data"].([]any)
	if len(userItems) != 1 {
		t.Fatalf("expected 1 user invite, got %d", len(userItems))
	}

	// 5. In-App Accept
	acceptUserCall := func(actorID int64, invID string) (int, map[string]any) {
		status, _, payload := doV1(t, engine, v1Call{
			method: http.MethodPost,
			path:   runtimehttp.ContractV1GroupPath + "/user/invites/" + invID + "/accept",
			userID: actorID,
		})
		return status, payload
	}

	// Stranger cannot accept invite addressed to invitee@example.com.
	// Anti-enumeration: email/user mismatch converges to 404 (not 403) so
	// existence is not leaked; see store Accept* comments.
	status, _ = acceptUserCall(strangerID, inviteID)
	if status != http.StatusNotFound {
		t.Fatalf("stranger accept = %d, want 404", status)
	}

	// Invitee accepts
	status, payload = acceptUserCall(inviteeID, inviteID)
	if status != http.StatusOK {
		t.Fatalf("invitee accept = %d %v, want 200", status, payload)
	}
	memberData := payload["data"].(map[string]any)
	if memberData["user_id"] != tenancy.FormatUserPublicID(dir.ensure(inviteeID)) {
		t.Fatalf("membership user id mismatch: %v", memberData)
	}

	// Workspace role is now confirmed
	role, isMember, err := store.GetWorkspaceRole(ctx, ws.PublicID, inviteeID)
	if err != nil || !isMember || role != tenancy.RoleMember {
		t.Fatalf("expected confirmed member, got member=%v role=%v err=%v", isMember, role, err)
	}

	// Accepting again fails with conflict
	status, _ = acceptUserCall(inviteeID, inviteID)
	if status != http.StatusConflict {
		t.Fatalf("re-accept = %d, want 409", status)
	}
}

func TestV1WorkspaceMemberInvitesTokenAcceptAndDecline(t *testing.T) {
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()

	const ownerID, targetID, target2ID int64 = 1, 2, 3
	dir.ensure(ownerID)
	dir.ensure(targetID)
	dir.ensure(target2ID)

	display := &staticMemberDisplayProvider{
		emails: map[int64]string{
			ownerID:   "owner@example.com",
			targetID:  "target@example.com",
			target2ID: "target2@example.com",
		},
	}

	engine := newV1InvitesEngine(store, dir, display)
	ws := v1WorkspaceFixture(t, ctx, store, "token-space", ownerID)

	// Create invite for target
	inv1, token1, err := store.CreateWorkspaceMemberInvite(ctx, ws.PublicID, ownerID, "target@example.com", tenancy.RoleAdmin, 0)
	if err != nil {
		t.Fatal(err)
	}
	_ = inv1

	// Decline by token
	status, _, payload := doV1(t, engine, v1Call{
		method: http.MethodPost,
		path:   runtimehttp.ContractV1GroupPath + "/invites/" + token1 + "/decline",
		userID: targetID,
	})
	if status != http.StatusOK {
		t.Fatalf("decline by token = %d %v, want 200", status, payload)
	}
	ack := payload["data"].(map[string]any)
	if ack["declined"] != true {
		t.Fatalf("expected declined=true, got %v", ack)
	}

	// Create invite for target2
	_, token2, err := store.CreateWorkspaceMemberInvite(ctx, ws.PublicID, ownerID, "target2@example.com", tenancy.RoleMember, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Accept by token
	status, _, payload = doV1(t, engine, v1Call{
		method: http.MethodPost,
		path:   runtimehttp.ContractV1GroupPath + "/invites/" + token2 + "/accept",
		userID: target2ID,
	})
	if status != http.StatusOK {
		t.Fatalf("accept by token = %d %v, want 200", status, payload)
	}
	role, isMember, _ := store.GetWorkspaceRole(ctx, ws.PublicID, target2ID)
	if !isMember || role != tenancy.RoleMember {
		t.Fatalf("expected member role, got %v %v", isMember, role)
	}
}

func TestV1WorkspaceMemberInvitesCancelAndLimit(t *testing.T) {
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()

	const ownerID int64 = 1
	dir.ensure(ownerID)
	display := &staticMemberDisplayProvider{
		emails: map[int64]string{ownerID: "owner@example.com"},
	}

	engine := newV1InvitesEngine(store, dir, display)
	ws := v1WorkspaceFixture(t, ctx, store, "limit-space", ownerID)

	// Default Workspace Protected
	def, err := store.EnsureDefaultWorkspace(ctx, ownerID)
	if err != nil {
		t.Fatal(err)
	}
	status, _, payload := doV1(t, engine, v1Call{
		method: http.MethodPost,
		path:   runtimehttp.ContractV1GroupPath + "/workspaces/" + def.PublicID + "/invites",
		userID: ownerID,
		body:   `{"email":"friend@example.com","role":"member"}`,
		headers: map[string]string{
			contract.IdempotencyKeyHeader: "key-def-1",
		},
	})
	if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceDefaultProtected) {
		t.Fatalf("default workspace invite = %d %v, want 403 workspace_default_protected", status, payload)
	}

	// Invariant: active_members + pending_invites <= 5
	// Currently 1 active member (owner). We can add up to 4 invites.
	for i := 2; i <= 5; i++ {
		email := fmt.Sprintf("user%d@example.com", i)
		st, _, pl := doV1(t, engine, v1Call{
			method: http.MethodPost,
			path:   runtimehttp.ContractV1GroupPath + "/workspaces/" + ws.PublicID + "/invites",
			userID: ownerID,
			body:   fmt.Sprintf(`{"email":%q,"role":"member"}`, email),
			headers: map[string]string{
				contract.IdempotencyKeyHeader: fmt.Sprintf("key-fill-%d", i),
			},
		})
		if st != http.StatusCreated {
			t.Fatalf("invite %d failed: %d %v", i, st, pl)
		}
	}

	// 5th invite (which would be 6th collaborator) must fail with 409 workspace_member_limit_reached
	status, _, payload = doV1(t, engine, v1Call{
		method: http.MethodPost,
		path:   runtimehttp.ContractV1GroupPath + "/workspaces/" + ws.PublicID + "/invites",
		userID: ownerID,
		body:   `{"email":"user6@example.com","role":"member"}`,
		headers: map[string]string{
			contract.IdempotencyKeyHeader: "key-fill-6",
		},
	})
	if status != http.StatusConflict || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceMemberLimitReached) {
		t.Fatalf("6th collaborator invite = %d %v, want 409 workspace_member_limit_reached", status, payload)
	}

	// Cancel one invite
	invites, err := store.ListWorkspaceMemberInvites(ctx, ws.PublicID)
	if err != nil || len(invites) == 0 {
		t.Fatal(err)
	}
	invToCancel := invites[0]

	status, _, payload = doV1(t, engine, v1Call{
		method: http.MethodDelete,
		path:   runtimehttp.ContractV1GroupPath + "/workspaces/" + ws.PublicID + "/invites/" + invToCancel.PublicID,
		userID: ownerID,
	})
	if status != http.StatusOK {
		t.Fatalf("cancel invite = %d %v, want 200", status, payload)
	}

	// Now that one pending invite is cancelled, we can invite user6@example.com!
	status, _, payload = doV1(t, engine, v1Call{
		method: http.MethodPost,
		path:   runtimehttp.ContractV1GroupPath + "/workspaces/" + ws.PublicID + "/invites",
		userID: ownerID,
		body:   `{"email":"user6@example.com","role":"member"}`,
		headers: map[string]string{
			contract.IdempotencyKeyHeader: "key-fill-6-retry",
		},
	})
	if status != http.StatusCreated {
		t.Fatalf("invite after cancel = %d %v, want 201", status, payload)
	}
}

type spyCollaborationDispatcher struct {
	invited   []string
	accepted  []string
	declined  []string
	transfers []string
}

func (s *spyCollaborationDispatcher) OnWorkspaceMemberInvited(ctx context.Context, invite *tenancy.WorkspaceMemberInvite, rawToken string) error {
	s.invited = append(s.invited, invite.PublicID)
	return nil
}

func (s *spyCollaborationDispatcher) OnWorkspaceMemberInviteAccepted(ctx context.Context, invite *tenancy.WorkspaceMemberInvite, member *tenancy.WorkspaceMember, recipientEmail string) error {
	s.accepted = append(s.accepted, invite.PublicID)
	return nil
}

func (s *spyCollaborationDispatcher) OnWorkspaceMemberInviteDeclined(ctx context.Context, invite *tenancy.WorkspaceMemberInvite) error {
	s.declined = append(s.declined, invite.PublicID)
	return nil
}

func (s *spyCollaborationDispatcher) OnMemberInviteCancelled(ctx context.Context, inviteID string) error {
	s.declined = append(s.declined, inviteID)
	return nil
}

func (s *spyCollaborationDispatcher) OnOrganizationMemberInvited(ctx context.Context, invite *tenancy.OrganizationMemberInvite) error {
	s.invited = append(s.invited, invite.PublicID)
	return nil
}

func (s *spyCollaborationDispatcher) OnOrganizationMemberInviteAccepted(ctx context.Context, invite *tenancy.OrganizationMemberInvite, member *tenancy.OrganizationMember, recipientEmail string) error {
	s.accepted = append(s.accepted, invite.PublicID)
	return nil
}

func (s *spyCollaborationDispatcher) OnOrganizationMemberInviteDeclined(ctx context.Context, invite *tenancy.OrganizationMemberInvite) error {
	s.declined = append(s.declined, invite.PublicID)
	return nil
}

func (s *spyCollaborationDispatcher) OnBillingTransferInitiated(ctx context.Context, transfer *tenancy.BillingTransferInvite, params tenancy.BillingTransferInitiatedParams) error {
	s.transfers = append(s.transfers, transfer.PublicID)
	return nil
}

func TestV1WorkspaceMemberInvites_CollaborationDispatcher(t *testing.T) {
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()

	const ownerID, invitee1ID, invitee2ID int64 = 10, 20, 30
	dir.ensure(ownerID)
	dir.ensure(invitee1ID)
	dir.ensure(invitee2ID)

	display := &staticMemberDisplayProvider{
		emails: map[int64]string{
			ownerID:    "owner@example.com",
			invitee1ID: "invitee1@example.com",
			invitee2ID: "invitee2@example.com",
		},
	}

	spy := &spyCollaborationDispatcher{}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(runtimehttp.ContractRequestIDMiddleware())
	router.Use(func(c *gin.Context) {
		raw := strings.TrimSpace(c.GetHeader(testsupport.HeaderTestUser))
		if raw == "" {
			c.Next()
			return
		}
		userID, _ := strconv.ParseInt(raw, 10, 64)
		id := dir.ensure(userID)
		identityPrincipal, _ := principal.New(userID, id, fmt.Sprintf("auth-%d", userID), fmt.Sprintf("user-%d", userID), "user")
		_ = principal.Set(c, identityPrincipal)
		c.Next()
	})
	v1Group := router.Group(runtimehttp.ContractV1GroupPath)
	RegisterContractV1Routes(v1Group, runtimehttp.AuthDeps{}, ContractV1Deps{
		Store:            store,
		Users:            dir,
		MemberDisplay:    display,
		InvitationReader: testInvitationReader(dir, display),
		Invitations:      testInvitations(store, nil, spy, NewInvitationIdentityReader(display, nil, dir)),
	})

	ws := v1WorkspaceFixture(t, ctx, store, "events-space", ownerID)

	// 1. Create invite -> triggers OnWorkspaceMemberInvited
	status, _, payload := doV1(t, router, v1Call{
		method: http.MethodPost,
		path:   runtimehttp.ContractV1GroupPath + "/workspaces/" + ws.PublicID + "/invites",
		userID: ownerID,
		body:   `{"email":"invitee1@example.com","role":"member"}`,
		headers: map[string]string{
			contract.IdempotencyKeyHeader: "key-evt-1",
		},
	})
	if status != http.StatusCreated {
		t.Fatalf("invite 1 status = %d: %v", status, payload)
	}
	data := v1Data(t, payload).(map[string]any)
	token1 := data["token"].(string)
	invite1ID := data["id"].(string)

	if len(spy.invited) != 1 || spy.invited[0] != invite1ID {
		t.Fatalf("expected spy to record invited %s, got %v", invite1ID, spy.invited)
	}

	// 2. Accept invite 1 -> triggers OnWorkspaceMemberInviteAccepted
	status, _, _ = doV1(t, router, v1Call{
		method: http.MethodPost,
		path:   runtimehttp.ContractV1GroupPath + "/invites/" + token1 + "/accept",
		userID: invitee1ID,
	})
	if status != http.StatusOK {
		t.Fatalf("accept 1 status = %d", status)
	}
	if len(spy.accepted) != 1 || spy.accepted[0] != invite1ID {
		t.Fatalf("expected spy to record accepted %s, got %v", invite1ID, spy.accepted)
	}

	// 3. Create invite 2 and decline -> triggers OnWorkspaceMemberInviteDeclined
	status, _, payload = doV1(t, router, v1Call{
		method: http.MethodPost,
		path:   runtimehttp.ContractV1GroupPath + "/workspaces/" + ws.PublicID + "/invites",
		userID: ownerID,
		body:   `{"email":"invitee2@example.com","role":"member"}`,
		headers: map[string]string{
			contract.IdempotencyKeyHeader: "key-evt-2",
		},
	})
	if status != http.StatusCreated {
		t.Fatalf("invite 2 status = %d: %v", status, payload)
	}
	data2 := v1Data(t, payload).(map[string]any)
	token2 := data2["token"].(string)
	invite2ID := data2["id"].(string)

	status, _, _ = doV1(t, router, v1Call{
		method: http.MethodPost,
		path:   runtimehttp.ContractV1GroupPath + "/invites/" + token2 + "/decline",
		userID: invitee2ID,
	})
	if status != http.StatusOK {
		t.Fatalf("decline status = %d", status)
	}
	if len(spy.declined) != 1 || spy.declined[0] != invite2ID {
		t.Fatalf("expected spy to record declined %s, got %v", invite2ID, spy.declined)
	}
}

func TestWorkspaceInviteDecisionRejectsMissingRecipientFacts(t *testing.T) {
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	dir.ensure(1)
	dir.ensure(2)
	ws := v1WorkspaceFixture(t, ctx, store, "recipient facts", 1)
	inv, token, err := store.CreateWorkspaceMemberInvite(ctx, ws.PublicID, 1, "intended@example.com", tenancy.RoleMember, 0)
	if err != nil {
		t.Fatal(err)
	}
	engine := newV1InvitesEngine(store, dir, &staticMemberDisplayProvider{emails: map[int64]string{}})
	for _, action := range []string{"accept", "decline"} {
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodPost, path: runtimehttp.ContractV1GroupPath + "/invites/" + token + "/" + action, userID: 2})
		if status != http.StatusNotFound {
			t.Fatalf("%s without recipient facts = %d %v", action, status, payload)
		}
	}
	pending, err := store.GetWorkspaceMemberInvite(ctx, inv.PublicID)
	if err != nil || pending.Status != tenancy.MemberInviteStatusPending {
		t.Fatalf("unauthorized decision mutated invitation: %+v %v", pending, err)
	}
}
