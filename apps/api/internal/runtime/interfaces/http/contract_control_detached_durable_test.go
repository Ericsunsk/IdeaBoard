package httpapi

// Detached mutations persist fenced dispatch intent before external I/O and
// settle through a separate bounded transaction. Completed responses replay;
// failed settlement retains uncertainty without re-sending. Only undispatched
// in-progress claims may be taken over after the lease.

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/idempotency"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPrincipalForDetached() (principal.Principal, error) {
	return principal.New(controlOperatorID, testUUIDForUser(controlOperatorID), "auth-11", "test-user-11", "admin")
}

func newDetachedMailHarness(t *testing.T, pool *pgxpool.Pool, mail *fakeControlMail) *controlHarness {
	t.Helper()
	return newControlHarness(t, func(d *ContractControlDeps) {
		d.Mail = mail
		d.TxScope = &pgControlTxScope{db: pool}
		d.TxAudit = &stubControlTxAudit{durable: true}
		d.IdempotencyDurable = idempotency.NewControlStore(pool)
	})
}

func newDetachedStripeHarness(t *testing.T, pool *pgxpool.Pool, payments *fakeControlPayments) *controlHarness {
	t.Helper()
	return newControlHarness(t, func(d *ContractControlDeps) {
		d.Payments = payments
		d.TxScope = &pgControlTxScope{db: pool}
		d.TxAudit = &stubControlTxAudit{durable: true}
		d.IdempotencyDurable = idempotency.NewControlStore(pool)
	})
}

func detachedMailTestCall(key, reason, body string) controlCall {
	if body == "" {
		body = `{"recipient":"ops@example.com"}`
	}
	return controlCall{
		method: http.MethodPost, path: "/api/control/v1/platform/settings/mail/test",
		body: body, userID: controlOperatorID, role: "admin",
		headers: mailMutationHeaders(key, reason),
	}
}

func TestControlDetachedMailProbeReplayAcrossInstances(t *testing.T) {
	pool := testdb.Open(t)
	mail1 := newFakeControlMail()
	h1 := newDetachedMailHarness(t, pool, mail1)
	status, _, _ := h1.serve(detachedMailTestCall("detached-mail-1", "verify", ""))
	if status != http.StatusOK {
		t.Fatalf("first probe status = %d, want 200", status)
	}
	mail1.mu.Lock()
	if len(mail1.recipients) != 1 {
		mail1.mu.Unlock()
		t.Fatalf("first sends = %d, want 1", len(mail1.recipients))
	}
	mail1.mu.Unlock()

	// Second instance: fresh L1, same DB. Must replay without resending.
	mail2 := newFakeControlMail()
	h2 := newDetachedMailHarness(t, pool, mail2)
	status, _, _ = h2.serve(detachedMailTestCall("detached-mail-1", "verify", ""))
	if status != http.StatusOK {
		t.Fatalf("cross-instance replay status = %d, want 200", status)
	}
	mail2.mu.Lock()
	defer mail2.mu.Unlock()
	if len(mail2.recipients) != 0 {
		t.Fatalf("cross-instance replay must not resend: recipients = %v", mail2.recipients)
	}
	records := h2.sink.Records()
	if last := records[len(records)-1]; !last.Replayed || last.Outcome != ControlAuditOK {
		t.Fatalf("replay audit = %+v, want replayed success", last)
	}
}

func TestControlDetachedMailProbeConflictBeforeSend(t *testing.T) {
	pool := testdb.Open(t)
	mail := newFakeControlMail()
	h := newDetachedMailHarness(t, pool, mail)
	if status, _, _ := h.serve(detachedMailTestCall("detached-mail-conflict-1", "verify", `{"recipient":"ops@example.com"}`)); status != http.StatusOK {
		t.Fatalf("first status = %d, want 200", status)
	}
	status, _, payload := h.serve(detachedMailTestCall("detached-mail-conflict-1", "verify", `{"recipient":"other@example.com"}`))
	if status != http.StatusConflict {
		t.Fatalf("divergent status = %d, want 409", status)
	}
	if code := controlErrorCode(t, payload); code != string(contract.CodeIdempotencyConflict) {
		t.Fatalf("code = %q, want idempotency_conflict", code)
	}
	mail.mu.Lock()
	defer mail.mu.Unlock()
	if len(mail.recipients) != 1 {
		t.Fatalf("conflict must not send: recipients = %v", mail.recipients)
	}
}

func TestControlDetachedMailProbeFailureReExecutes(t *testing.T) {
	pool := testdb.Open(t)
	mail := newFakeControlMail()
	mail.testErr = &fakeMailProbeError{status: http.StatusBadGateway}
	h := newDetachedMailHarness(t, pool, mail)
	call := detachedMailTestCall("detached-mail-fail-1", "verify", "")
	if status, _, _ := h.serve(call); status != http.StatusServiceUnavailable {
		t.Fatalf("failure status = %d, want 503", status)
	}
	mail.mu.Lock()
	mail.testErr = nil
	mail.mu.Unlock()
	if status, _, _ := h.serve(call); status != http.StatusOK {
		t.Fatalf("retry status = %d, want 200", status)
	}
	mail.mu.Lock()
	defer mail.mu.Unlock()
	if len(mail.recipients) != 2 {
		t.Fatalf("failure must not poison the key: sends = %d, want 2 (fail + retry)", len(mail.recipients))
	}
}

type fakeMailProbeError struct{ status int }

func (e *fakeMailProbeError) Error() string { return "probe down" }

func TestControlDetachedMailUpdateReplayAcrossInstances(t *testing.T) {
	pool := testdb.Open(t)
	mail1 := newFakeControlMail()
	h1 := newDetachedMailHarness(t, pool, mail1)
	call := controlCall{
		method: http.MethodPut, path: "/api/control/v1/platform/settings/mail",
		body:   `{"from_name":"Ops","from_email":"ops@example.com"}`,
		userID: controlOperatorID, role: "admin",
		headers: mailMutationHeaders("detached-mail-put-1", "rotate"),
	}
	if status, _, _ := h1.serve(call); status != http.StatusOK {
		t.Fatalf("first update status = %d, want 200", status)
	}
	mail2 := newFakeControlMail()
	h2 := newDetachedMailHarness(t, pool, mail2)
	if status, _, _ := h2.serve(call); status != http.StatusOK {
		t.Fatalf("replay update status = %d, want 200", status)
	}
	mail2.mu.Lock()
	defer mail2.mu.Unlock()
	if len(mail2.updates) != 0 {
		t.Fatalf("replay must not re-call Auth: updates = %d, want 0", len(mail2.updates))
	}
}

func TestControlDetachedStripeConfigReplayAcrossInstances(t *testing.T) {
	pool := testdb.Open(t)
	pay1 := &fakeControlPayments{}
	h1 := newDetachedStripeHarness(t, pool, pay1)
	call := controlCall{
		method: http.MethodPut, path: "/api/control/v1/payments/providers/stripe",
		body:   `{"purchases_enabled":true}`,
		userID: controlOperatorID, role: "admin",
		headers: controlMutationHeaders("detached-stripe-1", "rotate keys"),
	}
	if status, _, _ := h1.serve(call); status != http.StatusOK {
		t.Fatalf("first stripe status = %d, want 200", status)
	}
	if pay1.configureCalls != 1 {
		t.Fatalf("configure calls = %d, want 1", pay1.configureCalls)
	}
	pay2 := &fakeControlPayments{}
	h2 := newDetachedStripeHarness(t, pool, pay2)
	if status, _, _ := h2.serve(call); status != http.StatusOK {
		t.Fatalf("replay stripe status = %d, want 200", status)
	}
	pay2.mu.Lock()
	defer pay2.mu.Unlock()
	if pay2.configureCalls != 0 {
		t.Fatalf("replay must not re-prepare Stripe: calls = %d, want 0", pay2.configureCalls)
	}
}

func TestControlDetachedInProgressIsRetryableAndStaleTakeover(t *testing.T) {
	pool := testdb.Open(t)
	mail := newFakeControlMail()
	h := newDetachedMailHarness(t, pool, mail)
	ctx := context.Background()

	// Manually stage a fresh in-progress claim with the same durable
	// identity the guard will compute: same operator/operation/key plus
	// the canonical request hash for this exact request.
	p, _ := testPrincipalForDetached()
	record := &ControlAuditRecord{}
	durable := controlDurableClaim(p, "platform.test_mail_setting", "detached-inprog-1", record, http.MethodPost, "/api/control/v1/platform/settings/mail/test", []byte(`{"recipient":"ops@example.com"}`))
	store := idempotency.NewControlStore(pool)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Claim(ctx, tx, durable)
	if err != nil || !claimed.Winner {
		_ = tx.Rollback(ctx)
		t.Fatalf("stage in-progress: claimed=%+v err=%v", claimed, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit in-progress: %v", err)
	}
	// Fresh in-progress: concurrent same-key must 503 without sending.
	status, _, _ := h.serve(detachedMailTestCall("detached-inprog-1", "verify", ""))
	if status != http.StatusServiceUnavailable {
		t.Fatalf("in-progress status = %d, want 503 retryable", status)
	}
	mail.mu.Lock()
	if len(mail.recipients) != 0 {
		mail.mu.Unlock()
		t.Fatalf("in-progress must not send: %v", mail.recipients)
	}
	mail.mu.Unlock()

	// Age the orphan past the lease: the retry takes over and executes.
	if _, err := pool.Exec(ctx, `UPDATE control_idempotency_records SET created_at = now() - INTERVAL '5 minutes'
		WHERE operator_identity = $1 AND operation = $2 AND target_type = $3 AND target_id = $4 AND idempotency_key = $5`,
		durable.Operator, durable.Operation, durable.TargetType, durable.TargetID, durable.Key); err != nil {
		t.Fatalf("age orphan: %v", err)
	}
	status, _, _ = h.serve(detachedMailTestCall("detached-inprog-1", "verify", ""))
	if status != http.StatusOK {
		t.Fatalf("stale takeover status = %d, want 200", status)
	}
	mail.mu.Lock()
	defer mail.mu.Unlock()
	if len(mail.recipients) != 1 {
		t.Fatalf("takeover must execute once: recipients = %v", mail.recipients)
	}
}

// These failures occur after the external send. A fresh instance must observe
// durable uncertainty even after the lease, without relying on its L1 cache.
func TestControlDetachedSettlementFailureNeverResends(t *testing.T) {
	for _, failure := range []string{"begin", "complete", "commit", "commit-ack"} {
		t.Run(failure, func(t *testing.T) {
			pool := testdb.Open(t)
			store := &failedDetachedCompletion{ControlStore: idempotency.NewControlStore(pool), fail: failure == "complete"}
			scope := &failedDetachedOutcomeScope{pgControlTxScope: pgControlTxScope{db: pool}, failure: failure, t: t}
			mail := newFakeControlMail()
			h := newControlHarness(t, func(d *ContractControlDeps) { d.Mail = mail; d.TxScope = scope; d.IdempotencyDurable = store })
			call := detachedMailTestCall("detached-settlement-"+failure, "verify", "")
			status, _, payload := h.serve(call)
			if status != http.StatusServiceUnavailable || controlErrorCode(t, payload) != string(contract.CodeServiceUnavailable) {
				t.Fatalf("failed settlement: %d %+v", status, payload)
			}
			mail.mu.Lock()
			sends := len(mail.recipients)
			mail.mu.Unlock()
			if sends != 1 {
				t.Fatalf("first sends = %d", sends)
			}
			if _, err := pool.Exec(t.Context(), `UPDATE control_idempotency_records SET created_at = now() - INTERVAL '5 minutes' WHERE idempotency_key = $1`, call.headers["Idempotency-Key"]); err != nil {
				t.Fatal(err)
			}
			freshMail := newFakeControlMail()
			fresh := newDetachedMailHarness(t, pool, freshMail)
			status, _, payload = fresh.serve(call)
			if failure == "commit-ack" {
				if status != http.StatusOK {
					t.Fatalf("committed response did not replay: %d %+v", status, payload)
				}
			} else if status != http.StatusServiceUnavailable || controlErrorCode(t, payload) != string(contract.CodeServiceUnavailable) {
				t.Fatalf("unknown retry: %d %+v", status, payload)
			}
			freshMail.mu.Lock()
			sends = len(freshMail.recipients)
			freshMail.mu.Unlock()
			if sends != 0 {
				t.Fatalf("uncertain retry resent %d times", sends)
			}
			var unknown bool
			if err := pool.QueryRow(t.Context(), `SELECT settlement_unknown FROM control_idempotency_records WHERE idempotency_key = $1`, call.headers["Idempotency-Key"]).Scan(&unknown); err != nil || unknown != (failure != "commit-ack") {
				t.Fatalf("durable unknown = %v, err %v", unknown, err)
			}
		})
	}
}

type failedDetachedCompletion struct {
	*idempotency.ControlStore
	fail bool
}

func (s *failedDetachedCompletion) Complete(ctx context.Context, tx pgx.Tx, claim idempotency.ControlClaim, result idempotency.ControlResult) error {
	if s.fail {
		return errors.New("injected settlement failure")
	}
	return s.ControlStore.Complete(ctx, tx, claim, result)
}

type failedDetachedOutcomeScope struct {
	pgControlTxScope
	failure string
	begins  int
	commits int
	t       *testing.T
}

func (s *failedDetachedOutcomeScope) Begin(ctx context.Context) (context.Context, error) {
	s.begins++
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > detachedDurableSettlementTimeout {
		s.t.Fatal("detached transaction has no bounded deadline")
	}
	if s.begins == 2 && s.failure == "begin" {
		return nil, errors.New("injected settlement begin failure")
	}
	return s.pgControlTxScope.Begin(ctx)
}
func (s *failedDetachedOutcomeScope) Commit(ctx context.Context) error {
	s.commits++
	if s.commits == 2 && s.failure == "commit" {
		s.pgControlTxScope.Rollback(ctx)
		return errors.New("injected settlement commit failure")
	}
	if s.commits == 2 && s.failure == "commit-ack" {
		if err := s.pgControlTxScope.Commit(ctx); err != nil {
			return err
		}
		return errors.New("injected commit acknowledgement loss")
	}
	return s.pgControlTxScope.Commit(ctx)
}
