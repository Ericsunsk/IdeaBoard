package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/idempotency"
	"ideaboard/api/internal/platform/settings"
	statisticsapp "ideaboard/api/internal/tasking/application/statistics"
	tasksdomain "ideaboard/api/internal/tasking/domain"
	statisticshttp "ideaboard/api/internal/tasking/interfaces/http/statistics"
	statisticscontracts "ideaboard/api/internal/tasking/ports/statistics"
	"ideaboard/api/internal/testdb"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ---- merged from monitor_test.go ----
// Control monitor/statistics/backfill liveness tests. The
// workspace-backed statistics store wires ContractControlDeps.Monitor off
// its honest 501, so these routes must answer 200 with the unified
// contract envelope ({data} unwrapped from the domain DTO at the
// control boundary). The nil-Monitor fail-closed path stays covered by
// TestControlUnwiredDepsFailClosed in control/auth_test.go.

type fakeMonitorStatsStore struct{}

func (fakeMonitorStatsStore) CountByStatus(context.Context, string) (int64, error) {
	return 0, nil
}

func (fakeMonitorStatsStore) CountAllTaskCalls(context.Context) (int64, error) {
	return 0, nil
}

func (fakeMonitorStatsStore) ListBackfilledDatesByRange(context.Context, time.Time, time.Time) ([]time.Time, error) {
	return nil, nil
}

func (fakeMonitorStatsStore) ListDailyStatsByDateRange(context.Context, time.Time, time.Time) ([]statisticscontracts.DailyStatAggregate, error) {
	return nil, nil
}

func (fakeMonitorStatsStore) ListDailyCaptureCreditsByRange(context.Context, time.Time, time.Time, *time.Location) (map[string]int64, error) {
	return map[string]int64{}, nil
}

func (fakeMonitorStatsStore) ListExecutionBottleneckStatsByCreatedRange(context.Context, time.Time, time.Time) ([]statisticscontracts.ExecutionBottleneckAggregate, error) {
	return nil, nil
}

func (fakeMonitorStatsStore) ListExecutionFailureCategoryStatsByCreatedRange(context.Context, time.Time, time.Time) ([]statisticscontracts.ExecutionFailureCategoryAggregate, error) {
	return nil, nil
}

func (fakeMonitorStatsStore) ListStatEventsByOccurredRange(context.Context, time.Time, time.Time) ([]statisticscontracts.StatEventTrendRow, error) {
	return nil, nil
}

func (fakeMonitorStatsStore) ListStatEventLatencySamplesByDateRange(context.Context, time.Time, time.Time) ([]statisticscontracts.StatEventLatencySample, error) {
	return nil, nil
}

func (fakeMonitorStatsStore) ListMonitorExecutionPressureByRange(context.Context, time.Time, time.Time) ([]statisticscontracts.MonitorExecutionPressureAggregate, error) {
	return nil, nil
}

func (fakeMonitorStatsStore) ListMonitorGoldenSignalSamplesByRange(context.Context, time.Time, time.Time) ([]statisticscontracts.MonitorGoldenSignalSample, error) {
	return nil, nil
}

func (fakeMonitorStatsStore) BackfillDailyStatsByCreatedRange(context.Context, time.Time, time.Time, time.Time, bool) (statisticscontracts.BackfillDailyStatsResult, error) {
	return statisticscontracts.BackfillDailyStatsResult{}, nil
}

type fakeMonitorInfoProvider struct{}

func (fakeMonitorInfoProvider) GetMonitorInfo(context.Context) (*tasksdomain.MonitorInfo, error) {
	return &tasksdomain.MonitorInfo{}, nil
}

func newMonitorHarness(t *testing.T) *controlHarness {
	t.Helper()
	return newControlHarness(t, func(deps *ContractControlDeps) {
		deps.Monitor = statisticshttp.NewHandler(
			statisticsapp.NewService(fakeMonitorStatsStore{}),
			fakeMonitorInfoProvider{},
		)
	})
}

func controlMonitorData(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("success envelope requires data object: %v", payload)
	}
	return data
}

func TestControlMonitorReadsLive(t *testing.T) {
	h := newMonitorHarness(t)
	admin := func(c controlCall) controlCall {
		c.userID = controlOperatorID
		c.role = "admin"
		return c
	}
	if status, _, payload := h.serve(admin(controlCall{method: http.MethodGet, path: "/api/control/v1/platform/monitor"})); status != http.StatusOK {
		t.Fatalf("wired monitor = %d (%v), want 200", status, payload)
	} else if data := controlMonitorData(t, payload); data["monitor"] == nil || data["stats"] == nil {
		t.Fatalf("monitor data = %v, want monitor + stats keys", data)
	}
	if status, _, payload := h.serve(admin(controlCall{method: http.MethodGet, path: "/api/control/v1/platform/statistics?range=7d"})); status != http.StatusOK {
		t.Fatalf("wired statistics = %d (%v), want 200", status, payload)
	} else if data := controlMonitorData(t, payload); data["summary"] == nil {
		t.Fatalf("statistics data = %v, want summary key", data)
	}
}

func TestControlStatisticsBackfillLive(t *testing.T) {
	h := newMonitorHarness(t)
	admin := func(c controlCall) controlCall {
		c.userID = controlOperatorID
		c.role = "admin"
		return c
	}
	call := admin(controlCall{
		method: http.MethodPost, path: "/api/control/v1/platform/statistics/backfill",
		body: `{"range":"7d"}`, headers: controlMutationHeaders("monitor-backfill-1"),
	})
	if status, _, payload := h.serve(call); status != http.StatusOK {
		t.Fatalf("wired backfill = %d (%v), want 200", status, payload)
	} else if data := controlMonitorData(t, payload); data["range"] == nil {
		t.Fatalf("backfill data = %v, want range key", data)
	}
}

// ---- merged from login_lockout_test.go ----
func TestControlLoginLockoutIdempotency(t *testing.T) {
	db := testdb.Open(t)
	service := settings.NewRuntimeSettingsService(settings.NewSystemConfigRepository(db))
	h := newControlHarness(t, func(d *ContractControlDeps) { d.Settings = service })
	const path = "/api/control/v1/platform/settings/login-lockout"

	status, _, payload := h.serve(controlCall{method: http.MethodGet, path: path, userID: controlOperatorID, role: "admin"})
	if status != http.StatusOK {
		t.Fatalf("get login lockout = status %d payload %v", status, payload)
	}
	data, ok := controlData(t, payload).(map[string]any)
	if !ok {
		t.Fatalf("login lockout data is not an object: %v", payload)
	}
	if data["threshold"] != float64(10) || data["duration_seconds"] != float64(900) {
		t.Fatalf("login lockout defaults = %v, want threshold=10 duration=900", data)
	}

	const body = `{"threshold":5,"duration_seconds":300}`
	status, _, payload = h.serve(controlCall{method: http.MethodPut, path: path, userID: controlOperatorID, role: "admin", body: body, headers: map[string]string{contract.IdempotencyKeyHeader: "login-lockout-0"}})
	if status != http.StatusOK {
		t.Fatalf("key-only save = status %d payload %v, want 200", status, payload)
	}
	if last := h.sink.Records()[len(h.sink.Records())-1]; last.Operation != "platform.update_login_lockout" {
		t.Fatalf("key-only audit = %+v, want the recorded operation", last)
	}
	status, _, payload = h.serve(controlCall{method: http.MethodPut, path: path, userID: controlOperatorID, role: "admin", body: body})
	if status != http.StatusBadRequest || controlErrorCode(t, payload) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("missing idempotency key = status %d payload %v", status, payload)
	}
	status, _, payload = h.serve(controlCall{method: http.MethodPut, path: path, userID: controlOperatorID, role: "admin", body: body, headers: nil})
	if status != http.StatusBadRequest || controlErrorCode(t, payload) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("missing idempotency key = status %d payload %v", status, payload)
	}
	call := controlCall{method: http.MethodPut, path: path, userID: controlOperatorID, role: "admin", body: body, headers: controlMutationHeaders("login-lockout-1")}
	for i := 0; i < 2; i++ {
		status, _, payload = h.serve(call)
		if status != http.StatusOK {
			t.Fatalf("save/replay %d = status %d payload %v", i, status, payload)
		}
		controlData(t, payload)
	}
	records := h.sink.Records()
	last := records[len(records)-1]
	if last.Operation != "platform.update_login_lockout" || last.IdempotencyKey != "login-lockout-1" {
		t.Fatalf("control audit = %+v", last)
	}
	call.body = `{"threshold":6,"duration_seconds":300}`
	status, _, payload = h.serve(call)
	if status != http.StatusConflict || controlErrorCode(t, payload) != string(contract.CodeIdempotencyConflict) {
		t.Fatalf("key reuse conflict = status %d payload %v", status, payload)
	}
	got, err := service.GetLoginLockout(t.Context())
	if err != nil || got != (settings.LoginLockoutSetting{Threshold: 5, DurationSeconds: 300}) {
		t.Fatalf("login lockout after conflicting request = %+v err=%v", got, err)
	}

	badCall := controlCall{method: http.MethodPut, path: path, userID: controlOperatorID, role: "admin", body: `{"threshold":2,"duration_seconds":300}`, headers: controlMutationHeaders("login-lockout-2")}
	status, _, payload = h.serve(badCall)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("out-of-range threshold = status %d payload %v, want 422", status, payload)
	}
}

// ---- merged from detached_durable_test.go ----
// Detached mutations persist fenced dispatch intent before external I/O and
// settle through a separate bounded transaction. Completed responses replay;
// failed settlement retains uncertainty without re-sending. Only undispatched
// in-progress claims may be taken over after the lease.

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

func detachedMailTestCall(key, body string) controlCall {
	if body == "" {
		body = `{"recipient":"ops@example.com"}`
	}
	return controlCall{
		method: http.MethodPost, path: "/api/control/v1/platform/settings/mail/test",
		body: body, userID: controlOperatorID, role: "admin",
		headers: mailMutationHeaders(key),
	}
}

func TestControlDetachedMailProbeReplayAcrossInstances(t *testing.T) {
	pool := testdb.Open(t)
	mail1 := newFakeControlMail()
	h1 := newDetachedMailHarness(t, pool, mail1)
	status, _, _ := h1.serve(detachedMailTestCall("detached-mail-1", ""))
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
	status, _, _ = h2.serve(detachedMailTestCall("detached-mail-1", ""))
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
	if status, _, _ := h.serve(detachedMailTestCall("detached-mail-conflict-1", `{"recipient":"ops@example.com"}`)); status != http.StatusOK {
		t.Fatalf("first status = %d, want 200", status)
	}
	status, _, payload := h.serve(detachedMailTestCall("detached-mail-conflict-1", `{"recipient":"other@example.com"}`))
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
	call := detachedMailTestCall("detached-mail-fail-1", "")
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
		headers: mailMutationHeaders("detached-mail-put-1"),
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
		headers: controlMutationHeaders("detached-stripe-1"),
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
	status, _, _ := h.serve(detachedMailTestCall("detached-inprog-1", ""))
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
	status, _, _ = h.serve(detachedMailTestCall("detached-inprog-1", ""))
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
			call := detachedMailTestCall("detached-settlement-"+failure, "")
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
		s.Rollback(ctx)
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

// ---- merged from request_schema_test.go ----
func TestControlMutationRequestSchemas(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "..", "..", "..", "openapi.control.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		payload string
	}{
		{"ControlBanRequest", `{"expires_in_days":7}`},
		{"ControlCreditCommandRequest", `{"user_id":"usr_test","amount_credits":1,"business_reference_id":"test"}`},
		{"ControlCreditRefundRequest", `{"user_id":"usr_test","amount_credits":1,"business_reference_id":"test","charge_transaction_id":"charge_test"}`},
		{"ControlCreditCorrectionRequest", `{"user_id":"usr_test","amount_credits":1,"business_reference_id":"test","original_transaction_id":"charge_test"}`},
		{"ControlPaymentRefundRequest", `{"idea_quantity":1}`},
		{"ControlMailConfigRequest", `{"from_name":"IdeaBoard","from_email":"mail@example.com","reply_to":"support@example.com","enabled":false,"api_key":"test-key"}`},
		{"ControlMailTestRequest", `{"recipient":"recipient@example.com"}`},
		{"ControlOAuthProviderUpdateRequest", `{"enabled":true,"client_id":"test-client","client_secret":"test-secret"}`},
		{"ControlRewardCodeCreateRequest", `{"display_name":"Test","code":"TEST","credit_amount":1,"max_redemptions":10,"expires_at":"2030-01-01T00:00:00Z"}`},
		{"ControlRewardCodeUpdateRequest", `{"display_name":"Test","code":"TEST","credit_amount":1,"max_redemptions":10,"expires_at":null}`},
		{"ControlRewardCodeEnabledRequest", `{"enabled":false}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, ok := doc.Components.Schemas[tt.name]
			if !ok || ref.Value == nil {
				t.Fatalf("missing schema %s", tt.name)
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(tt.payload), &payload); err != nil {
				t.Fatal(err)
			}
			if err := ref.Value.VisitJSON(payload); err != nil {
				t.Fatalf("valid business payload rejected: %v", err)
			}
			for _, field := range []string{"unknown_field", "reason", "operator_note"} {
				payload[field] = "text"
				if err := ref.Value.VisitJSON(payload); err == nil {
					t.Fatalf("unknown field %s accepted", field)
				}
				delete(payload, field)
			}
		})
	}
}

// ---- merged from support_channels_test.go ----
func TestControlSupportChannelsIdempotency(t *testing.T) {
	db := testdb.Open(t)
	service := settings.NewRuntimeSettingsService(settings.NewSystemConfigRepository(db))
	h := newControlHarness(t, func(d *ContractControlDeps) { d.Settings = service })
	const path = "/api/control/v1/platform/settings/support-channels"
	const body = `{"channels":[{"id":"discord","name":"Discord","kind":"discord","url":"https://discord.gg/example","enabled":true,"sort_order":0}]}`

	status, _, payload := h.serve(controlCall{method: http.MethodPut, path: path, userID: controlOperatorID, role: "admin", body: body, headers: map[string]string{contract.IdempotencyKeyHeader: "support-settings-0"}})
	if status != http.StatusOK {
		t.Fatalf("key-only save = status %d payload %v, want 200", status, payload)
	}
	if last := h.sink.Records()[len(h.sink.Records())-1]; last.Operation != "platform.update_support_channels" {
		t.Fatalf("key-only audit = %+v, want the recorded operation", last)
	}
	// The idempotency key stays required for mutations.
	status, _, payload = h.serve(controlCall{method: http.MethodPut, path: path, userID: controlOperatorID, role: "admin", body: body})
	if status != http.StatusBadRequest || controlErrorCode(t, payload) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("missing idempotency key = status %d payload %v", status, payload)
	}
	status, _, payload = h.serve(controlCall{method: http.MethodPut, path: path, userID: controlOperatorID, role: "admin", body: body, headers: nil})
	if status != http.StatusBadRequest || controlErrorCode(t, payload) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("missing idempotency key = status %d payload %v", status, payload)
	}
	call := controlCall{method: http.MethodPut, path: path, userID: controlOperatorID, role: "admin", body: body, headers: controlMutationHeaders("support-settings-1")}
	for i := 0; i < 2; i++ {
		status, _, payload = h.serve(call)
		if status != http.StatusOK {
			t.Fatalf("save/replay %d = status %d payload %v", i, status, payload)
		}
		controlData(t, payload)
	}
	records := h.sink.Records()
	last := records[len(records)-1]
	if last.Operation != "platform.update_support_channels" || last.IdempotencyKey != "support-settings-1" {
		t.Fatalf("control audit = %+v", last)
	}
	call.body = `{"channels":[]}`
	status, _, payload = h.serve(call)
	if status != http.StatusConflict || controlErrorCode(t, payload) != string(contract.CodeIdempotencyConflict) {
		t.Fatalf("key reuse conflict = status %d payload %v", status, payload)
	}
	got, err := service.GetSupportChannels(t.Context())
	if err != nil || len(got.Channels) != 1 {
		t.Fatalf("channels after conflicting request = %+v err=%v", got, err)
	}
}

// ---- merged from support_channels_validation_test.go ----
func TestControlSupportChannelsRequiresExplicitFields(t *testing.T) {
	h := newControlHarness(t, func(d *ContractControlDeps) {
		d.Settings = settings.NewRuntimeSettingsService(nil)
	})
	const path = "/api/control/v1/platform/settings/support-channels"
	tests := []struct {
		name string
		body string
		want int
	}{
		{"missing channels", `{}`, http.StatusBadRequest},
		{"null body", `null`, http.StatusBadRequest},
		{"null channels", `{"channels":null}`, http.StatusBadRequest},
		{"missing enabled", `{"channels":[{"id":"x","name":"X","kind":"custom","url":"https://example.com","sort_order":0}]}`, http.StatusBadRequest},
		{"missing sort order", `{"channels":[{"id":"x","name":"X","kind":"custom","url":"https://example.com","enabled":false}]}`, http.StatusBadRequest},
		{"unknown field", `{"channels":[],"extra":true}`, http.StatusBadRequest},
		{"unknown channel field", `{"channels":[{"id":"x","name":"X","kind":"custom","url":"https://example.com","enabled":false,"sort_order":0,"extra":true}]}`, http.StatusBadRequest},
		{"malformed body", `{"channels":`, http.StatusBadRequest},
		{"explicit zero values", `{"channels":[{"id":"x","name":"X","kind":"custom","url":"https://example.com","enabled":false,"sort_order":0}]}`, http.StatusInternalServerError},
		{"explicit clear", `{"channels":[]}`, http.StatusInternalServerError},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _, payload := h.serve(controlCall{method: http.MethodPut, path: path, body: tt.body,
				userID: controlOperatorID, role: "admin", headers: controlMutationHeaders(fmt.Sprintf("support-validation-%d", i))})
			if status != tt.want {
				t.Fatalf("status = %d payload=%v, want %d", status, payload, tt.want)
			}
			wantCode := contract.CodeBadRequest
			if tt.want == http.StatusInternalServerError {
				wantCode = contract.CodeServiceUnavailable
			}
			if code := controlErrorCode(t, payload); code != string(wantCode) {
				t.Fatalf("code = %q, want %q", code, wantCode)
			}
		})
	}
}

func TestControlSupportChannelsDeniesUser(t *testing.T) {
	h := newControlHarness(t, nil)
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		status, _, payload := h.serve(controlCall{method: method, path: "/api/control/v1/platform/settings/support-channels",
			userID: controlUserAID, role: "user", body: `{"channels":[]}`, headers: controlMutationHeaders("user-support-change")})
		if status != http.StatusForbidden || controlErrorCode(t, payload) != string(contract.CodeForbidden) {
			t.Fatalf("%s user access = status %d payload %v", method, status, payload)
		}
	}
}
