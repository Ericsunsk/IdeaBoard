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

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/contract"

	"github.com/jackc/pgx/v5/pgxpool"
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
	Reason         string
	Outcome        string
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
// (including 4xx/5xx handler outcomes) map to failure.
func MapOutcome(outcome string) string {
	switch outcome {
	case "denied":
		return "denied"
	default:
		if outcome == "failure" || outcome == "failed" {
			return "failure"
		}
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
	publicID, err := contract.MintPublicID("audit_", 16)
	if err != nil {
		return err
	}
	metadata, err := json.Marshal(map[string]any{
		"route":    params.Route,
		"status":   params.Status,
		"replayed": params.Replayed,
	})
	if err != nil {
		return err
	}
	query := s.dbWithContext(ctx)
	return dbutil.WithTx(query.ctx, query.db, func(q *dbsqlc.Queries) error {
		_, err := q.InsertControlAuditRecord(query.ctx, dbsqlc.InsertControlAuditRecordParams{
			PublicID:            publicID,
			OperatorUserID:      params.OperatorUserID,
			OperationType:       params.Operation,
			Capability:          params.Capability,
			TargetType:          params.TargetType,
			TargetID:            params.TargetID,
			Reason:              params.Reason,
			Outcome:             MapOutcome(params.Outcome),
			ErrorMessage:        params.ErrorMessage,
			RequestCorrelationID: params.RequestID,
			IdempotencyKey:      params.IdempotencyKey,
			Metadata:            metadata,
		})
		return err
	})
}
