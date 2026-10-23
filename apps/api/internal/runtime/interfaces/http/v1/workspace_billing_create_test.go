package v1

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

// spyWorkspaceStore records creation params while delegating to the
// in-memory store (which carries no billing state).
type spyWorkspaceStore struct {
	*tenancy.InMemoryStore
	mu       sync.Mutex
	params   []tenancy.CreateWorkspaceParams
	orgCalls int
}

func (s *spyWorkspaceStore) CreateWorkspace(ctx context.Context, params tenancy.CreateWorkspaceParams) (*tenancy.Workspace, error) {
	s.mu.Lock()
	s.params = append(s.params, params)
	s.mu.Unlock()
	return s.InMemoryStore.CreateWorkspace(ctx, params)
}

func (s *spyWorkspaceStore) CreateOrganizationWorkspace(ctx context.Context, organizationPublicID string, ownerUserID int64, name string) (*tenancy.Workspace, error) {
	s.mu.Lock()
	s.orgCalls++
	s.mu.Unlock()
	return s.InMemoryStore.CreateOrganizationWorkspace(ctx, organizationPublicID, ownerUserID, name)
}

func (s *spyWorkspaceStore) lastParams() tenancy.CreateWorkspaceParams {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.params) == 0 {
		return tenancy.CreateWorkspaceParams{}
	}
	return s.params[len(s.params)-1]
}

// stubBillingAccounts serves the fixed Personal survivor for every workspace:
// creation scope alone decides the payer, so handler plumbing (not
// resolution) is under test. Resolution itself is pinned by the postgres
// workspace_create_billing suite.
type stubBillingAccounts struct {
	mu       sync.Mutex
	personal tenancypg.BillingAccount
}

func newStubBillingAccounts() *stubBillingAccounts {
	return &stubBillingAccounts{personal: tenancypg.BillingAccount{
		InternalID: 9001, PublicID: "ba_personal_test9", OwnerType: tenancypg.BillingOwnerWorkspace,
	}}
}

func (f *stubBillingAccounts) EnsureBillingAccountForWorkspace(_ context.Context, workspaceInternalID int64, workspacePublicID string) (tenancypg.BillingAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.personal
	out.WorkspaceInternalID = workspaceInternalID
	out.WorkspacePublicID = workspacePublicID
	out.Shared = workspacePublicID != ""
	return out, nil
}

func (f *stubBillingAccounts) GetBillingAccountByPublicID(_ context.Context, publicID string) (tenancypg.BillingAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if publicID == f.personal.PublicID {
		return f.personal, nil
	}
	return tenancypg.BillingAccount{}, tenancy.ErrBillingNotFound
}

func (f *stubBillingAccounts) GetBillingAccountByInternalID(_ context.Context, id int64) (tenancypg.BillingAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id == f.personal.InternalID {
		return f.personal, nil
	}
	return tenancypg.BillingAccount{}, tenancy.ErrBillingNotFound
}

func (f *stubBillingAccounts) GetBillingAccountRole(_ context.Context, _, _ int64) (string, bool, error) {
	return tenancy.BillingRoleOwner, true, nil
}

func (f *stubBillingAccounts) ListBillingAccountMembers(_ context.Context, _, _ int64) ([]tenancy.BillingAccountMember, error) {
	return nil, tenancy.ErrBillingForbidden
}

func (f *stubBillingAccounts) AddBillingAccountMember(_ context.Context, _, _, _ int64, _ string) (*tenancy.BillingAccountMember, error) {
	return nil, tenancy.ErrBillingForbidden
}

func (f *stubBillingAccounts) UpdateBillingAccountMemberRole(_ context.Context, _, _, _ int64, _ string) (*tenancy.BillingAccountMember, error) {
	return nil, tenancy.ErrBillingForbidden
}

func (f *stubBillingAccounts) InitiateBillingTransferInvite(_ context.Context, _, _, _ int64, _ string) (*tenancy.BillingTransferInvite, error) {
	return nil, tenancy.ErrBillingStoreNotConfigured
}

func (f *stubBillingAccounts) GetBillingTransferInvite(_ context.Context, _ string) (*tenancy.BillingTransferInvite, error) {
	return nil, tenancy.ErrBillingNotFound
}

func (f *stubBillingAccounts) ListBillingTransferInvitesForAccount(_ context.Context, _, _, _ int64, _ int) ([]*tenancy.BillingTransferInvite, error) {
	return nil, tenancy.ErrBillingStoreNotConfigured
}

func (f *stubBillingAccounts) AcceptBillingTransferInvite(_ context.Context, _ int64, _ string, _ int64) (*tenancy.BillingTransferInvite, error) {
	return nil, tenancy.ErrBillingStoreNotConfigured
}

func (f *stubBillingAccounts) RejectBillingTransferInvite(_ context.Context, _ int64, _ string, _ int64) (*tenancy.BillingTransferInvite, error) {
	return nil, tenancy.ErrBillingStoreNotConfigured
}

func (f *stubBillingAccounts) CancelBillingTransferInvite(_ context.Context, _ int64, _ string, _ int64) (*tenancy.BillingTransferInvite, error) {
	return nil, tenancy.ErrBillingStoreNotConfigured
}

func (f *stubBillingAccounts) RemoveBillingAccountMember(_ context.Context, _, _, _ int64) error {
	return tenancy.ErrBillingStoreNotConfigured
}

func newBillingCreateEngine(store *spyWorkspaceStore, dir *staticUserDirectory, accounts billingAccountStore) *gin.Engine {
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
	RegisterContractV1RoutesOnEngine(router, runtimehttp.AuthDeps{}, ContractV1Deps{
		Store: store, Users: dir, BillingAccounts: accounts,
	})
	return router
}

func postWorkspace(t *testing.T, engine *gin.Engine, userID int64, key, body string) (int, map[string]any) {
	t.Helper()
	status, _, payload := doV1(t, engine, v1Call{
		method: http.MethodPost, path: "/api/v1/workspaces", userID: userID,
		body: body, headers: map[string]string{contract.IdempotencyKeyHeader: key},
	})
	return status, payload
}

func TestV1CreateWorkspaceResolvesScopeAccount(t *testing.T) {
	store := &spyWorkspaceStore{InMemoryStore: tenancy.NewInMemoryStoreForTests()}
	dir := newStaticUserDirectory()
	engine := newBillingCreateEngine(store, dir, newStubBillingAccounts())

	status, payload := postWorkspace(t, engine, 11, "scope-1", `{"data":{"name":"biz one"}}`)
	if status != http.StatusCreated {
		t.Fatalf("create = %d %v, want 201", status, payload)
	}
	// Creation carries no billing choice: scope alone decides the payer.
	got := store.lastParams()
	if got.Name != "biz one" || got.OrganizationID != nil || got.OwnerUserID != 11 {
		t.Fatalf("create params = %+v, want name-only personal scope", got)
	}
	data := v1Data(t, payload).(map[string]any)
	if data["billing_account_id"] != "ba_personal_test9" {
		t.Fatalf("response billing_account_id = %v, want ba_personal_test9", data)
	}
}

func TestV1CreateWorkspaceRejectsBillingFields(t *testing.T) {
	store := &spyWorkspaceStore{InMemoryStore: tenancy.NewInMemoryStoreForTests()}
	dir := newStaticUserDirectory()
	engine := newBillingCreateEngine(store, dir, newStubBillingAccounts())

	// Retired billing options fail closed at strict binding without
	// touching the store (no compat, no silent reinterpretation).
	for _, body := range []string{
		`{"data":{"name":"shared","billing_account_id":"ba_personal_test9"}}`,
		`{"data":{"name":"solo","new_billing_account":true}}`,
		`{"data":{"name":"both","billing_account_id":"ba_personal_test9","new_billing_account":true}}`,
	} {
		before := len(store.params)
		status, payload := postWorkspace(t, engine, 12, "retired-"+body[:8], body)
		if status != http.StatusBadRequest || v1ErrorCode(t, payload) != string(contract.CodeBadRequest) {
			t.Fatalf("retired option %s = %d %v, want 400 bad_request", body, status, payload)
		}
		if len(store.params) != before {
			t.Fatalf("retired option %s created a workspace", body)
		}
	}
}

func TestV1CreateWorkspaceOrgScope(t *testing.T) {
	store := &spyWorkspaceStore{InMemoryStore: tenancy.NewInMemoryStoreForTests()}
	dir := newStaticUserDirectory()
	engine := newBillingCreateEngine(store, dir, newStubBillingAccounts())

	org, err := store.CreateOrganization(context.Background(), 13, "Acme")
	if err != nil {
		t.Fatal(err)
	}
	orgBody := fmt.Sprintf(`{"data":{"name":"orgbiz","organization_id":%q}}`, org.PublicID)
	status, payload := postWorkspace(t, engine, 13, "org-scope-1", orgBody)
	if status != http.StatusCreated {
		t.Fatalf("org create = %d %v, want 201", status, payload)
	}
	if store.orgCalls != 1 {
		t.Fatalf("org calls = %d, want 1 (org pool, never personal)", store.orgCalls)
	}
}

func TestV1CreateWorkspaceBillingIdempotency(t *testing.T) {
	store := &spyWorkspaceStore{InMemoryStore: tenancy.NewInMemoryStoreForTests()}
	dir := newStaticUserDirectory()
	engine := newBillingCreateEngine(store, dir, newStubBillingAccounts())

	status, first := postWorkspace(t, engine, 15, "idem-scope-1", `{"data":{"name":"idem"}}`)
	if status != http.StatusCreated {
		t.Fatalf("first = %d %v, want 201", status, first)
	}
	firstID := first["data"].(map[string]any)["id"].(string)
	status, replay := postWorkspace(t, engine, 15, "idem-scope-1", `{"data":{"name":"idem"}}`)
	if status != http.StatusCreated || replay["data"].(map[string]any)["id"] != firstID {
		t.Fatalf("replay = %d %v, want same %s", status, replay, firstID)
	}
	// Same key, different scope: conflict (never replay another scope).
	org, err := store.CreateOrganization(context.Background(), 15, "Idem Org")
	if err != nil {
		t.Fatal(err)
	}
	orgBody := fmt.Sprintf(`{"data":{"name":"idem","organization_id":%q}}`, org.PublicID)
	status, payload := postWorkspace(t, engine, 15, "idem-scope-1", orgBody)
	if status != http.StatusConflict || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceIdempotencyConflict) {
		t.Fatalf("scope conflict = %d %v, want 409 workspace_idempotency_conflict", status, payload)
	}
}
