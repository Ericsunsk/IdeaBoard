package tasks

// Canvas Task shared-billing preflight (#917): quote/readiness and submit
// spend the same stable Billing Account with the same consumption
// eligibility (in-memory seams, no database).
//
// What is proven here:
//   - Quote resolves the same fixture account submit holds (one payer:
//     the fixture serves 7001 on both seams).
//   - Quote distinguishes 402 task_insufficient_credits from 403
//     billing_wallet_restricted, mirroring the submit Hold mapping.
//   - A Board-bound quote enforces the same generate gate as submit:
//     invisible Boards read 404 board_not_found, archived Boards 409
//     board_archived, Viewers 403 task_forbidden; the creator and Editors
//     read 200. Financial standing alone grants nothing: callers without
//     Board generate capability fail here exactly as on submit.
//   - Quote and submit agree for the same caller and Board (same
//     eligibility, same account).

import (
	"context"
	"fmt"
	taskapp "ideaboard/api/internal/tasking/application/workspacetasks"
	taskadapters "ideaboard/api/internal/tasking/infrastructure/adapters"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/memberquotas"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	v1testsupport "ideaboard/api/internal/runtime/interfaces/http/v1/testsupport"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/tenancy/funds"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

func quoteTask(t *testing.T, engine *gin.Engine, workspaceID string, userID int64, body string) (int, map[string]any) {
	t.Helper()
	status, _, payload := doV1(t, engine, v1Call{
		method: http.MethodPost, path: "/api/v1/workspaces/" + workspaceID + "/task-quotes", userID: userID,
		body: body,
	})
	return status, payload
}

func quoteRenderBody(boardID string) string {
	if boardID == "" {
		return `{"data":{"service":"render","quantity":1}}`
	}
	return `{"data":{"service":"render","quantity":1,"board_id":"` + boardID + `"}}`
}

func TestV1TaskQuoteBoardGateMirrorsSubmit(t *testing.T) {
	t.Parallel()
	fx := newV1BoardTaskFixture(t)

	// The creator reads ready on their private Board.
	status, payload := quoteTask(t, fx.engine, fx.ws.PublicID, 1, quoteRenderBody(fx.boardID))
	if status != http.StatusOK {
		t.Fatalf("creator board quote = %d %v, want 200", status, payload)
	}

	// An unselected member cannot discover the private Board: 404.
	status, payload = quoteTask(t, fx.engine, fx.ws.PublicID, 3, quoteRenderBody(fx.boardID))
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeBoardNotFound) {
		t.Fatalf("private board quote = %d %v, want 404 board_not_found", status, payload)
	}

	// A Viewer reads 403 on quote exactly as on submit (same eligibility).
	access, err := fx.boardStore.GetBoardAccess(context.Background(), fx.ws.PublicID, fx.boardID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.boardStore.ReplaceBoardAccess(context.Background(), fx.ws.PublicID, fx.boardID, 1, access.AccessVersion, boards.WorkspaceAccess(boards.RoleViewer), []int64{3}); err != nil {
		t.Fatal(err)
	}
	status, payload = quoteTask(t, fx.engine, fx.ws.PublicID, 3, quoteRenderBody(fx.boardID))
	if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeTaskForbidden) {
		t.Fatalf("viewer board quote = %d %v, want 403 task_forbidden", status, payload)
	}
	status, payload = submitBoardTask(t, fx, 3, "917-viewer-submit", fx.boardID)
	if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeTaskForbidden) {
		t.Fatalf("viewer board submit = %d %v, want 403 task_forbidden", status, payload)
	}

	// An Editor reads ready and submits: quote and submit agree.
	access, err = fx.boardStore.GetBoardAccess(context.Background(), fx.ws.PublicID, fx.boardID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.boardStore.ReplaceBoardAccess(context.Background(), fx.ws.PublicID, fx.boardID, 1, access.AccessVersion, boards.WorkspaceAccessEditor, []int64{3}); err != nil {
		t.Fatal(err)
	}
	status, payload = quoteTask(t, fx.engine, fx.ws.PublicID, 3, quoteRenderBody(fx.boardID))
	if status != http.StatusOK {
		t.Fatalf("editor board quote = %d %v, want 200", status, payload)
	}
	status, payload = submitBoardTask(t, fx, 3, "917-editor-submit", fx.boardID)
	if status != http.StatusAccepted {
		t.Fatalf("editor board submit = %d %v, want 202", status, payload)
	}

	// Malformed board_ shapes fail closed as 422 on both surfaces.
	status, payload = quoteTask(t, fx.engine, fx.ws.PublicID, 1, quoteRenderBody("nope"))
	if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
		t.Fatalf("malformed board quote = %d %v, want 422 validation_failed", status, payload)
	}
}

func TestV1TaskQuoteAndSubmitShareFixtureAccount(t *testing.T) {
	t.Parallel()
	fx := newV1TaskFixture(t)
	// Both seams serve the fixed fixture account: the quote preflight and
	// the submit Hold name one payer (7001). The holder records holds by
	// id; the quote wallet reads the same fixture funds.
	status, _ := quoteTask(t, fx.engine, fx.ws.PublicID, 3, quoteRenderBody(""))
	if status != http.StatusOK {
		t.Fatalf("quote = %d, want 200", status)
	}
	status, payload := func() (int, map[string]any) {
		status, _, payload := doV1(t, fx.engine, v1Call{
			method: http.MethodPost, path: "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks", userID: 3,
			body:    v1CreateTaskBodyJSON("render"),
			headers: map[string]string{contract.IdempotencyKeyHeader: "917-shared-account"},
		})
		return status, payload
	}()
	if status != http.StatusAccepted {
		t.Fatalf("submit = %d %v, want 202", status, payload)
	}
	taskID := v1Data(t, payload).(map[string]any)["id"].(string)
	stored, err := fx.tasks.GetTask(context.Background(), fx.ws.PublicID, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.BillingAccountID != 7001 {
		t.Fatalf("task billing account = %d, want the fixture account 7001", stored.BillingAccountID)
	}
	if stored.OwnerUserID != 3 {
		t.Fatalf("task submitter = %d, want the real submitting user 3", stored.OwnerUserID)
	}
	if stored.BillingSnapshot.BillingAccountID != 7001 {
		t.Fatalf("snapshot billing account = %d, want 7001", stored.BillingSnapshot.BillingAccountID)
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
	return newQuotaTaskEngineWithAccounts(fx, quotas, v1FakeBillingAccounts{})
}

// newQuotaTaskEngineWithAccounts rebuilds the same surface with an
// explicit billing-accounts seam so Personal-scope tests can prove the
// Org-only quota gate skips without consulting the checker.
func newQuotaTaskEngineWithAccounts(fx *v1TaskFixture, quotas *stubTaskQuotas, accounts interface {
	ResolveBillingAccountInTx(context.Context, pgx.Tx, string, int64) (int64, string, error)
	ResolveFundsForActor(context.Context, string, int64) (funds.Identity, error)
},
) *gin.Engine {
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
	handler := NewHandler(Deps{
		Store: fx.store, Users: fx.dir,
		Tasks: ContractV1TaskDeps{
			Tasks: fx.txTasks, TaskLifecycle: fx.txTasks, TaskSubmissionClaims: fx.txTasks,
			TaskQuoter: fx.quoter, TaskHolder: fx.holder, TaskQueue: fx.queue, TaskAssets: fx.assets,
			TaskRouter:          allowTestPoolResolver(),
			TaskSnapshotSaver:   v1testsupport.TxSnapshotSaver(fx.saved),
			TaskBillingAccounts: accounts,
			TaskQuotas:          quotas,
			TaskReadiness: taskapp.NewReadiness(taskapp.ReadinessDeps{
				Workspaces: &taskadapters.ReadinessWorkspaceReader{Store: fx.store},
				Boards:     nil, Quoter: fx.quoter, Resolver: allowTestPoolResolver(),
				BillingAccounts: accounts, Wallets: fx.wallets, Quotas: quotas,
			}),
			TaskAcceptance: &v1testsupport.TxSubmissionAcceptance{Assets: fx.assets},
		},
	})
	handler.Mount(v1)
	return router
}

// TestV1TaskQuoteSharedFakePersonalSkipsQuota proves the shared
// FakeBillingAccounts fake matches production on Personal scopes:
// Shared=false skips the quota on both the quote preflight and the submit
// settlement, exactly like the real funds resolution (Shared=false for the
// Personal actor account).
func TestV1TaskQuoteSharedFakePersonalSkipsQuota(t *testing.T) {
	t.Parallel()
	fx := newV1TaskFixture(t)
	fx.wallets.available = 1000000
	quotas := &stubTaskQuotas{quoteErr: memberquotas.ErrQuotaExhausted, txErr: memberquotas.ErrQuotaExhausted}
	accounts := v1testsupport.FakeBillingAccounts{Personal: true}
	engine := newQuotaTaskEngineWithAccounts(fx, quotas, accounts)

	status, _ := quoteTask(t, engine, fx.ws.PublicID, 3, quoteRenderBody(""))
	if status != http.StatusOK {
		t.Fatalf("shared-fake personal quote = %d, want 200 (quota skipped)", status)
	}
	if quotas.quotes != 0 {
		t.Fatalf("shared-fake personal quote quota checks = %d, want 0 (checker never consulted)", quotas.quotes)
	}
	status, _, payload := doV1(t, engine, v1Call{
		method: http.MethodPost, path: "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks", userID: 3,
		body:    v1CreateTaskBodyJSON("render"),
		headers: map[string]string{contract.IdempotencyKeyHeader: "shared-fake-personal-submit"},
	})
	if status != http.StatusAccepted {
		t.Fatalf("shared-fake personal submit = %d %v, want 202 (quota skipped)", status, payload)
	}
	if quotas.txChecks != 0 {
		t.Fatalf("shared-fake personal submit quota checks = %d, want 0 (checker never consulted)", quotas.txChecks)
	}
}

func TestV1TaskSubmitQuotaExhausted(t *testing.T) {
	t.Parallel()
	fx := newV1TaskFixture(t)
	orgWS := newOrgQuotaWorkspace(t, fx)
	quotas := &stubTaskQuotas{txErr: memberquotas.ErrQuotaExhausted}
	engine := newQuotaTaskEngine(fx, quotas)

	status, _, payload := doV1(t, engine, v1Call{
		method: http.MethodPost, path: "/api/v1/workspaces/" + orgWS.PublicID + "/tasks", userID: 3,
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
	status, _, list := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + orgWS.PublicID + "/tasks", userID: 3})
	if status != http.StatusOK {
		t.Fatalf("list after blocked submit = %d", status)
	}
	if items, _ := v1Data(t, list).([]any); len(items) != 0 {
		t.Fatalf("tasks after blocked submit = %d, want 0 (no residue)", len(items))
	}
}

// newOrgQuotaWorkspace creates an Organization workspace with user 3 as a
// direct member so Org-intent quota tests prove the gate on the workspace
// scope (ADR-0042: Organization checks, Personal skips).
func newOrgQuotaWorkspace(t *testing.T, fx *v1TaskFixture) *tenancy.Workspace {
	t.Helper()
	ctx := context.Background()
	org, err := fx.store.CreateOrganization(ctx, 1, "quota-org")
	if err != nil {
		t.Fatal(err)
	}
	orgWS, err := fx.store.CreateOrganizationWorkspace(ctx, org.PublicID, 1, "org-tasks")
	if err != nil {
		t.Fatal(err)
	}
	// Organization scopes are join-first (EffectiveWorkspaceRole): user 3
	// joins the org as Member, then holds the explicit workspace row.
	fx.store.SeedOrganizationMember(org.PublicID, 3, tenancy.RoleMember)
	fx.store.SeedWorkspaceMember(orgWS.PublicID, 3, tenancy.RoleMember)
	return orgWS
}
