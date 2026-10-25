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
	"github.com/google/uuid"
)

func TestV1OrganizationMemberInvites_FullFlow(t *testing.T) {
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()

	const (
		ownerID     int64 = 101
		inviteeID   int64 = 102
		otherUserID int64 = 103
	)
	dir.ensure(ownerID)
	dir.ensure(inviteeID)
	dir.ensure(otherUserID)

	display := &staticMemberDisplayProvider{
		emails: map[int64]string{
			ownerID:     "owner@example.com",
			inviteeID:   "invitee@example.com",
			otherUserID: "other@example.com",
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
	lookup := &fakeMemberLookup{ids: map[string]uuid.UUID{"invitee@example.com": dir.ensure(inviteeID)}}
	RegisterContractV1Routes(v1Group, runtimehttp.AuthDeps{}, ContractV1Deps{
		Store:            store,
		Users:            dir,
		MemberDisplay:    display,
		InvitationReader: testInvitationReader(dir, display),
		Invitations:      testInvitations(store, nil, spy, NewInvitationIdentityReader(display, lookup, dir)),
		MemberLookup:     lookup,
	})

	org, err := store.CreateOrganization(ctx, ownerID, "Acme Cloud")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Create invite (owner -> invitee)
	status, _, payload := doV1(t, router, v1Call{
		method: http.MethodPost,
		path:   runtimehttp.ContractV1GroupPath + "/organizations/" + org.PublicID + "/invites",
		userID: ownerID,
		body:   `{"email":"invitee@example.com","role":"member"}`,
		headers: map[string]string{
			contract.IdempotencyKeyHeader: "key-org-inv-1",
		},
	})
	if status != http.StatusCreated {
		t.Fatalf("create invite status = %d: %v", status, payload)
	}
	data := v1Data(t, payload).(map[string]any)
	inviteID := data["id"].(string)
	if inviteID == "" {
		t.Fatalf("expected non-empty invite id, got %v", data)
	}
	if len(spy.invited) != 1 || spy.invited[0] != inviteID {
		t.Fatalf("expected spy.invited to have %s, got %v", inviteID, spy.invited)
	}

	// 2. Non-owner non-admin cannot invite
	status, _, _ = doV1(t, router, v1Call{
		method: http.MethodPost,
		path:   runtimehttp.ContractV1GroupPath + "/organizations/" + org.PublicID + "/invites",
		userID: otherUserID,
		body:   `{"email":"another@example.com","role":"member"}`,
		headers: map[string]string{
			contract.IdempotencyKeyHeader: "key-org-inv-2",
		},
	})
	if status != http.StatusNotFound && status != http.StatusForbidden {
		t.Fatalf("expected forbidden or not found for outsider, got %d", status)
	}

	// 3. List organization invites (owner)
	status, _, payload = doV1(t, router, v1Call{
		method: http.MethodGet,
		path:   runtimehttp.ContractV1GroupPath + "/organizations/" + org.PublicID + "/invites",
		userID: ownerID,
	})
	if status != http.StatusOK {
		t.Fatalf("list org invites status = %d: %v", status, payload)
	}
	list := v1Data(t, payload).([]any)
	if len(list) != 1 {
		t.Fatalf("expected 1 invite in org, got %d", len(list))
	}

	// 4. List user organization invites (invitee)
	status, _, payload = doV1(t, router, v1Call{
		method: http.MethodGet,
		path:   runtimehttp.ContractV1GroupPath + "/user/organization-invites",
		userID: inviteeID,
	})
	if status != http.StatusOK {
		t.Fatalf("list user org invites status = %d: %v", status, payload)
	}
	userList := v1Data(t, payload).([]any)
	if len(userList) != 1 {
		t.Fatalf("expected 1 user invite, got %d", len(userList))
	}

	// 5. Accept invite (invitee accepts)
	status, _, payload = doV1(t, router, v1Call{
		method: http.MethodPost,
		path:   runtimehttp.ContractV1GroupPath + "/user/organization-invites/" + inviteID + "/accept",
		userID: inviteeID,
	})
	if status != http.StatusOK {
		t.Fatalf("accept invite status = %d: %v", status, payload)
	}
	memberData := v1Data(t, payload).(map[string]any)
	if memberData["role"] != "member" {
		t.Fatalf("expected role 'member', got %v", memberData["role"])
	}
	if len(spy.accepted) != 1 || spy.accepted[0] != inviteID {
		t.Fatalf("expected spy.accepted to have %s, got %v", inviteID, spy.accepted)
	}

	// Verify membership
	members, err := store.ListOrganizationMembers(ctx, org.PublicID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members in org, got %d", len(members))
	}
}

func TestV1OrganizationMemberInvites_DeclineAndCancel(t *testing.T) {
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()

	const (
		ownerID   int64 = 201
		inviteeID int64 = 202
	)
	dir.ensure(ownerID)
	dir.ensure(inviteeID)

	display := &staticMemberDisplayProvider{
		emails: map[int64]string{
			ownerID:   "owner@example.com",
			inviteeID: "invitee@example.com",
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
	lookup := &fakeMemberLookup{ids: map[string]uuid.UUID{"invitee@example.com": dir.ensure(inviteeID)}}
	RegisterContractV1Routes(v1Group, runtimehttp.AuthDeps{}, ContractV1Deps{
		Store:            store,
		Users:            dir,
		MemberDisplay:    display,
		InvitationReader: testInvitationReader(dir, display),
		Invitations:      testInvitations(store, nil, spy, NewInvitationIdentityReader(display, lookup, dir)),
		MemberLookup:     lookup,
	})

	org, err := store.CreateOrganization(ctx, ownerID, "Cancel Corp")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Create invite
	status, _, payload := doV1(t, router, v1Call{
		method: http.MethodPost,
		path:   runtimehttp.ContractV1GroupPath + "/organizations/" + org.PublicID + "/invites",
		userID: ownerID,
		body:   `{"email":"invitee@example.com","role":"member"}`,
		headers: map[string]string{
			contract.IdempotencyKeyHeader: "key-org-inv-dec-1",
		},
	})
	if status != http.StatusCreated {
		t.Fatalf("create invite status = %d: %v", status, payload)
	}
	data := v1Data(t, payload).(map[string]any)
	inviteID := data["id"].(string)

	// 2. Decline invite
	status, _, payload = doV1(t, router, v1Call{
		method: http.MethodPost,
		path:   runtimehttp.ContractV1GroupPath + "/user/organization-invites/" + inviteID + "/decline",
		userID: inviteeID,
	})
	if status != http.StatusOK {
		t.Fatalf("decline invite status = %d: %v", status, payload)
	}
	if len(spy.declined) != 1 || spy.declined[0] != inviteID {
		t.Fatalf("expected spy.declined to have %s, got %v", inviteID, spy.declined)
	}

	// 3. Create another invite and cancel it
	status, _, payload = doV1(t, router, v1Call{
		method: http.MethodPost,
		path:   runtimehttp.ContractV1GroupPath + "/organizations/" + org.PublicID + "/invites",
		userID: ownerID,
		body:   `{"email":"invitee@example.com","role":"member"}`,
		headers: map[string]string{
			contract.IdempotencyKeyHeader: "key-org-inv-can-2",
		},
	})
	if status != http.StatusCreated {
		t.Fatalf("create invite 2 status = %d: %v", status, payload)
	}
	data2 := v1Data(t, payload).(map[string]any)
	inviteID2 := data2["id"].(string)

	status, _, payload = doV1(t, router, v1Call{
		method: http.MethodDelete,
		path:   runtimehttp.ContractV1GroupPath + "/organizations/" + org.PublicID + "/invites/" + inviteID2,
		userID: ownerID,
	})
	if status != http.StatusOK {
		t.Fatalf("cancel invite status = %d: %v", status, payload)
	}
}
