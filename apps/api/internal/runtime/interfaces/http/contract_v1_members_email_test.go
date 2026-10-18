package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/postgres"
	"ideaboard/api/internal/testdb"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// fakeMemberEmails is a deterministic V1MemberEmailProvider for tests: known
// numeric ids resolve to the configured email, unknown ids are omitted, and
// failNext forces a provider error to prove the list fails closed.
type fakeMemberEmails struct {
	emails   map[int64]string
	failNext error
}

func (f *fakeMemberEmails) GetMemberEmails(_ context.Context, userIDs []int64) (map[int64]string, error) {
	if f.failNext != nil {
		err := f.failNext
		f.failNext = nil
		return nil, err
	}
	out := make(map[int64]string, len(userIDs))
	for _, id := range userIDs {
		if email := strings.TrimSpace(f.emails[id]); email != "" {
			out[id] = email
		}
	}
	return out, nil
}

func newV1MembersEmailEngine(store tenancy.Store, dir *staticUserDirectory, emails V1MemberEmailProvider) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ContractRequestIDMiddleware())
	router.Use(func(c *gin.Context) {
		raw := strings.TrimSpace(c.GetHeader("X-Test-User"))
		if raw == "" {
			c.Next()
			return
		}
		userID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || userID <= 0 {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		role := strings.TrimSpace(c.GetHeader("X-Test-Role"))
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
	v1 := router.Group(ContractV1GroupPath)
	RegisterContractV1Routes(v1, AuthDeps{}, ContractV1Deps{Store: store, Users: dir, MemberEmails: emails})
	return router
}

func memberListEmails(t *testing.T, payload map[string]any) map[string]string {
	t.Helper()
	items, ok := v1Data(t, payload).([]any)
	if !ok {
		t.Fatalf("member list data is not an array: %v", payload)
	}
	out := make(map[string]string, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("member item is not an object: %v", raw)
		}
		userID, _ := item["user_id"].(string)
		if userID == "" {
			t.Fatalf("member item misses user_id: %v", item)
		}
		email, _ := item["email"].(string)
		out[userID] = email
	}
	return out
}

func TestV1MemberListEmailsInMemory(t *testing.T) {
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	const owner, member, outsider = 1, 2, 3
	ws := v1WorkspaceFixture(t, ctx, store, "member-emails", owner)
	store.SeedWorkspaceMember(ws.PublicID, member, tenancy.RoleMember)
	for _, id := range []int64{owner, member, outsider} {
		dir.ensure(id)
	}
	usr := func(userID int64) string { return tenancy.FormatUserPublicID(dir.ensure(userID)) }
	emails := &fakeMemberEmails{emails: map[int64]string{owner: "owner@example.com"}}
	engine := newV1MembersEmailEngine(store, dir, emails)
	membersPath := "/api/v1/workspaces/" + ws.PublicID + "/members"

	t.Run("member with email carries it, member without omits it", func(t *testing.T) {
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: membersPath, userID: owner})
		if status != http.StatusOK {
			t.Fatalf("list = %d %v, want 200", status, payload)
		}
		got := memberListEmails(t, payload)
		if got[usr(owner)] != "owner@example.com" {
			t.Fatalf("owner email = %q, want owner@example.com (%v)", got[usr(owner)], payload)
		}
		if got[usr(member)] != "" {
			t.Fatalf("member without email must omit the field, got %q (%v)", got[usr(member)], payload)
		}
		// The raw JSON must omit the key (omitempty), not send an empty string.
		rawItems := v1Data(t, payload).([]any)
		for _, raw := range rawItems {
			item := raw.(map[string]any)
			if item["user_id"] == usr(member) {
				if _, ok := item["email"]; ok {
					t.Fatalf("member without email must not carry the key: %v", item)
				}
			}
		}
	})

	t.Run("provider error fails closed to no email with 200", func(t *testing.T) {
		emails.failNext = errors.New("auth down")
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: membersPath, userID: member})
		if status != http.StatusOK {
			t.Fatalf("list with failing emails = %d %v, want 200", status, payload)
		}
		got := memberListEmails(t, payload)
		if got[usr(owner)] != "" || got[usr(member)] != "" {
			t.Fatalf("failing provider must omit all emails: %v", payload)
		}
	})

	t.Run("nil provider fails closed to no email with 200", func(t *testing.T) {
		plain := newV1TestEngine(store, dir)
		status, _, payload := doV1(t, plain, v1Call{method: http.MethodGet, path: membersPath, userID: owner})
		if status != http.StatusOK {
			t.Fatalf("list without provider = %d %v, want 200", status, payload)
		}
		got := memberListEmails(t, payload)
		if got[usr(owner)] != "" {
			t.Fatalf("nil provider must omit emails: %v", payload)
		}
	})

	t.Run("non-member stays invisible without email leak", func(t *testing.T) {
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: membersPath, userID: outsider})
		if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
			t.Fatalf("outsider list = %d %v, want 404 workspace_not_found", status, payload)
		}
		if strings.Contains(fmt.Sprintf("%v", payload), "owner@example.com") {
			t.Fatalf("outsider response must not leak emails: %v", payload)
		}
	})

	t.Run("member writes stay usr_-only and unknown users stay 404", func(t *testing.T) {
		// Add carries no email even though the provider knows the owner.
		status, _, payload := doV1(t, engine, v1Call{
			method: http.MethodPost, path: membersPath, userID: owner,
			body:    fmt.Sprintf(`{"data":{"user_id":%q,"role":"member"}}`, usr(member)),
			headers: map[string]string{contract.IdempotencyKeyHeader: "member-email-add"},
		})
		if status != http.StatusCreated && status != http.StatusOK {
			// Re-adding an existing member keeps the role with 201; either
			// way the shape must stay usr_-only.
			t.Fatalf("add = %d %v", status, payload)
		}
		if data, ok := v1Data(t, payload).(map[string]any); ok {
			if _, hasEmail := data["email"]; hasEmail {
				t.Fatalf("member write must not carry email: %v", data)
			}
		}
		// Unknown users stay invisible (404), never 403, on writes.
		randomUnknown := tenancy.FormatUserPublicID(uuid.New())
		status, _, payload = doV1(t, engine, v1Call{
			method: http.MethodPost, path: membersPath, userID: owner,
			body:    fmt.Sprintf(`{"data":{"user_id":%q,"role":"member"}}`, randomUnknown),
			headers: map[string]string{contract.IdempotencyKeyHeader: "member-email-unknown"},
		})
		if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceMemberNotFound) {
			t.Fatalf("unknown add = %d %v, want 404 member not found", status, payload)
		}
	})
}

func TestV1MemberListEmailsPG(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	tstore := tenancypg.NewStore(db)
	dir := newStaticUserDirectory()
	const owner, member, outsider = 1, 10, 8
	for _, id := range []int64{owner, member, outsider} {
		dir.ensure(id)
	}
	ws, err := tstore.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "member-emails-pg", OwnerUserID: owner})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tstore.AddWorkspaceMember(ctx, ws.PublicID, member, tenancy.RoleMember); err != nil {
		t.Fatal(err)
	}
	usr := func(userID int64) string { return tenancy.FormatUserPublicID(dir.ensure(userID)) }
	emails := &fakeMemberEmails{emails: map[int64]string{owner: "pg-owner@example.com"}}
	engine := newV1MembersEmailEngine(tstore, dir, emails)
	membersPath := "/api/v1/workspaces/" + ws.PublicID + "/members"

	t.Run("pg member with email is listed, member without email omits it", func(t *testing.T) {
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: membersPath, userID: owner})
		if status != http.StatusOK {
			t.Fatalf("pg list = %d %v, want 200", status, payload)
		}
		got := memberListEmails(t, payload)
		if got[usr(owner)] != "pg-owner@example.com" {
			t.Fatalf("pg owner email = %q, want pg-owner@example.com (%v)", got[usr(owner)], payload)
		}
		if got[usr(member)] != "" {
			t.Fatalf("pg member without email must omit it, got %q (%v)", got[usr(member)], payload)
		}
	})

	t.Run("pg provider error fails closed", func(t *testing.T) {
		emails.failNext = errors.New("auth down")
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: membersPath, userID: member})
		if status != http.StatusOK {
			t.Fatalf("pg list with failing emails = %d %v, want 200", status, payload)
		}
	})

	t.Run("pg non-member stays invisible", func(t *testing.T) {
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: membersPath, userID: outsider})
		if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
			t.Fatalf("pg outsider list = %d %v, want 404 workspace_not_found", status, payload)
		}
	})
}
