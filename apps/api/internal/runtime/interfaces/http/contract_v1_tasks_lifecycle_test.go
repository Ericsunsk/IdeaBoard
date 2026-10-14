package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/generation/routing"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/tenancy"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

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
	if !strings.HasPrefix(taskID, "task_") {
		t.Fatalf("task id = %q, want task_...", taskID)
	}
	if !contract.IsTaskID(taskID) {
		t.Fatalf("task id shape invalid: %q", taskID)
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
	if len(fx.holder.released) == 0 {
		t.Fatal("hold must be released when enqueue fails (no stranded hold)")
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
	fx.assets.assets[fx.ws.PublicID+"\x00asset_ready1"] = &workspacetasks.AssetInfo{ID: "asset_ready1", WorkspaceID: fx.ws.PublicID, Status: "ready"}
	checksum := strings.Repeat("b", 64)
	_, err := fx.tasks.AddOutputs(ctx, fx.ws.PublicID, taskID, []workspacetasks.OutputRef{
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
	for _, leaked := range []string{"storage_key", "builtin-local", "ws-uploads", "bucket"} {
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
	fx.quoter.err = fmt.Errorf("workspace task quoter: %w", workspacetasks.ErrQuoteUnavailable)
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
		fx.quoter.err = fmt.Errorf("workspace task quoter: %w: %w", quoteErr, workspacetasks.ErrQuoteUnavailable)
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
	fx.holder.holdErr = workspacetasks.ErrWalletRestricted
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
	router.Use(ContractRequestIDMiddleware())
	router.Use(func(c *gin.Context) {
		raw := strings.TrimSpace(c.GetHeader("X-Test-User"))
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
	v1 := router.Group(ContractV1GroupPath)
	RegisterContractV1Routes(v1, AuthDeps{}, ContractV1Deps{Store: store, Users: dir})
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
