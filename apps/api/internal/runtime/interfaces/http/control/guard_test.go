package control

// Control Plane behavior tests. The fakes live in
// helpers_test.go; this file holds the matrix, audit,
// idempotency, envelope, rate-limit, and scope tests.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/testsupport"
)

type controlHarness struct {
	engine   *gin.Engine
	serve    func(call controlCall) (int, http.Header, map[string]any)
	sink     *ControlAuditMemorySink
	users    *fakeControlUsers
	identity *fakeIdentityAdminCommands
	credits  *fakeControlCredits
	tasks    *fakeControlTasks
	catalog  *fakeControlCatalog
	payments *fakeControlPayments
	store    *tenancy.InMemoryStore
	ws       *tenancy.Workspace
	dir      *staticUserDirectory
}

func newControlHarness(t *testing.T, mutate func(*ContractControlDeps)) *controlHarness {
	t.Helper()
	h := &controlHarness{
		sink:     &ControlAuditMemorySink{},
		users:    newFakeControlUsers(),
		identity: &fakeIdentityAdminCommands{},
		credits:  &fakeControlCredits{},
		tasks:    &fakeControlTasks{},
		catalog:  &fakeControlCatalog{},
		payments: &fakeControlPayments{},
		store:    tenancy.NewInMemoryStoreForTests(),
		dir:      newStaticUserDirectory(),
	}
	for _, id := range []int64{controlOperatorID, controlUserAID, controlUserBID, controlAdminTID} {
		h.dir.ensure(id)
	}
	ws, err := h.store.CreateWorkspace(context.Background(), tenancy.CreateWorkspaceParams{
		Name: "control-harness", OwnerUserID: controlOperatorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	h.ws = ws
	deps := ContractControlDeps{
		Users: h.users, Identity: h.identity, Impersonator: &fakeImpersonator{},
		Credits: h.credits, CreditUsers: fakeControlCreditDirectory{},
		CreditReads: fakeControlCreditReads{}, Reconciliation: fakeControlReconciliation{},
		Tasks: h.tasks, Tenancy: h.store, UserDirectory: h.dir, Boards: boards.NewInMemoryWorkspaceBoardStoreForTests(h.store, h.dir),
		BillingAccounts: fakeControlBillingAccounts{}, Catalog: h.catalog,
		Payments: h.payments, AuditSink: h.sink,
		Idempotency: contract.NewIdempotencyStore(contract.DefaultIdempotencyStoreOptions()),
		RateLimiter: NewControlRateLimiter(1000, time.Minute),
	}
	if mutate != nil {
		mutate(&deps)
	}
	engine := newControlTestEngine(deps)
	h.engine = engine
	h.serve = func(call controlCall) (int, http.Header, map[string]any) {
		return doControl(t, engine, call)
	}
	return h
}

func controlMutationHeaders(key string) map[string]string {
	return map[string]string{
		contract.IdempotencyKeyHeader: key,
	}
}

func TestControlAuthMatrix(t *testing.T) {
	h := newControlHarness(t, nil)
	targetUser := tenancy.FormatUserPublicID(testUUIDForUser(controlUserAID))
	grantBody := fmt.Sprintf(`{"user_id":%q,"amount_credits":10,"business_reference_id":"matrix-1"}`, targetUser)

	// canary routes: one read + one mutation per family-representative.
	reads := []controlCall{
		{method: http.MethodGet, path: "/api/control/v1/users"},
		{method: http.MethodGet, path: "/api/control/v1/tasks"},
		{method: http.MethodGet, path: "/api/control/v1/platform/monitor"},
	}
	mutations := []controlCall{
		{method: http.MethodPost, path: "/api/control/v1/credits/grants", body: grantBody,
			headers: controlMutationHeaders("matrix-key-1")},
		{method: http.MethodPost, path: "/api/control/v1/tasks/task_matrix123/cancel",
			headers: controlMutationHeaders("matrix-key-2")},
		{method: http.MethodPost, path: fmt.Sprintf("/api/control/v1/users/%s/impersonation", targetUser),
			headers: controlMutationHeaders("matrix-key-3")},
	}

	t.Run("anonymous denied with 401", func(t *testing.T) {
		for _, call := range append(reads, mutations...) {
			status, _, payload := h.serve(call)
			if status != http.StatusUnauthorized {
				t.Fatalf("%s %s anon status = %d, want 401", call.method, call.path, status)
			}
			if code := controlErrorCode(t, payload); code != string(contract.CodeUnauthorized) {
				t.Fatalf("anon code = %q, want unauthorized", code)
			}
		}
	})

	t.Run("non-admin denied with 403 even with public caps", func(t *testing.T) {
		for _, call := range append(reads, mutations...) {
			call.userID = controlUserAID
			call.role = "user"
			call.caps = []string{"workspace:read", "workspace:write", "board:read"}
			status, _, payload := h.serve(call)
			if status != http.StatusForbidden {
				t.Fatalf("%s %s member status = %d, want 403", call.method, call.path, status)
			}
			if code := controlErrorCode(t, payload); code != string(contract.CodeForbidden) {
				t.Fatalf("member code = %q, want forbidden", code)
			}
		}
	})

	t.Run("admin operator allowed via role grant", func(t *testing.T) {
		for _, call := range reads {
			call.userID = controlOperatorID
			call.role = "admin"
			call.caps = nil
			status, _, _ := h.serve(call)
			if status == http.StatusUnauthorized || status == http.StatusForbidden {
				t.Fatalf("%s %s admin status = %d, want allowed", call.method, call.path, status)
			}
		}
		for i, call := range mutations {
			call.userID = controlOperatorID
			call.role = "admin"
			call.caps = nil
			call.headers = controlMutationHeaders(fmt.Sprintf("matrix-allow-%d", i))
			status, _, _ := h.serve(call)
			if status == http.StatusUnauthorized || status == http.StatusForbidden {
				t.Fatalf("%s %s admin status = %d, want allowed", call.method, call.path, status)
			}
		}
	})

	t.Run("admin-only grant ignores family scoping", func(t *testing.T) {
		// Admin-only convergence: production sessions mint no family
		// capabilities, so the guard enforces the admin role directly and
		// family strings are audit labels only. A scoped-cap admin passes
		// every family.
		status, _, _ := h.serve(controlCall{
			method: http.MethodPost, path: "/api/control/v1/credits/grants", body: grantBody,
			userID: controlOperatorID, role: "admin", caps: []string{ControlCapabilityCredits},
			headers: controlMutationHeaders("matrix-scope-1"),
		})
		if status != http.StatusOK {
			t.Fatalf("scoped grant status = %d, want 200", status)
		}
		status, _, _ = h.serve(controlCall{
			method: http.MethodGet, path: "/api/control/v1/users",
			userID: controlOperatorID, role: "admin", caps: []string{ControlCapabilityCredits},
		})
		if status != http.StatusOK {
			t.Fatalf("scoped users list status = %d, want 200 (admin-only, caps ignored)", status)
		}
	})
}

func TestControlAuditFields(t *testing.T) {
	h := newControlHarness(t, nil)
	targetUser := tenancy.FormatUserPublicID(testUUIDForUser(controlUserAID))
	operatorUser := tenancy.FormatUserPublicID(testUUIDForUser(controlOperatorID))

	t.Run("mutation audit carries full fields", func(t *testing.T) {
		status, _, payload := h.serve(controlCall{
			method: http.MethodPost, path: fmt.Sprintf("/api/control/v1/users/%s/ban", targetUser),
			body:   `{"expires_in_days":7}`,
			userID: controlOperatorID, role: "admin",
			headers: controlMutationHeaders("audit-key-1"),
		})
		if status != http.StatusOK {
			t.Fatalf("ban status = %d, body=%v", status, payload)
		}
		records := h.sink.Records()
		if len(records) == 0 {
			t.Fatal("expected an audit record")
		}
		record := records[len(records)-1]
		if record.OperatorType != "human_session" || record.OperatorID != operatorUser {
			t.Fatalf("operator = %q %q, want human_session %q", record.OperatorType, record.OperatorID, operatorUser)
		}
		if record.TargetType != "user" || record.TargetID != targetUser {
			t.Fatalf("target = %q %q, want user %q", record.TargetType, record.TargetID, targetUser)
		}
		if record.Capability != ControlCapabilityUsers || record.Operation != "users.ban" {
			t.Fatalf("capability/operation = %q %q", record.Capability, record.Operation)
		}
		if record.RequestID == "" || !strings.HasPrefix(record.RequestID, "test-control-req-") {
			t.Fatalf("request_id = %q", record.RequestID)
		}
		if record.IdempotencyKey != "audit-key-1" {
			t.Fatalf("idempotency = %q", record.IdempotencyKey)
		}
		if record.Outcome != ControlAuditOK || record.Status != http.StatusOK {
			t.Fatalf("outcome/status = %q %d", record.Outcome, record.Status)
		}
		if record.OccurredAt.IsZero() {
			t.Fatal("occurred_at must be set")
		}
		if strings.Contains(record.OperatorID, "11") && !contract.IsUserID(record.OperatorID) {
			t.Fatal("operator id must be a public id, never a numeric id")
		}
	})

	t.Run("denied audit on forbidden read", func(t *testing.T) {
		before := len(h.sink.Records())
		status, _, _ := h.serve(controlCall{
			method: http.MethodGet, path: "/api/control/v1/users",
			userID: controlUserAID, role: "user",
		})
		if status != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", status)
		}
		records := h.sink.Records()
		if len(records) != before+1 {
			t.Fatalf("denied call must emit exactly one audit record, got %d new", len(records)-before)
		}
		last := records[len(records)-1]
		if last.Outcome != ControlAuditDenied || last.Status != http.StatusForbidden {
			t.Fatalf("denied outcome/status = %q %d", last.Outcome, last.Status)
		}
	})

	t.Run("failed audit on service error", func(t *testing.T) {
		h.tasks.failCancel = errControlBoom
		status, _, _ := h.serve(controlCall{
			method: http.MethodPost, path: "/api/control/v1/tasks/task_boom123/cancel",
			userID: controlOperatorID, role: "admin",
			headers: controlMutationHeaders("audit-key-2"),
		})
		h.tasks.failCancel = nil
		if status != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", status)
		}
		last := h.sink.Records()[len(h.sink.Records())-1]
		if last.Outcome != ControlAuditFailed {
			t.Fatalf("outcome = %q, want failed", last.Outcome)
		}
	})

	t.Run("impersonation audit names target", func(t *testing.T) {
		status, _, _ := h.serve(controlCall{
			method: http.MethodPost, path: fmt.Sprintf("/api/control/v1/users/%s/impersonation", targetUser),
			userID: controlOperatorID, role: "admin",
			headers: controlMutationHeaders("audit-key-3"),
		})
		if status != http.StatusOK {
			t.Fatalf("impersonation status = %d", status)
		}
		last := h.sink.Records()[len(h.sink.Records())-1]
		if last.Operation != "users.impersonate" || last.TargetID != targetUser || last.Outcome != ControlAuditOK {
			t.Fatalf("impersonation audit = %+v", last)
		}
	})

	t.Run("read audit outcome is read", func(t *testing.T) {
		status, _, _ := h.serve(controlCall{
			method: http.MethodGet, path: "/api/control/v1/tasks",
			userID: controlOperatorID, role: "admin",
		})
		if status != http.StatusOK {
			t.Fatalf("tasks list status = %d", status)
		}
		last := h.sink.Records()[len(h.sink.Records())-1]
		if last.Outcome != ControlAuditRead {
			t.Fatalf("read outcome = %q, want read", last.Outcome)
		}
	})
}

var errControlBoom = controlBoomError("boom")

type controlBoomError string

func (e controlBoomError) Error() string { return string(e) }

func TestControlIdempotencyReplayAndConflict(t *testing.T) {
	h := newControlHarness(t, nil)
	targetUser := tenancy.FormatUserPublicID(testUUIDForUser(controlUserAID))
	body := fmt.Sprintf(`{"user_id":%q,"amount_credits":5,"business_reference_id":"idem-1"}`, targetUser)

	t.Run("same key and body replays without re-executing", func(t *testing.T) {
		headers := controlMutationHeaders("idem-key-1")
		status, _, first := h.serve(controlCall{
			method: http.MethodPost, path: "/api/control/v1/credits/grants", body: body,
			userID: controlOperatorID, role: "admin", headers: headers,
		})
		if status != http.StatusOK {
			t.Fatalf("first status = %d, body=%v", status, first)
		}
		if h.credits.calls != 1 {
			t.Fatalf("credit calls = %d, want 1", h.credits.calls)
		}
		status, _, second := h.serve(controlCall{
			method: http.MethodPost, path: "/api/control/v1/credits/grants", body: body,
			userID: controlOperatorID, role: "admin", headers: headers,
		})
		if status != http.StatusOK {
			t.Fatalf("replay status = %d, body=%v", status, second)
		}
		if h.credits.calls != 1 {
			t.Fatalf("credit calls after replay = %d, want 1", h.credits.calls)
		}
		firstTx, _ := first["data"].(map[string]any)["transaction_id"]
		secondTx, _ := second["data"].(map[string]any)["transaction_id"]
		if firstTx != secondTx {
			t.Fatalf("replay transaction = %v, want %v", secondTx, firstTx)
		}
		records := h.sink.Records()
		last := records[len(records)-1]
		if !last.Replayed || last.Outcome != ControlAuditOK {
			t.Fatalf("replay audit = %+v, want replayed success", last)
		}
	})

	t.Run("same key with different body conflicts", func(t *testing.T) {
		other := fmt.Sprintf(`{"user_id":%q,"amount_credits":6,"business_reference_id":"idem-1"}`, targetUser)
		status, _, payload := h.serve(controlCall{
			method: http.MethodPost, path: "/api/control/v1/credits/grants", body: other,
			userID: controlOperatorID, role: "admin",
			headers: controlMutationHeaders("idem-key-1"),
		})
		if status != http.StatusConflict {
			t.Fatalf("conflict status = %d, want 409", status)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeIdempotencyConflict) {
			t.Fatalf("conflict code = %q, want idempotency_conflict", code)
		}
	})
}

func TestControlMutationIdempotency(t *testing.T) {
	h := newControlHarness(t, nil)
	targetUser := tenancy.FormatUserPublicID(testUUIDForUser(controlUserAID))

	t.Run("missing idempotency key is 400", func(t *testing.T) {
		status, _, payload := h.serve(controlCall{
			method: http.MethodPost, path: fmt.Sprintf("/api/control/v1/users/%s/unban", targetUser),
			userID: controlOperatorID, role: "admin",
		})
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", status)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeIdempotencyKeyRequired) {
			t.Fatalf("code = %q, want idempotency_key_required", code)
		}
	})

	t.Run("mutation executes with a key", func(t *testing.T) {
		status, _, _ := h.serve(controlCall{
			method: http.MethodPost, path: fmt.Sprintf("/api/control/v1/users/%s/ban", targetUser),
			body:   `{"expires_in_days":1}`,
			userID: controlOperatorID, role: "admin",
			headers: map[string]string{contract.IdempotencyKeyHeader: "mutation-key-2"},
		})
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
	})
}

func TestControlEnvelopeAndIdentifiers(t *testing.T) {
	h := newControlHarness(t, nil)

	t.Run("list envelope carries data and cursor meta", func(t *testing.T) {
		status, _, payload := h.serve(controlCall{
			method: http.MethodGet, path: "/api/control/v1/users",
			userID: controlOperatorID, role: "admin",
		})
		if status != http.StatusOK {
			t.Fatalf("status = %d, body=%v", status, payload)
		}
		data := controlData(t, payload)
		items, ok := data.([]any)
		if !ok || len(items) == 0 {
			t.Fatalf("data must be a non-empty array: %v", data)
		}
		meta, ok := payload["meta"].(map[string]any)
		if !ok {
			t.Fatalf("paginated response requires meta: %v", payload)
		}
		if _, ok := meta["has_more"]; !ok {
			t.Fatalf("meta requires has_more: %v", meta)
		}
		first, _ := items[0].(map[string]any)
		id, _ := first["id"].(string)
		if !contract.IsUserID(id) {
			t.Fatalf("user id = %q, want usr_ public id", id)
		}
	})

	t.Run("malformed user id is 400", func(t *testing.T) {
		status, _, payload := h.serve(controlCall{
			method: http.MethodGet, path: "/api/control/v1/users/not-a-user",
			userID: controlOperatorID, role: "admin",
		})
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", status)
		}
		controlErrorCode(t, payload)
	})

	t.Run("unknown user is 404 control_not_found", func(t *testing.T) {
		unknown := tenancy.FormatUserPublicID(testUUIDForUser(999))
		status, _, payload := h.serve(controlCall{
			method: http.MethodGet, path: "/api/control/v1/users/" + unknown,
			userID: controlOperatorID, role: "admin",
		})
		if status != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", status)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeControlNotFound) {
			t.Fatalf("code = %q, want control_not_found", code)
		}
	})
}

func TestControlCreditsOperatorAndErrors(t *testing.T) {
	h := newControlHarness(t, nil)
	targetUser := tenancy.FormatUserPublicID(testUUIDForUser(controlUserAID))
	operatorUser := tenancy.FormatUserPublicID(testUUIDForUser(controlOperatorID))

	t.Run("ledger metadata names explicit operator and header key", func(t *testing.T) {
		body := fmt.Sprintf(`{"user_id":%q,"amount_credits":3,"business_reference_id":"meta-1"}`, targetUser)
		status, _, payload := h.serve(controlCall{
			method: http.MethodPost, path: "/api/control/v1/credits/grants", body: body,
			userID: controlOperatorID, role: "admin",
			headers: controlMutationHeaders("meta-key-1"),
		})
		if status != http.StatusOK {
			t.Fatalf("status = %d, body=%v", status, payload)
		}
		if h.credits.lastMeta.Actor.Type != domain.ActorAdmin || h.credits.lastMeta.Actor.ID != operatorUser {
			t.Fatalf("actor = %+v, want admin %q", h.credits.lastMeta.Actor, operatorUser)
		}
		if h.credits.lastMeta.IdempotencyKey != "meta-key-1" {
			t.Fatalf("ledger idempotency key = %q", h.credits.lastMeta.IdempotencyKey)
		}
		if h.credits.lastMeta.Source != "control_grant" {
			t.Fatalf("source = %q, want control_grant", h.credits.lastMeta.Source)
		}

		data := controlData(t, payload).(map[string]any)
		wallet, _ := data["wallet"].(map[string]any)
		if wallet["user_id"] != targetUser {
			t.Fatalf("wallet user = %v, want %q", wallet["user_id"], targetUser)
		}
	})

	t.Run("restricted wallet renders billing_wallet_restricted", func(t *testing.T) {
		h.credits.fail = domain.ErrWalletRestricted
		defer func() { h.credits.fail = nil }()
		body := fmt.Sprintf(`{"user_id":%q,"amount_credits":3,"business_reference_id":"meta-2"}`, targetUser)
		status, _, payload := h.serve(controlCall{
			method: http.MethodPost, path: "/api/control/v1/credits/deductions", body: body,
			userID: controlOperatorID, role: "admin",
			headers: controlMutationHeaders("meta-key-2"),
		})
		if status != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", status)
		}
		if code := controlErrorCode(t, payload); code != string(contract.CodeBillingWalletRestricted) {
			t.Fatalf("code = %q, want billing_wallet_restricted", code)
		}
	})
}

func TestControlImpersonationGuards(t *testing.T) {
	h := newControlHarness(t, nil)
	targetUser := tenancy.FormatUserPublicID(testUUIDForUser(controlUserAID))
	operatorUser := tenancy.FormatUserPublicID(testUUIDForUser(controlOperatorID))
	adminTarget := tenancy.FormatUserPublicID(testUUIDForUser(controlAdminTID))

	t.Run("success passes cookies through", func(t *testing.T) {
		status, header, payload := h.serve(controlCall{
			method: http.MethodPost, path: fmt.Sprintf("/api/control/v1/users/%s/impersonation", targetUser),
			userID: controlOperatorID, role: "admin",
			headers: controlMutationHeaders("imp-key-1"),
		})
		if status != http.StatusOK {
			t.Fatalf("status = %d, body=%v", status, payload)
		}
		if len(header.Values("Set-Cookie")) == 0 {
			t.Fatal("impersonation must pass Set-Cookie through")
		}
		controlData(t, payload)
	})

	t.Run("self impersonation refused", func(t *testing.T) {
		status, _, _ := h.serve(controlCall{
			method: http.MethodPost, path: fmt.Sprintf("/api/control/v1/users/%s/impersonation", operatorUser),
			userID: controlOperatorID, role: "admin",
			headers: controlMutationHeaders("imp-key-2"),
		})
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", status)
		}
	})

	t.Run("admin target refused", func(t *testing.T) {
		status, _, _ := h.serve(controlCall{
			method: http.MethodPost, path: fmt.Sprintf("/api/control/v1/users/%s/impersonation", adminTarget),
			userID: controlOperatorID, role: "admin",
			headers: controlMutationHeaders("imp-key-3"),
		})
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", status)
		}
	})
}

func TestControlTaskScope(t *testing.T) {
	h := newControlHarness(t, nil)

	t.Run("cancel records operator actor", func(t *testing.T) {
		status, _, payload := h.serve(controlCall{
			method: http.MethodPost, path: "/api/control/v1/tasks/task_cancel123/cancel",
			userID: controlOperatorID, role: "admin",
			headers: controlMutationHeaders("cancel-key-1"),
		})
		if status != http.StatusOK {
			t.Fatalf("status = %d, body=%v", status, payload)
		}
		if h.tasks.cancelCalls != 1 || h.tasks.cancelActor != controlOperatorID || h.tasks.cancelTaskID != "task_cancel123" {
			t.Fatalf("cancel = calls %d actor %d task %q", h.tasks.cancelCalls, h.tasks.cancelActor, h.tasks.cancelTaskID)
		}
		last := h.sink.Records()[len(h.sink.Records())-1]
		if last.TargetID != "task_cancel123" || last.Capability != ControlCapabilityTasks {
			t.Fatalf("cancel audit = %+v", last)
		}
	})

	t.Run("malformed task id is 404", func(t *testing.T) {
		status, _, payload := h.serve(controlCall{
			method: http.MethodGet, path: "/api/control/v1/tasks/nope",
			userID: controlOperatorID, role: "admin",
		})
		if status != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", status)
		}
		controlErrorCode(t, payload)
	})

	t.Run("no task creation path on control", func(t *testing.T) {
		// POST /tasks/submit is not registered on control: gin has no
		// route, so the engine 404s (proving no duplicate creation
		// path). doControl requires JSON, so assert via raw round trip.
		req := httptest.NewRequest(http.MethodPost, "/api/control/v1/tasks/submit", strings.NewReader(`{"feature":"render"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(contract.RequestIDHeader, "test-control-submit")
		testsupport.SetTestUser(req, 11)
		testsupport.SetTestRole(req, "admin")
		for key, value := range controlMutationHeaders("submit-key-1") {
			req.Header.Set(key, value)
		}
		recorder := httptest.NewRecorder()
		h.engine.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("submit status = %d, want 404 (no creation path)", recorder.Code)
		}
	})
}

func TestControlWorkspaceOverrides(t *testing.T) {
	h := newControlHarness(t, nil)
	memberUser := tenancy.FormatUserPublicID(testUUIDForUser(controlUserAID))

	t.Run("membership override round trip", func(t *testing.T) {
		addBody := fmt.Sprintf(`{"user_id":%q,"role":"member"}`, memberUser)
		status, _, payload := h.serve(controlCall{
			method: http.MethodPost, path: fmt.Sprintf("/api/control/v1/workspaces/%s/members", h.ws.PublicID),
			body: addBody, userID: controlOperatorID, role: "admin",
			headers: controlMutationHeaders("ws-key-1"),
		})
		if status != http.StatusCreated {
			t.Fatalf("add status = %d, body=%v", status, payload)
		}
		status, _, payload = h.serve(controlCall{
			method: http.MethodGet, path: fmt.Sprintf("/api/control/v1/workspaces/%s/members", h.ws.PublicID),
			userID: controlOperatorID, role: "admin",
		})
		if status != http.StatusOK {
			t.Fatalf("members status = %d, body=%v", status, payload)
		}
		updateBody := `{"role":"admin"}`
		status, _, _ = h.serve(controlCall{
			method: http.MethodPatch, path: fmt.Sprintf("/api/control/v1/workspaces/%s/members/%s", h.ws.PublicID, memberUser),
			body: updateBody, userID: controlOperatorID, role: "admin",
			headers: controlMutationHeaders("ws-key-2"),
		})
		if status != http.StatusOK {
			t.Fatalf("update status = %d", status)
		}
		status, _, _ = h.serve(controlCall{
			method: http.MethodDelete, path: fmt.Sprintf("/api/control/v1/workspaces/%s/members/%s", h.ws.PublicID, memberUser),
			userID: controlOperatorID, role: "admin",
			headers: controlMutationHeaders("ws-key-3"),
		})
		if status != http.StatusOK {
			t.Fatalf("remove status = %d", status)
		}
	})

	t.Run("hidden workspace converges to 404", func(t *testing.T) {
		status, _, payload := h.serve(controlCall{
			method: http.MethodGet, path: "/api/control/v1/workspaces/ws_doesnotexist1",
			userID: controlOperatorID, role: "admin",
		})
		if status != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", status)
		}
		controlErrorCode(t, payload)
	})
}

func TestControlBillingAndPayments(t *testing.T) {
	h := newControlHarness(t, nil)

	t.Run("billing account resolves by workspace", func(t *testing.T) {
		status, _, payload := h.serve(controlCall{
			method: http.MethodGet, path: "/api/control/v1/billing/accounts?workspace_id=" + h.ws.PublicID,
			userID: controlOperatorID, role: "admin",
		})
		if status != http.StatusOK {
			t.Fatalf("status = %d, body=%v", status, payload)
		}
		items := controlData(t, payload).([]any)
		if len(items) != 1 || items[0].(map[string]any)["workspace_id"] != h.ws.PublicID {
			t.Fatalf("accounts = %v", items)
		}
	})

	t.Run("catalog publish records operator actor", func(t *testing.T) {
		status, _, _ := h.serve(controlCall{
			method: http.MethodPost, path: "/api/control/v1/billing/catalog/ai-image/prices",
			body:   `{"prices":{"1k":10,"2k":20}}`,
			userID: controlOperatorID, role: "admin",
			headers: controlMutationHeaders("cat-key-1"),
		})
		if status != http.StatusOK {
			t.Fatalf("status = %d", status)
		}
		if h.catalog.publishActor != controlOperatorID {
			t.Fatalf("publish actor = %d", h.catalog.publishActor)
		}
	})

	t.Run("payment refund carries header key and actor", func(t *testing.T) {
		status, _, _ := h.serve(controlCall{
			method: http.MethodPost, path: "/api/control/v1/payments/orders/ord_123/refunds",
			body:   `{"idea_quantity":5}`,
			userID: controlOperatorID, role: "admin",
			headers: controlMutationHeaders("pay-key-1"),
		})
		if status != http.StatusOK {
			t.Fatalf("status = %d", status)
		}
		if h.payments.refundCalls != 1 {
			t.Fatalf("refund calls = %d", h.payments.refundCalls)
		}
		cmd := h.payments.refundCmd
		if cmd.IdempotencyKey != "pay-key-1" || cmd.ActorID != controlOperatorID {
			t.Fatalf("refund command = %+v", cmd)
		}
	})
}

func TestControlRateLimitBucketSeparate(t *testing.T) {
	h := newControlHarness(t, func(d *ContractControlDeps) {
		d.RateLimiter = NewControlRateLimiter(2, time.Minute)
	})
	call := controlCall{method: http.MethodGet, path: "/api/control/v1/users", userID: controlOperatorID, role: "admin"}
	for i := 0; i < 2; i++ {
		if status, _, _ := h.serve(call); status != http.StatusOK {
			t.Fatalf("request %d status = %d, want 200", i, status)
		}
	}
	status, header, payload := h.serve(call)
	if status != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", status)
	}
	if code := controlErrorCode(t, payload); code != string(contract.CodeRateLimited) {
		t.Fatalf("code = %q, want rate_limited", code)
	}
	for _, name := range []string{"RateLimit-Limit", "RateLimit-Remaining", "RateLimit-Reset", "Retry-After"} {
		if header.Get(name) == "" {
			t.Fatalf("429 must carry %s", name)
		}
	}
}

func TestControlFailClosedWithoutDeps(t *testing.T) {
	engine := newControlTestEngine(ContractControlDeps{AuditSink: &ControlAuditMemorySink{}})
	call := func(c controlCall) (int, map[string]any) {
		status, _, payload := doControl(t, engine, c)
		return status, payload
	}
	admin := func(c controlCall) controlCall {
		c.userID = controlOperatorID
		c.role = "admin"
		return c
	}
	if status, _ := call(admin(controlCall{method: http.MethodGet, path: "/api/control/v1/users"})); status != http.StatusServiceUnavailable {
		t.Fatalf("unwired users list = %d, want 503", status)
	}
	if status, payload := call(admin(controlCall{
		method: http.MethodPut, path: "/api/control/v1/platform/settings/branding",
		body: `{}`, headers: controlMutationHeaders("fc-1"),
	})); status != http.StatusServiceUnavailable {
		t.Fatalf("unwired settings update = %d (%v), want 503", status, payload)
	}
	if status, payload := call(admin(controlCall{method: http.MethodGet, path: "/api/control/v1/platform/monitor"})); status != http.StatusNotImplemented {
		t.Fatalf("unwired monitor = %d (%v), want 501", status, payload)
	}
}

func TestControlNeverTouchesEventLayer(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{
			`"ideaboard/api/internal/events"`,
			`events.NATSJetStreamEventBus`,
			`events.WorkspaceEventLog`,
			`events.WebhookStore`,
			"JetStream",
			"PublishEvent",
		} {
			if strings.Contains(string(raw), banned) {
				t.Fatalf("%s must never touch the event layer (found %q)", name, banned)
			}
		}
	}
}

func TestControlMutationRejectsUnknownFields(t *testing.T) {
	targetUser := tenancy.FormatUserPublicID(testUUIDForUser(controlUserAID))
	for _, field := range []string{"reason", "operator_note"} {
		t.Run(field, func(t *testing.T) {
			h := newControlHarness(t, nil)
			calls := []controlCall{
				{method: http.MethodPost, path: "/api/control/v1/credits/grants",
					body: fmt.Sprintf(`{"user_id":%q,"amount_credits":5,"business_reference_id":"unknown-field",%q:"text"}`, targetUser, field)},
				{method: http.MethodPost, path: "/api/control/v1/payments/orders/ord_123/refunds",
					body: fmt.Sprintf(`{"idea_quantity":5,%q:"text"}`, field)},
				{method: http.MethodPost, path: "/api/control/v1/billing/catalog/ai-image/prices",
					body: fmt.Sprintf(`{"prices":{"1k":10},%q:"text"}`, field)},
			}
			for i, call := range calls {
				call.userID = controlOperatorID
				call.role = "admin"
				call.headers = controlMutationHeaders(fmt.Sprintf("unknown-field-%d", i))
				status, _, payload := h.serve(call)
				if status != http.StatusBadRequest || controlErrorCode(t, payload) != string(contract.CodeBadRequest) {
					t.Fatalf("%s status = %d payload = %v, want bad_request", call.path, status, payload)
				}
			}
			if h.credits.calls != 0 || h.payments.refundCalls != 0 || h.catalog.publishActor != 0 {
				t.Fatal("rejected payloads must not reach command seams")
			}
		})
	}
}
