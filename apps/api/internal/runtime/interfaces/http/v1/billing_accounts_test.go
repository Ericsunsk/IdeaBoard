package v1

import (
	"context"
	"net/http"
	"testing"

	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"
)

// Focused HTTP coverage for #915: stable ba_ identity, space-admin cannot
// recharge (billing_account_forbidden), member role changes stick.
func TestV1BillingAccountReturnsStableIdentity(t *testing.T) {
	ctx := context.Background()
	fx := newBillingTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "billing-stable", 11)
	engine := fx.engine()
	path := "/api/v1/workspaces/" + ws.PublicID + "/billing-account"

	status, _, first := doV1(t, engine, v1Call{method: http.MethodGet, path: path, userID: 11})
	if status != http.StatusOK {
		t.Fatalf("GET billing-account status = %d (%v)", status, first)
	}
	data := v1Data(t, first).(map[string]any)
	accountID, _ := data["billing_account_id"].(string)
	if !contract.IsBillingAccountID(accountID) {
		t.Fatalf("billing_account_id = %q, want ba_ identity (%v)", accountID, data)
	}
	if data["workspace_id"] != ws.PublicID {
		t.Fatalf("workspace_id = %v, want %q", data["workspace_id"], ws.PublicID)
	}
	if data["owner_type"] == "" {
		t.Fatalf("owner_type missing (%v)", data)
	}
	status, _, second := doV1(t, engine, v1Call{method: http.MethodGet, path: path, userID: 11})
	if status != http.StatusOK {
		t.Fatalf("second GET status = %d", status)
	}
	if v1Data(t, second).(map[string]any)["billing_account_id"] != accountID {
		t.Fatalf("stable identity violated: %v vs %q", second, accountID)
	}
}

func TestV1SpaceAdminCannotCheckout(t *testing.T) {
	ctx := context.Background()
	fx := newBillingTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "billing-space-admin", 7)
	fx.store.SeedWorkspaceMember(ws.PublicID, 8, tenancy.RoleAdmin)
	// Caller 8 is a Workspace Admin but holds no billing grant.
	fx.accounts.deny = map[int64]bool{8: true}
	fx.seedWallet(7, 100000, 0)
	engine := fx.engine()
	path := "/api/v1/workspaces/" + ws.PublicID + "/checkout-sessions"

	status, _, payload := doV1(t, engine, v1Call{
		method: http.MethodPost, path: path, userID: 8, body: billingCheckoutBody(1000),
		headers: map[string]string{contract.IdempotencyKeyHeader: "space-admin-key-1"},
	})
	if status != http.StatusForbidden {
		t.Fatalf("space-admin checkout status = %d, want 403 (%v)", status, payload)
	}
	if v1ErrorCode(t, payload) != string(contract.CodeBillingAccountForbidden) {
		t.Fatalf("space-admin checkout code = %v, want billing_account_forbidden", payload)
	}
}

func TestV1BillingAdminCanCheckout(t *testing.T) {
	ctx := context.Background()
	fx := newBillingTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "billing-admin", 7)
	fx.store.SeedWorkspaceMember(ws.PublicID, 9, tenancy.RoleMember)
	wsRow, err := fx.store.GetWorkspace(ctx, ws.PublicID)
	if err != nil {
		t.Fatalf("get workspace failed: %v", err)
	}
	real, err := fx.accounts.EnsureBillingAccountForWorkspace(ctx, wsRow.InternalID, ws.PublicID)
	if err != nil {
		t.Fatalf("ensure failed: %v", err)
	}
	// Caller 9 is a plain Workspace Member but an explicit Billing Admin:
	// financial management never grants Board access, and workspace rank
	// alone never grants financial management — the billing grant decides.
	fx.accounts.roles = map[int64]map[int64]string{real.InternalID: {9: tenancy.BillingRoleAdmin}}
	fx.seedWallet(7, 100000, 0)
	engine := fx.engine()
	path := "/api/v1/workspaces/" + ws.PublicID + "/checkout-sessions"

	status, _, payload := doV1(t, engine, v1Call{
		method: http.MethodPost, path: path, userID: 9, body: billingCheckoutBody(1000),
		headers: map[string]string{contract.IdempotencyKeyHeader: "billing-admin-key-1"},
	})
	if status != http.StatusCreated {
		t.Fatalf("billing-admin checkout status = %d, want 201 (%v)", status, payload)
	}
}

func TestV1BillingMemberRoleChanges(t *testing.T) {
	ctx := context.Background()
	fx := newBillingTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "billing-members", 7)
	for _, id := range []int64{7, 8} {
		fx.dir.ensure(id)
	}
	engine := fx.engine()
	base := "/api/v1/workspaces/" + ws.PublicID + "/billing-account/members"
	usr8 := tenancy.FormatUserPublicID(fx.dir.ensure(8))

	status, _, listed := doV1(t, engine, v1Call{method: http.MethodGet, path: base, userID: 7})
	if status != http.StatusOK {
		t.Fatalf("list members status = %d (%v)", status, listed)
	}
	status, _, added := doV1(t, engine, v1Call{
		method: http.MethodPost, path: base, userID: 7,
		body:    `{"data":{"user_id":` + quoteJSON(usr8) + `,"role":"admin"}}`,
		headers: map[string]string{contract.IdempotencyKeyHeader: "billing-member-add-1"},
	})
	if status != http.StatusCreated {
		t.Fatalf("add member status = %d (%v)", status, added)
	}
	status, _, updated := doV1(t, engine, v1Call{
		method: http.MethodPatch, path: base + "/" + usr8, userID: 7,
		body: `{"data":{"role":"owner"}}`,
	})
	if status != http.StatusOK {
		t.Fatalf("update member status = %d (%v)", status, updated)
	}
	if v1Data(t, updated).(map[string]any)["role"] != tenancy.BillingRoleOwner {
		t.Fatalf("updated role = %v, want owner", updated)
	}
}

func quoteJSON(s string) string {
	return `"` + s + `"`
}

// Billing management keeps its own unavailable copy and checks dependency
// readiness before mutation headers/body, even when resolution uses the
// shared wallet-read helper.
func TestV1BillingManagementUnavailablePrecedesInput(t *testing.T) {
	for _, route := range []struct{ method, suffix string }{
		{http.MethodGet, "/billing-account"},
		{http.MethodGet, "/billing-account/members"},
		{http.MethodPost, "/billing-account/link"},
		{http.MethodPost, "/billing-account/members"},
		{http.MethodPatch, "/billing-account/members/usr_missing"},
		{http.MethodPost, "/billing-account/transfer"},
		{http.MethodGet, "/purchase-options"},
	} {
		t.Run(route.method+route.suffix, func(t *testing.T) {
			fx := newBillingTestFixture()
			ws := v1WorkspaceFixture(t, context.Background(), fx.store, "billing-unavailable", 11)
			fx.deps.BillingAccounts = nil
			fx.deps.PurchaseOptions = &fakeGapPurchaseOptions{}
			status, _, payload := doV1(t, fx.engine(), v1Call{
				method: route.method, path: "/api/v1/workspaces/" + ws.PublicID + route.suffix, userID: 11, body: "{",
			})
			if status != http.StatusServiceUnavailable || v1ErrorCode(t, payload) != string(contract.CodeServiceUnavailable) {
				t.Fatalf("unavailable status=%d payload=%v", status, payload)
			}
			message := payload["error"].(map[string]any)["message"]
			if message != runtimehttp.MsgServiceUnavailable39 {
				t.Fatalf("unavailable message=%v, want %q", message, runtimehttp.MsgServiceUnavailable39)
			}
		})
	}
}
