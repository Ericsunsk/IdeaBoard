package control

// Control audit durable adapter tests (step 1).
//
// The Control Plane tests prove the guard EMITS one record per call.
// These prove the other half: that the composition adapter actually
// PERSISTS that record into control_audit_records, so the wiring is
// not a silent no-op behind the slog trail. Counts are asserted
// against REAL postgres rows because the failure this guards is "the
// guard called the sink and nothing landed".

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/identity/principal"
	identitydomain "ideaboard/api/internal/identity/users/domain"
	identitypostgres "ideaboard/api/internal/identity/users/infrastructure/postgres"
	identityports "ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/platform/audit"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/runtime/interfaces/http/control"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// stubControlAuditDirectory is a request-scoped resolver fake: one
// lookup, no cache, no global state.
type stubControlAuditDirectory struct {
	id  int64
	err error
}

func (s stubControlAuditDirectory) ResolveUserID(context.Context, uuid.UUID) (int64, error) {
	if s.err != nil {
		return 0, s.err
	}
	return s.id, nil
}

func (s stubControlAuditDirectory) GetUserPublicID(context.Context, int64) (uuid.UUID, error) {
	return uuid.Nil, errors.New("not implemented")
}

func (s stubControlAuditDirectory) GetUserPublicIDs(_ context.Context, userIDs []int64) (map[int64]uuid.UUID, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := make(map[int64]uuid.UUID, len(userIDs))
	for _, id := range userIDs {
		if id == s.id {
			out[id] = uuid.Nil
		}
	}
	return out, nil
}

// failingControlAuditStore always fails the INSERT.
type failingControlAuditStore struct {
	calls int
	err   error
}

func (f *failingControlAuditStore) AppendRecord(context.Context, audit.RecordParams) error {
	f.calls++
	if f.err != nil {
		return f.err
	}
	return errors.New("audit store down")
}

// AppendRecordTx implements the step-2 participant seam with the same
// always-fail behavior, so atomic-path tests prove the error is
// returned (rolled back) rather than swallowed.
func (f *failingControlAuditStore) AppendRecordTx(context.Context, pgx.Tx, audit.RecordParams) error {
	f.calls++
	if f.err != nil {
		return f.err
	}
	return errors.New("audit store down")
}

// recordingControlAuditStore captures params without a database.
type recordingControlAuditStore struct {
	params []audit.RecordParams
}

func (f *recordingControlAuditStore) AppendRecord(_ context.Context, params audit.RecordParams) error {
	f.params = append(f.params, params)
	return nil
}

// AppendRecordTx implements the step-2 participant seam by capturing
// params without a database (the transaction itself is never touched).
func (f *recordingControlAuditStore) AppendRecordTx(_ context.Context, _ pgx.Tx, params audit.RecordParams) error {
	f.params = append(f.params, params)
	return nil
}

// stubControlIdentityCommands succeeds every mutation so the HTTP test
// can prove the business call still succeeds when durable audit fails.
type stubControlIdentityCommands struct {
	banCalls int
	fail     error
}

func (s *stubControlIdentityCommands) SetRole(context.Context, principal.Principal, string, string) (identityports.IdentityAdminMutationOutcome, error) {
	return identityports.IdentityAdminMutationOutcome{}, nil
}

func (s *stubControlIdentityCommands) Ban(context.Context, principal.Principal, string, identityports.IdentityAdminBanInput) (identityports.IdentityAdminMutationOutcome, error) {
	s.banCalls++
	return identityports.IdentityAdminMutationOutcome{}, s.fail
}

func (s *stubControlIdentityCommands) Unban(context.Context, principal.Principal, string) (identityports.IdentityAdminMutationOutcome, error) {
	return identityports.IdentityAdminMutationOutcome{}, nil
}

func (s *stubControlIdentityCommands) RevokeSessions(context.Context, principal.Principal, string) (identityports.IdentityAdminMutationOutcome, error) {
	return identityports.IdentityAdminMutationOutcome{}, nil
}

func (s *stubControlIdentityCommands) Delete(context.Context, principal.Principal, string) (identityports.IdentityAdminMutationOutcome, error) {
	return identityports.IdentityAdminMutationOutcome{}, nil
}

func seedControlAuditUser(t *testing.T, pool *pgxpool.Pool, display, role string) (int64, string) {
	t.Helper()
	storageUID, err := identitydomain.NewStorageUID()
	if err != nil {
		t.Fatalf("mint storage uid: %v", err)
	}
	authID := fmt.Sprintf("auth-ctl-%d", time.Now().UnixNano())
	var numeric int64
	var publicText string
	if err := pool.QueryRow(context.Background(), `
INSERT INTO users (display_name, auth_user_id, storage_uid, role, status)
VALUES ($1, $2, $3, $4, 1)
RETURNING id, public_id::text`, display, authID, storageUID, role).Scan(&numeric, &publicText); err != nil {
		t.Fatalf("seed control audit user: %v", err)
	}
	publicUUID, err := uuid.Parse(strings.TrimSpace(publicText))
	if err != nil {
		t.Fatalf("parse seeded public_id: %v", err)
	}
	return numeric, tenancy.FormatUserPublicID(publicUUID)
}

type controlAuditRow struct {
	operator   int64
	operation  string
	capability string
	targetType string
	targetID   string
	outcome    string
	requestID  string
	idemKey    string
	publicID   string
	route      string
	status     int
}

func TestControlAuditCapabilityDefaultsToUnknown(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{"family label passes through", "control:users", "control:users"},
		{"empty degrades to unknown", "", "control:unknown"},
		{"non-family never fabricated", "workspace:read", "control:unknown"},
		{"bare prefix without family is unknown", "control:", "control:unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := controlAuditCapability(control.ControlAuditRecord{Capability: tc.in})
			if got != tc.want {
				t.Fatalf("capability(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestControlAuditTargetFallsBackToRoute(t *testing.T) {
	t.Run("named target passes through", func(t *testing.T) {
		typ, id := controlAuditTarget(control.ControlAuditRecord{TargetType: "user", TargetID: "usr_abc", Route: "POST /users/{user_id}/ban"})
		if typ != "user" || id != "usr_abc" {
			t.Fatalf("target = %q %q, want user usr_abc", typ, id)
		}
	})
	t.Run("empty target uses route template", func(t *testing.T) {
		typ, id := controlAuditTarget(control.ControlAuditRecord{Route: "GET /users"})
		if typ != "route" || id != "GET /users" {
			t.Fatalf("target = %q %q, want route GET /users", typ, id)
		}
	})
	t.Run("empty route falls back to operation", func(t *testing.T) {
		typ, id := controlAuditTarget(control.ControlAuditRecord{Operation: "users.list"})
		if typ != "route" || id != "users.list" {
			t.Fatalf("target = %q %q, want route users.list", typ, id)
		}
	})
}

func TestControlAuditDurableSinkSkipsWithoutStore(t *testing.T) {
	var nilSink *ControlAuditDurableSink
	nilSink.AppendControlAudit(control.ControlAuditRecord{Operation: "users.ban"})
	NewControlAuditDurableSink(nil, stubControlAuditDirectory{id: 1}).AppendControlAudit(control.ControlAuditRecord{
		OperatorID: "usr_0123456789abcdef0123456789abcdef",
		Operation:  "users.ban",
	})
}

func TestControlAuditDurableSinkSkipsServiceSubjects(t *testing.T) {
	store := &recordingControlAuditStore{}
	sink := NewControlAuditDurableSink(store, stubControlAuditDirectory{id: 7})
	sink.AppendControlAudit(control.ControlAuditRecord{
		OperatorType: "service",
		OperatorID:   "control-plane",
		Capability:   "control:users",
		Operation:    "users.ban",
		Route:        "POST /users/{user_id}/ban",
		Outcome:      control.ControlAuditOK,
		Status:       http.StatusOK,
	})
	if len(store.params) != 0 {
		t.Fatalf("service subject must degrade to slog only, got %d params", len(store.params))
	}
}

func TestControlAuditDurableSinkPersistsBanOnce(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	operatorNumeric, operatorUsr := seedControlAuditUser(t, pool, "ctl-operator", "admin")
	_, targetUsr := seedControlAuditUser(t, pool, "ctl-target", "user")

	repo := identitypostgres.NewRepository(pool)
	dir := runtimehttp.NewUsersRepositoryDirectory(repo)
	sink := NewControlAuditDurableSink(audit.NewStore(pool), dir)

	record := control.ControlAuditRecord{
		OperatorType:   "human_session",
		OperatorID:     operatorUsr,
		TargetType:     "user",
		TargetID:       targetUsr,
		Capability:     "control:users",
		Operation:      "users.ban",
		Route:          "POST /users/{user_id}/ban",
		RequestID:      "req-ban-1",
		IdempotencyKey: "ban-key-1",
		Outcome:        control.ControlAuditOK,
		Status:         http.StatusOK,
		OccurredAt:     time.Now().UTC(),
	}
	sink.AppendControlAudit(record)

	rows, err := pool.Query(ctx, `
SELECT operator_user_id, operation_type, capability, target_type, target_id,
       outcome, request_correlation_id, idempotency_key, public_id,
       metadata->>'route', (metadata->>'status')::int
FROM control_audit_records WHERE operator_user_id = $1 ORDER BY internal_id`, operatorNumeric)
	if err != nil {
		t.Fatalf("query audit rows: %v", err)
	}
	var got []controlAuditRow
	for rows.Next() {
		var row controlAuditRow
		var route *string
		var status *int
		if err := rows.Scan(&row.operator, &row.operation, &row.capability, &row.targetType,
			&row.targetID, &row.outcome, &row.requestID, &row.idemKey, &row.publicID, &route, &status); err != nil {
			rows.Close()
			t.Fatalf("scan audit row: %v", err)
		}
		if route != nil {
			row.route = *route
		}
		if status != nil {
			row.status = *status
		}
		got = append(got, row)
	}
	rows.Close()
	if rows.Err() != nil {
		t.Fatalf("iterate audit rows: %v", rows.Err())
	}
	if len(got) != 1 {
		t.Fatalf("audit rows = %+v, want exactly 1 for users.ban", got)
	}
	row := got[0]
	if row.operator != operatorNumeric {
		t.Errorf("operator = %d, want %d", row.operator, operatorNumeric)
	}
	if row.operation != "users.ban" {
		t.Errorf("operation = %q, want users.ban", row.operation)
	}
	if row.capability != "control:users" {
		t.Errorf("capability = %q, want control:users", row.capability)
	}
	if row.targetType != "user" || row.targetID != targetUsr {
		t.Errorf("target = %q %q, want user %q", row.targetType, row.targetID, targetUsr)
	}
	if row.outcome != "success" {
		t.Errorf("outcome = %q, want success (read maps via store, success passes through)", row.outcome)
	}
	if row.requestID != "req-ban-1" {
		t.Errorf("request_id = %q, want req-ban-1", row.requestID)
	}
	if row.idemKey != "ban-key-1" {
		t.Errorf("idempotency_key = %q, want ban-key-1", row.idemKey)
	}
	if !strings.HasPrefix(row.publicID, "audit_") {
		t.Errorf("public_id = %q, want audit_ prefix", row.publicID)
	}
	if row.route != "POST /users/{user_id}/ban" {
		t.Errorf("metadata route = %q, want POST /users/{user_id}/ban", row.route)
	}
	if row.status != http.StatusOK {
		t.Errorf("metadata status = %d, want 200", row.status)
	}
	var createdCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM control_audit_records WHERE operator_user_id = $1 AND created_at IS NOT NULL`, operatorNumeric).Scan(&createdCount); err != nil {
		t.Fatalf("count timestamped rows: %v", err)
	}
	if createdCount != 1 {
		t.Errorf("timestamped rows = %d, want 1", createdCount)
	}
}

func TestControlAuditDurableSinkUsesRoutePlaceholderWithoutTarget(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	operatorNumeric, operatorUsr := seedControlAuditUser(t, pool, "ctl-operator-2", "admin")

	repo := identitypostgres.NewRepository(pool)
	dir := runtimehttp.NewUsersRepositoryDirectory(repo)
	sink := NewControlAuditDurableSink(audit.NewStore(pool), dir)

	// A route with no domain target (users.list) plus the middleware
	// operator-grant denial shape (empty capability, empty target,
	// route "operator grant"): both must land rows that satisfy the
	// CHECK constraints via the conservative placeholders.
	sink.AppendControlAudit(control.ControlAuditRecord{
		OperatorType: "human_session",
		OperatorID:   operatorUsr,
		Capability:   "control:users",
		Operation:    "users.list",
		Route:        "GET /users",
		RequestID:    "req-list-1",
		Outcome:      control.ControlAuditRead,
		Status:       http.StatusOK,
	})
	sink.AppendControlAudit(control.ControlAuditRecord{
		OperatorType: "human_session",
		OperatorID:   operatorUsr,
		Capability:   "",
		Operation:    "operator.grant",
		Route:        "operator grant",
		RequestID:    "req-denied-1",
		Outcome:      control.ControlAuditDenied,
		Status:       http.StatusForbidden,
	})

	rows, err := pool.Query(ctx, `
SELECT operation_type, capability, target_type, target_id, outcome, request_correlation_id
FROM control_audit_records WHERE operator_user_id = $1 ORDER BY internal_id`, operatorNumeric)
	if err != nil {
		t.Fatalf("query placeholder rows: %v", err)
	}
	type placeholder struct {
		operation, capability, targetType, targetID, outcome, requestID string
	}
	var got []placeholder
	for rows.Next() {
		var p placeholder
		if err := rows.Scan(&p.operation, &p.capability, &p.targetType, &p.targetID, &p.outcome, &p.requestID); err != nil {
			rows.Close()
			t.Fatalf("scan placeholder row: %v", err)
		}
		got = append(got, p)
	}
	rows.Close()
	if rows.Err() != nil {
		t.Fatalf("iterate placeholder rows: %v", rows.Err())
	}
	if len(got) != 2 {
		t.Fatalf("placeholder rows = %+v, want exactly 2", got)
	}
	if got[0].targetType != "route" || got[0].targetID != "GET /users" {
		t.Errorf("list target = %q %q, want route GET /users", got[0].targetType, got[0].targetID)
	}
	if got[0].capability != "control:users" {
		t.Errorf("list capability = %q, want control:users", got[0].capability)
	}
	if got[0].outcome != "success" {
		t.Errorf("list outcome = %q, want success (read converges via store)", got[0].outcome)
	}
	if got[1].capability != "control:unknown" {
		t.Errorf("denied capability = %q, want control:unknown", got[1].capability)
	}
	if got[1].targetType != "route" || got[1].targetID != "operator grant" {
		t.Errorf("denied target = %q %q, want route operator grant", got[1].targetType, got[1].targetID)
	}
	if got[1].outcome != "denied" {
		t.Errorf("denied outcome = %q, want denied", got[1].outcome)
	}
}

func TestControlAuditDurableSinkFailureNeverFailsBusiness(t *testing.T) {
	// Adapter level: a failing store must be swallowed (slog.Warn only).
	failing := &failingControlAuditStore{err: errors.New("audit store down")}
	sink := NewControlAuditDurableSink(failing, stubControlAuditDirectory{id: 11})
	sink.AppendControlAudit(control.ControlAuditRecord{
		OperatorType: "human_session",
		OperatorID:   "usr_0123456789abcdef0123456789abcdef",
		Capability:   "control:users",
		Operation:    "users.ban",
		Route:        "POST /users/{user_id}/ban",
		RequestID:    "req-fail-1",
		Outcome:      control.ControlAuditOK,
		Status:       http.StatusOK,
	})
	if failing.calls != 1 {
		t.Fatalf("failing store calls = %d, want 1 (attempted, then swallowed)", failing.calls)
	}

	// HTTP level: the guarded ban still returns 200 when durable audit
	// is down. The business command ran; only its trail was lost.
	gin.SetMode(gin.TestMode)
	identity := &stubControlIdentityCommands{}
	auditSink := NewControlAuditDurableSink(&failingControlAuditStore{err: errors.New("db unavailable")}, stubControlAuditDirectory{id: 11})
	engine := gin.New()
	engine.Use(runtimehttp.ContractRequestIDMiddleware())
	operatorUUID := uuid.New()
	engine.Use(func(c *gin.Context) {
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
		p, err := principal.New(userID, operatorUUID, "auth-11", "test-operator", "admin")
		if err != nil || principal.Set(c, p) != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Next()
	})
	ctrlGroup := engine.Group(runtimehttp.ContractControlGroupPath)
	ctrlGroup.Use(runtimehttp.ContractRequestIDMiddleware())
	control.RegisterContractControlRoutes(ctrlGroup, runtimehttp.AuthDeps{}, control.ContractControlDeps{
		Identity:    identity,
		AuditSink:   auditSink,
		Idempotency: contract.NewIdempotencyStore(contract.DefaultIdempotencyStoreOptions()),
		RateLimiter: control.NewControlRateLimiter(1000, time.Minute),
	})

	targetUsr := tenancy.FormatUserPublicID(uuid.New())
	req := httptest.NewRequest(http.MethodPost, "/api/control/v1/users/"+targetUsr+"/ban", strings.NewReader(`{"expires_in_days":7}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(contract.RequestIDHeader, "req-http-fail-1")
	testsupport.SetTestUser(req, 11)
	req.Header.Set(contract.IdempotencyKeyHeader, "http-fail-key-1")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("ban with audit down = %d, want 200 (business must still succeed); body=%s", recorder.Code, recorder.Body.String())
	}
	if identity.banCalls != 1 {
		t.Fatalf("ban calls = %d, want 1 (audit failure must not block the command)", identity.banCalls)
	}
}
