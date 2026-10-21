package v1

// Account default new-member quota tests (issue #947).
//
// HTTP + PostgreSQL coverage uses the canonical membership transaction.
// Behavioral coverage: default read absent fail-closed (409, never implicit
// unlimited), Billing Owner/Admin writes with five-period validation,
// space-only admin 403, invite blocked without default (409 + guidance),
// invite seeds quota from default, duplicate invite preserves the
// per-member row, shared-account second-space join never copies, remove +
// rejoin keeps the cap and usage, and concurrent joins collapse to one row.

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/credits"
	creditsdomain "ideaboard/api/internal/credits/domain"
	creditsports "ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/memberquotas"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type defaultQuotaInviteFixture struct {
	db       *pgxpool.Pool
	store    *tenancypg.Store
	dir      *staticUserDirectory
	accounts *tenancypg.BillingAccountStore
	quotas   *memberquotas.Store
	engine   *gin.Engine
}

func newDefaultQuotaInviteFixture(t *testing.T) *defaultQuotaInviteFixture {
	t.Helper()
	db := testdb.Open(t)
	store := tenancypg.NewStore(db, nil)
	dir := newStaticUserDirectory()
	accounts := tenancypg.NewBillingAccountStore(db)
	quotas := memberquotas.NewStore(db)
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
	boardStore := boards.NewPostgresWorkspaceBoardStore(db, nil)
	v1group := router.Group(runtimehttp.ContractV1GroupPath)
	RegisterContractV1Routes(v1group, runtimehttp.AuthDeps{}, ContractV1Deps{
		Store: store, Users: dir, Boards: boardStore,
		BillingAccounts: accounts,
	})
	RegisterContractV1BillingRoutesOnEngine(router, runtimehttp.AuthDeps{}, ContractV1BillingDeps{
		Tenancy: store, Users: dir, BillingAccounts: accounts, Quotas: quotas,
	})
	return &defaultQuotaInviteFixture{db: db, store: store, dir: dir, accounts: accounts, quotas: quotas, engine: router}
}

func (fx *defaultQuotaInviteFixture) workspace(t *testing.T, name string, owner int64) *tenancy.Workspace {
	t.Helper()
	fx.dir.ensure(owner)
	ws, err := fx.store.CreateWorkspace(context.Background(), tenancy.CreateWorkspaceParams{Name: name, OwnerUserID: owner})
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

func (fx *defaultQuotaInviteFixture) account(t *testing.T, ws *tenancy.Workspace) int64 {
	t.Helper()
	account, err := fx.accounts.EnsureBillingAccountForWorkspace(context.Background(), ws.InternalID, ws.PublicID)
	if err != nil {
		t.Fatalf("resolve fixture billing account: %v", err)
	}
	return account.InternalID
}

// pendingUsage persists a real Hold and its Vision reservation so removal/rejoin
// assertions exercise the same immutable facts read by the consumption projection.
func (fx *defaultQuotaInviteFixture) pendingUsage(t *testing.T, ws *tenancy.Workspace, account, user, creditsUsed int64) {
	t.Helper()
	amount, err := creditsdomain.NewCreditAmount(creditsUsed)
	if err != nil {
		t.Fatal(err)
	}
	metadata := creditsdomain.CommandMetadata{
		IdempotencyKey: "invite-usage-funds",
		Actor:          creditsdomain.Actor{Type: creditsdomain.ActorUser, ID: fmt.Sprint(user)},
		Source:         "test", Reason: "retained membership usage",
		BusinessReference: creditsdomain.BusinessReference{Type: "test", ID: "invite-usage"},
	}
	commands := credits.NewService(fx.db)
	if _, err := commands.Grant(t.Context(), creditsports.GrantCommand{BillingAccountID: account, Amount: amount, Metadata: metadata}); err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	metadata.IdempotencyKey = "invite-usage-hold"
	metadata.BusinessReference = creditsdomain.BusinessReference{Type: "ai_search_operation", ID: id}
	hold, err := commands.Hold(t.Context(), creditsports.HoldCommand{BillingAccountID: account, Amount: amount, Metadata: metadata})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.db.Exec(t.Context(), `INSERT INTO ai_search_operations (operation_id, user_id, workspace_id, billing_account_id, idempotency_key, request_hash, sku, price_version_id, unit_credits, hold_transaction_id) VALUES ($1::uuid, $2, $3, $4, $1::text, $5, 'ai-search:vision', 1, $6, $7)`, id, user, ws.InternalID, account, strings.Repeat("a", 64), creditsUsed, hold.TransactionID); err != nil {
		t.Fatal(err)
	}
}

func (fx *defaultQuotaInviteFixture) invite(t *testing.T, wsPublicID string, actor, target int64, role, key string) (int, map[string]any) {
	t.Helper()
	fx.dir.ensure(target)
	status, _, payload := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: "/api/v1/workspaces/" + wsPublicID + "/members", userID: actor,
		body: fmt.Sprintf(`{"data":{"user_id":%q,"role":%q}}`,
			tenancy.FormatUserPublicID(fx.dir.ensure(target)), role),
		headers: map[string]string{contract.IdempotencyKeyHeader: key},
	})
	return status, payload
}

func TestV1DefaultQuotaReadAbsentFailClosed(t *testing.T) {
	fx := newDefaultQuotaInviteFixture(t)
	ws := fx.workspace(t, "defread", 11)
	path := "/api/v1/workspaces/" + ws.PublicID + "/default-member-quota"

	status, _, payload := doV1(t, fx.engine, v1Call{method: http.MethodGet, path: path, userID: 11})
	if status != http.StatusConflict || v1ErrorCode(t, payload) != string(contract.CodeDefaultMemberQuotaNotConfigured) {
		t.Fatalf("GET absent default = %d %v, want 409 default_member_quota_not_configured", status, payload)
	}
	// Hidden workspace stays 404, never the default code.
	status, _, payload = doV1(t, fx.engine, v1Call{method: http.MethodGet, path: path, userID: 99})
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("outsider GET default = %d %v, want 404 workspace_not_found", status, payload)
	}
}

func TestV1DefaultQuotaOwnerSetsAndReadsBack(t *testing.T) {
	fx := newDefaultQuotaInviteFixture(t)
	ws := fx.workspace(t, "defset", 11)
	path := "/api/v1/workspaces/" + ws.PublicID + "/default-member-quota"

	status, _, payload := doV1(t, fx.engine, v1Call{
		method: http.MethodPut, path: path, userID: 11,
		body: `{"data":{"period":"monthly","limit_credits":1000}}`,
	})
	if status != http.StatusOK {
		t.Fatalf("PUT default status = %d (%v)", status, payload)
	}
	if data := v1Data(t, payload).(map[string]any); data["period"] != "monthly" || data["limit_credits"] != float64(1000) {
		t.Fatalf("default after set = %v, want monthly 1000", data)
	}
	status, _, payload = doV1(t, fx.engine, v1Call{method: http.MethodGet, path: path, userID: 11})
	if status != http.StatusOK {
		t.Fatalf("GET after set status = %d (%v)", status, payload)
	}
	if data := v1Data(t, payload).(map[string]any); data["period"] != "monthly" {
		t.Fatalf("GET after set = %v, want monthly", data)
	}
	// Switching the default never touches existing per-member rows (covered
	// by TestV1InviteDuplicatePreservesQuota below); unlimited clears it.
	status, _, payload = doV1(t, fx.engine, v1Call{
		method: http.MethodPut, path: path, userID: 11, body: `{"data":{"period":"unlimited"}}`,
	})
	if status != http.StatusOK {
		t.Fatalf("PUT unlimited status = %d (%v)", status, payload)
	}
	if data := v1Data(t, payload).(map[string]any); data["period"] != "unlimited" {
		t.Fatalf("default after unlimit = %v, want unlimited", data)
	}
}

func TestV1DefaultQuotaSpaceAdminCannotRaise(t *testing.T) {
	fx := newDefaultQuotaInviteFixture(t)
	ws := fx.workspace(t, "defadmin", 11)
	account := fx.account(t, ws)
	if _, err := fx.quotas.SetDefaultPolicy(t.Context(), account, memberquotas.PeriodMonthly, ptrInt64(1000), 11); err != nil {
		t.Fatal(err)
	}
	fx.dir.ensure(12)
	if _, err := fx.store.AddWorkspaceMember(t.Context(), ws.PublicID, 12, tenancy.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/workspaces/" + ws.PublicID + "/default-member-quota"

	status, _, payload := doV1(t, fx.engine, v1Call{
		method: http.MethodPut, path: path, userID: 12,
		body: `{"data":{"period":"monthly","limit_credits":2000}}`,
	})
	if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeBillingAccountForbidden) {
		t.Fatalf("space-only PUT default = %d %v, want 403 billing_account_forbidden", status, payload)
	}
}

func TestV1DefaultQuotaSetValidation(t *testing.T) {
	fx := newDefaultQuotaInviteFixture(t)
	ws := fx.workspace(t, "defvalid", 11)
	path := "/api/v1/workspaces/" + ws.PublicID + "/default-member-quota"

	for _, tc := range []struct{ name, body string }{
		{"monthly without limit", `{"data":{"period":"monthly"}}`},
		{"daily zero", `{"data":{"period":"daily","limit_credits":0}}`},
		{"unlimited with limit", `{"data":{"period":"unlimited","limit_credits":100}}`},
		{"unknown period", `{"data":{"period":"yearly"}}`},
		{"missing period", `{"data":{}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _, payload := doV1(t, fx.engine, v1Call{method: http.MethodPut, path: path, userID: 11, body: tc.body})
			if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
				t.Fatalf("PUT %s = %d %v, want 422 validation_failed", tc.name, status, payload)
			}
		})
	}
}

func TestV1InviteBlockedWithoutDefault(t *testing.T) {
	fx := newDefaultQuotaInviteFixture(t)
	ws := fx.workspace(t, "invblock", 11)

	status, payload := fx.invite(t, ws.PublicID, 11, 12, "member", "inv-block-1")
	if status != http.StatusConflict || v1ErrorCode(t, payload) != string(contract.CodeDefaultMemberQuotaNotConfigured) {
		t.Fatalf("invite without default = %d %v, want 409 default_member_quota_not_configured", status, payload)
	}
	// Fail closed leaves no membership behind.
	if _, member, _ := fx.store.GetWorkspaceRole(context.Background(), ws.PublicID, 12); member {
		t.Fatal("blocked invite must not create a membership")
	}
	account := fx.account(t, ws)
	if _, _, exists, err := fx.quotas.GetQuotaRow(t.Context(), account, 12); err != nil || exists {
		t.Fatalf("blocked invite quota exists=%v err=%v, want no row", exists, err)
	}
	if _, err := fx.quotas.SetDefaultPolicy(t.Context(), account, memberquotas.PeriodMonthly, ptrInt64(1000), 11); err != nil {
		t.Fatal(err)
	}
	// A rejected attempt does not consume the key: configuring the default
	// allows the original request to succeed with the same idempotency key.
	if status, payload := fx.invite(t, ws.PublicID, 11, 12, "member", "inv-block-1"); status != http.StatusCreated {
		t.Fatalf("same-key retry after configuration = %d %v, want 201", status, payload)
	}
}

func TestV1InviteQuotaSeedFailureRollsBackAndRetries(t *testing.T) {
	fx := newDefaultQuotaInviteFixture(t)
	ws := fx.workspace(t, "invrollback", 11)
	account := fx.account(t, ws)
	if _, err := fx.quotas.SetDefaultPolicy(t.Context(), account, memberquotas.PeriodMonthly, ptrInt64(1000), 11); err != nil {
		t.Fatal(err)
	}
	// Fail only the quota insert in this isolated schema, after the canonical
	// AddWorkspaceMember transaction has inserted its membership.
	if _, err := fx.db.Exec(t.Context(), `CREATE FUNCTION reject_invite_quota() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'quota insert unavailable'; END $$; CREATE TRIGGER reject_invite_quota BEFORE INSERT ON billing_account_member_quotas FOR EACH ROW EXECUTE FUNCTION reject_invite_quota()`); err != nil {
		t.Fatal(err)
	}
	status, payload := fx.invite(t, ws.PublicID, 11, 12, "member", "inv-rollback")
	if status != http.StatusServiceUnavailable || v1ErrorCode(t, payload) != string(contract.CodeServiceUnavailable) {
		t.Fatalf("failed quota seed = %d %v, want 503 service_unavailable", status, payload)
	}
	if _, exists, err := fx.store.GetWorkspaceRole(t.Context(), ws.PublicID, 12); err != nil || exists {
		t.Fatalf("membership after failure exists=%v err=%v, want no membership", exists, err)
	}
	if _, _, exists, err := fx.quotas.GetQuotaRow(t.Context(), account, 12); err != nil || exists {
		t.Fatalf("quota after failure exists=%v err=%v, want no quota", exists, err)
	}
	if _, err := fx.db.Exec(t.Context(), `DROP TRIGGER reject_invite_quota ON billing_account_member_quotas`); err != nil {
		t.Fatal(err)
	}
	if status, payload := fx.invite(t, ws.PublicID, 11, 12, "member", "inv-rollback"); status != http.StatusCreated {
		t.Fatalf("same-key retry after storage recovery = %d %v, want 201", status, payload)
	}
}

func TestV1InviteSeedsQuotaFromDefault(t *testing.T) {
	fx := newDefaultQuotaInviteFixture(t)
	ws := fx.workspace(t, "invseed", 11)
	account := fx.account(t, ws)
	if _, err := fx.quotas.SetDefaultPolicy(context.Background(), account, memberquotas.PeriodMonthly, ptrInt64(1000), 11); err != nil {
		t.Fatal(err)
	}

	status, payload := fx.invite(t, ws.PublicID, 11, 12, "member", "inv-seed-1")
	if status != http.StatusCreated {
		t.Fatalf("invite with default = %d %v, want 201", status, payload)
	}
	period, limit, ok, err := fx.quotas.GetQuotaRow(context.Background(), account, 12)
	if err != nil || !ok || period != memberquotas.PeriodMonthly || limit == nil || *limit != 1000 {
		t.Fatalf("seeded quota = %q %v %v %v, want monthly 1000", period, limit, ok, err)
	}
	// The member reads the seeded cap on the self endpoint.
	self := "/api/v1/workspaces/" + ws.PublicID + "/member-quotas/self"
	status, _, payload = doV1(t, fx.engine, v1Call{method: http.MethodGet, path: self, userID: 12})
	if status != http.StatusOK {
		t.Fatalf("GET self after seed = %d %v, want 200", status, payload)
	}
	if data := v1Data(t, payload).(map[string]any); data["period"] != "monthly" || data["limit_credits"] != float64(1000) {
		t.Fatalf("self after seed = %v, want monthly 1000", data)
	}
}

func TestV1InviteDuplicatePreservesQuota(t *testing.T) {
	fx := newDefaultQuotaInviteFixture(t)
	ws := fx.workspace(t, "invdup", 11)
	account := fx.account(t, ws)
	if _, err := fx.quotas.SetDefaultPolicy(context.Background(), account, memberquotas.PeriodMonthly, ptrInt64(1000), 11); err != nil {
		t.Fatal(err)
	}
	if status, _ := fx.invite(t, ws.PublicID, 11, 12, "member", "inv-dup-1"); status != http.StatusCreated {
		t.Fatalf("first invite = %d, want 201", status)
	}
	// Billing manager tunes the member away from the default (usage kept).
	weekly := int64(200)
	if _, err := fx.quotas.SetQuota(context.Background(), account, 12, memberquotas.PeriodWeekly, &weekly, 11); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.quotas.SetDefaultPolicy(t.Context(), account, memberquotas.PeriodFixed, ptrInt64(2000), 11); err != nil {
		t.Fatal(err)
	}
	// Changing the default only affects later joins; retrying this existing
	// membership must retain its tuned quota, independently of the new policy.
	status, payload := fx.invite(t, ws.PublicID, 11, 12, "member", "inv-dup-2")
	if status != http.StatusCreated {
		t.Fatalf("duplicate invite = %d %v, want 201", status, payload)
	}
	period, limit, ok, err := fx.quotas.GetQuotaRow(context.Background(), account, 12)
	if err != nil || !ok || period != memberquotas.PeriodWeekly || limit == nil || *limit != 200 {
		t.Fatalf("quota after duplicate = %q %v %v %v, want weekly 200 preserved", period, limit, ok, err)
	}
	// Existing membership remains idempotent even if the default is later
	// absent: this request does not create a new member or issue a new cap.
	if _, err := fx.db.Exec(t.Context(), `DELETE FROM billing_account_default_member_quotas WHERE billing_account_id = $1`, account); err != nil {
		t.Fatal(err)
	}
	if status, payload := fx.invite(t, ws.PublicID, 11, 12, "member", "inv-dup-3"); status != http.StatusCreated {
		t.Fatalf("duplicate without default = %d %v, want 201", status, payload)
	}
}

func TestV1InviteSharedAccountSecondSpaceKeepsFirstRow(t *testing.T) {
	fx := newDefaultQuotaInviteFixture(t)
	ws1 := fx.workspace(t, "invshare1", 11)
	ws2 := fx.workspace(t, "invshare2", 11)
	account := fx.account(t, ws1)
	// Pin the second workspace to the same shared account (studio sharing).
	if err := fx.accounts.LinkWorkspaceToBillingAccount(t.Context(), ws2.InternalID, account, 11); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.quotas.SetDefaultPolicy(context.Background(), account, memberquotas.PeriodMonthly, ptrInt64(1000), 11); err != nil {
		t.Fatal(err)
	}
	if status, _ := fx.invite(t, ws1.PublicID, 11, 12, "member", "inv-share-1"); status != http.StatusCreated {
		t.Fatalf("first-space invite = %d, want 201", status)
	}
	daily := int64(200)
	if _, err := fx.quotas.SetQuota(context.Background(), account, 12, memberquotas.PeriodDaily, &daily, 11); err != nil {
		t.Fatal(err)
	}
	if status, _ := fx.invite(t, ws2.PublicID, 11, 12, "member", "inv-share-2"); status != http.StatusCreated {
		t.Fatalf("second-space invite = %d, want 201", status)
	}
	period, limit, ok, err := fx.quotas.GetQuotaRow(context.Background(), account, 12)
	if err != nil || !ok || period != memberquotas.PeriodDaily || limit == nil || *limit != 200 {
		t.Fatalf("quota after shared second join = %q %v %v %v, want daily 200 preserved", period, limit, ok, err)
	}
}

func TestV1InviteRemoveAndRejoinKeepsHistory(t *testing.T) {
	fx := newDefaultQuotaInviteFixture(t)
	ws := fx.workspace(t, "invrejoin", 11)
	account := fx.account(t, ws)
	if _, err := fx.quotas.SetDefaultPolicy(context.Background(), account, memberquotas.PeriodMonthly, ptrInt64(1000), 11); err != nil {
		t.Fatal(err)
	}
	if status, _ := fx.invite(t, ws.PublicID, 11, 12, "member", "inv-rejoin-1"); status != http.StatusCreated {
		t.Fatalf("first invite = %d, want 201", status)
	}
	fixed := int64(2000)
	if _, err := fx.quotas.SetQuota(context.Background(), account, 12, memberquotas.PeriodFixed, &fixed, 11); err != nil {
		t.Fatal(err)
	}
	fx.pendingUsage(t, ws, account, 12, 250)
	// Remove from the last workspace: quota row + usage history survive,
	// billing facts stay readable under the account (no deletion here).
	fx.dir.ensure(12)
	status, _, payload := doV1(t, fx.engine, v1Call{
		method: http.MethodDelete, path: "/api/v1/workspaces/" + ws.PublicID + "/members/" + tenancy.FormatUserPublicID(fx.dir.ensure(12)),
		userID: 11, headers: map[string]string{contract.IdempotencyKeyHeader: "inv-rejoin-remove"},
	})
	if status != http.StatusOK {
		t.Fatalf("remove = %d %v, want 200", status, payload)
	}
	if _, _, ok, _ := fx.quotas.GetQuotaRow(context.Background(), account, 12); !ok {
		t.Fatal("quota row must survive removal (retained, frozen)")
	}
	if used, _ := fx.quotas.UsageForPeriod(context.Background(), account, 12, nil, nil); used != 250 {
		t.Fatalf("usage after removal = %d, want 250 retained", used)
	}
	// Rejoin restores eligibility without re-issuing (same cap, same usage).
	if status, _ := fx.invite(t, ws.PublicID, 11, 12, "member", "inv-rejoin-2"); status != http.StatusCreated {
		t.Fatalf("rejoin invite = %d, want 201", status)
	}
	period, limit, ok, err := fx.quotas.GetQuotaRow(context.Background(), account, 12)
	if err != nil || !ok || period != memberquotas.PeriodFixed || limit == nil || *limit != 2000 {
		t.Fatalf("quota after rejoin = %q %v %v %v, want fixed 2000 restored", period, limit, ok, err)
	}
	if used, _ := fx.quotas.UsageForPeriod(context.Background(), account, 12, nil, nil); used != 250 {
		t.Fatalf("usage after rejoin = %d, want 250 restored", used)
	}
}

func TestV1InviteConcurrentJoinsCollapse(t *testing.T) {
	fx := newDefaultQuotaInviteFixture(t)
	ws := fx.workspace(t, "invconc", 11)
	account := fx.account(t, ws)
	if _, err := fx.quotas.SetDefaultPolicy(context.Background(), account, memberquotas.PeriodMonthly, ptrInt64(1000), 11); err != nil {
		t.Fatal(err)
	}
	targets := []int64{12, 42, 88, 90}
	var wg sync.WaitGroup
	statuses := make([]int, len(targets))
	for i := range targets {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			target := targets[slot]
			status, _ := fx.invite(t, ws.PublicID, 11, target, "member", fmt.Sprintf("inv-conc-%d", slot))
			statuses[slot] = status
		}(i)
	}
	wg.Wait()
	for i, status := range statuses {
		if status != http.StatusCreated {
			t.Fatalf("concurrent invite %d = %d, want 201", i, status)
		}
	}
	for _, target := range targets {
		if _, _, ok, _ := fx.quotas.GetQuotaRow(context.Background(), account, target); !ok {
			t.Fatalf("quota for %d missing after concurrent joins", target)
		}
	}
}

func ptrInt64(v int64) *int64 { return &v }
