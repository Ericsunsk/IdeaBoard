// Package idempotency durable Control Plane facts (#891).
//
// ControlStore is the PostgreSQL authority behind Control mutation
// idempotency; the process-local Store stays a same-process fast-path
// optimization only. One row per guarded mutation attempt lives in
// control_idempotency_records, keyed by the durable scope
// (operator_identity, operation, target_type, target_id, idempotency_key)
// with the canonical request_hash pinning the request identity
// (method + route template + target + raw body).
//
// The guard claims the in-progress row INSIDE its guard-owned
// transaction before running the handler, so concurrent same-key
// claimants block on the primary key until the winner commits or rolls
// back: exactly one winner executes the business side effect, across
// processes and across restarts. In-progress rows are therefore never
// committed (a crash aborts them with the transaction, so there is no
// orphan lease to reap and no expiry clock to race). Only 2xx outcomes
// flip the row to completed, carrying the response for replay after a
// restart or a commit-after-response-loss retry; business failures
// delete the claim so the retry re-executes. Completed rows replay for
// ReplayTTL and are then treated as misses (PurgeExpired reclaims them;
// callers invoke it best-effort after a successful commit).
//
// Operator identity is the public subject only (usr_* or the stable
// service subject); there is deliberately no users FK so
// internal-worker operators share the same guarantee. Methods take
// context.Context first and execute participant writes on the
// caller-owned pgx.Tx: the claim, the business outcome, and the audit
// row commit or roll back atomically.
package idempotency

import (
	"context"
	"errors"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ControlClaim names one guarded mutation attempt: the durable scope
// (Operator, Operation, TargetType, TargetID, Key) plus the canonical
// RequestHash compared on key reuse.
type ControlClaim struct {
	Operator    string
	Operation   string
	TargetType  string
	TargetID    string
	Key         string
	RequestHash string
}

// ControlResult is the stored 2xx response replayed on key reuse.
type ControlResult struct {
	Status      int
	ContentType string
	Body        string
}

// ControlRecord is the committed fact a losing claimant observes.
type ControlRecord struct {
	RequestHash string
	Completed   bool
	Expired     bool
	Result      ControlResult
}

// ControlStore is the PostgreSQL durable idempotency fact store for
// Control mutations.
type ControlStore struct {
	db *pgxpool.Pool
	q  *dbsqlc.Queries
}

// NewControlStore builds a durable Control idempotency Store over pool.
// A nil pool keeps every method fail-closed (never a panic); the guard
// treats the failure as retryable instead of executing unfenced.
func NewControlStore(db *pgxpool.Pool) *ControlStore {
	if db == nil {
		return &ControlStore{}
	}
	return &ControlStore{db: db, q: dbsqlc.New(db)}
}

// ClaimResult reports whether the caller won the attempt (winner runs
// the handler) or must follow Existing (replay on same hash, 409 on a
// different hash).
type ClaimResult struct {
	Winner   bool
	Existing *ControlRecord
}

// errNotWired fails closed when the store has no pool: the guard
// treats the failure as retryable instead of executing unfenced.
var errNotWired = errors.New("idempotency: durable control store is not wired")

func controlClaimParams(claim ControlClaim) dbsqlc.ClaimControlIdempotencyParams {
	return dbsqlc.ClaimControlIdempotencyParams{
		OperatorIdentity: claim.Operator,
		Operation:        claim.Operation,
		TargetType:       claim.TargetType,
		TargetID:         claim.TargetID,
		IdempotencyKey:   claim.Key,
		RequestHash:      claim.RequestHash,
	}
}

func controlKeyParams(claim ControlClaim) dbsqlc.DeleteControlIdempotencyRecordParams {
	return dbsqlc.DeleteControlIdempotencyRecordParams{
		OperatorIdentity: claim.Operator,
		Operation:        claim.Operation,
		TargetType:       claim.TargetType,
		TargetID:         claim.TargetID,
		IdempotencyKey:   claim.Key,
	}
}

// Claim inserts the in-progress fact inside the caller-owned transaction
// tx and reports whether the caller won. A conflicting (already
// committed) row is returned as Existing: the caller replays it when
// the hash matches or renders 409 when it differs, before any side
// effect runs. An expired completed row is taken over transparently
// (deleted, then claimed), so retention expiry reads as a miss. A nil
// tx is a wiring error and fails the call; it never executes unfenced.
func (s *ControlStore) Claim(ctx context.Context, tx pgx.Tx, claim ControlClaim) (ClaimResult, error) {
	if s == nil || s.q == nil {
		return ClaimResult{}, errNotWired
	}
	if tx == nil {
		return ClaimResult{}, errors.New("idempotency: control claim requires the guard transaction")
	}
	q := s.q.WithTx(tx)
	_, err := q.ClaimControlIdempotency(ctx, controlClaimParams(claim))
	if err == nil {
		return ClaimResult{Winner: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ClaimResult{}, err
	}
	existing, err := s.get(ctx, q, claim)
	if err != nil {
		return ClaimResult{}, err
	}
	if existing == nil {
		// The winner rolled back between our INSERT and our SELECT:
		// the key is free again, so claim it.
		return s.reclaim(ctx, q, claim)
	}
	if existing.Expired {
		// Retention expiry reads as a miss: remove the stale fact and
		// take the key over in the same transaction.
		if err := q.DeleteControlIdempotencyRecord(ctx, controlKeyParams(claim)); err != nil {
			return ClaimResult{}, err
		}
		return s.reclaim(ctx, q, claim)
	}
	return ClaimResult{Existing: existing}, nil
}

// reclaim re-inserts the claim after the previous fact vanished (winner
// rollback) or was removed (retention expiry) and reports the outcome:
// winner on a clean insert, Existing when a concurrent claimant won the
// race in between.
func (s *ControlStore) reclaim(ctx context.Context, q *dbsqlc.Queries, claim ControlClaim) (ClaimResult, error) {
	_, err := q.ClaimControlIdempotency(ctx, controlClaimParams(claim))
	if err == nil {
		return ClaimResult{Winner: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ClaimResult{}, err
	}
	existing, err := s.get(ctx, q, claim)
	if err != nil {
		return ClaimResult{}, err
	}
	if existing == nil {
		return ClaimResult{}, errors.New("idempotency: control claim lost a race it should have won")
	}
	return ClaimResult{Existing: existing}, nil
}

// Complete flips the winner's claim to completed with the 2xx response
// inside the same transaction, so the dedup fact and the business
// outcome commit atomically. Non-2xx outcomes must call Abandon instead
// so the retry re-executes; only 2xx responses are retained.
func (s *ControlStore) Complete(ctx context.Context, tx pgx.Tx, claim ControlClaim, result ControlResult) error {
	if s == nil || s.q == nil {
		return errNotWired
	}
	if tx == nil {
		return errors.New("idempotency: control complete requires the guard transaction")
	}
	if result.Status < 200 || result.Status >= 300 {
		return errors.New("idempotency: only 2xx control responses are retained")
	}
	return s.q.WithTx(tx).CompleteControlIdempotency(ctx, dbsqlc.CompleteControlIdempotencyParams{
		OperatorIdentity:    claim.Operator,
		Operation:           claim.Operation,
		TargetType:          claim.TargetType,
		TargetID:            claim.TargetID,
		IdempotencyKey:      claim.Key,
		ResponseStatus:      pgtype.Int4{Int32: int32(result.Status), Valid: true},
		ResponseContentType: result.ContentType,
		ResponseBody:        result.Body,
		ExpiresAt:           pgtype.Timestamptz{Time: time.Now().UTC().Add(ReplayTTL), Valid: true},
	})
}

// Abandon deletes the winner's in-progress claim inside the same
// transaction (business failure, conflict, or audit-join failure), so
// the retry re-executes instead of replaying a failure. It is a no-op
// when the row already completed.
func (s *ControlStore) Abandon(ctx context.Context, tx pgx.Tx, claim ControlClaim) error {
	if s == nil || s.q == nil {
		return errNotWired
	}
	if tx == nil {
		return errors.New("idempotency: control abandon requires the guard transaction")
	}
	return s.q.WithTx(tx).AbandonControlIdempotency(ctx, dbsqlc.AbandonControlIdempotencyParams{
		OperatorIdentity: claim.Operator,
		Operation:        claim.Operation,
		TargetType:       claim.TargetType,
		TargetID:         claim.TargetID,
		IdempotencyKey:   claim.Key,
	})
}

// PurgeExpired reclaims expired completed rows in autocommit (outside
// any guard transaction). Callers invoke it best-effort after a
// successful commit; a failure never fails the mutation.
func (s *ControlStore) PurgeExpired(ctx context.Context) (int64, error) {
	if s == nil || s.q == nil || s.db == nil {
		return 0, errNotWired
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return s.q.PurgeExpiredControlIdempotency(ctx)
}

func (s *ControlStore) get(ctx context.Context, q *dbsqlc.Queries, claim ControlClaim) (*ControlRecord, error) {
	row, err := q.GetControlIdempotency(ctx, dbsqlc.GetControlIdempotencyParams{
		OperatorIdentity: claim.Operator,
		Operation:        claim.Operation,
		TargetType:       claim.TargetType,
		TargetID:         claim.TargetID,
		IdempotencyKey:   claim.Key,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	record := &ControlRecord{
		RequestHash: row.RequestHash,
		Completed:   row.Completed,
	}
	if row.Completed {
		record.Expired = !row.ExpiresAt.Valid || !time.Now().UTC().Before(row.ExpiresAt.Time)
		if !record.Expired {
			record.Result = ControlResult{
				ContentType: row.ResponseContentType,
				Body:        row.ResponseBody,
			}
			if row.ResponseStatus.Valid {
				record.Result.Status = int(row.ResponseStatus.Int32)
			}
		}
	}
	return record, nil
}
