// Organization quota role-matrix focused integration test (#953).
//
// Real PostgreSQL (SKIPs without API_TEST_DATABASE_DSN via testdb.Open):
// Organization Owner/Admin/Member plus the shared Organization pool
// exercise the HTTP matrix end to end against the production stores
// (tenancy + billing accounts + member quotas). Focused on the matrix
// and Owner-normalization core only: Owner sets Admin and Owner/Admin
// set Member pass; Admin editing Owner/Admin renders 403; capped writes
// for an Owner render 403 (only Owner unlimited clears pass); non-manager members
// render 403 on write/list while self reads stay visible; Owner self
// reads and list rows normalize a stale capped row to unlimited.

package v1

import (
	"context"
	"net/http"
	"testing"

	"ideaboard/api/internal/memberquotas"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
	"ideaboard/api/internal/testdb"

	"github.com/gin-gonic/gin"
)

type quotaMatrixPGFixture struct {
	engine    *gin.Engine
	workspace *tenancy.Workspace
	account   int64
	quotas    *memberquotas.Store
	dir       *staticUserDirectory
}

func newQuotaMatrixPGFixture(t *testing.T) *quotaMatrixPGFixture {
	t.Helper()
	db := testdb.Open(t)
	ctx := context.Background()
	tstore := tenancypg.NewStore(db, nil)
	baStore := tenancypg.NewBillingAccountStore(db)
	quotas := memberquotas.NewStore(db)
	dir := newStaticUserDirectory()
	const owner, admin, member, outsider = 1, 11, 3, 8
	for _, id := range []int64{owner, admin, member, outsider} {
		dir.ensure(id)
	}
	org, err := tstore.CreateOrganization(ctx, owner, "quota-matrix-org")
	if err != nil {
		t.Fatalf("create matrix org: %v", err)
	}
	if _, err := tstore.AddOrganizationMember(ctx, org.PublicID, admin, tenancy.RoleAdmin); err != nil {
		t.Fatalf("add matrix admin: %v", err)
	}
	if _, err := tstore.AddOrganizationMember(ctx, org.PublicID, member, tenancy.RoleMember); err != nil {
		t.Fatalf("add matrix member: %v", err)
	}
	ws, err := tstore.CreateOrganizationWorkspace(ctx, org.PublicID, owner, "quota-matrix-ws")
	if err != nil {
		t.Fatalf("create matrix workspace: %v", err)
	}
	// Org Members hold only explicit Workspace rows; grant the member so
	// the matrix (not invisibility) is exercised.
	if _, err := tstore.AddWorkspaceMember(ctx, ws.PublicID, member, tenancy.RoleMember); err != nil {
		t.Fatalf("grant matrix member workspace: %v", err)
	}
	engine := newV1TestEngine(tstore, dir)
	RegisterContractV1BillingRoutesOnEngine(engine, runtimehttp.AuthDeps{}, ContractV1BillingDeps{
		Tenancy: tstore, Users: dir, BillingAccounts: baStore, Quotas: quotas,
	})
	account, err := baStore.EnsureBillingAccountForWorkspace(ctx, ws.InternalID, ws.PublicID)
	if err != nil {
		t.Fatalf("resolve matrix billing account: %v", err)
	}
	return &quotaMatrixPGFixture{engine: engine, workspace: ws, account: account.InternalID, quotas: quotas, dir: dir}
}

func (f *quotaMatrixPGFixture) userPublicID(userID int64) string {
	return tenancy.FormatUserPublicID(f.dir.ensure(userID))
}

func (f *quotaMatrixPGFixture) putQuota(t *testing.T, actor, target int64, body string) (int, map[string]any) {
	t.Helper()
	path := "/api/v1/workspaces/" + f.workspace.PublicID + "/member-quotas/" + f.userPublicID(target)
	status, _, payload := doV1(t, f.engine, v1Call{method: http.MethodPut, path: path, userID: actor, body: body})
	return status, payload
}

func quotaMatrixForbidden(t *testing.T, status int, payload map[string]any) {
	t.Helper()
	if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeBillingAccountForbidden) {
		t.Fatalf("status = %d %v, want 403 billing_account_forbidden", status, payload)
	}
}

func TestV1QuotaOrgRoleMatrixPG(t *testing.T) {
	f := newQuotaMatrixPGFixture(t)
	const owner, admin, member = 1, 11, 3
	capped := `{"data":{"period":"daily","limit_credits":100}}`

	// UI capabilities use the same role matrix as the list/write gate.
	for _, actor := range []int64{owner, admin, member} {
		status, _, payload := doV1(t, f.engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + f.workspace.PublicID + "/billing-account", userID: actor})
		want := actor != member
		if status != http.StatusOK || v1Data(t, payload).(map[string]any)["can_manage_member_quotas"] != want {
			t.Fatalf("actor %d quota capability = %d %v, want %v", actor, status, payload, want)
		}
	}

	// Owner sets Admin, Owner/Admin set Member: all pass.
	if status, payload := f.putQuota(t, owner, admin, capped); status != http.StatusOK {
		t.Fatalf("owner sets admin = %d %v, want 200", status, payload)
	}
	if status, payload := f.putQuota(t, admin, member, capped); status != http.StatusOK {
		t.Fatalf("admin sets member = %d %v, want 200", status, payload)
	}
	if status, payload := f.putQuota(t, owner, member, `{"data":{"period":"monthly","limit_credits":50}}`); status != http.StatusOK {
		t.Fatalf("owner sets member = %d %v, want 200", status, payload)
	}

	// Admin editing Owner/Admin renders 403.
	status, payload := f.putQuota(t, admin, owner, capped)
	quotaMatrixForbidden(t, status, payload)
	status, payload = f.putQuota(t, admin, admin, capped)
	quotaMatrixForbidden(t, status, payload)
	status, payload = f.putQuota(t, admin, owner, `{"data":{"period":"unlimited"}}`)
	quotaMatrixForbidden(t, status, payload)
	status, payload = f.putQuota(t, admin, admin, `{"data":{"period":"unlimited"}}`)
	quotaMatrixForbidden(t, status, payload)

	// Capped writes for an Owner render 403; only Owner unlimited clears pass.
	status, payload = f.putQuota(t, owner, owner, capped)
	quotaMatrixForbidden(t, status, payload)
	if status, payload := f.putQuota(t, owner, owner, `{"data":{"period":"unlimited"}}`); status != http.StatusOK {
		t.Fatalf("owner clears owner to unlimited = %d %v, want 200", status, payload)
	}

	// Non-manager members render 403 on write/list; self stays visible.
	status, payload = f.putQuota(t, member, member, capped)
	quotaMatrixForbidden(t, status, payload)
	status, _, listPayload := doV1(t, f.engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + f.workspace.PublicID + "/member-quotas", userID: member})
	quotaMatrixForbidden(t, status, listPayload)
	status, _, selfPayload := doV1(t, f.engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + f.workspace.PublicID + "/member-quotas/self", userID: member})
	if status != http.StatusOK {
		t.Fatalf("member self read = %d %v, want 200", status, selfPayload)
	}
}

func TestV1QuotaOrgOwnerReadsNormalizeUnlimitedPG(t *testing.T) {
	f := newQuotaMatrixPGFixture(t)
	ctx := context.Background()
	const owner, admin = 1, 11
	// A stale capped Owner row (the store persists it; the API never
	// writes one): reads normalize to unlimited.
	stale := int64(100)
	if _, err := f.quotas.SetQuota(ctx, f.account, owner, memberquotas.PeriodDaily, &stale, owner); err != nil {
		t.Fatalf("seed stale owner quota: %v", err)
	}

	status, _, selfPayload := doV1(t, f.engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + f.workspace.PublicID + "/member-quotas/self", userID: owner})
	if status != http.StatusOK {
		t.Fatalf("owner self read = %d %v, want 200", status, selfPayload)
	}
	self := v1Data(t, selfPayload).(map[string]any)
	if self["period"] != memberquotas.PeriodUnlimited {
		t.Fatalf("owner self period = %v, want unlimited", self["period"])
	}
	if _, ok := self["limit_credits"]; ok {
		t.Fatalf("owner self must omit limit_credits: %v", self)
	}
	if _, ok := self["remaining_credits"]; ok {
		t.Fatalf("owner self must omit remaining_credits: %v", self)
	}

	// Both managers list; the Owner row reads unlimited on each.
	for actor, name := range map[int64]string{owner: "owner", admin: "admin"} {
		status, _, listPayload := doV1(t, f.engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + f.workspace.PublicID + "/member-quotas", userID: actor})
		if status != http.StatusOK {
			t.Fatalf("%s list = %d %v, want 200", name, status, listPayload)
		}
		items, ok := v1Data(t, listPayload).([]any)
		if !ok || len(items) != 1 {
			t.Fatalf("%s list = %v, want the single stale owner row", name, listPayload)
		}
		row, ok := items[0].(map[string]any)
		if !ok || row["user_id"] != f.userPublicID(owner) || row["period"] != memberquotas.PeriodUnlimited {
			t.Fatalf("%s list row = %v, want owner normalized to unlimited", name, items[0])
		}
		if _, ok := row["limit_credits"]; ok {
			t.Fatalf("%s list row must omit limit_credits: %v", name, row)
		}
	}
}
