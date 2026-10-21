package v1

// Workspace shared budget handler tests (issue #945).
//
// Fresh-DB friendly: in-memory tenancy store + fakes only, no testdb.
// Behavioral coverage: unlimited default read, monthly cap + usage +
// remaining read, hidden-workspace 404, Billing Owner/Admin writes,
// space-only admin 403 (a Workspace admin cannot raise a pool-granted
// budget; API and UI permissions agree), and set-input 422 shapes.

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/testsupport"
	"ideaboard/api/internal/workspacebudgets"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type fakeWorkspaceBudgets struct {
	mu     sync.Mutex
	period map[int64]string
	limit  map[int64]*int64
	used   map[int64]int64
}

func (f *fakeWorkspaceBudgets) GetBudgetRow(_ context.Context, workspaceInternalID int64) (string, *int64, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	period, ok := f.period[workspaceInternalID]
	if !ok {
		return workspacebudgets.PeriodUnlimited, nil, false, nil
	}
	return period, f.limit[workspaceInternalID], true, nil
}

func (f *fakeWorkspaceBudgets) SetBudget(_ context.Context, workspaceInternalID int64, period string, limitCredits *int64, _ int64) (workspacebudgets.SetResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := workspacebudgets.ValidateSet(period, limitCredits); err != nil {
		return workspacebudgets.SetResult{}, err
	}
	if period == "" {
		period = workspacebudgets.PeriodUnlimited
	}
	if f.period == nil {
		f.period = make(map[int64]string)
		f.limit = make(map[int64]*int64)
	}
	f.period[workspaceInternalID] = period
	f.limit[workspaceInternalID] = limitCredits
	return workspacebudgets.SetResult{Period: period, LimitCredits: limitCredits}, nil
}

func (f *fakeWorkspaceBudgets) UsageForPeriod(_ context.Context, workspaceInternalID int64, _, _ *time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.used[workspaceInternalID], nil
}

func newBudgetTestFixture() (*billingTestFixture, *fakeWorkspaceBudgets) {
	fx := newBillingTestFixture()
	budgets := &fakeWorkspaceBudgets{period: make(map[int64]string), limit: make(map[int64]*int64), used: make(map[int64]int64)}
	fx.deps.Budgets = budgets
	return fx, budgets
}

func TestV1BudgetDefaultsUnlimited(t *testing.T) {
	ctx := context.Background()
	fx, _ := newBudgetTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "budget", 11)
	engine := fx.engine()
	path := "/api/v1/workspaces/" + ws.PublicID + "/budget"

	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: path, userID: 11})
	if status != http.StatusOK {
		t.Fatalf("GET budget status = %d (%v)", status, payload)
	}
	data := v1Data(t, payload).(map[string]any)
	if data["workspace_id"] != ws.PublicID || data["period"] != "unlimited" {
		t.Fatalf("budget = %v, want unlimited default", data)
	}
	if _, ok := data["limit_credits"]; ok {
		t.Fatalf("unlimited budget must omit limit_credits: %v", data)
	}
	if _, ok := data["remaining_credits"]; ok {
		t.Fatalf("unlimited budget must omit remaining_credits: %v", data)
	}
	if data["used_credits"] != float64(0) {
		t.Fatalf("used_credits = %v, want 0", data["used_credits"])
	}
}

func TestV1BudgetReadsCapAndUsage(t *testing.T) {
	ctx := context.Background()
	fx, budgets := newBudgetTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "budget", 11)
	account := fx.mustAccount(t, ctx, ws)
	budgets.period[account] = workspacebudgets.PeriodMonthly
	limit := int64(1000)
	budgets.limit[account] = &limit
	budgets.used[account] = 250
	engine := fx.engine()
	path := "/api/v1/workspaces/" + ws.PublicID + "/budget"

	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: path, userID: 11})
	if status != http.StatusOK {
		t.Fatalf("GET budget status = %d (%v)", status, payload)
	}
	data := v1Data(t, payload).(map[string]any)
	if data["period"] != "monthly" || data["limit_credits"] != float64(1000) || data["used_credits"] != float64(250) || data["remaining_credits"] != float64(750) {
		t.Fatalf("budget = %v, want monthly 1000/250/750", data)
	}
	if _, ok := data["period_start"].(string); !ok {
		t.Fatalf("monthly budget must carry period_start: %v", data)
	}
	if _, ok := data["period_end"].(string); !ok {
		t.Fatalf("monthly budget must carry period_end: %v", data)
	}
	if data["billing_account_id"] != "ba_test_"+ws.PublicID {
		t.Fatalf("billing_account_id = %v, want the stable payer", data["billing_account_id"])
	}
}

func TestV1BudgetHiddenWorkspace(t *testing.T) {
	ctx := context.Background()
	fx, _ := newBudgetTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "budget", 11)
	engine := fx.engine()
	path := "/api/v1/workspaces/" + ws.PublicID + "/budget"

	// Outsider (no membership): hidden on both read and write.
	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: path, userID: 99})
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("outsider GET = %d %v, want 404 workspace_not_found", status, payload)
	}
	status, _, payload = doV1(t, engine, v1Call{method: http.MethodPut, path: path, userID: 99, body: `{"data":{"period":"monthly","limit_credits":100}}`})
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("outsider PUT = %d %v, want 404 workspace_not_found", status, payload)
	}
}

func TestV1BudgetOwnerSetsAndReadsBack(t *testing.T) {
	ctx := context.Background()
	fx, _ := newBudgetTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "budget", 11)
	engine := fx.engine()
	path := "/api/v1/workspaces/" + ws.PublicID + "/budget"

	status, _, payload := doV1(t, engine, v1Call{method: http.MethodPut, path: path, userID: 11, body: `{"data":{"period":"monthly","limit_credits":500}}`})
	if status != http.StatusOK {
		t.Fatalf("PUT budget status = %d (%v)", status, payload)
	}
	data := v1Data(t, payload).(map[string]any)
	if data["period"] != "monthly" || data["limit_credits"] != float64(500) || data["used_credits"] != float64(0) || data["remaining_credits"] != float64(500) {
		t.Fatalf("budget after set = %v, want monthly 500/0/500", data)
	}

	// Lowering below usage keeps usage: remaining clamps at zero and the
	// next submit (not this write) is what the cap blocks.
	status, _, payload = doV1(t, engine, v1Call{method: http.MethodPut, path: path, userID: 11, body: `{"data":{"period":"unlimited"}}`})
	if status != http.StatusOK {
		t.Fatalf("PUT unlimited status = %d (%v)", status, payload)
	}
	if data := v1Data(t, payload).(map[string]any); data["period"] != "unlimited" {
		t.Fatalf("budget after unlimit = %v, want unlimited", data)
	}
}

func TestV1BudgetSpaceAdminCannotRaise(t *testing.T) {
	ctx := context.Background()
	fx, _ := newBudgetTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "budget", 11)
	account := fx.mustAccount(t, ctx, ws)
	// The caller is the workspace owner but holds no Billing grant:
	// space-only standing never manages the pool-granted budget.
	fx.accounts.roles = map[int64]map[int64]string{account: {}}
	engine := fx.engine()
	path := "/api/v1/workspaces/" + ws.PublicID + "/budget"

	status, _, payload := doV1(t, engine, v1Call{method: http.MethodPut, path: path, userID: 11, body: `{"data":{"period":"monthly","limit_credits":500}}`})
	if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeBillingAccountForbidden) {
		t.Fatalf("space-only PUT = %d %v, want 403 billing_account_forbidden", status, payload)
	}
	// Reads stay member-visible: the denial is write-only.
	status, _, payload = doV1(t, engine, v1Call{method: http.MethodGet, path: path, userID: 11})
	if status != http.StatusOK {
		t.Fatalf("space-only GET status = %d (%v)", status, payload)
	}
}

func TestV1BudgetSetValidation(t *testing.T) {
	ctx := context.Background()
	fx, _ := newBudgetTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "budget", 11)
	engine := fx.engine()
	path := "/api/v1/workspaces/" + ws.PublicID + "/budget"

	for _, tc := range []struct {
		name   string
		body   string
		fields []string
	}{
		{"monthly without limit", `{"data":{"period":"monthly"}}`, []string{"limit_credits"}},
		{"monthly zero", `{"data":{"period":"monthly","limit_credits":0}}`, []string{"limit_credits"}},
		{"unlimited with limit", `{"data":{"period":"unlimited","limit_credits":100}}`, []string{"limit_credits"}},
		{"unknown period", `{"data":{"period":"weekly"}}`, []string{"period"}},
		{"missing period", `{"data":{}}`, []string{"period"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _, payload := doV1(t, engine, v1Call{method: http.MethodPut, path: path, userID: 11, body: tc.body})
			if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
				t.Fatalf("PUT %s = %d %v, want 422 validation_failed", tc.name, status, payload)
			}
		})
	}
}

// stubTaskBudgets enforces a scripted budget outcome on the task
// quote/submit seams (in-memory HTTP tests, no database).
type stubTaskBudgets struct {
	quoteErr error
	txErr    error
	quotes   int
	txChecks int
}

func (s *stubTaskBudgets) CheckBudgetForQuote(context.Context, int64, int64, time.Time) error {
	s.quotes++
	return s.quoteErr
}

func (s *stubTaskBudgets) CheckBudgetInTx(context.Context, pgx.Tx, string, int64, time.Time) error {
	s.txChecks++
	return s.txErr
}

// newBudgetTaskEngine rebuilds the task surface of fx with the budget
// seams installed: the shared fixture registers them nil (unlimited).
func newBudgetTaskEngine(fx *v1TaskFixture, budgets *stubTaskBudgets) *gin.Engine {
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
		Tasks: fx.txTasks, TaskLifecycle: fx.txTasks,
		TaskQuoter: fx.quoter, TaskHolder: fx.holder, TaskQueue: fx.queue, TaskAssets: fx.assets,
		TaskRouter:               allowTestPoolResolver(),
		TaskSnapshotSaver:        v1TxSnapshotSaver(fx.saved),
		TaskBillingAccounts:      v1FakeBillingAccounts{},
		TaskBudgets:              budgets,
		TaskQuoteBillingAccounts: v1FakeBillingAccounts{},
		TaskQuoteWallets:         fx.wallets,
		TaskQuoteBudgets:         budgets,
		TaskAcceptance:           &v1TxSubmissionAcceptance{assets: fx.assets},
	})
	return router
}

func TestV1TaskQuoteBudgetExhausted(t *testing.T) {
	t.Parallel()
	fx := newV1TaskFixture(t)
	fx.wallets.available = 1000000
	budgets := &stubTaskBudgets{quoteErr: workspacebudgets.ErrBudgetExhausted}
	engine := newBudgetTaskEngine(fx, budgets)

	status, payload := quoteTask(t, engine, fx.ws.PublicID, 3, quoteRenderBody(""))
	if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceBudgetExhausted) {
		t.Fatalf("budget quote = %d %v, want 403 workspace_budget_exhausted", status, payload)
	}
	if budgets.quotes != 1 {
		t.Fatalf("quote budget checks = %d, want 1", budgets.quotes)
	}
	// Unlimited (nil error) keeps the funded quote green on the same seam.
	budgets.quoteErr = nil
	status, payload = quoteTask(t, engine, fx.ws.PublicID, 3, quoteRenderBody(""))
	if status != http.StatusOK {
		t.Fatalf("unlimited quote = %d %v, want 200", status, payload)
	}
}

func TestV1TaskSubmitBudgetExhausted(t *testing.T) {
	t.Parallel()
	fx := newV1TaskFixture(t)
	budgets := &stubTaskBudgets{txErr: workspacebudgets.ErrBudgetExhausted}
	engine := newBudgetTaskEngine(fx, budgets)

	status, _, payload := doV1(t, engine, v1Call{
		method: http.MethodPost, path: "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks", userID: 3,
		body:    v1CreateTaskBodyJSON("render"),
		headers: map[string]string{contract.IdempotencyKeyHeader: "budget-submit-1"},
	})
	_ = payload
	if status != http.StatusForbidden {
		t.Fatalf("budget submit = %d %v, want 403", status, payload)
	}
	if code := v1ErrorCode(t, payload); code != string(contract.CodeWorkspaceBudgetExhausted) {
		t.Fatalf("budget submit code = %q, want workspace_budget_exhausted", code)
	}
	if budgets.txChecks != 1 {
		t.Fatalf("submit budget checks = %d, want 1", budgets.txChecks)
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
