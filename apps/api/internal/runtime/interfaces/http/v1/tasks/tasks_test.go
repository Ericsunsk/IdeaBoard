package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/db/dbutil/testscope"
	"ideaboard/api/internal/generation/routing"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/prompts"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	v1testsupport "ideaboard/api/internal/runtime/interfaces/http/v1/testsupport"
	taskservice "ideaboard/api/internal/tasking/application/task"
	taskdomain "ideaboard/api/internal/tasking/domain"
	taskmemory "ideaboard/api/internal/tasking/infrastructure/memory"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/tenancy/funds"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
	"ideaboard/api/internal/testsupport"
	"ideaboard/api/internal/workflow/paramkeys"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// ---- merged from tasks_helpers_test.go ----
type fakeTaskQuoter struct {
	unitCredits int64
	calls       int
	err         error
}

// allowTestPoolResolver serves every service/resolution from one enabled
// entry (#854: replaces the legacy allowTestRouting gate; the resolver is
// the only availability seam).

func allowTestPoolResolver() *routing.RoutingResolver {
	seed := routing.SelectionSeed{4, 5, 6}
	return routing.NewRoutingResolver(allowAllV1PoolStore{},
		routing.WithSeedGenerator(func() (routing.SelectionSeed, error) { return seed, nil }))
}

type allowAllV1PoolStore struct{}

func (allowAllV1PoolStore) ListPool(_ context.Context, service string) (routing.Pool, error) {
	return routing.Pool{Service: service, Version: 1, Entries: []routing.ModelPoolEntry{
		{Provider: "openrouter", Model: "test-model", Enabled: true, SupportedResolutions: []routing.Resolution{
			routing.Resolution1K, routing.Resolution2K, routing.Resolution4K,
		}},
	}}, nil
}

func (allowAllV1PoolStore) ListPoolInTx(_ context.Context, _ pgx.Tx, service string) (routing.Pool, error) {
	return allowAllV1PoolStore{}.ListPool(context.Background(), service)
}

func (f *fakeTaskQuoter) Quote(ctx context.Context, kind, resolution string, quantity int64) (taskdomain.BillingSnapshot, error) {
	f.calls++
	if f.err != nil {
		return taskdomain.BillingSnapshot{}, f.err
	}
	if quantity <= 0 {
		quantity = 1
	}
	unit := f.unitCredits
	if unit == 0 {
		unit = 10
	}
	return taskdomain.BillingSnapshot{
		SKU: "ai-image:" + kind, DisplayName: kind, PriceVersionID: 1, PriceVersion: 1,
		OutputResolution: resolution, UnitCredits: unit, Quantity: quantity,
		TotalCredits: unit * quantity, QuotedAt: time.Now().UTC(), SettlementState: "held",
	}, nil
}

// QuoteInTx serves the settlement-transaction seam: quoting has no durable
// side effects, so no rollback hook is needed.
func (f *fakeTaskQuoter) QuoteInTx(ctx context.Context, _ pgx.Tx, kind, resolution string, quantity int64) (taskdomain.BillingSnapshot, error) {
	return f.Quote(ctx, kind, resolution, quantity)
}

// v1FakeBillingAccounts serves the stable funds-identity seam from a fixed
// fixture account for in-memory settlement tests.
type v1FakeBillingAccounts struct{}

func (v1FakeBillingAccounts) ResolveBillingAccountInTx(_ context.Context, _ pgx.Tx, _ string) (int64, error) {
	return 7001, nil
}

// ResolveFunds serves the quote-preflight resolution from the same fixed
// fixture account submit holds (#917: one payer for quote, readiness,
// and submit in these fixtures).
func (v1FakeBillingAccounts) ResolveFunds(_ context.Context, workspacePublicID string) (funds.Identity, error) {
	return funds.Identity{
		WorkspaceInternalID:    7001,
		WorkspacePublicID:      workspacePublicID,
		BillingAccountID:       7001,
		BillingAccountPublicID: "ba_quote0000001",
		OwnerType:              tenancypg.BillingOwnerWorkspace,
	}, nil
}

// v1FakeQuoteWallets serves the quote-preflight funds gate from a
// configurable fixture balance. Tests mutate available/restrictions/err
// to prove the 402-vs-403 split; the default funds every fixture quote.
type v1FakeQuoteWallets struct {
	mu           sync.Mutex
	available    int64
	reserved     int64
	restrictions []string
	err          error
}

func newV1FakeQuoteWallets() *v1FakeQuoteWallets {
	return &v1FakeQuoteWallets{available: 1000000}
}

func (f *v1FakeQuoteWallets) GetWallet(_ context.Context, _ int64) (domain.WalletDetails, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return domain.WalletDetails{}, f.err
	}
	details := domain.WalletDetails{Status: domain.WalletStatusActive}
	details.Available = f.available
	details.Reserved = f.reserved
	for _, restrictionType := range f.restrictions {
		details.ActiveRestrictions = append(details.ActiveRestrictions, domain.WalletRestriction{Type: restrictionType})
	}
	return details, nil
}

type fakeTaskHolder struct {
	holds    map[string]int64
	captured map[string]int64
	released map[string]int64
	holdErr  error
}

func newFakeTaskHolder() *fakeTaskHolder {
	return &fakeTaskHolder{holds: map[string]int64{}, captured: map[string]int64{}, released: map[string]int64{}}
}

func (f *fakeTaskHolder) Hold(ctx context.Context, billingAccountID, actorUserID int64, amount int64, key, taskID string) (string, error) {
	if f.holdErr != nil {
		return "", f.holdErr
	}
	holdID := "hold_" + taskID
	f.holds[holdID] = amount
	return holdID, nil
}

func (f *fakeTaskHolder) Capture(ctx context.Context, billingAccountID, actorUserID int64, holdID string, amount int64, taskID string) error {
	f.captured[holdID] = amount
	return nil
}

func (f *fakeTaskHolder) Release(ctx context.Context, billingAccountID, actorUserID int64, holdID string, amount int64, taskID string) error {
	f.released[holdID] = amount
	return nil
}

// HoldInTx serves the settlement-transaction seam from the same maps and
// registers its rollback undo, so a rejected submit strands no hold.
// CaptureInTx/ReleaseInTx delegate to the same call recording.
func (f *fakeTaskHolder) HoldInTx(ctx context.Context, tx pgx.Tx, billingAccountID, actorUserID int64, amount int64, key, taskID string) (string, error) {
	holdID, err := f.Hold(ctx, billingAccountID, actorUserID, amount, key, taskID)
	if err != nil {
		return "", err
	}
	if holdID != "" {
		testscope.OnRollback(tx, func() {
			delete(f.holds, holdID)
		})
	}
	return holdID, nil
}

func (f *fakeTaskHolder) CaptureInTx(ctx context.Context, _ pgx.Tx, billingAccountID, actorUserID int64, holdID string, amount int64, taskID string) (string, error) {
	if err := f.Capture(ctx, billingAccountID, actorUserID, holdID, amount, taskID); err != nil {
		return "", err
	}
	return "", nil
}

func (f *fakeTaskHolder) ReleaseInTx(ctx context.Context, _ pgx.Tx, billingAccountID, actorUserID int64, holdID string, amount int64, taskID string) (string, error) {
	if err := f.Release(ctx, billingAccountID, actorUserID, holdID, amount, taskID); err != nil {
		return "", err
	}
	return "", nil
}

type fakeTaskQueue struct {
	enqueued []*taskdomain.WorkspaceTask
	failNext error
}

func (f *fakeTaskQueue) Enqueue(ctx context.Context, task *taskdomain.WorkspaceTask) error {
	if f.failNext != nil {
		err := f.failNext
		f.failNext = nil
		return err
	}
	f.enqueued = append(f.enqueued, task)
	return nil
}

// EnqueueInTx serves the settlement-transaction seam from the same queue
// and registers its rollback undo.
func (f *fakeTaskQueue) EnqueueInTx(ctx context.Context, tx pgx.Tx, task *taskdomain.WorkspaceTask) error {
	if err := f.Enqueue(ctx, task); err != nil {
		return err
	}
	testscope.OnRollback(tx, func() {
		for i, enqueued := range f.enqueued {
			if enqueued == task {
				f.enqueued = append(f.enqueued[:i], f.enqueued[i+1:]...)
				return
			}
		}
	})
	return nil
}

type fakeTaskAssets struct {
	assets map[string]*taskdomain.AssetInfo
}

func (f *fakeTaskAssets) GetAsset(ctx context.Context, workspaceID, assetID string) (*taskdomain.AssetInfo, error) {
	info, ok := f.assets[workspaceID+"\x00"+assetID]
	if !ok {
		return nil, taskdomain.ErrMediaNotFound
	}
	return info, nil
}

type v1TaskFixture struct {
	engine  *gin.Engine
	store   *tenancy.InMemoryStore
	dir     *staticUserDirectory
	ws      *tenancy.Workspace
	tasks   *taskmemory.InMemoryStore
	txTasks *v1testsupport.TxTaskStore
	quoter  *fakeTaskQuoter
	holder  *fakeTaskHolder
	queue   *fakeTaskQueue
	assets  *fakeTaskAssets
	wallets *v1FakeQuoteWallets
	saved   map[string]routing.RoutingSnapshot
}

func newV1TaskFixture(t *testing.T, pools ...map[string]routing.Pool) *v1TaskFixture {
	t.Helper()
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	tasks := taskmemory.NewInMemoryStoreForTests()
	txTasks := v1testsupport.NewTxTaskStore(tasks)
	quoter := &fakeTaskQuoter{}
	holder := newFakeTaskHolder()
	queue := &fakeTaskQueue{}
	assets := &fakeTaskAssets{assets: map[string]*taskdomain.AssetInfo{}}
	wallets := newV1FakeQuoteWallets()
	saved := make(map[string]routing.RoutingSnapshot)
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
	v1 := router.Group(runtimehttp.ContractV1GroupPath)
	// An explicit pool map serves quote/submit through the shared
	// RoutingResolver; the default fixture allows every
	// service/resolution via one enabled entry. An empty pool map
	// converges to 422 service_resolution_unavailable.
	resolver := allowTestPoolResolver()
	if len(pools) > 0 {
		resolver = routing.NewRoutingResolver(&v1StubPoolStore{pools: pools[0]})
	}
	handler := NewHandler(Deps{
		Store: store, Users: dir,
		Tasks: ContractV1TaskDeps{
			Tasks: txTasks, TaskLifecycle: txTasks, TaskSubmissionClaims: txTasks,
			TaskQuoter: quoter, TaskHolder: holder, TaskQueue: queue, TaskAssets: assets,
			TaskRouter:          resolver,
			TaskSnapshotSaver:   v1testsupport.TxSnapshotSaver(saved),
			TaskBillingAccounts: v1FakeBillingAccounts{},
			// #917: the preflight resolves the same fixture account submit
			// holds and gates the same fixture funds.
			TaskQuoteBillingAccounts: v1FakeBillingAccounts{},
			TaskQuoteWallets:         wallets,
			TaskAcceptance:           &v1testsupport.TxSubmissionAcceptance{Assets: assets},
		},
	})
	handler.Mount(v1)
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "tasks", OwnerUserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	store.SeedWorkspaceMember(ws.PublicID, 2, tenancy.RoleAdmin)
	store.SeedWorkspaceMember(ws.PublicID, 3, tenancy.RoleMember)
	for _, id := range []int64{1, 2, 3, 9} {
		dir.ensure(id)
	}
	return &v1TaskFixture{engine: router, store: store, dir: dir, ws: ws, tasks: tasks, txTasks: txTasks, quoter: quoter, holder: holder, queue: queue, assets: assets, wallets: wallets, saved: saved}
}

func v1CreateTaskBodyJSON(kind string) string {
	return fmt.Sprintf(`{"data":{"service":%q,"inputs":{"media_asset_ids":[]},"params":{}}}`, kind)
}

type v1StubPoolStore struct {
	pools map[string]routing.Pool
}

func (s *v1StubPoolStore) ListPool(ctx context.Context, service string) (routing.Pool, error) {
	if p, ok := s.pools[service]; ok {
		return p, nil
	}
	return routing.Pool{Service: service}, nil
}

func (s *v1StubPoolStore) ListPoolInTx(ctx context.Context, _ pgx.Tx, service string) (routing.Pool, error) {
	return s.ListPool(ctx, service)
}

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

// ---- merged from tasks_lifecycle_test.go ----
func TestV1TaskRoutingUnavailableBeforeQuoteOrHold(t *testing.T) {
	t.Parallel()
	// An empty pool converges to the canonical 422 before any quote,
	// hold, task, or enqueue work (#854: no legacy gate).
	fx := newV1TaskFixture(t, map[string]routing.Pool{})
	base := "/api/v1/workspaces/" + fx.ws.PublicID
	for _, tc := range []struct{ path, body string }{
		{base + "/task-quotes", `{"data":{"service":"render","quantity":1}}`},
		{base + "/tasks", v1CreateTaskBodyJSON("render")},
	} {
		status, _, payload := doV1(t, fx.engine, v1Call{
			method: http.MethodPost, path: tc.path, userID: 3, body: tc.body,
			headers: map[string]string{contract.IdempotencyKeyHeader: "routing-unavailable"},
		})
		if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeServiceResolutionUnavailable) {
			t.Fatalf("%s = %d %v, want 422 service_resolution_unavailable", tc.path, status, payload)
		}
	}
	count, _ := fx.tasks.CountTasks(context.Background(), fx.ws.PublicID)
	if count != 0 || len(fx.holder.holds) != 0 || len(fx.queue.enqueued) != 0 || fx.quoter.calls != 0 {
		t.Fatalf("routing rejection created work: tasks=%d holds=%v queued=%d quotes=%d", count, fx.holder.holds, len(fx.queue.enqueued), fx.quoter.calls)
	}
}

func TestV1TaskCreateAcceptedWithLocation(t *testing.T) {
	t.Parallel()
	fx := newV1TaskFixture(t)
	tasksPath := "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks"
	status, header, payload := doV1RoundTrip(t, fx.engine, v1Call{
		method: http.MethodPost, path: tasksPath, userID: 3,
		body:    v1CreateTaskBodyJSON("render"),
		headers: map[string]string{contract.IdempotencyKeyHeader: "task-key-1"},
	}, "test-task-")
	if status != http.StatusAccepted {
		t.Fatalf("create = %d %s, want 202", status, payload)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	data := v1Data(t, parsed)
	task, _ := data.(map[string]any)
	taskID, _ := task["id"].(string)
	if !contract.IsTaskID(taskID) {
		t.Fatalf("task id = %q, want task_...", taskID)
	}
	location := header.Get("Location")
	wantLocation := "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks/" + taskID
	if location != wantLocation {
		t.Fatalf("Location = %q, want %q", location, wantLocation)
	}
	if task["status"] != "pending" {
		t.Fatalf("status = %v, want pending", task["status"])
	}
	if task["retry_count"].(float64) != 0 {
		t.Fatalf("retry_count = %v, want 0", task["retry_count"])
	}
	if task["execution_token"] == "" {
		t.Fatal("execution_token must persist")
	}
	snapshot, _ := task["billing_snapshot"].(map[string]any)
	if snapshot["sku"] != "ai-image:render" {
		t.Fatalf("sku = %v, want ai-image:render", snapshot["sku"])
	}
	if snapshot["settlement_state"] != "held" {
		t.Fatalf("settlement = %v, want held", snapshot["settlement_state"])
	}
}

func TestV1TaskRejectsLegacyWorkflowKindField(t *testing.T) {
	t.Parallel()
	// #847 breaking rename: `service` replaces `workflow_kind` with no
	// compatibility alias. Strict decoding rejects the old field with 400
	// bad_request (existing CodeBadRequest, no new error code); a missing
	// `service` renders 422 validation_failed on details.fields.service.
	fx := newV1TaskFixture(t)
	tasksPath := "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks"
	quotePath := "/api/v1/workspaces/" + fx.ws.PublicID + "/task-quotes"
	for _, tc := range []struct{ name, path, body string }{
		{"create old-field-only", tasksPath, `{"data":{"workflow_kind":"render","inputs":{"media_asset_ids":[]},"params":{}}}`},
		{"create dual-field", tasksPath, `{"data":{"service":"render","workflow_kind":"render","inputs":{"media_asset_ids":[]},"params":{}}}`},
		{"quote old-field-only", quotePath, `{"data":{"workflow_kind":"render","quantity":1}}`},
		{"quote dual-field", quotePath, `{"data":{"service":"render","workflow_kind":"render","quantity":1}}`},
	} {
		status, _, payload := doV1(t, fx.engine, v1Call{
			method: http.MethodPost, path: tc.path, userID: 3,
			body: tc.body, headers: map[string]string{contract.IdempotencyKeyHeader: "legacy-kind-" + tc.name},
		})
		if status != http.StatusBadRequest || v1ErrorCode(t, payload) != string(contract.CodeBadRequest) {
			t.Fatalf("%s = %d %v, want 400 bad_request", tc.name, status, payload)
		}
	}
	// Neither field binds, then fails validation on details.fields.service.
	status, _, payload := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: tasksPath, userID: 3,
		body: `{"data":{"inputs":{"media_asset_ids":[]},"params":{}}}`, headers: map[string]string{contract.IdempotencyKeyHeader: "legacy-kind-neither"},
	})
	if status != 422 || v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
		t.Fatalf("missing service = %d %v, want 422 validation_failed", status, payload)
	}
	// No task was created by any rejected submit.
	count, _ := fx.tasks.CountTasks(context.Background(), fx.ws.PublicID)
	if count != 0 {
		t.Fatalf("rejected submits persisted %d tasks, want 0", count)
	}
}

func TestV1TaskQuoteCreateSeparation(t *testing.T) {
	t.Parallel()
	fx := newV1TaskFixture(t)
	quotePath := "/api/v1/workspaces/" + fx.ws.PublicID + "/task-quotes"
	status, _, quoted := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: quotePath, userID: 3,
		body: `{"data":{"service":"render","quantity":2}}`,
	})
	if status != 200 {
		t.Fatalf("quote = %d %v, want 200", status, quoted)
	}
	snapshot := v1Data(t, quoted).(map[string]any)
	if snapshot["total_credits"].(float64) != 20 {
		t.Fatalf("total = %v, want 20 (10x2)", snapshot["total_credits"])
	}
	if snapshot["settlement_state"] != "quoted" {
		t.Fatalf("quote state = %v, want quoted (no persistence)", snapshot["settlement_state"])
	}
	count, _ := fx.tasks.CountTasks(context.Background(), fx.ws.PublicID)
	if count != 0 {
		t.Fatalf("quote persisted %d tasks, want 0", count)
	}
	if len(fx.queue.enqueued) != 0 {
		t.Fatalf("quote enqueued %d jobs, want 0", len(fx.queue.enqueued))
	}
	// Authoritative create persists exactly one task + one River job.
	status, _, created := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks", userID: 3,
		body:    v1CreateTaskBodyJSON("render"),
		headers: map[string]string{contract.IdempotencyKeyHeader: "quote-sep-1"},
	})
	if status != 202 {
		t.Fatalf("create = %d %v, want 202", status, created)
	}
	count, _ = fx.tasks.CountTasks(context.Background(), fx.ws.PublicID)
	if count != 1 || len(fx.queue.enqueued) != 1 {
		t.Fatalf("tasks=%d queue=%d, want 1/1", count, len(fx.queue.enqueued))
	}
}

func TestV1TaskAtomicityRollbackProof(t *testing.T) {
	t.Parallel()
	fx := newV1TaskFixture(t)
	tasksPath := "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks"
	fx.queue.failNext = fmt.Errorf("river down")
	status, _, payload := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: tasksPath, userID: 3,
		body:    v1CreateTaskBodyJSON("render"),
		headers: map[string]string{contract.IdempotencyKeyHeader: "atomic-1"},
	})
	if status != http.StatusServiceUnavailable {
		t.Fatalf("failing enqueue = %d %v, want 503", status, payload)
	}
	count, _ := fx.tasks.CountTasks(context.Background(), fx.ws.PublicID)
	if count != 0 {
		t.Fatalf("tasks = %d, want 0 (enqueue+hold+insert rolled back)", count)
	}
	if len(fx.holder.holds) != 0 {
		t.Fatalf("holds = %v, want none (rollback removes the hold)", fx.holder.holds)
	}
	if len(fx.holder.released) != 0 {
		t.Fatalf("released = %v, want none (no compensating release on rollback)", fx.holder.released)
	}
}

func TestV1TaskIdempotentReplayAndConflict(t *testing.T) {
	t.Parallel()
	fx := newV1TaskFixture(t)
	tasksPath := "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks"
	post := func(key, body string) (int, map[string]any) {
		status, _, payload := doV1(t, fx.engine, v1Call{
			method: http.MethodPost, path: tasksPath, userID: 3,
			body: body, headers: map[string]string{contract.IdempotencyKeyHeader: key},
		})
		return status, payload
	}
	status, first := post("idem-1", v1CreateTaskBodyJSON("render"))
	if status != 202 {
		t.Fatalf("create = %d %v, want 202", status, first)
	}
	firstID := first["data"].(map[string]any)["id"].(string)
	status, replay := post("idem-1", v1CreateTaskBodyJSON("render"))
	if status != 202 || replay["data"].(map[string]any)["id"] != firstID {
		t.Fatalf("replay = %d %v, want same task_id %s", status, replay, firstID)
	}
	status, conflict := post("idem-1", v1CreateTaskBodyJSON("upscale"))
	if status != 409 || v1ErrorCode(t, conflict) != string(contract.CodeTaskIdempotencyConflict) {
		t.Fatalf("conflict = %d %v, want 409 task_idempotency_conflict", status, conflict)
	}
	status, missing := post("", v1CreateTaskBodyJSON("render"))
	_ = missing
	if status != 400 {
		// Missing key renders 400 via requireV1IdempotencyKey; the
		// doV1 helper still asserts envelope + Request-Id.
		t.Fatalf("missing key = %d, want 400", status)
	}
}

func TestV1TaskCancel(t *testing.T) {
	t.Parallel()
	fx := newV1TaskFixture(t)
	tasksPath := "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks"
	status, _, created := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: tasksPath, userID: 3,
		body:    v1CreateTaskBodyJSON("render"),
		headers: map[string]string{contract.IdempotencyKeyHeader: "cancel-setup"},
	})
	if status != 202 {
		t.Fatalf("create = %d %v", status, created)
	}
	taskID := created["data"].(map[string]any)["id"].(string)
	cancelPath := tasksPath + "/" + taskID + "/cancel"
	status, _, cancelled := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: cancelPath, userID: 3,
		body:    `{"data":{"reason":"no longer needed"}}`,
		headers: map[string]string{contract.IdempotencyKeyHeader: "cancel-1"},
	})
	if status != 200 {
		t.Fatalf("cancel = %d %v, want 200", status, cancelled)
	}
	if v1Data(t, cancelled).(map[string]any)["status"] != "cancelled" {
		t.Fatalf("status = %v, want cancelled", cancelled)
	}
	// Replay with same key+hash returns the same cancelled task.
	status, _, replay := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: cancelPath, userID: 3,
		body:    `{"data":{"reason":"no longer needed"}}`,
		headers: map[string]string{contract.IdempotencyKeyHeader: "cancel-1"},
	})
	if status != 200 || replay["data"].(map[string]any)["id"] != taskID {
		t.Fatalf("cancel replay = %d %v, want same task", status, replay)
	}
	// Different hash conflicts.
	status, _, conflict := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: cancelPath, userID: 3,
		body:    `{"data":{"reason":"different"}}`,
		headers: map[string]string{contract.IdempotencyKeyHeader: "cancel-1"},
	})
	if status != 409 || v1ErrorCode(t, conflict) != string(contract.CodeTaskIdempotencyConflict) {
		t.Fatalf("cancel conflict = %d %v, want 409", status, conflict)
	}
}

func TestV1TaskOutputRefs(t *testing.T) {
	t.Parallel()
	fx := newV1TaskFixture(t)
	ctx := context.Background()
	tasksPath := "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks"
	status, _, created := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: tasksPath, userID: 3,
		body:    v1CreateTaskBodyJSON("render"),
		headers: map[string]string{contract.IdempotencyKeyHeader: "outputs-setup"},
	})
	if status != 202 {
		t.Fatalf("create = %d %v", status, created)
	}
	taskID := created["data"].(map[string]any)["id"].(string)
	// Seed a ready asset + a remote output via the store seam.
	fx.assets.assets[fx.ws.PublicID+"\x00asset_ready1"] = &taskdomain.AssetInfo{ID: "asset_ready1", WorkspaceID: fx.ws.PublicID, Status: "ready"}
	checksum := strings.Repeat("b", 64)
	_, err := fx.tasks.AddOutputs(ctx, fx.ws.PublicID, taskID, []taskdomain.OutputRef{
		{Kind: "asset", AssetID: "asset_ready1", ResultIndex: 0},
		{Kind: "remote", URL: "https://cdn.example.com/o.png", ChecksumSHA256: checksum, ResultIndex: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	status, _, outputs := doV1(t, fx.engine, v1Call{
		method: http.MethodGet, path: tasksPath + "/" + taskID + "/outputs", userID: 3,
	})
	if status != 200 {
		t.Fatalf("outputs = %d %v, want 200", status, outputs)
	}
	items := v1Data(t, outputs).(map[string]any)["outputs"].([]any)
	if len(items) != 2 {
		t.Fatalf("outputs = %d, want 2", len(items))
	}
	first := items[0].(map[string]any)
	if first["kind"] != "asset" || first["asset_id"] != "asset_ready1" {
		t.Fatalf("first output = %v, want asset ref", first)
	}
	serialized := fmt.Sprintf("%v", outputs)
	for _, leaked := range []string{"storage_key", "builtin-local", "ws-uploads", "uploads/ws", "bucket"} {
		if strings.Contains(serialized, leaked) {
			t.Fatalf("outputs leak %q: %s", leaked, serialized)
		}
	}
}

func TestV1TaskAuthMatrix(t *testing.T) {
	t.Parallel()
	fx := newV1TaskFixture(t)
	tasksPath := "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks"
	// Member creates a task for the visibility checks below.
	status, _, created := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: tasksPath, userID: 3,
		body:    v1CreateTaskBodyJSON("render"),
		headers: map[string]string{contract.IdempotencyKeyHeader: "auth-setup"},
	})
	if status != 202 {
		t.Fatalf("create = %d %v", status, created)
	}
	taskID := created["data"].(map[string]any)["id"].(string)
	taskPath := tasksPath + "/" + taskID

	// Outsider (no workspace membership) sees 404 task_not_found, never 403.
	status, _, hidden := doV1(t, fx.engine, v1Call{method: http.MethodGet, path: taskPath, userID: 9})
	if status != 404 || v1ErrorCode(t, hidden) != string(contract.CodeTaskNotFound) {
		t.Fatalf("outsider = %d %v, want 404 task_not_found", status, hidden)
	}
	// Cross-workspace id is invisible even to its own owner.
	other, err := fx.store.CreateWorkspace(context.Background(), tenancy.CreateWorkspaceParams{Name: "other", OwnerUserID: 8})
	if err != nil {
		t.Fatal(err)
	}
	status, _, cross := doV1(t, fx.engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + other.PublicID + "/tasks/" + taskID, userID: 8})
	if status != 404 || v1ErrorCode(t, cross) != string(contract.CodeTaskNotFound) {
		t.Fatalf("cross-workspace = %d %v, want 404 task_not_found", status, cross)
	}
	// Malformed task_ shape is invisible (404, not 400).
	status, _, malformed := doV1(t, fx.engine, v1Call{method: http.MethodGet, path: tasksPath + "/nope", userID: 3})
	if status != 404 || v1ErrorCode(t, malformed) != string(contract.CodeTaskNotFound) {
		t.Fatalf("malformed = %d %v, want 404 task_not_found", status, malformed)
	}
	// Anonymous callers fail closed with 401.
	status, _, anon := doV1(t, fx.engine, v1Call{method: http.MethodGet, path: taskPath})
	if status != 401 || v1ErrorCode(t, anon) != string(contract.CodeUnauthorized) {
		t.Fatalf("anon = %d %v, want 401 unauthorized", status, anon)
	}
	// Outputs inherit task visibility.
	status, _, outHidden := doV1(t, fx.engine, v1Call{method: http.MethodGet, path: taskPath + "/outputs", userID: 9})
	if status != 404 || v1ErrorCode(t, outHidden) != string(contract.CodeTaskNotFound) {
		t.Fatalf("outsider outputs = %d %v, want 404 task_not_found", status, outHidden)
	}
	// Unknown kind + raw URL + unknown field all 422 with details.fields.
	for _, tc := range []struct{ name, body, field string }{
		{"unknown kind", `{"data":{"service":"nope","inputs":{"media_asset_ids":[]},"params":{}}}`, "service"},
		{"raw url", `{"data":{"service":"render","inputs":{"media_asset_ids":["https://x/y.png"]},"params":{}}}`, "media_asset_ids[0]"},
		{"unknown params", `{"data":{"service":"render","inputs":{"media_asset_ids":[]},"params":{"mystery":1}}}`, "params.mystery"},
	} {
		status, _, payload := doV1(t, fx.engine, v1Call{
			method: http.MethodPost, path: tasksPath, userID: 3,
			body: tc.body, headers: map[string]string{contract.IdempotencyKeyHeader: "val-" + tc.name},
		})
		if status != 422 || v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
			t.Fatalf("%s = %d %v, want 422 validation_failed", tc.name, status, payload)
		}
	}
}

func TestV1TaskCreateBillingBranches(t *testing.T) {
	t.Parallel()
	tasksPath := func(fx *v1TaskFixture) string { return "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks" }
	quotePath := func(fx *v1TaskFixture) string { return "/api/v1/workspaces/" + fx.ws.PublicID + "/task-quotes" }
	create := func(fx *v1TaskFixture, key string) (int, map[string]any) {
		status, _, payload := doV1(t, fx.engine, v1Call{
			method: http.MethodPost, path: tasksPath(fx), userID: 3,
			body:    v1CreateTaskBodyJSON("render"),
			headers: map[string]string{contract.IdempotencyKeyHeader: key},
		})
		return status, payload
	}
	quote := func(fx *v1TaskFixture) (int, map[string]any) {
		status, _, payload := doV1(t, fx.engine, v1Call{
			method: http.MethodPost, path: quotePath(fx), userID: 3,
			body: `{"data":{"service":"render","quantity":1}}`,
		})
		return status, payload
	}

	fx := newV1TaskFixture(t)
	// The fake stands in for the production WorkspaceTaskQuoter, which
	// always keeps the ErrQuoteUnavailable routing identity (multi-%w):
	// transient failures carry only that identity, pricing failures
	// carry the catalog cause alongside it.
	fx.quoter.err = fmt.Errorf("workspace task quoter: %w", taskdomain.ErrQuoteUnavailable)
	status, payload := create(fx, "bill-q")
	if status != http.StatusServiceUnavailable || v1ErrorCode(t, payload) != string(contract.CodeBillingQuoteUnavailable) {
		t.Fatalf("quote failure = %d %v, want 503 billing_quote_unavailable", status, payload)
	}
	status, payload = quote(fx)
	if status != http.StatusServiceUnavailable || v1ErrorCode(t, payload) != string(contract.CodeBillingQuoteUnavailable) {
		t.Fatalf("preflight quote failure = %d %v, want 503 billing_quote_unavailable", status, payload)
	}

	// Unpriced/unregistered SKUs are deterministic 422s (no retry),
	// whether the catalog surfaces ErrPricingUnavailable (the production
	// catalog normalizes unknown SKUs to it) or ErrSKUNotFound. The
	// quoter preserves the cause alongside the quote-unavailable
	// identity (multi-%w), and the 422-first branch wins.
	for i, quoteErr := range []error{billing.ErrPricingUnavailable, billing.ErrSKUNotFound} {
		fx = newV1TaskFixture(t)
		fx.quoter.err = fmt.Errorf("workspace task quoter: %w: %w", quoteErr, taskdomain.ErrQuoteUnavailable)
		status, payload = create(fx, fmt.Sprintf("bill-p-%d", i))
		if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeBillingPricingUnavailable) {
			t.Fatalf("pricing failure (%v) = %d %v, want 422 billing_pricing_unavailable", quoteErr, status, payload)
		}
		status, payload = quote(fx)
		if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeBillingPricingUnavailable) {
			t.Fatalf("preflight pricing failure (%v) = %d %v, want 422 billing_pricing_unavailable", quoteErr, status, payload)
		}
	}

	fx = newV1TaskFixture(t)
	fx.holder.holdErr = taskdomain.ErrWalletRestricted
	status, payload = create(fx, "bill-h")
	if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeBillingWalletRestricted) {
		t.Fatalf("hold rejection = %d %v, want 403 billing_wallet_restricted", status, payload)
	}
}

func TestV1TaskStoresNilFailsClosed(t *testing.T) {
	t.Parallel()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
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
		id := dir.ensure(userID)
		identityPrincipal, err := principal.New(userID, id, fmt.Sprintf("auth-%d", userID), "t", "user")
		if err != nil || principal.Set(c, identityPrincipal) != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Next()
	})
	v1 := router.Group(runtimehttp.ContractV1GroupPath)
	handler := NewHandler(Deps{Store: store, Users: dir})
	handler.Mount(v1)
	ws, err := store.CreateWorkspace(context.Background(), tenancy.CreateWorkspaceParams{Name: "nil", OwnerUserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	status, _, payload := doV1(t, router, v1Call{
		method: http.MethodPost, path: "/api/v1/workspaces/" + ws.PublicID + "/tasks", userID: 1,
		body:    v1CreateTaskBodyJSON("render"),
		headers: map[string]string{contract.IdempotencyKeyHeader: "nil-key"},
	})
	if status != 501 || v1ErrorCode(t, payload) != string(contract.CodeContractNotImplemented) {
		t.Fatalf("create without stores = %d %v, want 501", status, payload)
	}
}

// ---- merged from tasks_prompt_mask_test.go ----
// Prompt/mask submit plane (v1 task creation gap batch). Unlike
// newV1TaskFixture, this fixture wires the REAL canonical compiler
// (TaskPreparer: taskservice.NewWorkspacePreparer, production
// parity) so the tests prove submit -> canonical params -> worker input
// end to end at the handler level.
type v1PromptMaskFixture struct {
	*v1TaskFixture
}

func newV1PromptMaskFixture(t *testing.T) *v1PromptMaskFixture {
	return newV1PromptMaskFixtureWithPrompts(t, nil)
}

func newV1PromptMaskFixtureWithPrompts(t *testing.T, reader taskservice.FeaturePromptRuntimeReader) *v1PromptMaskFixture {
	t.Helper()
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	tasks := taskmemory.NewInMemoryStoreForTests()
	txTasks := v1testsupport.NewTxTaskStore(tasks)
	quoter := &fakeTaskQuoter{}
	holder := newFakeTaskHolder()
	queue := &fakeTaskQueue{}
	assets := &fakeTaskAssets{assets: map[string]*taskdomain.AssetInfo{}}
	wallets := newV1FakeQuoteWallets()
	saved := make(map[string]routing.RoutingSnapshot)
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
		id := dir.ensure(userID)
		identityPrincipal, err := principal.New(userID, id,
			fmt.Sprintf("auth-%d", userID), fmt.Sprintf("test-user-%d", userID), "user")
		if err != nil || principal.Set(c, identityPrincipal) != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Next()
	})
	v1 := router.Group(runtimehttp.ContractV1GroupPath)
	handler := NewHandler(Deps{
		Store: store, Users: dir,
		Tasks: ContractV1TaskDeps{
			Tasks: txTasks, TaskLifecycle: txTasks, TaskSubmissionClaims: txTasks,
			TaskQuoter: quoter, TaskHolder: holder, TaskQueue: queue, TaskAssets: assets,
			TaskPreparer:             taskservice.NewWorkspacePreparer(reader),
			TaskRouter:               allowTestPoolResolver(),
			TaskSnapshotSaver:        v1testsupport.TxSnapshotSaver(saved),
			TaskBillingAccounts:      v1FakeBillingAccounts{},
			TaskQuoteBillingAccounts: v1FakeBillingAccounts{},
			TaskQuoteWallets:         wallets,
			TaskAcceptance:           &v1testsupport.TxSubmissionAcceptance{Assets: assets},
		},
	})
	handler.Mount(v1)
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "prompt-mask", OwnerUserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	store.SeedWorkspaceMember(ws.PublicID, 3, tenancy.RoleMember)
	for _, id := range []int64{1, 3} {
		dir.ensure(id)
	}
	fx := &v1TaskFixture{engine: router, store: store, dir: dir, ws: ws, tasks: tasks, txTasks: txTasks, quoter: quoter, holder: holder, queue: queue, assets: assets, saved: saved}
	fx.assets.assets[ws.PublicID+"\x00asset_input1"] = &taskdomain.AssetInfo{ID: "asset_input1", WorkspaceID: ws.PublicID, Status: "ready"}
	fx.assets.assets[ws.PublicID+"\x00asset_mask1"] = &taskdomain.AssetInfo{ID: "asset_mask1", WorkspaceID: ws.PublicID, Status: "ready"}
	return &v1PromptMaskFixture{fx}
}

func (fx *v1PromptMaskFixture) postTask(t *testing.T, key, body string) (int, map[string]any) {
	t.Helper()
	status, _, payload := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks", userID: 3,
		body: body, headers: map[string]string{contract.IdempotencyKeyHeader: key},
	})
	return status, payload
}

func (fx *v1PromptMaskFixture) canonicalParams(t *testing.T, taskID string) *taskdomain.WorkflowTaskParams {
	t.Helper()
	stored, err := fx.tasks.GetTask(context.Background(), fx.ws.PublicID, taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if strings.TrimSpace(stored.CanonicalParams) == "" {
		t.Fatal("canonical params empty, want compiled params")
	}
	typed, err := taskdomain.UnmarshalParams(stored.CanonicalParams)
	if err != nil {
		t.Fatalf("decode canonical params: %v", err)
	}
	return typed
}

// Submit-with-prompt: data.prompt threads into the canonical Generation
// snapshot the worker bridges to the provider; replays converge, prompt
// changes conflict.
func TestV1TaskCreateWithPromptEndToEnd(t *testing.T) {
	fx := newV1PromptMaskFixture(t)
	body := `{"data":{"service":"generation","inputs":{"media_asset_ids":[]},"prompt":"a red bicycle","params":{"output_count":1}}}`
	status, created := fx.postTask(t, "prompt-1", body)
	if status != http.StatusAccepted {
		t.Fatalf("create = %d %v, want 202", status, created)
	}
	taskID := v1Data(t, created).(map[string]any)["id"].(string)
	typed := fx.canonicalParams(t, taskID)
	if typed.Generation == nil {
		t.Fatal("missing generation snapshot")
	}
	if typed.Generation.UserPrompt != "a red bicycle" || typed.Generation.CompiledPrompt != "a red bicycle" {
		t.Fatalf("generation=%+v, want the submitted prompt", typed.Generation)
	}
	// Same key + same body replays the same task.
	status, replay := fx.postTask(t, "prompt-1", body)
	if status != http.StatusAccepted || replay["data"].(map[string]any)["id"] != taskID {
		t.Fatalf("replay = %d %v, want same task %s", status, replay, taskID)
	}
	// Same key + different prompt conflicts (prompt is in the hash).
	other := `{"data":{"service":"generation","inputs":{"media_asset_ids":[]},"prompt":"a blue bicycle","params":{"output_count":1}}}`
	status, conflict := fx.postTask(t, "prompt-1", other)
	if status != http.StatusConflict || v1ErrorCode(t, conflict) != string(contract.CodeTaskIdempotencyConflict) {
		t.Fatalf("prompt change = %d %v, want 409 task_idempotency_conflict", status, conflict)
	}
}

// Prompt boundaries: over-long prompts, missing prompts on required
// features, and params.prompt (still unknown_field) all fail 422.
func TestV1TaskCreatePromptBoundaries(t *testing.T) {
	fx := newV1PromptMaskFixture(t)
	for _, tc := range []struct{ name, body string }{
		{"over-long prompt", fmt.Sprintf(`{"data":{"service":"generation","inputs":{"media_asset_ids":[]},"prompt":%q,"params":{}}}`,
			strings.Repeat("x", taskdomain.MaxWorkspaceTaskPromptBytes+1))},
		{"missing prompt on generation", `{"data":{"service":"generation","inputs":{"media_asset_ids":[]},"params":{}}}`},
		{"params prompt still rejected", `{"data":{"service":"generation","inputs":{"media_asset_ids":[]},"prompt":"ok","params":{"prompt":"stowaway"}}}`},
	} {
		status, payload := fx.postTask(t, "bound-"+tc.name, tc.body)
		if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
			t.Fatalf("%s = %d %v, want 422 validation_failed", tc.name, status, payload)
		}
	}
}

// Inpaint-with-mask: the upload-session mask asset links by id through
// params.mask_asset_id into canonical media facts (typed mask + generation
// intent + plan mask input).
func TestV1TaskCreateInpaintWithMaskEndToEnd(t *testing.T) {
	fx := newV1PromptMaskFixture(t)
	body := `{"data":{"service":"inpaint","inputs":{"media_asset_ids":["asset_input1"]},"prompt":"fill the hole","params":{"mask_asset_id":"asset_mask1"}}}`
	status, created := fx.postTask(t, "inpaint-mask-1", body)
	if status != http.StatusAccepted {
		t.Fatalf("create = %d %v, want 202", status, created)
	}
	taskID := v1Data(t, created).(map[string]any)["id"].(string)
	typed := fx.canonicalParams(t, taskID)
	if typed.Media == nil || typed.Media.Mask == nil || typed.Media.Mask.PublicAssetID != "asset_mask1" {
		t.Fatalf("media=%+v, want mask public asset id", typed.Media)
	}
	if typed.Generation == nil || len(typed.Generation.Media) != 2 {
		t.Fatalf("generation=%+v, want input + mask intents", typed.Generation)
	}
	if typed.Generation.CompiledPrompt != "fill the hole" {
		t.Fatalf("generation=%+v, want the submitted prompt", typed.Generation)
	}
}

// Mask policy: inpaint without a mask, a mask on a non-inpaint feature,
// and data-URL masks all fail 422 (strict union preserved).
func TestV1TaskCreateMaskPolicy(t *testing.T) {
	fx := newV1PromptMaskFixture(t)
	for _, tc := range []struct{ name, body string }{
		{"inpaint without mask", `{"data":{"service":"inpaint","inputs":{"media_asset_ids":["asset_input1"]},"prompt":"fill it","params":{}}}`},
		{"mask on render", `{"data":{"service":"render","inputs":{"media_asset_ids":["asset_input1"]},"prompt":"edit it","params":{"mask_asset_id":"asset_mask1"}}}`},
		{"data-url mask", `{"data":{"service":"inpaint","inputs":{"media_asset_ids":["asset_input1"]},"prompt":"fill it","params":{"mask_asset_id":"data:image/png;base64,AAAA"}}}`},
		{"unknown params still rejected", `{"data":{"service":"generation","inputs":{"media_asset_ids":[]},"prompt":"ok","params":{"mystery":1}}}`},
	} {
		status, payload := fx.postTask(t, "maskpol-"+tc.name, tc.body)
		if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
			t.Fatalf("%s = %d %v, want 422 validation_failed", tc.name, status, payload)
		}
	}
}

// ---- merged from resolution_test.go ----
type v1RoutingFixture struct {
	*v1TaskFixture
	pools map[string]routing.Pool
	saved map[string]routing.RoutingSnapshot
}

// newV1TaskRoutingFixture mirrors newV1TaskFixture but serves quote/submit
// through the shared RoutingResolver (same instance) with a capture saver,
// so the edge tests prove the 422 fields shape, atomic pinning, and
// idempotency against the real selection contract.

func newV1TaskRoutingFixture(t *testing.T, pools map[string]routing.Pool) *v1RoutingFixture {
	t.Helper()
	base := newV1TaskFixture(t)
	saved := make(map[string]routing.RoutingSnapshot)
	seed := routing.SelectionSeed{11, 22, 33}
	base.engine = nil
	resolver := routing.NewRoutingResolver(&v1StubPoolStore{pools: pools},
		routing.WithSeedGenerator(func() (routing.SelectionSeed, error) { return seed, nil }))
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
		id := base.dir.ensure(userID)
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
		Store: base.store, Users: base.dir,
		Tasks: ContractV1TaskDeps{
			Tasks: base.txTasks, TaskLifecycle: base.txTasks, TaskSubmissionClaims: base.txTasks,
			TaskQuoter: base.quoter, TaskHolder: base.holder, TaskQueue: base.queue, TaskAssets: base.assets,
			TaskRouter: resolver,
			TaskSnapshotSaver: func(_ context.Context, _ pgx.Tx, taskID string, snap routing.RoutingSnapshot) error {
				saved[taskID] = snap
				return nil
			},
			TaskAcceptance:      &v1testsupport.TxSubmissionAcceptance{Assets: base.assets},
			TaskBillingAccounts: v1FakeBillingAccounts{},
			// #917: the preflight resolves the same fixture account submit
			// holds and gates the same fixture funds.
			TaskQuoteBillingAccounts: v1FakeBillingAccounts{},
			TaskQuoteWallets:         base.wallets,
		},
	})
	handler.Mount(v1)
	base.engine = router
	return &v1RoutingFixture{v1TaskFixture: base, pools: pools, saved: saved}
}

func v1CreateTaskBodyWithResolution(kind, resolution string) string {
	return fmt.Sprintf(`{"data":{"service":%q,"inputs":{"media_asset_ids":[]},"params":{},"resolution":%q}}`, kind, resolution)
}

func v1ErrorFields(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	errBody, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("error envelope requires error object: %v", payload)
	}
	details, ok := errBody["details"].(map[string]any)
	if !ok {
		t.Fatalf("unavailable error requires details: %v", payload)
	}
	fields, ok := details["fields"].(map[string]any)
	if !ok {
		t.Fatalf("unavailable error requires details.fields: %v", payload)
	}
	return fields
}

func TestV1TaskResolverUnavailableRendersFields(t *testing.T) {
	t.Parallel()
	fx := newV1TaskRoutingFixture(t, map[string]routing.Pool{})
	base := "/api/v1/workspaces/" + fx.ws.PublicID
	// #851: an omitted resolution applies the service policy default
	// (render is scalable, so 1k — converging with the readiness plan),
	// and the unavailable error reports those normalized values.
	for _, tc := range []struct{ path, body string }{
		{base + "/task-quotes", `{"data":{"service":"render","quantity":1}}`},
		{base + "/tasks", v1CreateTaskBodyJSON("render")},
	} {
		status, _, payload := doV1(t, fx.engine, v1Call{
			method: http.MethodPost, path: tc.path, userID: 3, body: tc.body,
			headers: map[string]string{contract.IdempotencyKeyHeader: "resolver-unavailable"},
		})
		if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeServiceResolutionUnavailable) {
			t.Fatalf("%s = %d %v, want 422 service_resolution_unavailable", tc.path, status, payload)
		}
		fields := v1ErrorFields(t, payload)
		if fields["service"] != "render" || fields["resolution"] != "1k" {
			t.Fatalf("%s fields = %v, want normalized render/1k", tc.path, fields)
		}
	}
	// The unavailable error is not cached as a success: repeating the same
	// idempotent submit still renders 422 (never 409), and still creates
	// nothing.
	status, _, payload := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: base + "/tasks", userID: 3, body: v1CreateTaskBodyJSON("render"),
		headers: map[string]string{contract.IdempotencyKeyHeader: "resolver-unavailable"},
	})
	if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeServiceResolutionUnavailable) {
		t.Fatalf("replay = %d %v, want 422 service_resolution_unavailable", status, payload)
	}
	count, _ := fx.tasks.CountTasks(context.Background(), fx.ws.PublicID)
	if count != 0 || len(fx.holder.holds) != 0 || len(fx.queue.enqueued) != 0 || fx.quoter.calls != 0 {
		t.Fatalf("unavailable resolution created work: tasks=%d holds=%v queued=%d quotes=%d", count, fx.holder.holds, len(fx.queue.enqueued), fx.quoter.calls)
	}
	if len(fx.saved) != 0 {
		t.Fatalf("snapshots pinned = %d, want 0", len(fx.saved))
	}
}

func TestV1TaskSubmitResolutionAgreement(t *testing.T) {
	t.Parallel()
	// #851 / ADR-0012 rule 11: params.output_resolution is the client
	// source of truth. The top-level billing resolution must agree with
	// it exactly; a mismatch fails closed before availability, quote,
	// hold, task, or enqueue work.
	pools := map[string]routing.Pool{"render": {
		Service: "render", Version: 5,
		Entries: []routing.ModelPoolEntry{
			{Provider: "a", Model: "first", Enabled: true, SupportedResolutions: []routing.Resolution{routing.Resolution2K}},
			{Provider: "b", Model: "second", Enabled: true, SupportedResolutions: []routing.Resolution{routing.Resolution2K, routing.Resolution4K}},
		},
	}}
	fx := newV1TaskRoutingFixture(t, pools)
	tasksPath := "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks"

	mismatched := `{"data":{"service":"render","inputs":{"media_asset_ids":[]},"params":{"output_resolution":"1k"},"resolution":"2k"}}`
	status, _, payload := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: tasksPath, userID: 3,
		body:    mismatched,
		headers: map[string]string{contract.IdempotencyKeyHeader: "resolution-agreement-1"},
	})
	if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
		t.Fatalf("mismatch = %d %v, want 422 validation_failed", status, payload)
	}
	if fields := v1ErrorFields(t, payload); fields["resolution"] != "mismatch" {
		t.Fatalf("mismatch fields = %v, want resolution=mismatch", fields)
	}

	// Case-insensitive agreement submits; the settled billing snapshot
	// retains the requested Service and resolution.
	agreed := `{"data":{"service":"render","inputs":{"media_asset_ids":[]},"params":{"output_resolution":"2K"},"resolution":"2k"}}`
	status, _, created := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: tasksPath, userID: 3,
		body:    agreed,
		headers: map[string]string{contract.IdempotencyKeyHeader: "resolution-agreement-2"},
	})
	if status != http.StatusAccepted {
		t.Fatalf("agreement = %d %v, want 202", status, created)
	}
	snapshot := created["data"].(map[string]any)["billing_snapshot"].(map[string]any)
	if snapshot["sku"] != "ai-image:render" || snapshot["output_resolution"] != "2k" {
		t.Fatalf("billing snapshot = %v, want ai-image:render @2k", snapshot)
	}

	// An absent top level carries the params value through: the quote
	// below prices 4k and the submit settles 4k.
	quotePath := "/api/v1/workspaces/" + fx.ws.PublicID + "/task-quotes"
	status, _, quoted := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: quotePath, userID: 3,
		body: `{"data":{"service":"render","quantity":1,"resolution":"4k"}}`,
	})
	if status != 200 || v1Data(t, quoted).(map[string]any)["output_resolution"] != "4k" {
		t.Fatalf("quote = %d %v, want 200 @4k", status, quoted)
	}
	derived := `{"data":{"service":"render","inputs":{"media_asset_ids":[]},"params":{"output_resolution":"4k"}}}`
	status, _, derivedCreated := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: tasksPath, userID: 3,
		body:    derived,
		headers: map[string]string{contract.IdempotencyKeyHeader: "resolution-agreement-3"},
	})
	if status != http.StatusAccepted {
		t.Fatalf("derived = %d %v, want 202", status, derivedCreated)
	}
	derivedSnapshot := derivedCreated["data"].(map[string]any)["billing_snapshot"].(map[string]any)
	if derivedSnapshot["output_resolution"] != "4k" {
		t.Fatalf("derived billing snapshot = %v, want @4k carried from params", derivedSnapshot)
	}

	count, _ := fx.tasks.CountTasks(context.Background(), fx.ws.PublicID)
	if count != 2 {
		t.Fatalf("tasks = %d, want exactly the two agreed submits (mismatch created nothing)", count)
	}
}

func TestV1TaskSubmitFoldsTopLevelResolutionIntoCanonicalParams(t *testing.T) {
	t.Parallel()
	pools := map[string]routing.Pool{"render": {
		Service: "render", Version: 5,
		Entries: []routing.ModelPoolEntry{{
			Model: "a/first", Enabled: true,
			SupportedResolutions: []routing.Resolution{routing.Resolution2K},
		}},
	}}
	fx := newV1TaskRoutingFixture(t, pools)
	path := "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks"
	status, _, payload := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: path, userID: 3,
		body:    v1CreateTaskBodyWithResolution("render", "2K"),
		headers: map[string]string{contract.IdempotencyKeyHeader: "resolution-fold-1"},
	})
	if status != http.StatusAccepted {
		t.Fatalf("folded resolution submit = %d %v, want 202", status, payload)
	}
	if len(fx.queue.enqueued) != 1 {
		t.Fatalf("enqueued = %d, want 1", len(fx.queue.enqueued))
	}
	task := fx.queue.enqueued[0]
	if got := task.Params[paramkeys.OutputResolution]; got != "2k" {
		t.Fatalf("task params output_resolution = %#v, want canonical 2k", got)
	}
	if task.BillingSnapshot.OutputResolution != "2k" {
		t.Fatalf("billing resolution = %q, want 2k", task.BillingSnapshot.OutputResolution)
	}
}

func TestV1TaskSubmitResolutionNormalizationPreservesIdempotencyReplay(t *testing.T) {
	t.Parallel()
	pools := map[string]routing.Pool{"render": {
		Service: "render", Version: 5,
		Entries: []routing.ModelPoolEntry{{
			Model: "a/first", Enabled: true,
			SupportedResolutions: []routing.Resolution{routing.Resolution2K},
		}},
	}}
	fx := newV1TaskRoutingFixture(t, pools)
	path := "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks"
	first := `{"data":{"service":"render","inputs":{"media_asset_ids":[]},"params":{"output_resolution":"2K"}}}`
	second := `{"data":{"service":"render","inputs":{"media_asset_ids":[]},"params":{"output_resolution":"2k"}}}`
	firstStatus, _, firstPayload := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: path, userID: 3, body: first,
		headers: map[string]string{contract.IdempotencyKeyHeader: "resolution-replay-1"},
	})
	if firstStatus != http.StatusAccepted {
		t.Fatalf("first normalized submit = %d %v, want 202", firstStatus, firstPayload)
	}
	firstID := firstPayload["data"].(map[string]any)["id"]
	secondStatus, _, secondPayload := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: path, userID: 3, body: second,
		headers: map[string]string{contract.IdempotencyKeyHeader: "resolution-replay-1"},
	})
	if secondStatus != http.StatusAccepted {
		t.Fatalf("second normalized submit = %d %v, want replay 202", secondStatus, secondPayload)
	}
	if got := secondPayload["data"].(map[string]any)["id"]; got != firstID {
		t.Fatalf("replay task id = %v, want %v", got, firstID)
	}
	if count, _ := fx.tasks.CountTasks(context.Background(), fx.ws.PublicID); count != 1 {
		t.Fatalf("tasks = %d, want one idempotent task", count)
	}
}

func TestV1TaskSubmitExplicitDefaultForScalable(t *testing.T) {
	t.Parallel()
	// ADR-0012 rule 3 (#854): explicit "default" for a scalable service
	// is a deterministic caller bug. Submit fails closed as 422
	// validation_failed on details.fields.resolution before any
	// availability, quote, hold, task, or enqueue work.
	pools := map[string]routing.Pool{"render": {
		Service: "render", Version: 5,
		Entries: []routing.ModelPoolEntry{
			{Provider: "a", Model: "first", Enabled: true, SupportedResolutions: []routing.Resolution{routing.Resolution1K}},
		},
	}}
	fx := newV1TaskRoutingFixture(t, pools)
	tasksPath := "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks"

	explicit := `{"data":{"service":"render","inputs":{"media_asset_ids":[]},"params":{"output_resolution":"default"}}}`
	status, _, payload := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: tasksPath, userID: 3,
		body:    explicit,
		headers: map[string]string{contract.IdempotencyKeyHeader: "explicit-default-1"},
	})
	if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
		t.Fatalf("explicit default = %d %v, want 422 validation_failed", status, payload)
	}
	if fields := v1ErrorFields(t, payload); fields["resolution"] != "invalid" {
		t.Fatalf("explicit default fields = %v, want resolution=invalid", fields)
	}
	if count, _ := fx.tasks.CountTasks(context.Background(), fx.ws.PublicID); count != 0 {
		t.Fatalf("tasks = %d, want 0 (explicit default created nothing)", count)
	}
	if len(fx.saved) != 0 {
		t.Fatalf("snapshots pinned = %d, want 0", len(fx.saved))
	}
}

func TestV1TaskResolverPinsSnapshotAndPreservesIdempotency(t *testing.T) {
	t.Parallel()
	pools := map[string]routing.Pool{"render": {
		Service: "render", Version: 5,
		Entries: []routing.ModelPoolEntry{
			{Provider: "a", Model: "first", Enabled: true, SupportedResolutions: []routing.Resolution{routing.Resolution2K}},
			{Provider: "b", Model: "second", Enabled: true, SupportedResolutions: []routing.Resolution{routing.Resolution2K, routing.Resolution4K}},
		},
	}}
	fx := newV1TaskRoutingFixture(t, pools)
	tasksPath := "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks"
	body := v1CreateTaskBodyWithResolution("render", "2k")

	status, _, created := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: tasksPath, userID: 3,
		body:    body,
		headers: map[string]string{contract.IdempotencyKeyHeader: "resolver-pin-1"},
	})
	if status != http.StatusAccepted {
		t.Fatalf("create = %d %v, want 202", status, created)
	}
	taskID := created["data"].(map[string]any)["id"].(string)
	snap, ok := fx.saved[taskID]
	if !ok {
		t.Fatalf("no snapshot pinned for task %s", taskID)
	}
	if err := snap.Verify(); err != nil {
		t.Fatalf("pinned snapshot must replay without the live pool: %v", err)
	}
	if snap.Service != "render" || snap.RequestedResolution != routing.Resolution2K || snap.PoolVersion != 5 {
		t.Fatalf("pinned pinning = %+v, want render/2k/version 5", snap)
	}

	// Idempotent replay returns the same task without re-pinning.
	status, _, replay := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: tasksPath, userID: 3,
		body:    body,
		headers: map[string]string{contract.IdempotencyKeyHeader: "resolver-pin-1"},
	})
	if status != http.StatusAccepted || replay["data"].(map[string]any)["id"] != taskID {
		t.Fatalf("replay = %d %v, want same task_id %s", status, replay, taskID)
	}

	// Later pool edits never invalidate the accepted task: empty the pool
	// and prove new submits fail while the pinned snapshot still replays.
	fx.pools["render"] = routing.Pool{Service: "render", Version: 6}
	status, _, payload := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: tasksPath, userID: 3,
		body:    body,
		headers: map[string]string{contract.IdempotencyKeyHeader: "resolver-pin-2"},
	})
	if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeServiceResolutionUnavailable) {
		t.Fatalf("post-edit submit = %d %v, want 422 service_resolution_unavailable", status, payload)
	}
	if err := fx.saved[taskID].Verify(); err != nil {
		t.Fatalf("pinned snapshot after pool edit must still replay: %v", err)
	}
	count, _ := fx.tasks.CountTasks(context.Background(), fx.ws.PublicID)
	if count != 1 {
		t.Fatalf("tasks = %d, want exactly the one accepted task", count)
	}
}

type submitPromptReader struct {
	template string
	err      error
}

func (r *submitPromptReader) GetRuntimeConfig(_ context.Context, feature string) (prompts.FeaturePromptRuntimeConfig, error) {
	return prompts.FeaturePromptRuntimeConfig{Prompt: r.template, UserPromptMergePolicy: prompts.DefaultFeaturePromptMergePolicy(feature)}, r.err
}

func TestV1TaskPromptPreparationFailurePrecedesAcceptanceAndReplay(t *testing.T) {
	reader := &submitPromptReader{err: errors.New("prompt store unavailable")}
	fx := newV1PromptMaskFixtureWithPrompts(t, reader)
	body := `{"data":{"service":"generation","prompt":"bicycle","params":{}}}`
	status, payload := fx.postTask(t, "prepared", body)
	if status < 400 || len(fx.queue.enqueued) != 0 || len(fx.holder.holds) != 0 {
		t.Fatalf("failed preparation accepted: status=%d payload=%v queue=%d holds=%d", status, payload, len(fx.queue.enqueued), len(fx.holder.holds))
	}
	reader.err = nil
	reader.template = "stored {{user_prompt}}"
	status, created := fx.postTask(t, "prepared", body)
	if status != http.StatusAccepted {
		t.Fatalf("create=%d %v", status, created)
	}
	id := v1Data(t, created).(map[string]any)["id"].(string)
	if got := fx.canonicalParams(t, id).Generation.CompiledPrompt; got != "stored bicycle" {
		t.Fatalf("compiled=%q", got)
	}
	reader.template = "edited {{user_prompt}}"
	status, replay := fx.postTask(t, "prepared", body)
	if status != http.StatusAccepted || v1Data(t, replay).(map[string]any)["id"] != id {
		t.Fatalf("replay=%d %v", status, replay)
	}
	if got := fx.canonicalParams(t, id).Generation.CompiledPrompt; got != "stored bicycle" {
		t.Fatalf("replay rewrote frozen prompt: %q", got)
	}
	// Preserve the existing read-before-fast-replay timing: a later storage
	// failure still fails closed even when this action was accepted earlier.
	reader.err = errors.New("prompt store unavailable again")
	status, payload = fx.postTask(t, "prepared", body)
	if status < 400 || len(fx.queue.enqueued) != 1 {
		t.Fatalf("read failure replay=%d %v queue=%d", status, payload, len(fx.queue.enqueued))
	}
}
