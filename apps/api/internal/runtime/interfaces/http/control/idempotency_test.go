package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ideaboard/api/internal/identity/principal"
	identityports "ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/idempotency"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ---- merged from (base idempotency_test.go) ----
// Control durable request identity unit tests:
// the canonical hash pins method + route template + the real target +
// body, so the same key against a different target or body is a
// different request (409).

func testDurablePrincipal(t *testing.T) principal.Principal {
	t.Helper()
	p, err := principal.New(11, testUUIDForUser(11), "auth-11", "test-operator", "admin")
	if err != nil {
		t.Fatalf("principal.New: %v", err)
	}
	return p
}

func TestControlDurableHashPinsTarget(t *testing.T) {
	p := testDurablePrincipal(t)
	record := &ControlAuditRecord{TargetType: "user", TargetID: "usr_aaa"}
	same := controlDurableClaim(p, "users.ban", "hash-key-1", record, http.MethodPost, "/api/control/v1/users/:user_id/ban", []byte(`{"expires_in_days":7}`))

	otherTarget := &ControlAuditRecord{TargetType: "user", TargetID: "usr_bbb"}
	divergent := controlDurableClaim(p, "users.ban", "hash-key-1", otherTarget, http.MethodPost, "/api/control/v1/users/:user_id/ban", []byte(`{"expires_in_days":7}`))
	if divergent.RequestHash == same.RequestHash {
		t.Fatal("same key + same body + different target must hash differently (target conflict 409s)")
	}
	if divergent.Operator != same.Operator || divergent.Operation != same.Operation || divergent.Key != same.Key {
		t.Fatalf("scope moved with the target: %+v vs %+v", divergent, same)
	}

	otherBody := controlDurableClaim(p, "users.ban", "hash-key-1", record, http.MethodPost, "/api/control/v1/users/:user_id/ban", []byte(`{"expires_in_days":30}`))
	if otherBody.RequestHash == same.RequestHash {
		t.Fatal("same key + same target + different body must hash differently (body conflict 409s)")
	}

	repeat := controlDurableClaim(p, "users.ban", "hash-key-1", &ControlAuditRecord{TargetType: "user", TargetID: "usr_aaa"}, http.MethodPost, "/api/control/v1/users/:user_id/ban", []byte(`{"expires_in_days":7}`))
	if repeat.RequestHash != same.RequestHash {
		t.Fatal("same request must hash identically (retry replays)")
	}
	if repeat.TargetType != "user" || repeat.TargetID != "usr_aaa" {
		t.Fatalf("claim target = %q/%q, want the real audit target", repeat.TargetType, repeat.TargetID)
	}
}

func TestControlDurableHashTracksBody(t *testing.T) {
	p := testDurablePrincipal(t)
	record := &ControlAuditRecord{TargetType: "user", TargetID: "usr_aaa"}
	body := []byte(`{"expires_in_days":7}`)
	first := controlDurableClaim(p, "users.ban", "hash-key-2", record, http.MethodPost, "/api/control/v1/users/:user_id/ban", body)
	again := controlDurableClaim(p, "users.ban", "hash-key-2", record, http.MethodPost, "/api/control/v1/users/:user_id/ban", body)
	if again.RequestHash != first.RequestHash {
		t.Fatal("identical requests must hash identically ")
	}
	changed := controlDurableClaim(p, "users.ban", "hash-key-2", record, http.MethodPost, "/api/control/v1/users/:user_id/ban", []byte(`{"expires_in_days":8}`))
	if changed.RequestHash == first.RequestHash {
		t.Fatal("different bodies must hash differently")
	}
}

// ---- merged from idempotency_durable_test.go ----
// Control Plane durable idempotency guard tests, against real
// PostgreSQL with production-equivalent wiring pieces.
//
// Each harness wires the real guard transaction scope over the test
// pool (pgControlTxScope mirrors composition/control.ControlTxScope statement
// for statement), the real durable authority
// (idempotency.NewControlStore), a dedicated process-local replay
// store, and the stub participant audit writer. The business seam is
// the fake identity admin commands, which carry NO durable dedup fact
// of their own: every replay/conflict/single-winner assertion below
// therefore proves the guard-level durable guarantee, including for
// Control mutations without their own dedup.

// pgControlTxScope is the test-local guard transaction scope over a
// real pool. It mirrors composition/control.ControlTxScope (one pgx transaction
// per guarded mutation, carried on the request context) so these tests
// run the production transaction discipline.
type pgControlTxScope struct {
	db *pgxpool.Pool
}

func (s *pgControlTxScope) Begin(ctx context.Context) (context.Context, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("control tx scope: database is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	return ContextWithControlTx(ctx, tx), nil
}

func (s *pgControlTxScope) Commit(ctx context.Context) error {
	tx, ok := ControlTxFromContext(ctx)
	if !ok {
		return errors.New("control atomic audit requires the guard transaction")
	}
	return tx.Commit(ctx)
}

func (s *pgControlTxScope) Rollback(ctx context.Context) {
	tx, ok := ControlTxFromContext(ctx)
	if !ok {
		return
	}
	rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = tx.Rollback(rollbackCtx)
}

type durableControlHarness struct {
	*controlHarness
	pool  *pgxpool.Pool
	scope *pgControlTxScope
	audit *stubControlTxAudit
}

func newDurableControlHarness(t *testing.T, pool *pgxpool.Pool) *durableControlHarness {
	t.Helper()
	scope := &pgControlTxScope{db: pool}
	txAudit := &stubControlTxAudit{durable: true}
	h := newControlHarness(t, func(d *ContractControlDeps) {
		d.TxScope = scope
		d.TxAudit = txAudit
		d.IdempotencyDurable = idempotency.NewControlStore(pool)
	})
	return &durableControlHarness{controlHarness: h, pool: pool, scope: scope, audit: txAudit}
}

func durableBanCall(targetUser, key, body string) controlCall {
	if body == "" {
		body = `{"expires_in_days":7}`
	}
	return controlCall{
		method: http.MethodPost, path: fmt.Sprintf("/api/control/v1/users/%s/ban", targetUser),
		body: body, userID: controlOperatorID, role: "admin",
		headers: controlMutationHeaders(key),
	}
}

func durableTargetUser(id int64) string {
	return tenancy.FormatUserPublicID(testUUIDForUser(id))
}

// serveConcurrent issues one guarded call without touching *testing.T,
// so concurrent goroutines can share it (testing.T is confined to the
// test goroutine; assertions happen on the collected outcomes).
func serveConcurrent(engine *gin.Engine, seq *atomic.Int64, call controlCall) (int, []byte, error) {
	reader := strings.NewReader(call.body)
	req := httptest.NewRequest(call.method, call.path, reader)
	if call.body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	requestID := fmt.Sprintf("test-control-durable-%d", seq.Add(1))
	req.Header.Set(contract.RequestIDHeader, requestID)
	if call.userID > 0 {
		testsupport.SetTestUser(req, call.userID)
	}
	testsupport.SetTestRole(req, call.role)
	for key, value := range call.headers {
		req.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if got := recorder.Header().Get(contract.RequestIDHeader); got != requestID {
		return 0, nil, fmt.Errorf("Request-Id = %q, want %q", got, requestID)
	}
	return recorder.Code, recorder.Body.Bytes(), nil
}

type durableOutcome struct {
	status int
	body   []byte
	err    error
}

func TestControlDurableConcurrentSameKeySingleWinner(t *testing.T) {
	pool := testdb.Open(t)
	h := newDurableControlHarness(t, pool)
	targetUser := durableTargetUser(controlUserAID)

	const callers = 8
	outcomes := make([]durableOutcome, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			status, body, err := serveConcurrent(h.engine, &controlRequestSeq, durableBanCall(targetUser, "durable-winner-1", ""))
			outcomes[i] = durableOutcome{status: status, body: body, err: err}
		}(i)
	}
	wg.Wait()
	var first map[string]any
	for i, outcome := range outcomes {
		if outcome.err != nil {
			t.Fatalf("caller %d error: %v", i, outcome.err)
		}
		if outcome.status != http.StatusOK {
			t.Fatalf("caller %d status = %d, want 200 (body=%s)", i, outcome.status, outcome.body)
		}
		var payload map[string]any
		if err := json.Unmarshal(outcome.body, &payload); err != nil {
			t.Fatalf("caller %d response is not JSON: %v", i, err)
		}
		if first == nil {
			first = payload
		} else if fmt.Sprintf("%v", payload["data"]) != fmt.Sprintf("%v", first["data"]) {
			t.Fatalf("caller %d data = %v, want identical replay %v", i, payload["data"], first["data"])
		}
	}
	// users.ban carries no domain dedup fact: exactly one business
	// execution proves the guard-level durable election.
	if got := h.identity.callCount("ban"); got != 1 {
		t.Fatalf("ban calls = %d, want 1 (single winner across concurrent claimants)", got)
	}
}

func TestControlDurableReplayAcrossInstancesAndRestart(t *testing.T) {
	pool := testdb.Open(t)
	targetUser := durableTargetUser(controlUserAID)

	h1 := newDurableControlHarness(t, pool)
	status, _, first := h1.serve(durableBanCall(targetUser, "durable-restart-1", ""))
	if status != http.StatusOK {
		t.Fatalf("first ban status = %d, want 200", status)
	}
	if got := h1.identity.callCount("ban"); got != 1 {
		t.Fatalf("first ban calls = %d, want 1", got)
	}

	// A second instance shares the database but nothing else: its own
	// process-local store starts empty, exactly like a restarted
	// process. The retry must replay without executing.
	h2 := newDurableControlHarness(t, pool)
	status, _, second := h2.serve(durableBanCall(targetUser, "durable-restart-1", ""))
	if status != http.StatusOK {
		t.Fatalf("cross-instance replay status = %d, want 200", status)
	}
	if got := h2.identity.callCount("ban"); got != 0 {
		t.Fatalf("cross-instance ban calls = %d, want 0 (durable replay, no execution)", got)
	}
	if fmt.Sprintf("%v", second["data"]) != fmt.Sprintf("%v", first["data"]) {
		t.Fatalf("replay data = %v, want %v (commit-after-response-loss retry replays)", second["data"], first["data"])
	}
	// The atomic path joins the replay audit into the participant sink,
	// never the detached sink (no double trail).
	record, ok := h2.audit.last()
	if !ok || !record.Replayed || record.Outcome != ControlAuditOK {
		t.Fatalf("replay audit = %+v, want replayed success", record)
	}
	if len(h2.sink.Records()) != 0 {
		t.Fatalf("detached sink records = %d, want 0 (atomic path must not double-trail)", len(h2.sink.Records()))
	}
}

func TestControlDurableTargetConflictBeforeSideEffect(t *testing.T) {
	pool := testdb.Open(t)
	h := newDurableControlHarness(t, pool)
	userA := durableTargetUser(controlUserAID)
	userB := durableTargetUser(controlUserBID)

	status, _, _ := h.serve(durableBanCall(userA, "durable-target-1", ""))
	if status != http.StatusOK {
		t.Fatalf("first ban status = %d, want 200", status)
	}
	// Same key, same body, different target: the real target enters the
	// request identity, so this is a different request and must 409
	// before user B's ban ever runs.
	status, _, payload := h.serve(durableBanCall(userB, "durable-target-1", ""))
	if status != http.StatusConflict {
		t.Fatalf("target-conflict status = %d, want 409", status)
	}
	if code := controlErrorCode(t, payload); code != string(contract.CodeIdempotencyConflict) {
		t.Fatalf("target-conflict code = %q, want idempotency_conflict", code)
	}
	if got := h.identity.callCount("ban"); got != 1 {
		t.Fatalf("ban calls = %d, want 1 (conflict runs no side effect)", got)
	}
	// The conflict never commits, so its trail lives only in the detached
	// sink (failTx rolls the guard transaction back); the TxAudit stub
	// still holds the first request's committed success record.
	records := h.sink.Records()
	if len(records) == 0 {
		t.Fatalf("conflict audit missing from detached sink")
	}
	record := records[len(records)-1]
	if record.Outcome != ControlAuditFailed || record.Status != http.StatusConflict {
		t.Fatalf("conflict audit = %+v, want failed 409", record)
	}
}

func TestControlDurableBodyConflictBeforeSideEffect(t *testing.T) {
	pool := testdb.Open(t)
	h := newDurableControlHarness(t, pool)
	targetUser := durableTargetUser(controlUserAID)

	status, _, _ := h.serve(durableBanCall(targetUser, "durable-body-1", `{"expires_in_days":7}`))
	if status != http.StatusOK {
		t.Fatalf("first ban status = %d, want 200", status)
	}
	status, _, payload := h.serve(durableBanCall(targetUser, "durable-body-1", `{"expires_in_days":30}`))
	if status != http.StatusConflict {
		t.Fatalf("body-conflict status = %d, want 409", status)
	}
	if code := controlErrorCode(t, payload); code != string(contract.CodeIdempotencyConflict) {
		t.Fatalf("body-conflict code = %q, want idempotency_conflict", code)
	}
	if got := h.identity.callCount("ban"); got != 1 {
		t.Fatalf("ban calls = %d, want 1 (conflict runs no side effect)", got)
	}
}

// errTestControlDurableBusiness is the retryable business failure for
// the re-execution test: a 400 the guard never retains.
var errTestControlDurableBusiness = &identityports.IdentityAdminCommandError{
	Kind:    identityports.IdentityAdminInvalid,
	Message: "durable retry probe",
}

func TestControlDurableBusinessFailureRetriesReExecute(t *testing.T) {
	pool := testdb.Open(t)
	h := newDurableControlHarness(t, pool)
	targetUser := durableTargetUser(controlUserAID)

	// Business 4xx outcomes are never retained: the claim is abandoned
	// in the same transaction, so the retry re-executes.
	h.identity.fail = errTestControlDurableBusiness
	status, _, _ := h.serve(durableBanCall(targetUser, "durable-retry-1", ""))
	if status != http.StatusBadRequest {
		t.Fatalf("failing ban status = %d, want 400", status)
	}
	h.identity.fail = nil
	status, _, _ = h.serve(durableBanCall(targetUser, "durable-retry-1", ""))
	if status != http.StatusOK {
		t.Fatalf("retry ban status = %d, want 200", status)
	}
	if got := h.identity.callCount("ban"); got != 2 {
		t.Fatalf("ban calls = %d, want 2 (failure is not replayed)", got)
	}
	// The success is now retained: a third identical call replays.
	status, _, _ = h.serve(durableBanCall(targetUser, "durable-retry-1", ""))
	if status != http.StatusOK {
		t.Fatalf("replay ban status = %d, want 200", status)
	}
	if got := h.identity.callCount("ban"); got != 2 {
		t.Fatalf("ban calls = %d, want 2 (success replays)", got)
	}
}

// failingDurableControlStore fails every claim: the guard must render
// a retryable 503 without running the handler and without poisoning
// the key.
type failingDurableControlStore struct {
	err error
}

func (s *failingDurableControlStore) Claim(_ context.Context, _ pgx.Tx, _ idempotency.ControlClaim) (idempotency.ClaimResult, error) {
	return idempotency.ClaimResult{}, s.err
}

func (s *failingDurableControlStore) Complete(_ context.Context, _ pgx.Tx, _ idempotency.ControlClaim, _ idempotency.ControlResult) error {
	return s.err
}

func (s *failingDurableControlStore) Abandon(_ context.Context, _ pgx.Tx, _ idempotency.ControlClaim) error {
	return s.err
}

func (s *failingDurableControlStore) MarkUnknown(_ context.Context, _ pgx.Tx, _ idempotency.ControlClaim) error {
	return s.err
}

func (s *failingDurableControlStore) TryAbandonStale(_ context.Context, _ pgx.Tx, _ idempotency.ControlClaim, _ time.Time) (bool, error) {
	return false, s.err
}

func (s *failingDurableControlStore) PurgeExpired(_ context.Context) (int64, error) {
	return 0, s.err
}

func TestControlDurableClaimFailureIsRetryable(t *testing.T) {
	pool := testdb.Open(t)
	targetUser := durableTargetUser(controlUserAID)

	broken := newControlHarness(t, func(d *ContractControlDeps) {
		d.TxScope = &pgControlTxScope{db: pool}
		d.TxAudit = &stubControlTxAudit{durable: true}
		d.IdempotencyDurable = &failingDurableControlStore{err: errTestControlDurableDown}
	})
	status, _, payload := broken.serve(durableBanCall(targetUser, "durable-down-1", ""))
	if status != http.StatusServiceUnavailable {
		t.Fatalf("claim-down status = %d, want 503 (retryable, never 409/200)", status)
	}
	if code := controlErrorCode(t, payload); code != string(contract.CodeServiceUnavailable) {
		t.Fatalf("claim-down code = %q, want service_unavailable", code)
	}
	if got := broken.identity.callCount("ban"); got != 0 {
		t.Fatalf("ban calls = %d, want 0 (claim failure runs no side effect)", got)
	}

	// The key is not poisoned: the same request against the healthy
	// durable authority executes exactly once.
	h := newDurableControlHarness(t, pool)
	status, _, _ = h.serve(durableBanCall(targetUser, "durable-down-1", ""))
	if status != http.StatusOK {
		t.Fatalf("retry status = %d, want 200", status)
	}
	if got := h.identity.callCount("ban"); got != 1 {
		t.Fatalf("retry ban calls = %d, want 1", got)
	}
}

var errTestControlDurableDown = errors.New("durable control store down")
