package v1

// Workspace billing overview aggregation tests (panel fan-out in one
// request). Fresh-DB friendly: in-memory tenancy store + fakes only.

import (
	"context"
	"net/http"
	"testing"

	"ideaboard/api/internal/memberquotas"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"
)

func overviewData(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	data, ok := v1Data(t, payload).(map[string]any)
	if !ok {
		t.Fatalf("overview data is not an object: %v", payload)
	}
	return data
}

func TestV1BillingOverviewAggregatesPanelReads(t *testing.T) {
	ctx := context.Background()
	fx, quotas := newQuotaTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "overview", 11)
	fx.dir.ensure(11)
	fx.dir.ensure(12)
	if _, err := fx.store.AddWorkspaceMember(ctx, ws.PublicID, 12, tenancy.RoleMember); err != nil {
		t.Fatal(err)
	}
	account := fx.mustAccount(t, ctx, ws)
	fx.seedWallet(account, 1000, 100)
	fx.accounts.roles = map[int64]map[int64]string{account: {11: tenancy.BillingRoleOwner}}
	dailyLimit := int64(100)
	quotas.period[quotaKey{account, 11}] = memberquotas.PeriodDaily
	quotas.limit[quotaKey{account, 11}] = &dailyLimit
	quotas.setUsed(account, 11, 25)
	engine := fx.engine()
	path := "/api/v1/workspaces/" + ws.PublicID + "/billing-overview"

	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: path, userID: 11})
	if status != http.StatusOK {
		t.Fatalf("GET billing-overview status = %d (%v)", status, payload)
	}
	data := overviewData(t, payload)

	billingAccount, ok := data["billing_account"].(map[string]any)
	if !ok || billingAccount["billing_account_id"] == nil || billingAccount["can_manage_member_quotas"] != true {
		t.Fatalf("billing_account section = %v, want resolved account with manager capability", data["billing_account"])
	}
	balance, ok := data["balance"].(map[string]any)
	if !ok || balance["available_credits"] != float64(1000) || balance["reserved_credits"] != float64(100) {
		t.Fatalf("balance section = %v, want seeded wallet", data["balance"])
	}
	if _, ok := data["usage"].(map[string]any); !ok {
		t.Fatalf("usage section = %v, want composed usage snapshot", data["usage"])
	}
	quota, ok := data["quota"].(map[string]any)
	if !ok || quota["user_id"] != quotaUserPublicID(11) || quota["period"] != "daily" || quota["used_credits"] != float64(25) {
		t.Fatalf("quota section = %v, want caller daily 25", data["quota"])
	}
	quotasList, ok := data["quotas"].([]any)
	if !ok || len(quotasList) != 1 {
		t.Fatalf("quotas section = %v, want manager list with one row", data["quotas"])
	}
	managers, ok := data["billing_members"].([]any)
	if !ok || len(managers) != 1 {
		t.Fatalf("billing_members section = %v, want manager list", data["billing_members"])
	}
	if data["members_count"] != float64(2) {
		t.Fatalf("members_count = %v, want 2", data["members_count"])
	}
	if _, present := data["live_invites"]; present {
		t.Fatalf("live_invites = %v, want omitted when no pending invites", data["live_invites"])
	}
}

func TestV1BillingOverviewMemberViewOmitsManagerSections(t *testing.T) {
	ctx := context.Background()
	fx, quotas := newQuotaTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "overview-member", 11)
	fx.dir.ensure(11)
	fx.dir.ensure(12)
	if _, err := fx.store.AddWorkspaceMember(ctx, ws.PublicID, 12, tenancy.RoleMember); err != nil {
		t.Fatal(err)
	}
	account := fx.mustAccount(t, ctx, ws)
	fx.seedWallet(account, 500, 0)
	fx.accounts.roles = map[int64]map[int64]string{account: {11: tenancy.BillingRoleOwner}}
	monthlyLimit := int64(200)
	quotas.period[quotaKey{account, 11}] = memberquotas.PeriodMonthly
	quotas.limit[quotaKey{account, 11}] = &monthlyLimit
	quotas.setUsed(account, 11, 50)
	engine := fx.engine()
	path := "/api/v1/workspaces/" + ws.PublicID + "/billing-overview"

	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: path, userID: 12})
	if status != http.StatusOK {
		t.Fatalf("GET billing-overview status = %d (%v)", status, payload)
	}
	data := overviewData(t, payload)

	quota, ok := data["quota"].(map[string]any)
	if !ok || quota["user_id"] != quotaUserPublicID(12) || quota["period"] != "unlimited" {
		t.Fatalf("quota section = %v, want member unlimited self", data["quota"])
	}
	if _, present := data["quotas"]; present {
		t.Fatalf("quotas = %v, want omitted for non-managers", data["quotas"])
	}
	if _, present := data["billing_members"]; present {
		t.Fatalf("billing_members = %v, want omitted for non-managers", data["billing_members"])
	}
	if data["members_count"] != float64(2) {
		t.Fatalf("members_count = %v, want 2", data["members_count"])
	}
	if _, ok := data["balance"].(map[string]any); !ok {
		t.Fatalf("balance section = %v, want readable balance", data["balance"])
	}
}

func TestV1BillingOverviewLiveInvitesPendingOnly(t *testing.T) {
	ctx := context.Background()
	fx, _ := newQuotaTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "overview-invites", 11)
	fx.dir.ensure(11)
	fx.dir.ensure(12)
	if _, err := fx.store.AddWorkspaceMember(ctx, ws.PublicID, 12, tenancy.RoleMember); err != nil {
		t.Fatal(err)
	}
	account := fx.mustAccount(t, ctx, ws)
	fx.seedWallet(account, 500, 0)
	fx.accounts.roles = map[int64]map[int64]string{account: {11: tenancy.BillingRoleOwner}}
	engine := fx.engine()

	invitePath := "/api/v1/workspaces/" + ws.PublicID + "/billing-account/transfer-invites"
	status, _, payload := doV1(t, engine, v1Call{
		method: http.MethodPost, path: invitePath, userID: 11,
		body:    `{"data":{"new_owner_user_id":` + `"` + quotaUserPublicID(12) + `"}}`,
		headers: map[string]string{contract.IdempotencyKeyHeader: "overview-invite-1"},
	})
	if status != http.StatusCreated {
		t.Fatalf("initiate transfer invite = %d (%v)", status, payload)
	}

	overviewPath := "/api/v1/workspaces/" + ws.PublicID + "/billing-overview"
	status, _, payload = doV1(t, engine, v1Call{method: http.MethodGet, path: overviewPath, userID: 11})
	if status != http.StatusOK {
		t.Fatalf("GET billing-overview status = %d (%v)", status, payload)
	}
	invites, ok := overviewData(t, payload)["live_invites"].([]any)
	if !ok || len(invites) != 1 {
		t.Fatalf("live_invites = %v, want the pending invite", overviewData(t, payload)["live_invites"])
	}
	invite, ok := invites[0].(map[string]any)
	if !ok || invite["from_user_id"] != quotaUserPublicID(11) || invite["to_user_id"] != quotaUserPublicID(12) || invite["status"] != "pending" {
		t.Fatalf("live invite = %v, want pending 11 -> 12", invites[0])
	}
}

func TestV1BillingOverviewRangeValidation(t *testing.T) {
	ctx := context.Background()
	fx, _ := newQuotaTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "overview-range", 11)
	engine := fx.engine()
	path := "/api/v1/workspaces/" + ws.PublicID + "/billing-overview?range=fortnight"

	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: path, userID: 11})
	if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
		t.Fatalf("invalid range = %d %v, want 422 validation_failed", status, payload)
	}
}

func TestV1BillingOverviewHiddenWorkspace(t *testing.T) {
	ctx := context.Background()
	fx, _ := newQuotaTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "overview-hidden", 11)
	engine := fx.engine()
	path := "/api/v1/workspaces/" + ws.PublicID + "/billing-overview"

	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: path, userID: 12})
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("hidden workspace = %d %v, want 404 workspace_not_found", status, payload)
	}
}

func TestV1MemberQuotaScopeSelfShortcut(t *testing.T) {
	ctx := context.Background()
	fx, quotas := newQuotaTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "quota-scope", 11)
	fx.dir.ensure(11)
	fx.dir.ensure(12)
	if _, err := fx.store.AddWorkspaceMember(ctx, ws.PublicID, 12, tenancy.RoleMember); err != nil {
		t.Fatal(err)
	}
	account := fx.mustAccount(t, ctx, ws)
	fx.accounts.roles = map[int64]map[int64]string{account: {11: tenancy.BillingRoleOwner}}
	dailyLimit := int64(100)
	quotas.period[quotaKey{account, 12}] = memberquotas.PeriodDaily
	quotas.limit[quotaKey{account, 12}] = &dailyLimit
	quotas.setUsed(account, 12, 10)
	engine := fx.engine()
	path := "/api/v1/workspaces/" + ws.PublicID + "/member-quotas?scope=self"

	// Member without the manager gate reads only their own row.
	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: path, userID: 12})
	if status != http.StatusOK {
		t.Fatalf("GET quotas?scope=self status = %d (%v)", status, payload)
	}
	page, ok := v1Data(t, payload).([]any)
	if !ok || len(page) != 1 {
		t.Fatalf("scope=self page = %v, want single-item list", v1Data(t, payload))
	}
	row, ok := page[0].(map[string]any)
	if !ok || row["user_id"] != quotaUserPublicID(12) || row["used_credits"] != float64(10) {
		t.Fatalf("scope=self row = %v, want caller daily 10", page[0])
	}

	// Full list still requires the manager gate.
	status, _, denied := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + ws.PublicID + "/member-quotas", userID: 12})
	if status != http.StatusForbidden || v1ErrorCode(t, denied) != string(contract.CodeBillingAccountForbidden) {
		t.Fatalf("full quota list = %d %v, want 403 billing_account_forbidden", status, denied)
	}

	// Unknown scopes fail closed.
	status, _, invalid := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + ws.PublicID + "/member-quotas?scope=all", userID: 11})
	if status != http.StatusBadRequest || v1ErrorCode(t, invalid) != string(contract.CodeBadRequest) {
		t.Fatalf("unknown scope = %d %v, want 400 bad_request", status, invalid)
	}
}
