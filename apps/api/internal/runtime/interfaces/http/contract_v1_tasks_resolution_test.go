package httpapi

import (
	"context"
	"fmt"
	"ideaboard/api/internal/generation/routing"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/workflow/paramkeys"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

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
		role := strings.TrimSpace(c.GetHeader("X-Test-Role"))
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
	v1 := router.Group(ContractV1GroupPath)
	RegisterContractV1Routes(v1, AuthDeps{}, ContractV1Deps{
		Store: base.store, Users: base.dir,
		Tasks: base.tasks, TaskQuoter: base.quoter, TaskHolder: base.holder, TaskQueue: base.queue, TaskAssets: base.assets,
		AllowNonTransactionalTaskSettlement: true,
		TaskRouter:                          resolver,
		TaskSnapshotSaver: func(_ context.Context, _ pgx.Tx, taskID string, snap routing.RoutingSnapshot) error {
			saved[taskID] = snap
			return nil
		},
	})
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
			{Model: "a/first", Enabled: true, SupportedResolutions: []routing.Resolution{routing.Resolution2K}},
			{Model: "b/second", Enabled: true, SupportedResolutions: []routing.Resolution{routing.Resolution2K, routing.Resolution4K}},
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
			{Model: "a/first", Enabled: true, SupportedResolutions: []routing.Resolution{routing.Resolution1K}},
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
			{Model: "a/first", Enabled: true, SupportedResolutions: []routing.Resolution{routing.Resolution2K}},
			{Model: "b/second", Enabled: true, SupportedResolutions: []routing.Resolution{routing.Resolution2K, routing.Resolution4K}},
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
