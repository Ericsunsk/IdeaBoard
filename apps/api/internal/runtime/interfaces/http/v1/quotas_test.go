package v1

// Member cross-workspace quota handler tests (issue #946).
//
// Fresh-DB friendly: in-memory tenancy store + fakes only, no testdb.
// Behavioral coverage: self unlimited default, self capped read with
// usage + remaining, hidden-workspace 404, Personal Billing Owner writes,
// Billing Admin/space-only admin 403 (a Workspace admin cannot raise a pool-granted
// quota; API and UI permissions agree), manager-only list, set-input 422
// shapes, and task quote/submit quota exhaustion (403
// member_quota_exhausted, distinct from budget/insufficiency).

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/memberquotas"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type fakeMemberQuotas struct {
	mu     sync.Mutex
	period map[quotaKey]string
	limit  map[quotaKey]*int64
	used   map[quotaKey]int64
}

type quotaKey struct {
	account int64
	user    int64
}

func (f *fakeMemberQuotas) GetQuotaRow(_ context.Context, billingAccountID, userID int64) (string, *int64, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	period, ok := f.period[quotaKey{billingAccountID, userID}]
	if !ok {
		return memberquotas.PeriodUnlimited, nil, false, nil
	}
	return period, f.limit[quotaKey{billingAccountID, userID}], true, nil
}

func (f *fakeMemberQuotas) SetQuota(_ context.Context, billingAccountID, userID int64, period string, limitCredits *int64, _ int64) (memberquotas.SetResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := memberquotas.ValidateSet(period, limitCredits); err != nil {
		return memberquotas.SetResult{}, err
	}
	if period == "" {
		period = memberquotas.PeriodUnlimited
	}
	if f.period == nil {
		f.period = make(map[quotaKey]string)
		f.limit = make(map[quotaKey]*int64)
	}
	f.period[quotaKey{billingAccountID, userID}] = period
	f.limit[quotaKey{billingAccountID, userID}] = limitCredits
	return memberquotas.SetResult{Period: period, LimitCredits: limitCredits}, nil
}

func (f *fakeMemberQuotas) ListQuotas(_ context.Context, billingAccountID int64) ([]memberquotas.QuotaRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []memberquotas.QuotaRow
	for key, period := range f.period {
		if key.account != billingAccountID {
			continue
		}
		out = append(out, memberquotas.QuotaRow{
			BillingAccountID: key.account, UserID: key.user,
			Period: period, LimitCredits: f.limit[key],
		})
	}
	return out, nil
}

func (f *fakeMemberQuotas) UsageForPeriod(_ context.Context, billingAccountID, userID int64, _, _ *time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.used[quotaKey{billingAccountID, userID}], nil
}

func (f *fakeMemberQuotas) UsageForPeriodBatch(_ context.Context, billingAccountID int64, userIDs []int64, _, _ *time.Time) (map[int64]int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	used := make(map[int64]int64, len(userIDs))
	for _, id := range userIDs {
		used[id] = f.used[quotaKey{billingAccountID, id}]
	}
	return used, nil
}

func (f *fakeMemberQuotas) setUsed(account, user, used int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.used == nil {
		f.used = make(map[quotaKey]int64)
	}
	f.used[quotaKey{account, user}] = used
}

func newQuotaTestFixture() (*billingTestFixture, *fakeMemberQuotas) {
	fx := newBillingTestFixture()
	quotas := &fakeMemberQuotas{
		period: make(map[quotaKey]string),
		limit:  make(map[quotaKey]*int64),
		used:   make(map[quotaKey]int64),
	}
	fx.deps.Quotas = quotas
	return fx, quotas
}

func quotaUserPublicID(userID int64) string {
	return tenancy.FormatUserPublicID(testUUIDForUser(userID))
}

func TestV1QuotaSelfDefaultsUnlimited(t *testing.T) {
	ctx := context.Background()
	fx, _ := newQuotaTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "quota", 11)
	engine := fx.engine()
	path := "/api/v1/workspaces/" + ws.PublicID + "/member-quotas/self"

	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: path, userID: 11})
	if status != http.StatusOK {
		t.Fatalf("GET self quota status = %d (%v)", status, payload)
	}
	data := v1Data(t, payload).(map[string]any)
	if data["period"] != "unlimited" || data["user_id"] != quotaUserPublicID(11) {
		t.Fatalf("self quota = %v, want unlimited default for the caller", data)
	}
	if _, ok := data["limit_credits"]; ok {
		t.Fatalf("unlimited quota must omit limit_credits: %v", data)
	}
	if _, ok := data["remaining_credits"]; ok {
		t.Fatalf("unlimited quota must omit remaining_credits: %v", data)
	}
	if data["used_credits"] != float64(0) {
		t.Fatalf("used_credits = %v, want 0", data["used_credits"])
	}
}

func TestV1QuotaSelfReadsCapAndUsage(t *testing.T) {
	ctx := context.Background()
	fx, quotas := newQuotaTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "quota", 11)
	account := fx.mustAccount(t, ctx, ws)
	quotas.period[quotaKey{account, 11}] = memberquotas.PeriodDaily
	limit := int64(100)
	quotas.limit[quotaKey{account, 11}] = &limit
	quotas.setUsed(account, 11, 25)
	engine := fx.engine()
	path := "/api/v1/workspaces/" + ws.PublicID + "/member-quotas/self"

	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: path, userID: 11})
	if status != http.StatusOK {
		t.Fatalf("GET self quota status = %d (%v)", status, payload)
	}
	data := v1Data(t, payload).(map[string]any)
	if data["period"] != "daily" || data["limit_credits"] != float64(100) || data["used_credits"] != float64(25) || data["remaining_credits"] != float64(75) {
		t.Fatalf("self quota = %v, want daily 100/25/75", data)
	}
	if _, ok := data["period_start"].(string); !ok {
		t.Fatalf("daily quota must carry period_start: %v", data)
	}
	if _, ok := data["period_end"].(string); !ok {
		t.Fatalf("daily quota must carry period_end: %v", data)
	}
	if data["billing_account_id"] != "ba_test_"+ws.PublicID {
		t.Fatalf("billing_account_id = %v, want the stable payer", data["billing_account_id"])
	}
}

func TestV1QuotaHiddenWorkspace(t *testing.T) {
	ctx := context.Background()
	fx, _ := newQuotaTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "quota", 11)
	engine := fx.engine()
	self := "/api/v1/workspaces/" + ws.PublicID + "/member-quotas/self"
	list := "/api/v1/workspaces/" + ws.PublicID + "/member-quotas"
	set := "/api/v1/workspaces/" + ws.PublicID + "/member-quotas/" + quotaUserPublicID(11)

	// Outsider (no membership): hidden on every quota endpoint.
	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: self, userID: 99})
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("outsider self = %d %v, want 404 workspace_not_found", status, payload)
	}
	status, _, payload = doV1(t, engine, v1Call{method: http.MethodGet, path: list, userID: 99})
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("outsider list = %d %v, want 404 workspace_not_found", status, payload)
	}
	status, _, payload = doV1(t, engine, v1Call{method: http.MethodPut, path: set, userID: 99, body: `{"data":{"period":"daily","limit_credits":100}}`})
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("outsider set = %d %v, want 404 workspace_not_found", status, payload)
	}
}

func TestV1QuotaOwnerSetsAndReadsBack(t *testing.T) {
	ctx := context.Background()
	fx, _ := newQuotaTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "quota", 11)
	engine := fx.engine()
	set := "/api/v1/workspaces/" + ws.PublicID + "/member-quotas/" + quotaUserPublicID(11)
	self := "/api/v1/workspaces/" + ws.PublicID + "/member-quotas/self"

	status, _, payload := doV1(t, engine, v1Call{method: http.MethodPut, path: set, userID: 11, body: `{"data":{"period":"weekly","limit_credits":500}}`})
	if status != http.StatusOK {
		t.Fatalf("PUT quota status = %d (%v)", status, payload)
	}
	data := v1Data(t, payload).(map[string]any)
	if data["period"] != "weekly" || data["limit_credits"] != float64(500) || data["used_credits"] != float64(0) || data["remaining_credits"] != float64(500) {
		t.Fatalf("quota after set = %v, want weekly 500/0/500", data)
	}

	// The member reads the same cap back on the self endpoint.
	status, _, payload = doV1(t, engine, v1Call{method: http.MethodGet, path: self, userID: 11})
	if status != http.StatusOK {
		t.Fatalf("GET self after set status = %d (%v)", status, payload)
	}
	if data := v1Data(t, payload).(map[string]any); data["period"] != "weekly" || data["limit_credits"] != float64(500) {
		t.Fatalf("self after set = %v, want weekly 500", data)
	}

	// Switching back to unlimited clears the gate but keeps the facts.
	status, _, payload = doV1(t, engine, v1Call{method: http.MethodPut, path: set, userID: 11, body: `{"data":{"period":"unlimited"}}`})
	if status != http.StatusOK {
		t.Fatalf("PUT unlimited status = %d (%v)", status, payload)
	}
	if data := v1Data(t, payload).(map[string]any); data["period"] != "unlimited" {
		t.Fatalf("quota after unlimit = %v, want unlimited", data)
	}
}

func TestV1QuotaSpaceAdminCannotRaise(t *testing.T) {
	ctx := context.Background()
	fx, quotas := newQuotaTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "quota", 11)
	account := fx.mustAccount(t, ctx, ws)
	// The caller is the workspace owner but holds no Billing grant:
	// space-only standing never manages the pool-granted quota.
	fx.accounts.roles = map[int64]map[int64]string{account: {}}
	quotas.period[quotaKey{account, 11}] = memberquotas.PeriodDaily
	limit := int64(100)
	quotas.limit[quotaKey{account, 11}] = &limit
	engine := fx.engine()
	set := "/api/v1/workspaces/" + ws.PublicID + "/member-quotas/" + quotaUserPublicID(11)

	status, _, payload := doV1(t, engine, v1Call{method: http.MethodPut, path: set, userID: 11, body: `{"data":{"period":"daily","limit_credits":500}}`})
	if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeBillingAccountForbidden) {
		t.Fatalf("space-only PUT = %d %v, want 403 billing_account_forbidden", status, payload)
	}
	// The account-wide list is manager-only too...
	status, _, payload = doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + ws.PublicID + "/member-quotas", userID: 11})
	if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeBillingAccountForbidden) {
		t.Fatalf("space-only list = %d %v, want 403 billing_account_forbidden", status, payload)
	}
	// ...while the self read stays member-visible: the denial is
	// management-only.
	status, _, payload = doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + ws.PublicID + "/member-quotas/self", userID: 11})
	if status != http.StatusOK {
		t.Fatalf("space-only self status = %d (%v)", status, payload)
	}
}

func TestV1QuotaListShowsManagerView(t *testing.T) {
	ctx := context.Background()
	fx, quotas := newQuotaTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "quota", 11)
	account := fx.mustAccount(t, ctx, ws)
	fx.dir.ensure(12)
	quotas.period[quotaKey{account, 11}] = memberquotas.PeriodMonthly
	monthly := int64(1000)
	quotas.limit[quotaKey{account, 11}] = &monthly
	quotas.period[quotaKey{account, 12}] = memberquotas.PeriodFixed
	fixed := int64(200)
	quotas.limit[quotaKey{account, 12}] = &fixed
	quotas.setUsed(account, 11, 250)
	quotas.setUsed(account, 12, 200)
	engine := fx.engine()
	path := "/api/v1/workspaces/" + ws.PublicID + "/member-quotas"

	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: path, userID: 11})
	if status != http.StatusOK {
		t.Fatalf("GET list status = %d (%v)", status, payload)
	}
	items, ok := v1Data(t, payload).([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("quota list = %v, want 2 rows", payload)
	}
	byUser := map[string]map[string]any{}
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("quota row = %v, want object", item)
		}
		user, _ := row["user_id"].(string)
		byUser[user] = row
	}
	self := byUser[quotaUserPublicID(11)]
	if self["period"] != "monthly" || self["limit_credits"] != float64(1000) || self["used_credits"] != float64(250) || self["remaining_credits"] != float64(750) {
		t.Fatalf("manager row = %v, want monthly 1000/250/750", self)
	}
	other := byUser[quotaUserPublicID(12)]
	// Fixed cap reached exactly: remaining clamps at zero.
	if other["period"] != "fixed" || other["limit_credits"] != float64(200) || other["used_credits"] != float64(200) || other["remaining_credits"] != float64(0) {
		t.Fatalf("fixed row = %v, want fixed 200/200/0", other)
	}
	if _, ok := other["period_start"]; ok {
		t.Fatalf("fixed quota must omit period_start: %v", other)
	}
}

func TestV1QuotaSetValidation(t *testing.T) {
	ctx := context.Background()
	fx, _ := newQuotaTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "quota", 11)
	engine := fx.engine()
	set := "/api/v1/workspaces/" + ws.PublicID + "/member-quotas/" + quotaUserPublicID(11)

	for _, tc := range []struct {
		name string
		body string
	}{
		{"daily without limit", `{"data":{"period":"daily"}}`},
		{"weekly zero", `{"data":{"period":"weekly","limit_credits":0}}`},
		{"unlimited with limit", `{"data":{"period":"unlimited","limit_credits":100}}`},
		{"unknown period", `{"data":{"period":"yearly"}}`},
		{"missing period", `{"data":{}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _, payload := doV1(t, engine, v1Call{method: http.MethodPut, path: set, userID: 11, body: tc.body})
			if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
				t.Fatalf("PUT %s = %d %v, want 422 validation_failed", tc.name, status, payload)
			}
		})
	}
}

func TestV1QuotaSetUnknownUser(t *testing.T) {
	ctx := context.Background()
	fx, _ := newQuotaTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "quota", 11)
	engine := fx.engine()
	// Never-ensured usr_ identity: unknown target, never a quota status.
	set := "/api/v1/workspaces/" + ws.PublicID + "/member-quotas/" + quotaUserPublicID(77)

	status, _, payload := doV1(t, engine, v1Call{method: http.MethodPut, path: set, userID: 11, body: `{"data":{"period":"daily","limit_credits":100}}`})
	if status != http.StatusNotFound {
		t.Fatalf("PUT unknown user = %d %v, want 404", status, payload)
	}
}

// stubTaskQuotas enforces a scripted quota outcome on the task
// quote/submit seams (in-memory HTTP tests, no database).
type stubTaskQuotas struct {
	quoteErr error
	txErr    error
	quotes   int
	txChecks int
}

func (s *stubTaskQuotas) CheckQuotaForQuote(_ context.Context, _, _ int64, _ int64, _ time.Time) error {
	s.quotes++
	return s.quoteErr
}

func (s *stubTaskQuotas) CheckQuotaInTx(_ context.Context, _ pgx.Tx, _, _ int64, _ int64, _ time.Time) error {
	s.txChecks++
	return s.txErr
}

// newQuotaTaskEngine rebuilds the task surface of fx with the quota
// seams installed: the shared fixture registers them nil (unlimited).
func newQuotaTaskEngine(fx *v1TaskFixture, quotas *stubTaskQuotas) *gin.Engine {
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
		id := fx.dir.ensure(userID)
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
		Store: fx.store, Users: fx.dir,
		Tasks: fx.txTasks, TaskLifecycle: fx.txTasks, TaskSubmissionClaims: fx.txTasks,
		TaskQuoter: fx.quoter, TaskHolder: fx.holder, TaskQueue: fx.queue, TaskAssets: fx.assets,
		TaskRouter:               allowTestPoolResolver(),
		TaskSnapshotSaver:        v1TxSnapshotSaver(fx.saved),
		TaskBillingAccounts:      v1FakeBillingAccounts{},
		TaskQuotas:               quotas,
		TaskQuoteBillingAccounts: v1FakeBillingAccounts{},
		TaskQuoteWallets:         fx.wallets,
		TaskQuoteQuotas:          quotas,
		TaskAcceptance:           &v1TxSubmissionAcceptance{assets: fx.assets},
	})
	return router
}

func TestV1TaskQuoteQuotaExhausted(t *testing.T) {
	t.Parallel()
	fx := newV1TaskFixture(t)
	fx.wallets.available = 1000000
	quotas := &stubTaskQuotas{quoteErr: memberquotas.ErrQuotaExhausted}
	engine := newQuotaTaskEngine(fx, quotas)

	status, payload := quoteTask(t, engine, fx.ws.PublicID, 3, quoteRenderBody(""))
	if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeMemberQuotaExhausted) {
		t.Fatalf("quota quote = %d %v, want 403 member_quota_exhausted", status, payload)
	}
	if quotas.quotes != 1 {
		t.Fatalf("quote quota checks = %d, want 1", quotas.quotes)
	}
	// Unlimited (nil error) keeps the funded quote green on the same seam.
	quotas.quoteErr = nil
	status, payload = quoteTask(t, engine, fx.ws.PublicID, 3, quoteRenderBody(""))
	if status != http.StatusOK {
		t.Fatalf("unlimited quote = %d %v, want 200", status, payload)
	}
}

func TestV1TaskSubmitQuotaExhausted(t *testing.T) {
	t.Parallel()
	fx := newV1TaskFixture(t)
	quotas := &stubTaskQuotas{txErr: memberquotas.ErrQuotaExhausted}
	engine := newQuotaTaskEngine(fx, quotas)

	status, _, payload := doV1(t, engine, v1Call{
		method: http.MethodPost, path: "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks", userID: 3,
		body:    v1CreateTaskBodyJSON("render"),
		headers: map[string]string{contract.IdempotencyKeyHeader: "quota-submit-1"},
	})
	_ = payload
	if status != http.StatusForbidden {
		t.Fatalf("quota submit = %d %v, want 403", status, payload)
	}
	if code := v1ErrorCode(t, payload); code != string(contract.CodeMemberQuotaExhausted) {
		t.Fatalf("quota submit code = %q, want member_quota_exhausted", code)
	}
	if quotas.txChecks != 1 {
		t.Fatalf("submit quota checks = %d, want 1", quotas.txChecks)
	}
	// The blocked submit leaves no task behind.
	status, _, list := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks", userID: 3})
	if status != http.StatusOK {
		t.Fatalf("list after blocked submit = %d", status)
	}
	if items, _ := v1Data(t, list).([]any); len(items) != 0 {
		t.Fatalf("tasks after blocked submit = %d, want 0 (no residue)", len(items))
	}
}

func TestV1QuotaPersonalBillingAdminCannotManage(t *testing.T) {
	ctx := context.Background()
	fx, quotas := newQuotaTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "quota", 11)
	account := fx.mustAccount(t, ctx, ws)
	fx.accounts.roles = map[int64]map[int64]string{account: {11: tenancy.BillingRoleAdmin}}
	engine := fx.engine()
	path := "/api/v1/workspaces/" + ws.PublicID + "/member-quotas"
	for _, call := range []v1Call{
		{method: http.MethodGet, path: path, userID: 11},
		{method: http.MethodPut, path: path + "/" + quotaUserPublicID(11), userID: 11, body: `{"data":{"period":"daily","limit_credits":100}}`},
	} {
		status, _, payload := doV1(t, engine, call)
		if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeBillingAccountForbidden) {
			t.Fatalf("Personal Billing Admin %s = %d %v, want 403 billing_account_forbidden", call.method, status, payload)
		}
	}
	if len(quotas.period) != 0 {
		t.Fatal("denied quota write must not persist a row")
	}
}

type quotaRoleLookupFailure struct {
	tenancy.Store
	failedUserID int64
}

func (s quotaRoleLookupFailure) GetOrganizationRole(ctx context.Context, organizationID string, userID int64) (string, bool, error) {
	if userID == s.failedUserID {
		return "", false, errors.New("role lookup failed")
	}
	return s.Store.GetOrganizationRole(ctx, organizationID, userID)
}

func TestV1QuotaSetRoleLookupFailureDoesNotWrite(t *testing.T) {
	ctx := context.Background()
	fx, quotas := newQuotaTestFixture()
	org, err := fx.store.CreateOrganization(ctx, 11, "quota-org")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := fx.store.CreateOrganizationWorkspace(ctx, org.PublicID, 11, "quota-workspace")
	if err != nil {
		t.Fatal(err)
	}
	fx.dir.ensure(12)
	fx.deps.Tenancy = quotaRoleLookupFailure{Store: fx.store, failedUserID: 12}
	status, _, payload := doV1(t, fx.engine(), v1Call{
		method: http.MethodPut, userID: 11,
		path: "/api/v1/workspaces/" + ws.PublicID + "/member-quotas/" + quotaUserPublicID(12),
		body: `{"data":{"period":"daily","limit_credits":100}}`,
	})
	if status != http.StatusServiceUnavailable || v1ErrorCode(t, payload) != string(contract.CodeServiceUnavailable) {
		t.Fatalf("target lookup failure = %d %v, want 503 service_unavailable", status, payload)
	}
	if len(quotas.period) != 0 {
		t.Fatal("role lookup failure must not persist a quota row")
	}
}
