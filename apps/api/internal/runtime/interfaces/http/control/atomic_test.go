package control

// Control Plane atomic guard tests without a database.
//
// These prove the guard's transaction discipline around the
// participant writer with stub seams: success commits business plus
// audit together, an audit failure rolls the (already executed)
// business call back instead of warn-continuing, a business failure
// still commits its failure audit row, and replays/conflicts keep the
// idempotency contract inside the transaction. The same-transaction
// visibility of business and audit rows is proven against real
// postgres in composition (control_audit_atomic_test.go); here the
// "business" is the fake identity command seam.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"

	identityports "ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"

	"github.com/jackc/pgx/v5"
)

var errControlAtomicDown = errors.New("atomic audit down")

// stubControlTxScope records guard transaction boundaries without a
// database. The carried transaction is intentionally nil: the stub
// audit writer ignores it, and the carrier round-trip is covered
// separately below.
type stubControlTxScope struct {
	mu        sync.Mutex
	begins    int
	commits   int
	rollbacks int
	beginErr  error
	commitErr error
}

func (s *stubControlTxScope) Begin(ctx context.Context) (context.Context, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.begins++
	if s.beginErr != nil {
		return nil, s.beginErr
	}
	return ContextWithControlTx(ctx, nil), nil
}

func (s *stubControlTxScope) Commit(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commits++
	if s.commitErr != nil {
		return s.commitErr
	}
	return nil
}

func (s *stubControlTxScope) Rollback(_ context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rollbacks++
}

func (s *stubControlTxScope) counts() (begins, commits, rollbacks int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.begins, s.commits, s.rollbacks
}

// stubControlTxAudit captures participant writes without a database.
type stubControlTxAudit struct {
	mu      sync.Mutex
	records []ControlAuditRecord
	durable bool
	err     error
}

func (s *stubControlTxAudit) AppendControlAuditTx(_ context.Context, _ pgx.Tx, record ControlAuditRecord) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record)
	if s.err != nil {
		return false, s.err
	}
	return s.durable, nil
}

func (s *stubControlTxAudit) last() (ControlAuditRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.records) == 0 {
		return ControlAuditRecord{}, false
	}
	return s.records[len(s.records)-1], true
}

type controlAtomicHarness struct {
	*controlHarness
	scope *stubControlTxScope
	audit *stubControlTxAudit
}

func newControlAtomicHarness(t *testing.T) *controlAtomicHarness {
	t.Helper()
	scope := &stubControlTxScope{}
	txAudit := &stubControlTxAudit{durable: true}
	h := newControlHarness(t, func(d *ContractControlDeps) {
		d.TxScope = scope
		d.TxAudit = txAudit
	})
	return &controlAtomicHarness{controlHarness: h, scope: scope, audit: txAudit}
}

func (h *controlAtomicHarness) ban(targetUser, key string) (int, map[string]any) {
	status, _, payload := h.serve(controlCall{
		method: http.MethodPost, path: fmt.Sprintf("/api/control/v1/users/%s/ban", targetUser),
		body: `{"expires_in_days":7}`, userID: controlOperatorID, role: "admin",
		headers: controlMutationHeaders(key),
	})
	return status, payload
}

func TestControlAtomicSuccessCommits(t *testing.T) {
	h := newControlAtomicHarness(t)
	targetUser := tenancy.FormatUserPublicID(testUUIDForUser(controlUserAID))

	status, payload := h.ban(targetUser, "atomic-key-1")
	if status != http.StatusOK {
		t.Fatalf("ban status = %d, body=%v", status, payload)
	}
	if _, _, rollbacks := h.scope.counts(); rollbacks != 0 {
		t.Fatalf("rollbacks = %d, want 0", rollbacks)
	}
	if begins, commits, _ := h.scope.counts(); begins != 1 || commits != 1 {
		t.Fatalf("begin/commit = %d/%d, want 1/1", begins, commits)
	}
	if h.identity.callCount("ban") != 1 {
		t.Fatalf("ban calls = %d, want 1", h.identity.callCount("ban"))
	}
	record, ok := h.audit.last()
	if !ok {
		t.Fatal("atomic path must join exactly one audit row into the transaction")
	}
	if record.Outcome != ControlAuditOK || record.Operation != "users.ban" || record.Status != http.StatusOK {
		t.Fatalf("atomic audit = %+v, want success users.ban 200", record)
	}
	if len(h.sink.Records()) != 0 {
		t.Fatalf("detached sink records = %d, want 0 (atomic path must not double-trail)", len(h.sink.Records()))
	}
}

func TestControlAtomicAuditFailureRollsBackBusiness(t *testing.T) {
	h := newControlAtomicHarness(t)
	h.audit.err = errControlAtomicDown
	targetUser := tenancy.FormatUserPublicID(testUUIDForUser(controlUserAID))

	status, payload := h.ban(targetUser, "atomic-key-2")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("ban with audit down = %d, want 503 (audit failure fails the call: semantic inversion vs step 1)", status)
	}
	if code := controlErrorCode(t, payload); code != string(contract.CodeServiceUnavailable) {
		t.Fatalf("code = %q, want service_unavailable", code)
	}
	// The business command RAN (it was already executed when the audit
	// join failed), yet the request reports failure and the
	// transaction rolled back: the response is buffered until commit,
	// so no 200 ever reached the wire.
	if h.identity.callCount("ban") != 1 {
		t.Fatalf("ban calls = %d, want 1 (business ran, then rolled back with the audit)", h.identity.callCount("ban"))
	}
	if _, commits, rollbacks := h.scope.counts(); commits != 0 || rollbacks != 1 {
		t.Fatalf("commit/rollback = %d/%d, want 0/1", commits, rollbacks)
	}
}

func TestControlAtomicBusinessFailureStillAudits(t *testing.T) {
	h := newControlAtomicHarness(t)
	h.identity.fail = &identityports.IdentityAdminCommandError{
		Kind:    identityports.IdentityAdminInvalid,
		Message: "bad role",
	}
	targetUser := tenancy.FormatUserPublicID(testUUIDForUser(controlUserAID))

	status, _ := h.ban(targetUser, "atomic-key-3")
	if status != http.StatusBadRequest {
		t.Fatalf("ban with failing business = %d, want the handler's 400 (not the atomic 503)", status)
	}
	// The domain-owned business write rolled back itself; the only
	// durable write left is the failure audit row, which commits so
	// the failed call stays on the trail.
	if _, commits, rollbacks := h.scope.counts(); commits != 1 || rollbacks != 0 {
		t.Fatalf("commit/rollback = %d/%d, want 1/0 (failure audit commits)", commits, rollbacks)
	}
	record, ok := h.audit.last()
	if !ok {
		t.Fatal("business failure must still join its failure audit row")
	}
	if record.Outcome != ControlAuditFailed || record.Status != http.StatusBadRequest {
		t.Fatalf("failure audit = %+v, want failed 400", record)
	}
}

func TestControlAtomicReplayAndConflict(t *testing.T) {
	h := newControlAtomicHarness(t)
	targetUser := tenancy.FormatUserPublicID(testUUIDForUser(controlUserAID))
	body := `{"expires_in_days":7}`
	call := func(key string) (int, map[string]any) {
		status, _, payload := h.serve(controlCall{
			method: http.MethodPost, path: fmt.Sprintf("/api/control/v1/users/%s/ban", targetUser),
			body: body, userID: controlOperatorID, role: "admin",
			headers: controlMutationHeaders(key),
		})
		return status, payload
	}

	if status, _ := call("atomic-key-4"); status != http.StatusOK {
		t.Fatalf("first status = %d, want 200", status)
	}
	if status, _ := call("atomic-key-4"); status != http.StatusOK {
		t.Fatalf("replay status = %d, want 200", status)
	}
	if h.identity.callCount("ban") != 1 {
		t.Fatalf("ban calls = %d, want 1 (replay must not re-execute)", h.identity.callCount("ban"))
	}
	if _, commits, rollbacks := h.scope.counts(); commits != 2 || rollbacks != 0 {
		t.Fatalf("commit/rollback = %d/%d, want 2/0 (replay commits its replay audit)", commits, rollbacks)
	}
	record, ok := h.audit.last()
	if !ok || !record.Replayed || record.Outcome != ControlAuditOK {
		t.Fatalf("replay audit = %+v, want replayed success", record)
	}

	status, payload := func() (int, map[string]any) {
		status, _, payload := h.serve(controlCall{
			method: http.MethodPost, path: fmt.Sprintf("/api/control/v1/users/%s/ban", targetUser),
			body: `{"expires_in_days":8}`, userID: controlOperatorID, role: "admin",
			headers: controlMutationHeaders("atomic-key-4"),
		})
		return status, payload
	}()
	if status != http.StatusConflict {
		t.Fatalf("conflict status = %d, want 409", status)
	}
	if code := controlErrorCode(t, payload); code != string(contract.CodeIdempotencyConflict) {
		t.Fatalf("conflict code = %q, want idempotency_conflict", code)
	}
	// The conflicting call rolls back its guard transaction, then emits
	// its failure audit through the detached sink. The participant audit
	// writer therefore still contains only the earlier two committed rows.
	if _, commits, rollbacks := h.scope.counts(); commits != 2 || rollbacks != 1 {
		t.Fatalf("commit/rollback = %d/%d, want 2/1", commits, rollbacks)
	}
	if got := len(h.audit.records); got != 2 {
		t.Fatalf("participant audit rows = %d, want 2 (conflict row must not join rolled-back tx)", got)
	}
	records := h.sink.Records()
	if len(records) != 1 {
		t.Fatalf("detached failure audit rows = %d, want 1", len(records))
	}
	if records[0].Outcome != ControlAuditFailed || records[0].Status != http.StatusConflict {
		t.Fatalf("detached failure audit = %+v, want failed 409", records[0])
	}
}

func TestControlAtomicBeginFailureFailsClosed(t *testing.T) {
	h := newControlAtomicHarness(t)
	h.scope.beginErr = errControlAtomicDown
	targetUser := tenancy.FormatUserPublicID(testUUIDForUser(controlUserAID))

	status, _ := h.ban(targetUser, "atomic-key-5")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("ban with scope down = %d, want 503", status)
	}
	if h.identity.callCount("ban") != 0 {
		t.Fatal("a transaction that never began must never reach the business seam")
	}
	records := h.sink.Records()
	if len(records) != 1 || records[0].Outcome != ControlAuditFailed || records[0].Status != http.StatusServiceUnavailable {
		t.Fatalf("detached begin-failure audit = %+v, want one failed 503 row", records)
	}
}

func TestControlAtomicSlogOnlySkipStillCommits(t *testing.T) {
	h := newControlAtomicHarness(t)
	h.audit.durable = false // service subject shape: slog trail only
	targetUser := tenancy.FormatUserPublicID(testUUIDForUser(controlUserAID))

	status, _ := h.ban(targetUser, "atomic-key-6")
	if status != http.StatusOK {
		t.Fatalf("ban with slog-only audit = %d, want 200 (step-1 skip parity)", status)
	}
	if _, commits, rollbacks := h.scope.counts(); commits != 1 || rollbacks != 0 {
		t.Fatalf("commit/rollback = %d/%d, want 1/0", commits, rollbacks)
	}
}

// stubControlAtomicCarrier proves the context carrier round-trip the
// guard and the participant writer rely on.
type stubControlAtomicCarrier struct {
	pgx.Tx
}

func TestControlTxContextRoundTrip(t *testing.T) {
	if _, ok := ControlTxFromContext(context.Background()); ok {
		t.Fatal("empty context carries no transaction")
	}
	if _, ok := ControlTxFromContext(ContextWithControlTx(context.Background(), nil)); ok {
		t.Fatal("nil transaction must report absent, never a joinable tx")
	}
	txCtx := ContextWithControlTx(context.Background(), stubControlAtomicCarrier{})
	tx, ok := ControlTxFromContext(txCtx)
	if !ok || tx == nil {
		t.Fatal("staged transaction must round-trip through the context")
	}
}
