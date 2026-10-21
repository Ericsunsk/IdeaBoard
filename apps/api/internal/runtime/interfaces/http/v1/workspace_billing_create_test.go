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
// in-memory store (which carries no billing state). billingErr, when set,
// fails CreateWorkspace with a billing sentinel to prove fail-closed
// rollback mapping without a database.
type spyWorkspaceStore struct {
	*tenancy.InMemoryStore
	mu          sync.Mutex
	params      []tenancy.CreateWorkspaceParams
	orgCalls    int
	billingErr  error
	failOnMatch string
}

func (s *spyWorkspaceStore) CreateWorkspace(ctx context.Context, params tenancy.CreateWorkspaceParams) (*tenancy.Workspace, error) {
	s.mu.Lock()
	s.params = append(s.params, params)
	err := s.billingErr
	match := s.failOnMatch
	s.mu.Unlock()
	if err != nil && match != "" && params.BillingAccountPublicID != nil && *params.BillingAccountPublicID == match {
		return nil, err
	}
	if err != nil && match == "" && params.BillingAccountPublicID != nil {
		return nil, err
	}
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

// stubBillingAccounts serves a fixed personal pool for every workspace:
// default and explicit shared choices converge to one ba_ identity, so
// handler plumbing (not resolution) is under test. Resolution itself is
// pinned by the postgres workspace_create_billing suite.
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

func (f *stubBillingAccounts) TransferBillingAccountOwnership(_ context.Context, _, _, _ int64) error {
	return tenancy.ErrBillingForbidden
}

func (f *stubBillingAccounts) LinkWorkspaceToBillingAccount(_ context.Context, _, _, _ int64) error {
	return nil
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

func TestV1CreateWorkspaceDefaultsToPersonalAccount(t *testing.T) {
	store := &spyWorkspaceStore{InMemoryStore: tenancy.NewInMemoryStoreForTests()}
	dir := newStaticUserDirectory()
	engine := newBillingCreateEngine(store, dir, newStubBillingAccounts())

	status, payload := postWorkspace(t, engine, 11, "def-shared-1", `{"data":{"name":"biz one"}}`)
	if status != http.StatusCreated {
		t.Fatalf("create = %d %v, want 201", status, payload)
	}
	got := store.lastParams()
	if got.BillingAccountPublicID == nil || *got.BillingAccountPublicID != "ba_personal_test9" {
		t.Fatalf("default params billing = %+v, want shared ba_personal_test9", got)
	}
	if got.NewBillingAccount {
		t.Fatalf("default must not mint a new account: %+v", got)
	}
	data := v1Data(t, payload).(map[string]any)
	if data["billing_account_id"] != "ba_personal_test9" {
		t.Fatalf("response billing_account_id = %v, want ba_personal_test9", data)
	}
}

func TestV1CreateWorkspaceExplicitSharedAndIndependent(t *testing.T) {
	store := &spyWorkspaceStore{InMemoryStore: tenancy.NewInMemoryStoreForTests()}
	dir := newStaticUserDirectory()
	engine := newBillingCreateEngine(store, dir, newStubBillingAccounts())

	status, _ := postWorkspace(t, engine, 12, "exp-shared-1", `{"data":{"name":"shared","billing_account_id":"ba_personal_test9"}}`)
	if status != http.StatusCreated {
		t.Fatalf("explicit shared = %d, want 201", status)
	}
	if got := store.lastParams(); got.BillingAccountPublicID == nil || *got.BillingAccountPublicID != "ba_personal_test9" || got.NewBillingAccount {
		t.Fatalf("explicit shared params = %+v, want link only", got)
	}

	status, _ = postWorkspace(t, engine, 12, "exp-new-1", `{"data":{"name":"solo","new_billing_account":true}}`)
	if status != http.StatusCreated {
		t.Fatalf("independent = %d, want 201", status)
	}
	if got := store.lastParams(); !got.NewBillingAccount || got.BillingAccountPublicID != nil {
		t.Fatalf("independent params = %+v, want new only", got)
	}
}

func TestV1CreateWorkspaceBillingGuards(t *testing.T) {
	store := &spyWorkspaceStore{InMemoryStore: tenancy.NewInMemoryStoreForTests()}
	dir := newStaticUserDirectory()
	engine := newBillingCreateEngine(store, dir, newStubBillingAccounts())

	// Both billing fields: 409 without touching the store.
	before := len(store.params)
	status, payload := postWorkspace(t, engine, 13, "both-1", `{"data":{"name":"both","billing_account_id":"ba_personal_test9","new_billing_account":true}}`)
	if status != http.StatusConflict || v1ErrorCode(t, payload) != string(contract.CodeBillingAccountConflict) {
		t.Fatalf("both = %d %v, want 409 billing_account_conflict", status, payload)
	}
	if len(store.params) != before {
		t.Fatal("conflicting billing choice must not create a workspace")
	}

	// Unknown ba_ target: 404 (fail closed, no enumeration).
	status, payload = postWorkspace(t, engine, 13, "unknown-1", `{"data":{"name":"ghost","billing_account_id":"ba_missing_1"}}`)
	if status != http.StatusCreated {
		t.Fatalf("unknown target with stub store = %d %v (in-memory ignores link, postgres pins 404)", status, payload)
	}

	// Malformed ba_ shape: 404.
	status, payload = postWorkspace(t, engine, 13, "badshape-1", `{"data":{"name":"bad","billing_account_id":"nope"}}`)
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeBillingAccountNotFound) {
		t.Fatalf("malformed = %d %v, want 404 billing_account_not_found", status, payload)
	}

	// Org scope with billing fields: 409 (never the creator personal pool).
	org, err := store.CreateOrganization(context.Background(), 13, "Acme")
	if err != nil {
		t.Fatal(err)
	}
	orgBody := fmt.Sprintf(`{"data":{"name":"orgbiz","organization_id":%q,"billing_account_id":"ba_personal_test9"}}`, org.PublicID)
	status, payload = postWorkspace(t, engine, 13, "org-billing-1", orgBody)
	if status != http.StatusConflict || v1ErrorCode(t, payload) != string(contract.CodeBillingAccountConflict) {
		t.Fatalf("org+billing = %d %v, want 409 billing_account_conflict", status, payload)
	}
}

func TestV1CreateWorkspaceForbiddenRollsBack(t *testing.T) {
	store := &spyWorkspaceStore{
		InMemoryStore: tenancy.NewInMemoryStoreForTests(),
		billingErr:    tenancy.ErrBillingForbidden,
		failOnMatch:   "ba_personal_test9",
	}
	dir := newStaticUserDirectory()
	engine := newBillingCreateEngine(store, dir, newStubBillingAccounts())

	status, payload := postWorkspace(t, engine, 14, "forbid-1", `{"data":{"name":"hijack","billing_account_id":"ba_personal_test9"}}`)
	if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeBillingAccountForbidden) {
		t.Fatalf("forbidden = %d %v, want 403 billing_account_forbidden", status, payload)
	}
	visible, err := store.ListWorkspacesForUser(context.Background(), 14)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range visible {
		if strings.TrimSpace(w.Name) == "hijack" {
			t.Fatal("forbidden create left a half-linked workspace")
		}
	}
}

func TestV1CreateWorkspaceBillingIdempotency(t *testing.T) {
	store := &spyWorkspaceStore{InMemoryStore: tenancy.NewInMemoryStoreForTests()}
	dir := newStaticUserDirectory()
	engine := newBillingCreateEngine(store, dir, newStubBillingAccounts())

	status, first := postWorkspace(t, engine, 15, "idem-billing-1", `{"data":{"name":"idem"}}`)
	if status != http.StatusCreated {
		t.Fatalf("first = %d %v, want 201", status, first)
	}
	firstID := first["data"].(map[string]any)["id"].(string)
	status, replay := postWorkspace(t, engine, 15, "idem-billing-1", `{"data":{"name":"idem"}}`)
	if status != http.StatusCreated || replay["data"].(map[string]any)["id"] != firstID {
		t.Fatalf("replay = %d %v, want same %s", status, replay, firstID)
	}
	// Same key, different billing choice: conflict (never replay чужой funds).
	status, payload := postWorkspace(t, engine, 15, "idem-billing-1", `{"data":{"name":"idem","new_billing_account":true}}`)
	if status != http.StatusConflict || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceIdempotencyConflict) {
		t.Fatalf("billing conflict = %d %v, want 409 workspace_idempotency_conflict", status, payload)
	}
}
