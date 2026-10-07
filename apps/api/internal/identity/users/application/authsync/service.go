// Converge logic for durable Auth sync intents (issue #888).
//
// Claim/push/finalize runs in three steps so no database transaction is
// ever held across the Auth HTTP call:
//
//  1. claim: under the per-user advisory lock, lock the intent and the
//     business row, verify the intent still matches the committed facts
//     (link, version, desired state, no deletion ownership), and mark it
//     in_flight with its next backoff.
//  2. push: re-read the version lock-free immediately before the call;
//     a moved version aborts the push (supersede path, no Auth I/O), so
//     a stale intent never overwrites a newer business state.
//  3. finalize: under the lock again, re-verify the row is untouched
//     since the claim; only then mark done (or retryable/failed on Auth
//     error). A moved row supersedes the intent and ensures a repair
//     intent covers the current facts.
//
// Crash recovery falls out of the rows: a worker that dies after the
// claim leaves an in_flight intent whose lease expires, so the next sweep
// (including after a restart) reaps and retries it. Nothing lives only in
// process memory.
package authsync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"ideaboard/api/internal/identity/autherrors"
	"ideaboard/api/internal/identity/users/ports"
)

// Converge tunables. The sweep interval lives with the scheduler; these
// bound one sweep and one intent.
const (
	// ConvergeBatchLimit bounds one sweep so a large backlog cannot exceed
	// the schedule worker timeout.
	ConvergeBatchLimit = 50
	// ClaimLeaseExpiry reaps in_flight intents whose worker died between
	// claim and finalize.
	ClaimLeaseExpiry = 5 * time.Minute
	// RetryBaseBackoff is the first retry delay; RetryMaxBackoff caps the
	// exponential growth. Retries are unbounded: an intent stays
	// claimable until it converges or is superseded.
	RetryBaseBackoff = 30 * time.Second
	RetryMaxBackoff  = 30 * time.Minute
)

// Service converges sync intents and (issue #895) processes due ban
// expiries. Auth pushes go through the narrow manager seam; business
// writes go through the store in caller transactions plus the lifecycle
// seam for Wallet alignment.
type Service struct {
	store     *Store
	auth      ports.AuthUserManager
	lifecycle ports.UserLifecycle
	now       func() time.Time
}

// ServiceDeps wires the service without concrete infrastructure: the
// composition root passes the Auth service client for both Auth seams
// and the user lifecycle for Wallet-aligned status changes.
type ServiceDeps struct {
	Store     *Store
	Auth      ports.AuthUserManager
	Lifecycle ports.UserLifecycle
}

// NewService builds the converge service. Store and Auth are required;
// Lifecycle is required for ban expiry (issue #895) and may be nil only
// when the caller never processes expiries.
func NewService(deps ServiceDeps) *Service {
	return &Service{
		store:     deps.Store,
		auth:      deps.Auth,
		lifecycle: deps.Lifecycle,
		now:       time.Now,
	}
}

// SetClock overrides the clock (tests only).
func (s *Service) SetClock(now func() time.Time) {
	if s != nil && now != nil {
		s.now = now
	}
}

func (s *Service) clock() time.Time {
	if s == nil || s.now == nil {
		return time.Now()
	}
	return s.now()
}

// ConvergeDue claims every due intent (oldest-first, bounded) and
// converges it. It returns the number of intents that reached a terminal
// disposition (done/superseded/failed) or were confirmed still converging
// (claimed and pushed, retryable). A transient Auth error keeps the
// intent pending and returns nil: durability lives in the row, not in
// the River attempt.
func (s *Service) ConvergeDue(ctx context.Context) (int, error) {
	if s == nil || s.store == nil || s.auth == nil {
		return 0, fmt.Errorf("identity auth sync: converge service is not configured")
	}
	now := s.clock()
	ids, err := s.store.ListDueIDs(ctx, now, now.Add(-ClaimLeaseExpiry), ConvergeBatchLimit)
	if err != nil {
		return 0, err
	}
	converged := 0
	for _, id := range ids {
		done, err := s.convergeOne(ctx, id)
		if err != nil {
			slog.Warn("identity auth sync converge failed",
				"intent_id", id, "err", err,
			)
			continue
		}
		if done {
			converged++
		}
	}
	return converged, nil
}

// convergeOne runs claim/push/finalize for one intent. It reports whether
// the intent was handled (any terminal, superseded, or retryable
// disposition); a skipped intent (lost race, already terminal) reports
// false.
func (s *Service) convergeOne(ctx context.Context, intentID int64) (bool, error) {
	claim, handled, err := s.claim(ctx, intentID)
	if err != nil || claim == nil {
		return handled, err
	}
	if !s.versionStillCurrent(ctx, claim) {
		return true, s.supersede(ctx, claim, "business state moved before Auth push; a newer mutation owns convergence")
	}
	if err := s.push(ctx, claim); err != nil {
		if errors.Is(err, autherrors.ErrAuthUserNotFound) {
			return true, s.storeFinalize(ctx, claim, finalizeFailed, err)
		}
		return true, s.storeFinalize(ctx, claim, finalizeRetryable, err)
	}
	return true, s.storeFinalize(ctx, claim, finalizeDone, nil)
}

// convergeClaim is the locked claim snapshot: the intent plus the
// business facts it was verified against.
type convergeClaim struct {
	intent Intent
	state  UserSyncState
}

// claim locks the user, verifies the intent against the committed
// business facts, and marks it in_flight. A nil claim means another
// worker won the race or the intent no longer needs this push; handled
// reports whether this call reached a disposition (claimed, or
// superseded during the claim).
func (s *Service) claim(ctx context.Context, intentID int64) (*convergeClaim, bool, error) {
	userID := intentOwner(ctx, s.store, intentID)
	if userID <= 0 {
		return nil, false, nil
	}
	var claim *convergeClaim
	handled := false
	err := s.store.WithUserLock(ctx, userID, func(locked context.Context) error {
		intent, ok, err := s.store.ReadIntent(locked, intentID, true, false)
		if err != nil || !ok {
			return err
		}
		if !claimable(intent, s.clock()) {
			return nil
		}
		state, err := s.store.ReadUserState(locked, intent.UserID, true)
		if err != nil {
			return err
		}
		if supersedeReason, ok := checkIntentCurrent(intent, state); !ok {
			if err := s.supersedeWithRepair(locked, intent, state, supersedeReason); err != nil {
				return err
			}
			handled = true
			return nil
		}
		attempts, err := s.store.ClaimIntent(locked, intent.ID, backoffFor(intent.Attempts))
		if err != nil {
			return err
		}
		intent.Attempts = attempts
		intent.Status = StatusInFlight
		claim = &convergeClaim{intent: intent, state: state}
		handled = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return claim, handled, nil
}

// intentOwner resolves the intent owner for lock acquisition. The intent
// row is immutable w.r.t. ownership, so a lock-free read is safe: the
// per-user lock only serializes same-user units. Zero means the intent
// is already gone.
func intentOwner(ctx context.Context, store *Store, intentID int64) int64 {
	intent, ok, err := store.ReadIntent(ctx, intentID, false, false)
	if err != nil || !ok {
		return 0
	}
	return intent.UserID
}

// claimable reports whether the intent still needs a push attempt.
func claimable(intent Intent, now time.Time) bool {
	switch intent.Status {
	case StatusPending:
		return !intent.NextAttemptAt.After(now)
	case StatusInFlight:
		return intent.ClaimedAt == nil || !intent.ClaimedAt.After(now.Add(-ClaimLeaseExpiry))
	default:
		return false
	}
}

// checkIntentCurrent verifies the intent against the committed business
// facts. ok=false carries the operator-facing supersede note.
func checkIntentCurrent(intent Intent, state UserSyncState) (reason string, ok bool) {
	if !state.Found {
		return "business user is gone; intent retired", false
	}
	if state.DeletionPending {
		return "user deletion owns the lifecycle; intent retired", false
	}
	if state.AuthUserID == "" || state.AuthUserID != intent.AuthUserID {
		return "Auth identity link changed; a newer mutation owns convergence", false
	}
	if state.Version != intent.BaseVersion {
		return "business state moved on; a newer mutation owns convergence", false
	}
	if state.Role != intent.DesiredRole || state.Banned() != intent.DesiredBanned {
		return "business facts no longer match the intent snapshot; a newer mutation owns convergence", false
	}
	return "", true
}

// repairOpFor picks the cheapest op that re-asserts the current facts.
// Role is preferred: SetUserRole is unconditional and metadata-free,
// while ban/unban pushes carry reason/expiry payloads.
func repairOpFor(state UserSyncState) string {
	if state.Banned() {
		return OpBan
	}
	return OpRole
}

// versionStillCurrent re-reads the fencing facts lock-free immediately
// before the Auth call. A moved row aborts the push: the row changed
// after the claim, so the intent is stale by definition.
func (s *Service) versionStillCurrent(ctx context.Context, claim *convergeClaim) bool {
	state, err := s.store.ReadUserState(ctx, claim.intent.UserID, false)
	if err != nil || !state.Found {
		return false
	}
	_, ok := checkIntentCurrent(claim.intent, state)
	return ok
}

// push performs the Auth I/O outside any database transaction, from the
// committed desired snapshot only.
func (s *Service) push(ctx context.Context, claim *convergeClaim) error {
	intent := claim.intent
	switch intent.Op {
	case OpRole:
		return s.auth.SetUserRole(ctx, intent.AuthUserID, intent.DesiredRole)
	case OpBan:
		return s.auth.BanUser(ctx, intent.AuthUserID, intent.Reason, intent.ExpiresInSeconds)
	case OpUnban:
		return s.auth.UnbanUser(ctx, intent.AuthUserID)
	default:
		return fmt.Errorf("identity auth sync: unsupported intent op %q", intent.Op)
	}
}

type finalizeKind int

const (
	finalizeDone finalizeKind = iota
	finalizeRetryable
	finalizeFailed
)

// storeFinalize re-verifies the business row under the lock and records
// the push outcome. A row that moved during the push supersedes the
// intent (with a repair intent ensuring the current facts stay covered)
// instead of confirming a stale push.
func (s *Service) storeFinalize(ctx context.Context, claim *convergeClaim, kind finalizeKind, pushErr error) error {
	return s.withCurrentIntent(ctx, claim.intent.UserID, claim.intent.ID, func(locked context.Context, intent Intent, state UserSyncState, current bool, reason string) error {
		if !current {
			return s.supersedeWithRepair(locked, intent, state, reason)
		}
		switch kind {
		case finalizeDone:
			return s.store.MarkIntentDone(locked, intent.ID)
		case finalizeFailed:
			return s.store.MarkIntentFailed(locked, intent.ID, pushErr)
		default:
			return s.store.MarkIntentRetryable(locked, intent.ID, s.clock().Add(backoffFor(intent.Attempts)), pushErr)
		}
	})
}

// supersede retires a claim whose pre-push recheck failed, keeping a
// repair intent when the current facts are otherwise uncovered.
func (s *Service) supersede(ctx context.Context, claim *convergeClaim, note string) error {
	return s.withCurrentIntent(ctx, claim.intent.UserID, claim.intent.ID, func(locked context.Context, intent Intent, state UserSyncState, current bool, _ string) error {
		if current {
			return s.store.MarkIntentRetryable(locked, intent.ID, s.clock().Add(backoffFor(intent.Attempts)), nil)
		}
		return s.supersedeWithRepair(locked, intent, state, note)
	})
}

// withCurrentIntent re-reads the intent and the business row under the
// per-user lock and reports the fencing verdict. Stale rows arrive with
// current=false and the operator-facing reason; callers supersede them
// (with repair) instead of confirming a moved state.
func (s *Service) withCurrentIntent(ctx context.Context, userID, intentID int64, fn func(locked context.Context, intent Intent, state UserSyncState, current bool, reason string) error) error {
	return s.store.WithUserLock(ctx, userID, func(locked context.Context) error {
		intent, ok, err := s.store.ReadIntent(locked, intentID, true, false)
		if err != nil || !ok {
			return err
		}
		if intent.Status != StatusInFlight {
			return nil
		}
		state, err := s.store.ReadUserState(locked, intent.UserID, true)
		if err != nil {
			return err
		}
		reason, current := checkIntentCurrent(intent, state)
		return fn(locked, intent, state, current, reason)
	})
}

// supersedeWithRepair retires a stale intent and ensures a repair intent
// covers the current facts, so convergence stays owned by newer state.
func (s *Service) supersedeWithRepair(locked context.Context, intent Intent, state UserSyncState, note string) error {
	if err := s.store.MarkIntentSuperseded(locked, intent.ID, note); err != nil {
		return err
	}
	_, err := s.store.EnsurePendingIntent(locked, state, repairOpFor(state))
	return err
}

// backoffFor grows exponentially from the base and caps at the max, so
// transient Auth outages retry without hot-looping the Auth endpoint.
func backoffFor(attempts int) time.Duration {
	backoff := RetryBaseBackoff
	for i := 0; i < attempts && backoff < RetryMaxBackoff; i++ {
		backoff *= 2
		if backoff > RetryMaxBackoff {
			return RetryMaxBackoff
		}
	}
	return backoff
}

// BanExpiryBatchLimit bounds one expiry sweep so a large backlog cannot
// exceed the schedule worker timeout.
const BanExpiryBatchLimit = 50

// ProcessDueBanExpiries unbans every expired finite ban (issue #895).
// Each expiry is one atomic business unit: status flip + Wallet unfreeze
// (through the lifecycle seam) + deadline clear + version bump + unban
// intent share a single transaction under the per-user lock, reusing the
// issue #888 serialization and fencing. Auth convergence follows through
// the normal intent path with durable retry.
//
// Skips are load-bearing, not errors: permanent bans (NULL deadline),
// extended re-bans, already-active users, deletion-owned users, and
// unlinked/tombstoned rows must never be touched, so an old sweep can
// never lift a newer ban. now is explicit so tests drive time without
// real waiting.
func (s *Service) ProcessDueBanExpiries(ctx context.Context, now time.Time) (int, error) {
	if s == nil || s.store == nil {
		return 0, fmt.Errorf("identity auth sync: expiry service is not configured")
	}
	if s.lifecycle == nil {
		return 0, fmt.Errorf("identity auth sync: ban expiry needs a user lifecycle")
	}
	ids, err := s.store.ListDueBanUserIDs(ctx, now, BanExpiryBatchLimit)
	if err != nil {
		return 0, err
	}
	expired := 0
	for _, id := range ids {
		done, err := s.expireOne(ctx, id, now)
		if err != nil {
			slog.Warn("identity ban expiry failed", "user_id", id, "err", err)
			continue
		}
		if done {
			expired++
		}
	}
	return expired, nil
}

// expireOne unbans a single expired ban when the CURRENT row still
// carries an expired finite deadline. Every skip re-reads the row, so
// concurrent re-bans, manual unbans, deletions, and tombstones are all
// observed, never assumed.
func (s *Service) expireOne(ctx context.Context, userID int64, now time.Time) (bool, error) {
	expired := false
	err := s.store.Transact(ctx, func(txCtx context.Context) error {
		if err := s.store.LockUser(txCtx, userID); err != nil {
			return err
		}
		state, err := s.store.ReadUserState(txCtx, userID, true)
		if err != nil {
			return err
		}
		if !state.Found || !state.Banned() {
			return nil
		}
		if state.BanExpiresAt == nil || state.BanExpiresAt.After(now) {
			return nil
		}
		if state.DeletionPending || state.AuthUserID == "" {
			return nil
		}
		// The lifecycle seam rejects active-during-deletion as a second
		// guard; its Wallet unfreeze joins this same transaction.
		if err := s.lifecycle.SetUserStatus(txCtx, userID, 1); err != nil {
			return fmt.Errorf("identity ban expiry: unban user %d: %w", userID, err)
		}
		version, err := s.store.BumpVersionWithDeadline(txCtx, userID, nil)
		if err != nil {
			return err
		}
		if _, err := s.store.InsertIntent(txCtx, InsertIntentParams{
			UserID:        userID,
			AuthUserID:    state.AuthUserID,
			Op:            OpUnban,
			DesiredRole:   state.Role,
			DesiredBanned: false,
			BaseVersion:   version,
		}); err != nil {
			return err
		}
		expired = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return expired, nil
}
