// Package reconciliation runs the Identity admin reconciliation job.
//
// Background: the admin commands in
// internal/identity/users/application/administration apply Auth-then-local
// without an automatic convergence mechanism. A crash between the two
// writes (or a partially compensating rollback) leaves the local users row
// and the Better Auth identity diverged, with no record and no sweep.
//
// This job closes the detection gap only. Every scheduled run pages the
// local users table, batch-reads the Auth side through the read-only
// AuthUserStateReader seam, and persists one finding per drift. The local
// row always wins future repairs (recorded in finding details); the dry-run
// phase never pushes anything anywhere.
//
// Deliberately out of scope for this phase:
//   - banExpires/banReason comparison: the local users row has no such
//     columns, so Auth-side ban metadata is alert-only (kept in details).
//   - sessions: a deleted Auth session is unrecoverable, so sessions are
//     never reconciled.
//   - automatic remediation: ExecuteReconciliation fails closed when the
//     auto-remediation opt-in is set; opening it needs a dedicated switch
//     plus repair-path tests (see ErrAutoRemediationNotSupported).
package reconciliation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"ideaboard/api/internal/identity/authidentity"
	"ideaboard/api/internal/identity/users/application/authsync"
	"ideaboard/api/internal/identity/users/domain"
	"ideaboard/api/internal/identity/users/ports"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrAutoRemediationNotSupported is returned when the reconciler is asked to
// repair drift. The dry-run phase only records findings and emits alerts;
// automatic remediation needs a dedicated opt-in switch and a separate
// review round (including repair-path tests) before it may be enabled.
var ErrAutoRemediationNotSupported = errors.New("identity admin reconciliation: automatic remediation is not enabled (dry-run only)")

// ErrRunNotFound is returned when a reconciliation run ID does not exist.
var ErrRunNotFound = errors.New("identity admin reconciliation run not found")

// ErrRunNotExecutable is returned when a run is already completed. A
// completed run is immutable; fresh drift is observed by a new scheduled
// run, never by re-executing a finished one.
var ErrRunNotExecutable = errors.New("identity admin reconciliation run is already completed")

var errInvalidReconciliationCommand = errors.New("identity admin reconciliation: invalid command")

// Finding types recorded by the dry-run scan. Each type maps to one
// compared field pair; ban metadata and sessions intentionally have no
// finding type (see the package doc).
const (
	FindingRoleMismatch         = "role_mismatch"
	FindingBanStateMismatch     = "ban_state_mismatch"
	FindingAuthIdentityMissing  = "auth_identity_missing"
	FindingLocalAuthLinkMissing = "local_auth_link_missing"
)

// Finding severities and risks follow the credits reconciliation vocabulary
// (warning/error/critical x low/high) so operators read both surfaces the
// same way.
const (
	SeverityWarning = "warning"
	SeverityError   = "error"

	RiskLow  = "low"
	RiskHigh = "high"
)

// Scan bounds. Admin surfaces are small; the page size keeps each local
// query cheap and the Auth batch size respects the internal contract limit
// (1-100 user IDs per request).
const (
	scanPageSize  = 200
	authBatchSize = 100
)

// userScanner is the narrow local seam the job needs: a full ordered scan.
// Depending on the interface (not the concrete repository) keeps the
// dry-run guarantee reviewable: the job cannot write users rows because
// the seam has no write method.
type userScanner interface {
	List(ctx context.Context, keyword, role string, status int16, hasStatus bool, page, pageSize int) ([]*domain.User, ports.UserListCounts, error)
}

// Reconciler executes Identity admin reconciliation runs. The scan remains
// read-only; the worker may separately repair only the low-privilege side of
// a role mismatch (local user versus Auth admin). Pending Auth sync intents
// (issue #888) converge through the same service on a frequent sweep.
type Reconciler struct {
	store         *Store
	users         userScanner
	auth          ports.AuthUserStateReader
	manager       ports.AuthUserManager
	sync          *authsync.Service
	syncStore     *authsync.Store
	autoRemediate bool
	now           func() time.Time
}

// ReconcilerDeps wires the reconciler without importing concrete
// infrastructure: the composition root passes the users repository (which
// satisfies userScanner) and the Auth service client (which satisfies both
// the Auth state reader and the narrow role manager used by the worker).
// Lifecycle enables the ban expiry sweep (issue #895); it may be nil only
// when the caller never sweeps expiries.
type ReconcilerDeps struct {
	DB        *pgxpool.Pool
	Users     userScanner
	Auth      ports.AuthUserStateReader
	Manager   ports.AuthUserManager
	Lifecycle ports.UserLifecycle
	// AutoRemediate is the future opt-in switch. It must stay false: when
	// true, ExecuteReconciliation fails closed with
	// ErrAutoRemediationNotSupported instead of repairing anything.
	AutoRemediate bool
}

// NewReconciler builds the dry-run reconciler.
func NewReconciler(deps ReconcilerDeps) *Reconciler {
	syncStore := authsync.NewStore(deps.DB)
	return &Reconciler{
		store:     NewStore(deps.DB),
		users:     deps.Users,
		auth:      deps.Auth,
		manager:   deps.Manager,
		syncStore: syncStore,
		sync: authsync.NewService(authsync.ServiceDeps{
			Store:     syncStore,
			Auth:      deps.Manager,
			Lifecycle: deps.Lifecycle,
		}),
		autoRemediate: deps.AutoRemediate,
		now:           time.Now,
	}
}

// SweepAuthSync converges due Auth sync intents and unbans expired
// finite bans (issues #888/#895). It is the frequent counterpart to the
// daily drift scan: business mutations only persist intents, and this
// sweep performs the post-commit Auth I/O with per-user serialization
// and version fencing, then finalizes each intent. Expiries run first so
// a just-expired ban can converge in the same sweep. Transient failures
// stay pending for the next sweep; nothing depends on process-local
// state, so a restarted worker resumes from the rows.
func (r *Reconciler) SweepAuthSync(ctx context.Context) (int, error) {
	if r == nil || r.sync == nil {
		return 0, errInvalidReconciliationCommand
	}
	// Converge first (latency-sensitive pushes), then expiries (which may
	// create fresh unban intents for the next sweep). Either stage makes
	// durable progress independently, so both run even if one fails.
	converged, err := r.sync.ConvergeDue(ctx)
	expired, expiryErr := r.sync.ProcessDueBanExpiries(ctx, r.clock())
	if err != nil {
		return converged + expired, err
	}
	return converged + expired, expiryErr
}

func (r *Reconciler) clock() time.Time {
	if r != nil && r.now != nil {
		return r.now()
	}
	return time.Now()
}

// RepairRoleDemotions applies only demotion findings from a completed scan.
// SetUserRole is idempotent, and each finding is resolved only after Auth
// acknowledges the desired local user role. Promotion drift remains manual so
// a stale Auth read can never grant admin privileges automatically.
//
// Issue #896: a finding is only a clue, never the authorization fact. Before
// every push the repair re-reads the CURRENT business row (role, lifecycle,
// Auth identity link) under the per-user lock and re-reads the CURRENT Auth
// state; anything that moved since the scan (legal promotion, identity
// change, concurrent mutation) skips safely. Post-push, the business row is
// re-verified before the finding resolves, so a concurrent legal change
// during the push can never be confirmed away.
func (r *Reconciler) RepairRoleDemotions(ctx context.Context, runID int64) error {
	if r == nil || r.store == nil || r.manager == nil || runID <= 0 {
		return errInvalidReconciliationCommand
	}
	if r.auth == nil || r.syncStore == nil {
		return errInvalidReconciliationCommand
	}
	findings, err := r.store.ListRunFindings(ctx, runID)
	if err != nil {
		return err
	}
	for _, finding := range findings {
		if finding.Status != "open" || finding.FindingType != FindingRoleMismatch {
			continue
		}
		if err := r.repairRoleFinding(ctx, &finding); err != nil {
			return err
		}
	}
	return nil
}

// repairRoleFinding revalidates one role finding against current facts and
// repairs it only when the demotion is still the confirmed target.
// Stale findings skip quietly (the next scan re-observes or resolves
// them); only Auth I/O failures abort the run for retry.
func (r *Reconciler) repairRoleFinding(ctx context.Context, finding *Finding) error {
	authID, _ := finding.Details["auth_user_id"].(string)
	authID = strings.TrimSpace(authID)
	scanRole, _ := finding.Details["local_role"].(string)
	scanRole = strings.ToLower(strings.TrimSpace(scanRole))
	if authID == "" || finding.UserID == nil || *finding.UserID <= 0 {
		slog.Warn("identity admin reconciliation: skipping malformed role finding",
			"finding_id", finding.ID, "run_id", finding.LastSeenRunID)
		return nil
	}
	userID := *finding.UserID
	if scanRole != domain.RoleUser {
		// Promotion drift (local admin): never auto-repair upward.
		return nil
	}
	// Current Auth fact first (outside any transaction): a gone identity
	// or an already-converged role needs no push.
	states, err := r.auth.GetUserStates(ctx, []string{authID})
	if err != nil {
		return fmt.Errorf("identity admin reconciliation: read auth state for repair: %w", err)
	}
	authState, ok := states[authID]
	if !ok {
		// The Auth identity is gone; the scan reports
		// auth_identity_missing for operator review. Never push.
		return nil
	}
	if !isAdminRole(authState.Role) {
		// Auth already matches the local user fact: confirm the business
		// row still agrees, then resolve without any push.
		current, err := r.lockedRepairState(ctx, userID)
		if err != nil {
			return err
		}
		if authsync.IsDemotionTarget(current, authID) {
			return r.store.resolveFinding(ctx, finding.ID)
		}
		return nil
	}
	// Drift persists (Auth admin against local user): revalidate the
	// business row under the per-user lock shared with mutations.
	current, err := r.lockedRepairState(ctx, userID)
	if err != nil {
		return err
	}
	if !authsync.IsDemotionTarget(current, authID) {
		// Legal promotion, identity change, deletion ownership, or a
		// vanished row since the scan: skip, never push stale facts.
		return nil
	}
	if !r.repairStillCurrent(ctx, userID, authID, current.Version) {
		return nil
	}
	if err := r.manager.SetUserRole(ctx, authID, domain.RoleUser); err != nil {
		return fmt.Errorf("identity admin reconciliation: repair role finding %d: %w", finding.ID, err)
	}
	// Confirm before resolving: the business row must be untouched since
	// the pre-push check, otherwise a concurrent legal mutation owns the
	// new state and this finding stays open for re-evaluation.
	confirmed, err := r.lockedRepairState(ctx, userID)
	if err != nil {
		return err
	}
	if confirmed.Version != current.Version || !authsync.IsDemotionTarget(confirmed, authID) {
		slog.Warn("identity admin reconciliation: business moved during repair push; finding left open",
			"finding_id", finding.ID, "user_id", userID)
		return nil
	}
	return r.store.resolveFinding(ctx, finding.ID)
}

// lockedRepairState reads the current business facts under the per-user
// advisory lock shared with mutations, converge, and expiry (short unit,
// no I/O). The authsync snapshot is the fencing fact source, so repair
// reuses it instead of a parallel struct.
func (r *Reconciler) lockedRepairState(ctx context.Context, userID int64) (authsync.UserSyncState, error) {
	var out authsync.UserSyncState
	err := r.syncStore.WithUserLock(ctx, userID, func(locked context.Context) error {
		state, err := r.syncStore.ReadUserState(locked, userID, true)
		if err != nil {
			return err
		}
		out = state
		return nil
	})
	if err != nil {
		return authsync.UserSyncState{}, err
	}
	return out, nil
}

// repairStillCurrent re-reads the fencing facts lock-free immediately
// before the Auth push: any concurrent legal mutation aborts the push.
func (r *Reconciler) repairStillCurrent(ctx context.Context, userID int64, authID string, version int64) bool {
	state, err := r.syncStore.ReadUserState(ctx, userID, false)
	if err != nil || !state.Found {
		return false
	}
	return state.Version == version && authsync.IsDemotionTarget(state, authID)
}

func isAdminRole(role string) bool {
	return strings.ToLower(strings.TrimSpace(role)) == domain.RoleAdmin
}

// SetClock overrides the clock (tests only).
func (r *Reconciler) SetClock(now func() time.Time) {
	if r != nil && now != nil {
		r.now = now
	}
}

// ScheduleReconciliation creates (or reuses) the scheduled run for the
// given slot and enqueues its execute job when required. It mirrors the
// credits schedule path: the same slot reuses the same run, so overlapping
// scheduler ticks never duplicate work. Run creation and job enqueue share
// one transaction, so a scheduled run always has its execute job.
func (r *Reconciler) ScheduleReconciliation(ctx context.Context, dispatcher RunDispatcher, slot time.Time, onlyIfOverdue bool) (*Run, error) {
	if r == nil || r.store == nil || dispatcher == nil || slot.IsZero() {
		return nil, errInvalidReconciliationCommand
	}
	pool := r.store.dbWithContext(ctx)
	if pool == nil {
		return nil, errInvalidReconciliationCommand
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("identity admin reconciliation: begin schedule: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	runID, enqueueRequired, err := r.store.startRunTx(ctx, tx, runTriggerScheduled, nil, nil, &slot, onlyIfOverdue)
	if err != nil {
		return nil, err
	}
	if enqueueRequired {
		if runID == nil || *runID <= 0 {
			return nil, fmt.Errorf("identity admin reconciliation: enqueue-required run has invalid ID")
		}
		if err := dispatcher.DispatchReconciliationRun(ctx, tx, *runID); err != nil {
			return nil, fmt.Errorf("identity admin reconciliation: enqueue run: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("identity admin reconciliation: commit schedule: %w", err)
	}
	if runID == nil || *runID <= 0 {
		return nil, nil
	}
	run, err := r.store.getRun(ctx, *runID)
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// ExecuteReconciliation runs one dry-run scan for the given run: it pages
// the local users table, batch-reads Auth state, persists findings, and
// completes the run. Re-executing a completed run is a no-op; re-executing
// an unfinished run re-scans idempotently (fingerprint upserts never
// duplicate findings).
func (r *Reconciler) ExecuteReconciliation(ctx context.Context, runID int64) error {
	if r == nil || r.store == nil || r.users == nil || r.auth == nil || runID <= 0 {
		return errInvalidReconciliationCommand
	}
	if r.autoRemediate {
		return ErrAutoRemediationNotSupported
	}
	run, err := r.store.getRun(ctx, runID)
	if err != nil {
		return err
	}
	if run.Status == runStatusCompleted {
		return nil
	}
	if run.Status != runStatusQueued && run.Status != runStatusRunning && run.Status != runStatusFailed {
		return ErrRunNotExecutable
	}
	if err := r.store.markRunning(ctx, runID); err != nil {
		return err
	}
	// The scan runs outside any transaction: it fans out to Auth over HTTP
	// and must never hold database locks while waiting on the network.
	drafts, scanned, err := r.scan(ctx)
	if err != nil {
		return err
	}
	if err := r.store.persistScan(ctx, runID, drafts); err != nil {
		return err
	}
	for _, draft := range drafts {
		slog.Warn("identity admin reconciliation finding",
			"run_id", runID,
			"finding_type", draft.FindingType,
			"severity", draft.Severity,
			"user_id", draft.UserID,
			"subject_id", draft.SubjectID,
		)
	}
	slog.Info("identity admin reconciliation run completed",
		"run_id", runID,
		"users_scanned", scanned,
		"finding_count", len(drafts),
		"dry_run", true,
	)
	return nil
}

// FailReconciliation marks a run failed after the River job exhausts its
// attempts. It only touches unfinished runs, so a concurrently completed
// run is never regressed to failed.
func (r *Reconciler) FailReconciliation(ctx context.Context, runID int64, cause error) error {
	if r == nil || r.store == nil || runID <= 0 || cause == nil {
		return errInvalidReconciliationCommand
	}
	return r.store.failRun(ctx, runID, cause)
}

// GetReconciliationRun loads one run for worker failure logging.
func (r *Reconciler) GetReconciliationRun(ctx context.Context, runID int64) (Run, error) {
	if r == nil || r.store == nil || runID <= 0 {
		return Run{}, errInvalidReconciliationCommand
	}
	return r.store.getRun(ctx, runID)
}

// ListRunFindings returns the findings observed by one run (run-id
// isolation: re-running the same run never grows this set).
func (r *Reconciler) ListRunFindings(ctx context.Context, runID int64) ([]Finding, error) {
	if r == nil || r.store == nil || runID <= 0 {
		return nil, errInvalidReconciliationCommand
	}
	return r.store.ListRunFindings(ctx, runID)
}

// ListOpenFindings returns every unresolved finding, newest first.
func (r *Reconciler) ListOpenFindings(ctx context.Context) ([]Finding, error) {
	if r == nil || r.store == nil {
		return nil, errInvalidReconciliationCommand
	}
	return r.store.ListOpenFindings(ctx)
}

// CountFindings returns the total finding rows (idempotency assertion seam:
// re-runs must not grow this count for unchanged drift).
func (r *Reconciler) CountFindings(ctx context.Context) (int64, error) {
	if r == nil || r.store == nil {
		return 0, errInvalidReconciliationCommand
	}
	return r.store.CountFindings(ctx)
}

// scan pages every local user, resolves the linked Auth identities in
// bounded batches, and classifies drift. Any Auth batch failure aborts the
// whole scan: without Auth data every linked user would misreport as
// missing, so partial results are never persisted.
func (r *Reconciler) scan(ctx context.Context) ([]findingDraft, int, error) {
	var drafts []findingDraft
	linked := make(map[string]*domain.User)
	scanned := 0
	for page := 1; ; page++ {
		users, _, err := r.users.List(ctx, "", "", 0, false, page, scanPageSize)
		if err != nil {
			return nil, 0, fmt.Errorf("identity admin reconciliation: list users: %w", err)
		}
		for _, user := range users {
			if user == nil || user.ID <= 0 {
				continue
			}
			scanned++
			link := strings.TrimSpace(derefAuthUserID(user.AuthUserID))
			if link == "" {
				drafts = append(drafts, classifyUserDrift(user, nil, false)...)
				continue
			}
			if _, seen := linked[link]; !seen {
				linked[link] = user
			}
		}
		if len(users) < scanPageSize {
			break
		}
	}
	ids := make([]string, 0, len(linked))
	for id := range linked {
		ids = append(ids, id)
	}
	for offset := 0; offset < len(ids); offset += authBatchSize {
		end := offset + authBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		states, err := r.auth.GetUserStates(ctx, ids[offset:end])
		if err != nil {
			return nil, 0, fmt.Errorf("identity admin reconciliation: read auth states: %w", err)
		}
		for _, id := range ids[offset:end] {
			state, ok := states[id]
			if !ok {
				drafts = append(drafts, classifyUserDrift(linked[id], nil, true)...)
				continue
			}
			stateCopy := state
			drafts = append(drafts, classifyUserDrift(linked[id], &stateCopy, true)...)
		}
	}
	return drafts, scanned, nil
}

// classifyUserDrift compares one local user against its Auth state and
// returns zero or more finding drafts. A user can carry two findings when
// both role and ban state drifted; each has its own fingerprint.
// The local row always wins future repairs; dry-run records that intent
// without acting on it.
func classifyUserDrift(user *domain.User, state *authidentity.AuthUserState, hasLink bool) []findingDraft {
	if user == nil || user.ID <= 0 {
		return nil
	}
	subjectID := user.PublicID.String()
	if !hasLink {
		return []findingDraft{{
			FindingType: FindingLocalAuthLinkMissing,
			Severity:    SeverityWarning,
			Risk:        RiskHigh,
			UserID:      user.ID,
			SubjectType: "user",
			SubjectID:   subjectID,
			Details: map[string]any{
				"local_role":   user.Role,
				"local_banned": user.Status == domain.StatusBanned,
				"auth_found":   false,
				"winner":       "local",
				"remediation":  "manual_review",
				"applied":      false,
				"dry_run":      true,
				"note":         "local user has no auth_user_id link; linking requires operator review, never automatic provisioning",
			},
		}}
	}
	if state == nil {
		return []findingDraft{{
			FindingType: FindingAuthIdentityMissing,
			Severity:    SeverityError,
			Risk:        RiskHigh,
			UserID:      user.ID,
			SubjectType: "user",
			SubjectID:   subjectID,
			Details: map[string]any{
				"local_role":   user.Role,
				"local_banned": user.Status == domain.StatusBanned,
				"auth_user_id": strings.TrimSpace(derefAuthUserID(user.AuthUserID)),
				"auth_found":   false,
				"winner":       "local",
				"remediation":  "manual_review",
				"applied":      false,
				"dry_run":      true,
				"note":         "auth identity is gone while the local row still links to it; recovery requires operator review, never automatic deletion",
			},
		}}
	}
	var drafts []findingDraft
	if !sameRole(user.Role, state.Role) {
		drafts = append(drafts, findingDraft{
			FindingType: FindingRoleMismatch,
			Severity:    SeverityError,
			Risk:        RiskHigh,
			UserID:      user.ID,
			SubjectType: "user",
			SubjectID:   subjectID,
			Details: map[string]any{
				"local_role":   user.Role,
				"auth_role":    state.Role,
				"auth_user_id": strings.TrimSpace(derefAuthUserID(user.AuthUserID)),
				"auth_found":   true,
				"winner":       "local",
				"remediation":  "push_to_auth",
				"applied":      false,
				"dry_run":      true,
				"note":         "role drifted; local wins and a future repair would push the local role to Auth (demote drift leaves a residual Auth admin)",
			},
		})
	}
	localBanned := user.Status == domain.StatusBanned
	if localBanned != state.Banned {
		action := "push_unban_to_auth"
		if localBanned {
			action = "push_ban_to_auth"
		}
		details := map[string]any{
			"local_banned": localBanned,
			"auth_banned":  state.Banned,
			"auth_found":   true,
			"winner":       "local",
			"remediation":  action,
			"applied":      false,
			"dry_run":      true,
			"note":         "ban-state drifted; local wins. Auth-side ban reason/expiry (if any) is alert-only: the local row has no ban columns, so a future ban push cannot restore the original reason/expiry",
		}
		if state.BanReason != nil {
			details["auth_ban_reason"] = *state.BanReason
		} else {
			details["auth_ban_reason"] = nil
		}
		if state.BanExpiresAt != nil {
			details["auth_ban_expires"] = state.BanExpiresAt.UTC().Format(time.RFC3339)
		} else {
			details["auth_ban_expires"] = nil
		}
		drafts = append(drafts, findingDraft{
			FindingType: FindingBanStateMismatch,
			Severity:    SeverityError,
			Risk:        RiskHigh,
			UserID:      user.ID,
			SubjectType: "user",
			SubjectID:   subjectID,
			Details:     details,
		})
	}
	return drafts
}

func sameRole(local, auth string) bool {
	normalizedLocal := strings.ToLower(strings.TrimSpace(local))
	normalizedAuth := strings.ToLower(strings.TrimSpace(auth))
	if normalizedLocal == "" || normalizedAuth == "" {
		return normalizedLocal == normalizedAuth
	}
	return normalizedLocal == normalizedAuth
}

func derefAuthUserID(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// fingerprintForDraft derives the stable deduplication key: one finding
// type per drifted subject. Re-scans upsert on this key, so repeated runs
// refresh last_seen_run_id/occurrence_count instead of duplicating rows.
func fingerprintForDraft(draft findingDraft) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		draft.FindingType,
		fmt.Sprintf("%d", draft.UserID),
		draft.SubjectType,
		draft.SubjectID,
	}, "\x1f")))
	return hex.EncodeToString(sum[:])
}
