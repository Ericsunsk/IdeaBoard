// Package authsync owns the durable Auth synchronization lane for
// Identity admin mutations (issue #888).
//
// Contract: the business authorization facts (users.role/status) are the
// fact source. Every lane Role/Ban/Unban mutation persists one sync
// intent in the SAME transaction as the business write (and the control
// audit row on the guarded path, which joins through the ambient carrier).
// External Auth I/O only ever runs after that transaction commits, from
// the desired snapshot stored in the intent row.
//
// Convergence is retry-driven, never process-local compensation: a
// periodic sweep claims due intents, pushes them to Auth outside any
// database transaction, and finalizes them with per-user serialization
// (pg_advisory_xact_lock, shared with mutations, expiry, and repair) and
// version fencing (users.auth_sync_version). A stale intent whose base
// version no longer matches the business row is superseded without
// touching Auth, so an old intent can never overwrite a newer state.
package authsync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Intent ops recorded in identity_auth_sync_intents.
const (
	OpRole  = "role"
	OpBan   = "ban"
	OpUnban = "unban"
)

// Intent statuses recorded in identity_auth_sync_intents.
const (
	StatusPending    = "pending"
	StatusInFlight   = "in_flight"
	StatusDone       = "done"
	StatusSuperseded = "superseded"
	StatusFailed     = "failed"
)

// errIntentLockTimeout pins the 55P03 SQLSTATE so NOWAIT claim
// contention skips the intent instead of failing the sweep.
const intentLockNotAvailableCode = "55P03"

// Intent is one durable Auth sync request.
type Intent struct {
	ID               int64
	UserID           int64
	AuthUserID       string
	Op               string
	DesiredRole      string
	DesiredBanned    bool
	ExpiresInSeconds int
	BaseVersion      int64
	Status           string
	Attempts         int
	LastError        string
	NextAttemptAt    time.Time
	ClaimedAt        *time.Time
}

// InsertIntentParams carries the business-committed desired state for one
// lane mutation. Callers must invoke it inside the mutation transaction
// (see Store.Transact): the intent commits or rolls back with the
// business write, never alone.
type InsertIntentParams struct {
	UserID           int64
	AuthUserID       string
	Op               string
	DesiredRole      string
	DesiredBanned    bool
	ExpiresInSeconds int
	BaseVersion      int64
}

// UserSyncState is the business authorization fact snapshot the fencing
// decisions read. DeletionPending reports a user_deletion_requests row:
// while the deletion saga owns the user, sync pushes and expiry unbans
// must stand down. BanExpiresAt is nil for permanent bans.
type UserSyncState struct {
	ID              int64
	Role            string
	Status          int16
	AuthUserID      string
	Version         int64
	BanExpiresAt    *time.Time
	DeletionPending bool
	Found           bool
}

// Banned reports the business ban fact in Auth terms.
func (s UserSyncState) Banned() bool { return s.Status == 0 }

// IsDemotionTarget reports whether the current business facts still call
// for a demotion repair: a live local user on the same Auth link outside
// deletion ownership. Stale findings (legal promotion, identity change,
// deletion, vanished row) return false and must never push.
func IsDemotionTarget(state UserSyncState, authUserID string) bool {
	authUserID = strings.TrimSpace(authUserID)
	return state.Found && !state.DeletionPending && authUserID != "" &&
		strings.ToLower(strings.TrimSpace(state.Role)) == "user" &&
		strings.TrimSpace(state.AuthUserID) == authUserID
}

// Store persists sync intents and the version markers with raw pgx SQL.
// It never performs Auth I/O and never commits or rolls back a borrowed
// carrier transaction.
type Store struct {
	db  *pgxpool.Pool
	now func() time.Time
}

// NewStore builds the intent store over pool. A nil pool fails every call
// with an error instead of panicking.
func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db, now: time.Now}
}

// SetClock overrides the clock (tests only).
func (s *Store) SetClock(now func() time.Time) {
	if s != nil && now != nil {
		s.now = now
	}
}

// Now reports the store clock. Lane mutations derive ban deadlines from
// it so tests control time without real waiting.
func (s *Store) Now() time.Time {
	return s.clock()
}

func (s *Store) clock() time.Time {
	if s == nil || s.now == nil {
		return time.Now()
	}
	return s.now()
}

// Transact joins the ambient transaction or owns a new one. fn must
// only use statements; the scope owner controls commit and rollback.
func (s *Store) Transact(ctx context.Context, fn func(ctx context.Context) error) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("identity auth sync store is not configured")
	}
	scope, err := dbutil.Begin(ctx, s.db, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("identity auth sync: begin transaction: %w", err)
	}
	if scope.Owned() {
		defer func() { _ = scope.Tx().Rollback(ctx) }()
	}
	// Stage the borrowed handle as well, so downstream statements cannot
	// commit or roll back the guard's transaction.
	if err := fn(dbutil.ContextWithAmbientTx(ctx, scope.Tx())); err != nil {
		return err
	}
	if err := scope.Commit(ctx); err != nil {
		return fmt.Errorf("identity auth sync: commit transaction: %w", err)
	}
	return nil
}

// executor resolves the statement target: the carrier transaction when
// staged, the pool otherwise. Single-statement callers stay correct on
// both paths; multi-statement units must run inside Transact.
type statementTarget interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func (s *Store) executor(ctx context.Context) statementTarget {
	// Thin adapter on the deep Tx module: the carrier-vs-pool borrow
	// decision lives in dbutil.BorrowOrPool. The statement-only shape
	// exposes no lifecycle, so a borrowed carrier cannot be ended here.
	return dbutil.BorrowOrPool(ctx, s.db)
}

// LockUser serializes the caller's unit with every other same-user lane
// unit (mutations, converge claims/finalizes, expiry, repair). It must
// run inside a transaction (see Transact/WithUserLock): the advisory
// lock is xact-scoped, so it releases on commit/rollback. Lock order is
// always advisory-first, then row locks; no path takes them in reverse.
func (s *Store) LockUser(ctx context.Context, userID int64) error {
	if s == nil || s.db == nil || userID <= 0 {
		return fmt.Errorf("identity auth sync: invalid user lock request")
	}
	if _, err := s.executor(ctx).Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, fmt.Sprintf("identity_auth_sync:%d", userID)); err != nil {
		return fmt.Errorf("identity auth sync: acquire user lock: %w", err)
	}
	return nil
}

// WithUserLock runs fn in a store-owned transaction holding the
// per-user advisory lock. Short units only: no Auth I/O inside fn.
func (s *Store) WithUserLock(ctx context.Context, userID int64, fn func(ctx context.Context) error) error {
	if s == nil || s.db == nil || userID <= 0 {
		return fmt.Errorf("identity auth sync: invalid user lock request")
	}
	if _, ok := dbutil.AmbientTxFromContext(ctx); ok {
		return fmt.Errorf("identity auth sync: WithUserLock needs an owned transaction")
	}
	return s.Transact(ctx, func(txCtx context.Context) error {
		if err := s.LockUser(txCtx, userID); err != nil {
			return err
		}
		return fn(txCtx)
	})
}

// ReadUserState loads the business authorization facts. With forUpdate
// the row is locked for the caller's unit; without it the read is a
// lock-free snapshot for pre-push rechecks.
func (s *Store) ReadUserState(ctx context.Context, userID int64, forUpdate bool) (UserSyncState, error) {
	if s == nil || s.db == nil || userID <= 0 {
		return UserSyncState{}, fmt.Errorf("identity auth sync: invalid user state request")
	}
	query := `
SELECT u.id, u.role, u.status, COALESCE(u.auth_user_id, ''), u.auth_sync_version, u.ban_expires_at,
       EXISTS (SELECT 1 FROM user_deletion_requests d WHERE d.user_id = u.id)
FROM users u WHERE u.id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	var state UserSyncState
	var banExpiresAt pgtype.Timestamptz
	err := s.executor(ctx).QueryRow(ctx, query, userID).Scan(
		&state.ID, &state.Role, &state.Status, &state.AuthUserID, &state.Version, &banExpiresAt, &state.DeletionPending,
	)
	if err != nil {
		if isNoRows(err) {
			return UserSyncState{Found: false}, nil
		}
		return UserSyncState{}, fmt.Errorf("identity auth sync: read user state: %w", err)
	}
	if banExpiresAt.Valid {
		expires := banExpiresAt.Time
		state.BanExpiresAt = &expires
	}
	state.Found = true
	return state, nil
}

// BumpVersion advances the fencing counter after the caller applied its
// business write in the same transaction. It deliberately leaves
// ban_expires_at untouched (unlike BumpVersionWithDeadline): a role-only
// change on a banned user must not clear the deadline. It returns the new
// version the intent must record as its base.
func (s *Store) BumpVersion(ctx context.Context, userID int64) (int64, error) {
	if s == nil || s.db == nil || userID <= 0 {
		return 0, fmt.Errorf("identity auth sync: invalid version bump request")
	}
	var version int64
	if err := s.executor(ctx).QueryRow(ctx, `
UPDATE users SET auth_sync_version = auth_sync_version + 1, updated_at = now()
WHERE id = $1 RETURNING auth_sync_version`, userID).Scan(&version); err != nil {
		return 0, fmt.Errorf("identity auth sync: bump version: %w", err)
	}
	return version, nil
}

// BumpVersionWithDeadline advances the fencing counter and sets (or
// clears, when deadline is nil) the business ban deadline in the same
// statement. Ban/Unban and the expiry sweep use it so status, deadline,
// version, and intent share one fate.
func (s *Store) BumpVersionWithDeadline(ctx context.Context, userID int64, deadline *time.Time) (int64, error) {
	if s == nil || s.db == nil || userID <= 0 {
		return 0, fmt.Errorf("identity auth sync: invalid version bump request")
	}
	var expires pgtype.Timestamptz
	if deadline != nil {
		expires = pgtype.Timestamptz{Time: *deadline, Valid: true}
	}
	var version int64
	if err := s.executor(ctx).QueryRow(ctx, `
UPDATE users SET auth_sync_version = auth_sync_version + 1, ban_expires_at = $2, updated_at = now()
WHERE id = $1 RETURNING auth_sync_version`, userID, expires).Scan(&version); err != nil {
		return 0, fmt.Errorf("identity auth sync: bump version: %w", err)
	}
	return version, nil
}

// queryIDs runs a single-column ID select and returns the IDs in row
// order. Callers wrap failures with their own context.
func (s *Store) queryIDs(ctx context.Context, query string, args ...any) ([]int64, error) {
	rows, err := s.executor(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

// ListDueBanUserIDs returns banned users whose finite deadline passed,
// oldest deadline first. Permanent bans (NULL) never match.
func (s *Store) ListDueBanUserIDs(ctx context.Context, now time.Time, limit int) ([]int64, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("identity auth sync: invalid ban expiry request")
	}
	if limit <= 0 {
		limit = 50
	}
	ids, err := s.queryIDs(ctx, `
SELECT id FROM users
WHERE status = 0 AND ban_expires_at IS NOT NULL AND ban_expires_at <= $1
ORDER BY ban_expires_at, id LIMIT $2`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("identity auth sync: list due ban expiries: %w", err)
	}
	return ids, nil
}

// InsertIntent persists one sync intent. It must run inside the mutation
// transaction (see InsertIntentParams): the row commits or rolls back
// with the business write.
func (s *Store) InsertIntent(ctx context.Context, params InsertIntentParams) (int64, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("identity auth sync: invalid intent request")
	}
	op := strings.TrimSpace(params.Op)
	role := strings.ToLower(strings.TrimSpace(params.DesiredRole))
	authUserID := strings.TrimSpace(params.AuthUserID)
	if params.UserID <= 0 || authUserID == "" || (op != OpRole && op != OpBan && op != OpUnban) ||
		(role != "admin" && role != "user") || params.ExpiresInSeconds < 0 || params.BaseVersion < 0 {
		return 0, fmt.Errorf("identity auth sync: invalid intent request")
	}
	var id int64
	if err := s.executor(ctx).QueryRow(ctx, `
INSERT INTO identity_auth_sync_intents (
  user_id, auth_user_id, op, desired_role, desired_banned,
  expires_in_seconds, base_version
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id`,
		params.UserID, authUserID, op, role, params.DesiredBanned,
		params.ExpiresInSeconds, params.BaseVersion,
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("identity auth sync: insert intent: %w", err)
	}
	return id, nil
}

// HasPendingIntent reports whether the user has a claimable intent. The
// request path uses it to render sync-pending honestly on no-op retries
// without fabricating a new push.
func (s *Store) HasPendingIntent(ctx context.Context, userID int64) (bool, error) {
	if s == nil || s.db == nil || userID <= 0 {
		return false, fmt.Errorf("identity auth sync: invalid pending intent request")
	}
	var exists bool
	if err := s.executor(ctx).QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM identity_auth_sync_intents
  WHERE user_id = $1 AND status IN ('pending', 'in_flight')
)`, userID).Scan(&exists); err != nil {
		return false, fmt.Errorf("identity auth sync: check pending intent: %w", err)
	}
	return exists, nil
}

// ReadIntent loads one intent. With forUpdate the row is locked;
// with tryLock a contended row is skipped (ok=false, nil error) instead
// of blocking the sweep.
func (s *Store) ReadIntent(ctx context.Context, intentID int64, forUpdate, tryLock bool) (intent Intent, ok bool, err error) {
	if s == nil || s.db == nil || intentID <= 0 {
		return Intent{}, false, fmt.Errorf("identity auth sync: invalid intent request")
	}
	query := `
SELECT id, user_id, auth_user_id, op, desired_role, desired_banned,
       expires_in_seconds, base_version, status, attempts, last_error,
       next_attempt_at, claimed_at
FROM identity_auth_sync_intents WHERE id = $1`
	switch {
	case forUpdate && tryLock:
		query += ` FOR UPDATE NOWAIT`
	case forUpdate:
		query += ` FOR UPDATE`
	}
	err = s.executor(ctx).QueryRow(ctx, query, intentID).Scan(
		&intent.ID, &intent.UserID, &intent.AuthUserID, &intent.Op, &intent.DesiredRole,
		&intent.DesiredBanned, &intent.ExpiresInSeconds, &intent.BaseVersion,
		&intent.Status, &intent.Attempts, &intent.LastError, &intent.NextAttemptAt, &intent.ClaimedAt,
	)
	if err != nil {
		if isNoRows(err) {
			return Intent{}, false, nil
		}
		if tryLock && isLockNotAvailable(err) {
			return Intent{}, false, nil
		}
		return Intent{}, false, fmt.Errorf("identity auth sync: read intent: %w", err)
	}
	return intent, true, nil
}

// ListDueIDs returns claimable intent IDs oldest-first: pending rows
// whose backoff elapsed, plus in_flight rows whose claim lease expired
// (worker died between claim and finalize; the claim transaction rolled
// back or the process never finalized). Lock-free: claiming happens per
// intent under the user lock.
func (s *Store) ListDueIDs(ctx context.Context, now time.Time, reapBefore time.Time, limit int) ([]int64, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("identity auth sync: invalid due intent request")
	}
	if limit <= 0 {
		limit = 50
	}
	ids, err := s.queryIDs(ctx, `
SELECT id FROM identity_auth_sync_intents
WHERE (status = 'pending' AND next_attempt_at <= $1)
   OR (status = 'in_flight' AND claimed_at <= $2)
ORDER BY id LIMIT $3`, now, reapBefore, limit)
	if err != nil {
		return nil, fmt.Errorf("identity auth sync: list due intents: %w", err)
	}
	return ids, nil
}

// ClaimIntent marks a pending (or lease-expired in_flight) intent as
// claimed by this worker and schedules its next backoff. Callers hold
// the user lock with the intent row locked: at most one worker owns the
// push.
func (s *Store) ClaimIntent(ctx context.Context, intentID int64, backoff time.Duration) (attempts int, err error) {
	if s == nil || s.db == nil || intentID <= 0 {
		return 0, fmt.Errorf("identity auth sync: invalid claim request")
	}
	next := s.clock().Add(backoff)
	if err := s.executor(ctx).QueryRow(ctx, `
UPDATE identity_auth_sync_intents
SET status = 'in_flight', attempts = attempts + 1, claimed_at = now(),
    next_attempt_at = $2, last_error = '', updated_at = now()
WHERE id = $1 AND status IN ('pending', 'in_flight')
RETURNING attempts`, intentID, next).Scan(&attempts); err != nil {
		return 0, fmt.Errorf("identity auth sync: claim intent: %w", err)
	}
	return attempts, nil
}

// MarkIntentDone records a confirmed push: Auth acknowledged the exact
// desired snapshot while the business row still carried the base
// version.
func (s *Store) MarkIntentDone(ctx context.Context, intentID int64) error {
	return s.terminalIntent(ctx, intentID, StatusDone, "")
}

// MarkIntentRetryable keeps a transiently failed intent claimable after
// its backoff. The worker already verified the business row still wants
// this push; only the Auth call failed.
func (s *Store) MarkIntentRetryable(ctx context.Context, intentID int64, nextAttemptAt time.Time, lastErr error) error {
	if s == nil || s.db == nil || intentID <= 0 {
		return fmt.Errorf("identity auth sync: invalid retry request")
	}
	tag, err := s.execUpdate(ctx, `
UPDATE identity_auth_sync_intents
SET status = 'pending', next_attempt_at = $2, last_error = $3, updated_at = now()
WHERE id = $1 AND status = 'in_flight'`, intentID, nextAttemptAt, errorMessage(lastErr))
	if err != nil {
		return fmt.Errorf("identity auth sync: mark intent retryable: %w", err)
	}
	if tag != 1 {
		return fmt.Errorf("identity auth sync: mark intent retryable: intent %d is not claimed", intentID)
	}
	return nil
}

// MarkIntentFailed records a permanently failed intent (e.g. the Auth
// identity is gone). Terminal: an operator owns the next step, surfaced
// by the reconciliation auth_identity_missing finding for the same link.
func (s *Store) MarkIntentFailed(ctx context.Context, intentID int64, lastErr error) error {
	return s.terminalIntent(ctx, intentID, StatusFailed, errorMessage(lastErr))
}

// MarkIntentSuperseded retires an intent whose business row moved on: a
// newer mutation owns convergence, so this push must never run. The note
// records why for operators.
func (s *Store) MarkIntentSuperseded(ctx context.Context, intentID int64, note string) error {
	return s.terminalIntent(ctx, intentID, StatusSuperseded, truncateError(note, 2000))
}

// execUpdate runs one UPDATE/DELETE and returns the affected row count.
func (s *Store) execUpdate(ctx context.Context, sql string, args ...any) (int64, error) {
	tag, err := s.executor(ctx).Exec(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Store) terminalIntent(ctx context.Context, intentID int64, status, message string) error {
	if s == nil || s.db == nil || intentID <= 0 {
		return fmt.Errorf("identity auth sync: invalid terminal intent request")
	}
	if status != StatusDone && status != StatusSuperseded && status != StatusFailed {
		return fmt.Errorf("identity auth sync: invalid terminal status %q", status)
	}
	tag, err := s.execUpdate(ctx, `
UPDATE identity_auth_sync_intents
SET status = $2, last_error = $3, completed_at = now(), updated_at = now()
WHERE id = $1 AND status IN ('pending', 'in_flight')`, intentID, status, message)
	if err != nil {
		return fmt.Errorf("identity auth sync: mark intent %s: %w", status, err)
	}
	if tag != 1 {
		return fmt.Errorf("identity auth sync: mark intent %s: intent %d is not claimable", status, intentID)
	}
	return nil
}

// EnsurePendingIntent inserts a repair intent for the CURRENT business
// state when no claimable intent covers it. Self-heal seam: every lane
// mutation already persists its own intent, so this only fires when an
// out-of-band writer moved the row (or a bug skipped the insert). Never
// called for unlinked or deletion-owned users.
func (s *Store) EnsurePendingIntent(ctx context.Context, state UserSyncState, op string) (int64, error) {
	if s == nil || s.db == nil || !state.Found || state.AuthUserID == "" || state.DeletionPending {
		return 0, fmt.Errorf("identity auth sync: invalid repair intent request")
	}
	op = strings.TrimSpace(op)
	if op != OpRole && op != OpBan && op != OpUnban {
		return 0, fmt.Errorf("identity auth sync: invalid repair intent op %q", op)
	}
	pending, err := s.HasPendingIntent(ctx, state.ID)
	if err != nil {
		return 0, err
	}
	if pending {
		return 0, nil
	}
	return s.InsertIntent(ctx, InsertIntentParams{
		UserID:        state.ID,
		AuthUserID:    state.AuthUserID,
		Op:            op,
		DesiredRole:   state.Role,
		DesiredBanned: state.Banned(),
		BaseVersion:   state.Version,
	})
}

func isNoRows(err error) bool {
	return database.IsNotFound(err)
}

func isLockNotAvailable(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == intentLockNotAvailableCode
}

// errorMessage renders a bounded operator-facing error note (empty when
// the cause is nil).
func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	return truncateError(err.Error(), 2000)
}

func truncateError(message string, limit int) string {
	message = strings.TrimSpace(message)
	if len(message) > limit {
		return message[:limit]
	}
	return message
}
