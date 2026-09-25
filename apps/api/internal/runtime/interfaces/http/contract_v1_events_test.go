package httpapi

// Workspace event streams + webhook delivery contract tests (issue #807).
//
// Reuses the shared doV1 helper (envelope + Request-Id) and the
// cross-file fakes (tasks, library, uploads). Covers envelope emission
// per domain producer, SSE Last-Event-ID replay (gap + live + filter),
// subscription CRUD with secret masking, signature verification of
// dispatched webhooks, the 404-vs-403 matrix, idempotent replay/conflict
// for subscription writes, and retention-boundary resume.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/library"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type fakeEventSender struct {
	mu           sync.Mutex
	status       int
	err          error
	calls        int
	bodies       [][]byte
	headers      []map[string]string
	destinations []string
}

func (f *fakeEventSender) Send(_ context.Context, destination string, headers map[string]string, body []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.bodies = append(f.bodies, append([]byte(nil), body...))
	copied := map[string]string{}
	for key, value := range headers {
		copied[key] = value
	}
	f.headers = append(f.headers, copied)
	f.destinations = append(f.destinations, destination)
	if f.err != nil {
		return 0, f.err
	}
	return f.status, nil
}

func (f *fakeEventSender) snapshot() (int, [][]byte, []map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls, append([][]byte(nil), f.bodies...), append([]map[string]string(nil), f.headers...)
}

// inlineEventEnqueuer executes webhook deliveries synchronously through
// the River worker, so contract tests keep asserting sends immediately
// after the emitting call without running a River client.
type inlineEventEnqueuer struct {
	mu    sync.Mutex
	store *events.WebhookStore
}

func (m *inlineEventEnqueuer) Insert(ctx context.Context, args river.JobArgs, _ *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	delivery, ok := args.(events.WebhookDeliveryArgs)
	if !ok {
		return nil, fmt.Errorf("inline enqueuer: unexpected args %T", args)
	}
	worker := events.NewWebhookDeliverWorker(m.store)
	// Attempt 1 of the 8-budget runs inline; worker errors stand in for
	// River retries (no failing-sender contract paths exist today).
	_ = worker.Work(ctx, &river.Job[events.WebhookDeliveryArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 8},
		Args:   delivery,
	})
	return &rivertype.JobInsertResult{}, nil
}

type v1EventsFixture struct {
	engine       *gin.Engine
	store        *tenancy.InMemoryStore
	dir          *staticUserDirectory
	ws           *tenancy.Workspace
	tasks        *workspacetasks.InMemoryStore
	boards       *boards.InMemoryWorkspaceBoardStore
	boardID      string
	lib          *library.InMemoryWorkspaceLibraryStore
	eventLog     *events.WorkspaceEventLog
	webhooks     *events.WebhookStore
	sender       *fakeEventSender
	enqueuer     *inlineEventEnqueuer
	holder       *fakeTaskHolder
	materializer *fakeLibraryMaterializer
}

func newV1EventsFixture(t *testing.T) *v1EventsFixture {
	t.Helper()
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	tasks := workspacetasks.NewInMemoryStoreForTests()
	boardsStore := boards.NewInMemoryWorkspaceBoardStoreForTests()
	libStore := library.NewInMemoryWorkspaceLibraryStoreForTests()
	lib := libStore
	quoter := &fakeTaskQuoter{}
	holder := newFakeTaskHolder()
	queue := &fakeTaskQueue{}
	taskAssets := &fakeTaskAssets{assets: map[string]*workspacetasks.AssetInfo{}}
	materializer := &fakeLibraryMaterializer{mediaID: "asset_abc123def456"}
	eventLog := events.NewWorkspaceEventLog()
	sender := &fakeEventSender{status: 200}
	webhooks := events.NewWebhookStore(sender, nil)
	enqueuer := &inlineEventEnqueuer{store: webhooks}
	webhooks.SetEnqueuer(enqueuer)
	gin.SetMode(gin.TestMode)
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
		id := dir.ensure(userID)
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
		Store: store, Users: dir,
		Tasks: tasks, TaskQuoter: quoter, TaskHolder: holder, TaskQueue: queue, TaskAssets: taskAssets,
		Boards:   boardsStore,
		Library:  ContractV1LibraryDeps{Store: libStore, Materializer: materializer, Billing: &fakeLibraryBilling{}, Quoter: &fakeLibraryQuoter{amount: 5}},
		Events:   eventLog,
		Webhooks: webhooks,
	})
	const owner, admin, member, outsider = 1, 2, 3, 9
	for _, id := range []int64{owner, admin, member, outsider, 4} {
		dir.ensure(id)
	}
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "events", OwnerUserID: owner})
	if err != nil {
		t.Fatal(err)
	}
	store.SeedWorkspaceMember(ws.PublicID, admin, tenancy.RoleAdmin)
	store.SeedWorkspaceMember(ws.PublicID, member, tenancy.RoleMember)
	boardID, err := boards.NewV1BoardPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := boardsStore.CreateBoard(ctx, ws.PublicID, owner, "Events", boardID); err != nil {
		t.Fatal(err)
	}
	return &v1EventsFixture{
		engine: router, store: store, dir: dir, ws: ws, tasks: tasks,
		boards: boardsStore, boardID: boardID,
		lib: lib, eventLog: eventLog, webhooks: webhooks, sender: sender,
		enqueuer: enqueuer, holder: holder, materializer: materializer,
	}
}

func (f *v1EventsFixture) eventsPath() string {
	return "/api/v1/workspaces/" + f.ws.PublicID + "/events"
}

func (f *v1EventsFixture) subscriptionsPath() string {
	return "/api/v1/workspaces/" + f.ws.PublicID + "/event-subscriptions"
}

func (f *v1EventsFixture) logTypes() []string {
	missed, _, _ := f.eventLog.ReplayAfter(f.ws.PublicID, "evt_doesnotexist0000", events.WorkspaceEventFilter{})
	types := make([]string, 0, len(missed))
	for _, envelope := range missed {
		types = append(types, envelope.Type)
	}
	return types
}

func eventIDEM(t *testing.T, engine *gin.Engine, method, path string, userID int64, key, body string, extra map[string]string) (int, map[string]any) {
	t.Helper()
	headers := map[string]string{}
	for k, v := range extra {
		headers[k] = v
	}
	if key != "" {
		headers[contract.IdempotencyKeyHeader] = key
	}
	status, _, payload := doV1(t, engine, v1Call{method: method, path: path, userID: userID, body: body, headers: headers})
	return status, payload
}

func TestV1TaskCreateEmitsTaskCreatedAndBillingSettled(t *testing.T) {
	fx := newV1EventsFixture(t)
	status, created := eventIDEM(t, fx.engine, http.MethodPost,
		"/api/v1/workspaces/"+fx.ws.PublicID+"/tasks", 3, "evt-task-1", v1CreateTaskBodyJSON("render"), nil)
	if status != http.StatusAccepted {
		t.Fatalf("create = %d %v, want 202", status, created)
	}
	taskID := created["data"].(map[string]any)["id"].(string)
	types := fx.logTypes()
	counts := map[string]int{}
	for _, typ := range types {
		counts[typ]++
	}
	if counts[events.EventV1TaskCreated] != 1 {
		t.Fatalf("task.created count=%d, want 1 (types=%v)", counts[events.EventV1TaskCreated], types)
	}
	if counts[events.EventV1BillingSettled] != 1 {
		t.Fatalf("billing.settled count=%d, want 1 (types=%v)", counts[events.EventV1BillingSettled], types)
	}
	missed, _, _ := fx.eventLog.ReplayAfter(fx.ws.PublicID, "evt_doesnotexist0000", events.WorkspaceEventFilter{})
	for _, envelope := range missed {
		if err := events.ValidateV1Envelope(envelope); err != nil {
			t.Fatalf("envelope %s invalid: %v", envelope.ID, err)
		}
		if envelope.WorkspaceID != fx.ws.PublicID {
			t.Fatalf("envelope workspace=%q, want %q", envelope.WorkspaceID, fx.ws.PublicID)
		}
		switch envelope.Type {
		case events.EventV1TaskCreated:
			var data map[string]any
			if err := json.Unmarshal(envelope.Data, &data); err != nil {
				t.Fatal(err)
			}
			if data["task_id"] != taskID || data["workflow_kind"] != "render" {
				t.Fatalf("task.created data=%v", data)
			}
		case events.EventV1BillingSettled:
			var data map[string]any
			if err := json.Unmarshal(envelope.Data, &data); err != nil {
				t.Fatal(err)
			}
			if data["reference_id"] != taskID || data["settlement_state"] != "held" {
				t.Fatalf("billing.settled data=%v", data)
			}
			if _, hasAmount := data["total_credits"]; hasAmount {
				t.Fatalf("billing.settled must not carry amounts: %v", data)
			}
		}
	}
	// Idempotent replay of the same key must not re-emit.
	status, _ = eventIDEM(t, fx.engine, http.MethodPost,
		"/api/v1/workspaces/"+fx.ws.PublicID+"/tasks", 3, "evt-task-1", v1CreateTaskBodyJSON("render"), nil)
	if status != http.StatusAccepted {
		t.Fatalf("replay = %d, want 202", status)
	}
	if got := len(fx.logTypes()); got != 2 {
		t.Fatalf("replay emitted %d envelopes, want still 2", got)
	}
}

func TestV1TaskCancelEmitsCancelledAndReleased(t *testing.T) {
	fx := newV1EventsFixture(t)
	_, created := eventIDEM(t, fx.engine, http.MethodPost,
		"/api/v1/workspaces/"+fx.ws.PublicID+"/tasks", 3, "evt-cancel-1", v1CreateTaskBodyJSON("render"), nil)
	taskID := created["data"].(map[string]any)["id"].(string)
	before := len(fx.logTypes())
	status, _ := eventIDEM(t, fx.engine, http.MethodPost,
		"/api/v1/workspaces/"+fx.ws.PublicID+"/tasks/"+taskID+"/cancel", 3, "evt-cancel-2", `{"data":{"reason":"no longer needed"}}`, nil)
	if status != http.StatusOK {
		t.Fatalf("cancel = %d, want 200", status)
	}
	types := fx.logTypes()[before:]
	counts := map[string]int{}
	for _, typ := range types {
		counts[typ]++
	}
	if counts[events.EventV1TaskCancelled] != 1 || counts[events.EventV1BillingSettled] != 1 {
		t.Fatalf("cancel emission=%v, want 1x cancelled + 1x settled", counts)
	}
	// Terminal replay never re-emits.
	status, _ = eventIDEM(t, fx.engine, http.MethodPost,
		"/api/v1/workspaces/"+fx.ws.PublicID+"/tasks/"+taskID+"/cancel", 3, "evt-cancel-3", `{"data":{"reason":"again"}}`, nil)
	if status != http.StatusOK {
		t.Fatalf("cancel replay = %d, want 200", status)
	}
	if got := len(fx.logTypes()); got != before+2 {
		t.Fatalf("terminal replay emitted, total=%d want %d", got, before+2)
	}
}

func TestV1BoardProducersEmit(t *testing.T) {
	fx := newV1EventsFixture(t)
	docPath := "/api/v1/workspaces/" + fx.ws.PublicID + "/boards/" + fx.boardID + "/document"
	status, payload := eventIDEM(t, fx.engine, http.MethodPut, docPath, 1, "evt-doc-1",
		`{"data":{"schema_version":1,"payload":{"a":1}}}`,
		map[string]string{contract.IfMatchHeader: "1"})
	if status != http.StatusOK {
		t.Fatalf("replace document = %d %v, want 200", status, payload)
	}
	membersPath := "/api/v1/workspaces/" + fx.ws.PublicID + "/boards/" + fx.boardID + "/members"
	target := tenancy.FormatUserPublicID(fx.dir.ensure(4))
	fx.store.SeedWorkspaceMember(fx.ws.PublicID, 4, tenancy.RoleMember)
	status, payload = eventIDEM(t, fx.engine, http.MethodPost, membersPath, 1, "evt-madd-1",
		fmt.Sprintf(`{"data":{"user_id":%q,"role":"viewer"}}`, target), nil)
	if status != http.StatusCreated {
		t.Fatalf("member add = %d %v, want 201", status, payload)
	}
	sharesPath := "/api/v1/workspaces/" + fx.ws.PublicID + "/boards/" + fx.boardID + "/shares"
	status, payload = eventIDEM(t, fx.engine, http.MethodPost, sharesPath, 1, "evt-share-1", `{"data":{"role":"viewer"}}`, nil)
	if status != http.StatusCreated {
		t.Fatalf("share create = %d %v, want 201", status, payload)
	}
	tokenID := payload["data"].(map[string]any)["token_id"].(string)
	status, payload = eventIDEM(t, fx.engine, http.MethodDelete, sharesPath+"/"+tokenID, 1, "evt-unshare-1", "", nil)
	if status != http.StatusOK {
		t.Fatalf("share revoke = %d %v, want 200", status, payload)
	}
	types := fx.logTypes()
	counts := map[string]int{}
	for _, typ := range types {
		counts[typ]++
	}
	for _, want := range []string{
		events.EventV1BoardDocumentReplaced, events.EventV1BoardMemberChanged, events.EventV1BoardShareRevoked,
	} {
		if counts[want] != 1 {
			t.Fatalf("%s count=%d, want 1 (types=%v)", want, counts[want], types)
		}
	}
	missed, _, _ := fx.eventLog.ReplayAfter(fx.ws.PublicID, "evt_doesnotexist0000", events.WorkspaceEventFilter{})
	for _, envelope := range missed {
		if err := events.ValidateV1Envelope(envelope); err != nil {
			t.Fatalf("board envelope invalid: %v", err)
		}
		var data map[string]any
		_ = json.Unmarshal(envelope.Data, &data)
		switch envelope.Type {
		case events.EventV1BoardDocumentReplaced:
			if data["board_id"] != fx.boardID || data["revision"].(float64) != 2 {
				t.Fatalf("document_replaced data=%v", data)
			}
		case events.EventV1BoardMemberChanged:
			if data["board_id"] != fx.boardID || data["member_id"] != target {
				t.Fatalf("member_changed data=%v", data)
			}
		case events.EventV1BoardShareRevoked:
			if data["board_id"] != fx.boardID || data["share_token_id"] != tokenID {
				t.Fatalf("share_revoked data=%v", data)
			}
			raw, _ := json.Marshal(envelope)
			if bytes.Contains(raw, []byte("bearer")) {
				t.Fatal("share_revoked must never carry the bearer")
			}
		}
	}
}

func TestV1LibraryProducersEmit(t *testing.T) {
	fx := newV1EventsFixture(t)
	assetsPath := "/api/v1/workspaces/" + fx.ws.PublicID + "/library/assets"
	status, _, saved := doV1(t, fx.engine, v1Call{method: http.MethodPost, path: assetsPath, userID: 1,
		body: v1SaveAssetBody("evt-lib-1", ""), headers: map[string]string{contract.IdempotencyKeyHeader: "evt-lib-save-1"}})
	if status != http.StatusCreated {
		t.Fatalf("save = %d %v, want 201", status, saved)
	}
	assetID := saved["data"].(map[string]any)["id"].(string)
	matPath := "/api/v1/workspaces/" + fx.ws.PublicID + "/library/materializations"
	status, _, created := doV1(t, fx.engine, v1Call{method: http.MethodPost, path: matPath, userID: 3,
		body:    fmt.Sprintf(`{"data":{"library_asset_id":%q}}`, assetID),
		headers: map[string]string{contract.IdempotencyKeyHeader: "evt-lib-mat-1"}})
	if status != http.StatusAccepted {
		t.Fatalf("materialize = %d %v, want 202", status, created)
	}
	types := fx.logTypes()
	counts := map[string]int{}
	for _, typ := range types {
		counts[typ]++
	}
	if counts[events.EventV1LibraryMaterializedComplete] != 1 || counts[events.EventV1BillingSettled] != 1 {
		t.Fatalf("library emission=%v, want 1x completed + 1x settled", counts)
	}
	// Failed materialization emits failed + released.
	fx.materializer.err = fmt.Errorf("provider fetch exploded")
	status, _, failed := doV1(t, fx.engine, v1Call{method: http.MethodPost, path: matPath, userID: 3,
		body:    fmt.Sprintf(`{"data":{"library_asset_id":%q}}`, assetID),
		headers: map[string]string{contract.IdempotencyKeyHeader: "evt-lib-mat-2"}})
	if status != http.StatusAccepted || failed["data"].(map[string]any)["status"] != "failed" {
		t.Fatalf("failed job = %d %v, want 202 failed", status, failed)
	}
	types = fx.logTypes()
	counts = map[string]int{}
	for _, typ := range types {
		counts[typ]++
	}
	if counts[events.EventV1LibraryMaterializedFailed] != 1 || counts[events.EventV1BillingSettled] != 2 {
		t.Fatalf("library failure emission=%v, want 1x failed + 2x settled total", counts)
	}
	fx.materializer.err = nil
}

func TestV1EventVisibilityMatrix(t *testing.T) {
	fx := newV1EventsFixture(t)
	// Anonymous fails closed with 401.
	status, _, payload := doV1(t, fx.engine, v1Call{method: http.MethodGet, path: fx.eventsPath()})
	if status != http.StatusUnauthorized || v1ErrorCode(t, payload) != string(contract.CodeUnauthorized) {
		t.Fatalf("anon stream = %d %v, want 401", status, payload)
	}
	// Outsider (non-member) converges to 404 event_not_found.
	status, _, payload = doV1(t, fx.engine, v1Call{method: http.MethodGet, path: fx.eventsPath(), userID: 9})
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeEventNotFound) {
		t.Fatalf("outsider stream = %d %v, want 404 event_not_found", status, payload)
	}
	// Unknown workspace converges the same way (no existence leak).
	status, _, payload = doV1(t, fx.engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/ws_doesnotexist0000/events", userID: 3})
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeEventNotFound) {
		t.Fatalf("unknown workspace = %d %v, want 404 event_not_found", status, payload)
	}
	// Malformed cursor fails closed with 400 pagination_cursor_invalid.
	req := httptest.NewRequest(http.MethodGet, fx.eventsPath()+"?last_event_id=nope", nil)
	req.Header.Set("X-Test-User", "3")
	req.Header.Set(contract.RequestIDHeader, "test-evt-cursor-1")
	recorder := httptest.NewRecorder()
	fx.engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("malformed cursor = %d, want 400", recorder.Code)
	}
	var cursorPayload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &cursorPayload); err != nil {
		t.Fatal(err)
	}
	if v1ErrorCode(t, cursorPayload) != string(contract.CodePaginationCursorInvalid) {
		t.Fatalf("cursor error = %v, want pagination_cursor_invalid", cursorPayload)
	}
	// Unknown event-type filter fails closed with 422.
	status, _, payload = doV1(t, fx.engine, v1Call{method: http.MethodGet, path: fx.eventsPath() + "?types=task.exploded", userID: 3})
	if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
		t.Fatalf("bad filter = %d %v, want 422 validation_failed", status, payload)
	}
	// Visible member without admin role cannot manage subscriptions.
	status, payload = eventIDEM(t, fx.engine, http.MethodPost, fx.subscriptionsPath(), 3, "evt-sub-member",
		`{"data":{"url":"https://hooks.example.com/x"}}`, nil)
	if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeEventForbidden) {
		t.Fatalf("member subscribe = %d %v, want 403 event_forbidden", status, payload)
	}
	// Outsider subscription management converges to 404.
	status, payload = eventIDEM(t, fx.engine, http.MethodPost, fx.subscriptionsPath(), 9, "evt-sub-out",
		`{"data":{"url":"https://hooks.example.com/x"}}`, nil)
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeEventNotFound) {
		t.Fatalf("outsider subscribe = %d %v, want 404 event_not_found", status, payload)
	}
}

func TestV1EventSubscriptionCRUD(t *testing.T) {
	fx := newV1EventsFixture(t)
	// Missing Idempotency-Key fails closed.
	status, payload := eventIDEM(t, fx.engine, http.MethodPost, fx.subscriptionsPath(), 2, "",
		`{"data":{"url":"https://hooks.example.com/x"}}`, nil)
	if status != http.StatusBadRequest || v1ErrorCode(t, payload) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("missing key = %d %v, want 400 idempotency_key_required", status, payload)
	}
	// Plain http to a public host fails closed.
	status, payload = eventIDEM(t, fx.engine, http.MethodPost, fx.subscriptionsPath(), 2, "evt-sub-bad",
		`{"data":{"url":"http://hooks.example.com/x"}}`, nil)
	if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
		t.Fatalf("http url = %d %v, want 422 validation_failed", status, payload)
	}
	// Create with a minted secret: plaintext returned exactly once.
	status, payload = eventIDEM(t, fx.engine, http.MethodPost, fx.subscriptionsPath(), 2, "evt-sub-1",
		`{"data":{"url":"https://hooks.example.com/x","event_types":["task.completed"]}}`, nil)
	if status != http.StatusCreated {
		t.Fatalf("create = %d %v, want 201", status, payload)
	}
	data := v1Data(t, payload).(map[string]any)
	subscription := data["subscription"].(map[string]any)
	subID, _ := subscription["id"].(string)
	if !events.IsWebhookSubscriptionID(subID) {
		t.Fatalf("subscription id = %q, want sub_...", subID)
	}
	secretHex, _ := data["secret"].(string)
	secretBytes, ok := events.ParseWhsecSecret([]byte(secretHex))
	if !ok || len(secretBytes) == 0 {
		t.Fatalf("minted secret must be whsec_-form once, got %q", secretHex)
	}
	if subscription["has_secret"] != true {
		t.Fatalf("view must report has_secret: %v", subscription)
	}
	// Replay with the same key+hash returns the same subscription.
	status, replay := eventIDEM(t, fx.engine, http.MethodPost, fx.subscriptionsPath(), 2, "evt-sub-1",
		`{"data":{"url":"https://hooks.example.com/x","event_types":["task.completed"]}}`, nil)
	if status != http.StatusCreated {
		t.Fatalf("replay = %d, want 201", status)
	}
	if v1Data(t, replay).(map[string]any)["subscription"].(map[string]any)["id"] != subID {
		t.Fatalf("replay must return the same subscription: %v", replay)
	}
	if got := len(fx.webhooks.List(fx.ws.PublicID)); got != 1 {
		t.Fatalf("replay created %d subscriptions, want 1", got)
	}
	// Same key with a different request conflicts.
	status, payload = eventIDEM(t, fx.engine, http.MethodPost, fx.subscriptionsPath(), 2, "evt-sub-1",
		`{"data":{"url":"https://hooks.example.com/y"}}`, nil)
	if status != http.StatusConflict || v1ErrorCode(t, payload) != string(contract.CodeEventSubscriptionConflict) {
		t.Fatalf("conflict = %d %v, want 409 event_subscription_conflict", status, payload)
	}
	// Provided secrets never appear in any view.
	status, _ = eventIDEM(t, fx.engine, http.MethodPost, fx.subscriptionsPath(), 2, "evt-sub-2",
		`{"data":{"url":"https://hooks.example.com/y","secret":"whsec_cHJvdmlkZWQtc2VjcmV0LTEyMzQ1Ng=="}}`, nil)
	if status != http.StatusCreated {
		t.Fatalf("provided-secret create = %d, want 201", status)
	}
	status, _, listed := doV1(t, fx.engine, v1Call{method: http.MethodGet, path: fx.subscriptionsPath(), userID: 2})
	if status != http.StatusOK {
		t.Fatalf("list = %d, want 200", status)
	}
	serialized, _ := json.Marshal(listed)
	if bytes.Contains(serialized, []byte("provided-secret-123456")) {
		t.Fatalf("secret plaintext leaked in reads: %s", serialized)
	}
	items := v1Data(t, listed).(map[string]any)["subscriptions"].([]any)
	if len(items) != 2 {
		t.Fatalf("subscriptions = %d, want 2", len(items))
	}
	// Delete requires the key too; unknown ids converge to 404.
	status, payload = eventIDEM(t, fx.engine, http.MethodDelete, fx.subscriptionsPath()+"/sub_doesnotexist00", 2, "", "", nil)
	if status != http.StatusBadRequest {
		t.Fatalf("delete without key = %d, want 400", status)
	}
	status, payload = eventIDEM(t, fx.engine, http.MethodDelete, fx.subscriptionsPath()+"/sub_doesnotexist00", 2, "evt-del-1", "", nil)
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeEventSubscriptionNotFound) {
		t.Fatalf("delete unknown = %d %v, want 404 event_subscription_not_found", status, payload)
	}
	status, payload = eventIDEM(t, fx.engine, http.MethodDelete, fx.subscriptionsPath()+"/"+subID, 2, "evt-del-2", "", nil)
	if status != http.StatusOK {
		t.Fatalf("delete = %d %v, want 200", status, payload)
	}
	if got := len(fx.webhooks.List(fx.ws.PublicID)); got != 1 {
		t.Fatalf("after delete subscriptions = %d, want 1", got)
	}
}

func TestV1WebhookDispatchVerifiesSignature(t *testing.T) {
	fx := newV1EventsFixture(t)
	status, payload := eventIDEM(t, fx.engine, http.MethodPost, fx.subscriptionsPath(), 2, "evt-sub-dispatch",
		`{"data":{"url":"https://hooks.example.com/x"}}`, nil)
	if status != http.StatusCreated {
		t.Fatalf("create subscription = %d %v, want 201", status, payload)
	}
	status, _ = eventIDEM(t, fx.engine, http.MethodPost,
		"/api/v1/workspaces/"+fx.ws.PublicID+"/tasks", 3, "evt-dispatch-task", v1CreateTaskBodyJSON("render"), nil)
	if status != http.StatusAccepted {
		t.Fatalf("create task = %d, want 202", status)
	}
	calls, bodies, headers := fx.sender.snapshot()
	if calls != 2 {
		t.Fatalf("webhook calls = %d, want 2 (task.created + billing.settled)", calls)
	}
	subs := fx.webhooks.List(fx.ws.PublicID)
	if len(subs) != 1 {
		t.Fatalf("subscriptions = %d, want 1", len(subs))
	}
	stored, ok := fx.webhooks.Get(fx.ws.PublicID, subs[0].ID)
	if !ok {
		t.Fatal("subscription must resolve")
	}
	now := time.Now().UTC()
	for i, body := range bodies {
		var envelope events.V1Envelope
		if err := json.Unmarshal(body, &envelope); err != nil {
			t.Fatalf("delivery %d is not an envelope: %v", i, err)
		}
		if err := events.ValidateV1Envelope(envelope); err != nil {
			t.Fatalf("delivery %d envelope invalid: %v", i, err)
		}
		if err := events.VerifyWebhookSignature([]byte(events.FormatWhsecSecret(stored.Secret)), body, headers[i], now); err != nil {
			t.Fatalf("delivery %d signature rejected: %v", i, err)
		}
		if headers[i]["svix-id"] != envelope.ID {
			t.Fatalf("delivery id header = %q, want %q", headers[i]["svix-id"], envelope.ID)
		}
		if !strings.HasPrefix(headers[i]["svix-signature"], "v1,") {
			t.Fatalf("delivery %d svix-signature = %q, want v1,<base64>", i, headers[i]["svix-signature"])
		}
		if _, ok := headers[i]["svix-timestamp"]; !ok {
			t.Fatalf("delivery %d missing svix-timestamp", i)
		}
	}
}

// testSSEWriter is a race-safe SSE capture writer: the handler runs in a
// goroutine while the test polls the buffer, then cancels the request.
type testSSEWriter struct {
	mu      sync.Mutex
	header  http.Header
	body    bytes.Buffer
	status  int
	flushed chan struct{}
}

func newTestSSEWriter() *testSSEWriter {
	return &testSSEWriter{header: http.Header{}, flushed: make(chan struct{}, 64)}
}

func (w *testSSEWriter) Header() http.Header { return w.header }

func (w *testSSEWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.Write(p)
}

func (w *testSSEWriter) WriteHeader(status int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.status = status
}

func (w *testSSEWriter) Flush() {
	select {
	case w.flushed <- struct{}{}:
	default:
	}
}

func (w *testSSEWriter) snapshot() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.String()
}

func TestV1EventsSSEReplayAndLive(t *testing.T) {
	fx := newV1EventsFixture(t)
	_, first := eventIDEM(t, fx.engine, http.MethodPost,
		"/api/v1/workspaces/"+fx.ws.PublicID+"/tasks", 3, "evt-sse-1", v1CreateTaskBodyJSON("render"), nil)
	firstTaskID := first["data"].(map[string]any)["id"].(string)
	// Resume from the task.created envelope: replay must carry the gap
	// that follows it (billing.settled), then live delivery continues.
	missed, _, _ := fx.eventLog.ReplayAfter(fx.ws.PublicID, "evt_doesnotexist0000", events.WorkspaceEventFilter{})
	var cursor string
	for _, envelope := range missed {
		if envelope.Type == events.EventV1TaskCreated {
			cursor = envelope.ID
			break
		}
	}
	if cursor == "" {
		t.Fatal("expected a task.created envelope in the log")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, fx.eventsPath()+"?last_event_id="+cursor, nil).WithContext(ctx)
	req.Header.Set("X-Test-User", "3")
	req.Header.Set(contract.RequestIDHeader, "test-sse-replay-1")
	writer := newTestSSEWriter()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fx.engine.ServeHTTP(writer, req)
	}()
	waitFor := func(want string) string {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			snapshot := writer.snapshot()
			if strings.Contains(snapshot, want) {
				return snapshot
			}
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
		t.Fatalf("SSE stream never carried %q; got:\n%s", want, writer.snapshot())
		return ""
	}
	snapshot := waitFor(`"settlement_state":"held"`)
	if !strings.Contains(snapshot, "id: "+cursor) && !strings.Contains(snapshot, "id: evt_") {
		t.Fatalf("replay frames must carry id: cursors:\n%s", snapshot)
	}
	if strings.Contains(snapshot, `"type":"task.created"`) {
		t.Fatalf("replay after cursor must not re-deliver the cursor envelope:\n%s", snapshot)
	}
	// Live: cancelling the task while connected delivers task.cancelled.
	status, _ := eventIDEM(t, fx.engine, http.MethodPost,
		"/api/v1/workspaces/"+fx.ws.PublicID+"/tasks/"+firstTaskID+"/cancel", 3, "evt-sse-cancel", "", nil)
	if status != http.StatusOK {
		t.Fatalf("cancel = %d, want 200", status)
	}
	waitFor(`"type":"task.cancelled"`)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("SSE handler did not exit after cancel")
	}
}

func TestV1EventsSSEGapNotice(t *testing.T) {
	fx := newV1EventsFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, fx.eventsPath()+"?last_event_id=evt_doesnotexist0000", nil).WithContext(ctx)
	req.Header.Set("X-Test-User", "3")
	req.Header.Set(contract.RequestIDHeader, "test-sse-gap-1")
	writer := newTestSSEWriter()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fx.engine.ServeHTTP(writer, req)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if strings.Contains(writer.snapshot(), "event: gap") {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("unknown cursor must emit a gap notice; got:\n%s", writer.snapshot())
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("SSE handler did not exit after cancel")
	}
}

func TestV1EventsSSEFilterNarrows(t *testing.T) {
	fx := newV1EventsFixture(t)
	_, _ = eventIDEM(t, fx.engine, http.MethodPost,
		"/api/v1/workspaces/"+fx.ws.PublicID+"/tasks", 3, "evt-sse-filter", v1CreateTaskBodyJSON("render"), nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, fx.eventsPath()+"?last_event_id=evt_doesnotexist0000&types=media.asset_ready", nil).WithContext(ctx)
	req.Header.Set("X-Test-User", "3")
	req.Header.Set(contract.RequestIDHeader, "test-sse-filter-1")
	writer := newTestSSEWriter()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fx.engine.ServeHTTP(writer, req)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	snapshot := writer.snapshot()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("SSE handler did not exit after cancel")
	}
	if strings.Contains(snapshot, "task.created") {
		t.Fatalf("filtered stream must not carry task.created:\n%s", snapshot)
	}
	if !strings.Contains(snapshot, "event: gap") {
		t.Fatalf("filtered resume from unknown cursor must still gap-notice:\n%s", snapshot)
	}
}
