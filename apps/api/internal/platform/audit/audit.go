// Package audit owns the durable Control Plane audit sink (#809).
//
// The control guard always emits a structured slog record plus the
// in-memory/test sink; this package adds the durable dual-write into
// control_audit_records (fragments/control_audit.sql). Writes are
// best-effort: a persistence failure is surfaced as an error so the
// caller can log it, and must never fail the guarded request.
package audit

import (
	"context"
	"encoding/json"
	"errors"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/contract"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Outcome is the single wire-stable audit outcome vocabulary ( ONE typed
// outcome: no "failure" vs "failed" split). Values mirror the durable
// control_audit_records CHECK constraint plus the read word used by
// non-persisted guard observations:
//   - read: a guarded non-mutating observation (never persisted; the
//     durable store maps it to success via MapOutcome).
//   - success: the guarded call succeeded.
//   - denied: the call was refused (authZ, missing idempotency key).
//   - failure: the guarded call failed (handler 4xx/5xx outcomes).
//
// Do not add spellings without coordinating openapi.control.yaml
// AuditRecord.outcome and every emitter/consumer below.
type Outcome string

const (
	OutcomeRead    Outcome = "read"
	OutcomeSuccess Outcome = "success"
	OutcomeDenied  Outcome = "denied"
	OutcomeFailure Outcome = "failure"
)

// RecordParams carries one guarded Control Plane call for durable
// storage. OperatorUserID is the internal numeric id (FK target);
// OperatorType/OperatorID stay public-only for the slog record.
type RecordParams struct {
	OperatorUserID int64
	OperatorType   string
	OperatorID     string
	Operation      string
	Capability     string
	TargetType     string
	TargetID       string
	// AuthUserID is the Auth-service identity the attempt targeted (admin
	// impersonation). Empty for operations without an Auth target; when
	// set it is persisted inside metadata (never in error text).
	AuthUserID     string
	Outcome        Outcome
	ErrorMessage   string
	RequestID      string
	IdempotencyKey string
	Route          string
	Status         int
	Replayed       bool
}

// Store is the postgres durable audit sink.
type Store struct {
	db *pgxpool.Pool
	q  *dbsqlc.Queries
}

// NewStore builds a durable audit Store over pool.
func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db, q: dbsqlc.New(db)}
}

func (s *Store) dbWithContext(ctx context.Context) repositoryQueryer {
	return repositoryQueryer{ctx: ctx, db: s.db, q: s.q}
}

type repositoryQueryer struct {
	ctx context.Context
	db  *pgxpool.Pool
	q   *dbsqlc.Queries
}

// MapOutcome converges guard outcomes to the table CHECK constraint
// (success/failure/denied): reads are successful operations, failures
// (including 4xx/5xx handler outcomes) map to failure. The durable
// store has no read word, so OutcomeRead maps to success; the zero
// value keeps the historical success default. There is no "failed"
// spelling: emitters use the typed Outcome constants above.
func MapOutcome(outcome Outcome) string {
	switch outcome {
	case OutcomeDenied:
		return "denied"
	case OutcomeFailure:
		return "failure"
	default:
		return "success"
	}
}

// AppendRecord persists one audit record best-effort. A non-positive
// operator id (service subjects without a users row) skips the insert:
// the slog record remains the trail for those calls.
func (s *Store) AppendRecord(ctx context.Context, params RecordParams) error {
	if s == nil || s.db == nil || s.q == nil {
		return nil
	}
	if params.OperatorUserID <= 0 {
		return nil
	}
	query := s.dbWithContext(ctx)
	return dbutil.WithTx(query.ctx, query.db, func(q *dbsqlc.Queries) error {
		return insertControlAuditRow(query.ctx, q, params)
	})
}

// AppendRecordTx persists one audit record as a PARTICIPANT of the
// caller-owned transaction tx: it never begins or commits, so the audit
// row commits or rolls back atomically with whatever else the caller
// wrote in tx. A persistence failure is returned (never swallowed):
// the caller must fail its own transaction with it, which is the
// deliberate semantic inversion versus AppendRecord's best-effort
// warn-and-continue. A nil store, a nil pool, or a non-positive operator
// id still skips silently (service subjects keep the slog trail only);
// a nil tx is a wiring error and fails the call.
func (s *Store) AppendRecordTx(ctx context.Context, tx pgx.Tx, params RecordParams) error {
	if s == nil || s.db == nil || s.q == nil {
		return nil
	}
	if params.OperatorUserID <= 0 {
		return nil
	}
	if tx == nil {
		return errors.New("audit: participant write requires a transaction")
	}
	return insertControlAuditRow(ctx, s.q.WithTx(tx), params)
}

// insertControlAuditRow builds the wire-stable insert (public id,
// outcome convergence, route/status metadata) and executes it on q, so
// the detached best-effort path and the caller-owned atomic path share
// one construction and satisfy the table CHECK constraints identically.
func insertControlAuditRow(ctx context.Context, q *dbsqlc.Queries, params RecordParams) error {
	publicID, err := contract.MintPublicID("audit_", 16)
	if err != nil {
		return err
	}
	metadata, err := json.Marshal(map[string]any{
		"route":        params.Route,
		"status":       params.Status,
		"replayed":     params.Replayed,
		"auth_user_id": params.AuthUserID,
	})
	if err != nil {
		return err
	}
	_, err = q.InsertControlAuditRecord(ctx, dbsqlc.InsertControlAuditRecordParams{
		PublicID:             publicID,
		OperatorUserID:       params.OperatorUserID,
		OperationType:        params.Operation,
		Capability:           params.Capability,
		TargetType:           params.TargetType,
		TargetID:             params.TargetID,
		Outcome:              MapOutcome(params.Outcome),
		ErrorMessage:         params.ErrorMessage,
		RequestCorrelationID: params.RequestID,
		IdempotencyKey:       params.IdempotencyKey,
		Metadata:             metadata,
	})
	return err
}
