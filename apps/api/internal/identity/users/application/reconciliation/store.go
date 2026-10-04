package reconciliation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Run statuses and triggers mirror the credits reconciliation vocabulary.
const (
	runStatusQueued    = "queued"
	runStatusRunning   = "running"
	runStatusCompleted = "completed"
	runStatusFailed    = "failed"

	runTriggerScheduled = "scheduled"
)

// Run is the persisted identity admin reconciliation run.
type Run struct {
	ID           int64
	Status       string
	Trigger      string
	RequestedBy  *int64
	Reason       string
	ScheduledFor *time.Time
	FindingCount int64
	AttemptCount int
	ErrorMessage string
	RequestedAt  time.Time
	StartedAt    *time.Time
	CompletedAt  *time.Time
}

// Finding is one persisted drift observation.
type Finding struct {
	ID              int64
	Fingerprint     string
	FindingType     string
	Severity        string
	Risk            string
	UserID          *int64
	SubjectType     string
	SubjectID       string
	Details         map[string]any
	Status          string
	FirstSeenRunID  int64
	LastSeenRunID   int64
	OccurrenceCount int64
	FirstSeenAt     time.Time
	LastSeenAt      time.Time
	ResolvedAt      *time.Time
}

// findingDraft is one classified drift before persistence.
type findingDraft struct {
	FindingType string
	Severity    string
	Risk        string
	UserID      int64
	SubjectType string
	SubjectID   string
	Details     map[string]any
}

// Store persists reconciliation runs and findings with raw pgx queries,
// mirroring the credits reconciliation store. Queries take context first;
// dbWithContext resolves the pool for one call (the scan itself never
// holds a transaction across the Auth HTTP fan-out).
type Store struct {
	db *pgxpool.Pool
}

// NewStore builds the store. A nil pool fails every call with the invalid
// command error instead of panicking.
func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) dbWithContext(_ context.Context) *pgxpool.Pool {
	if s == nil {
		return nil
	}
	return s.db
}

// startRunTx creates (or reuses) a run inside the caller's transaction so
// run creation and execute-job enqueue commit atomically.
func (s *Store) startRunTx(ctx context.Context, tx pgx.Tx, trigger string, requestedBy *int64, reason *string, scheduledFor *time.Time, onlyIfOverdue bool) (*int64, bool, error) {
	if tx == nil {
		return nil, false, errInvalidReconciliationCommand
	}
	var runID *int64
	var enqueueRequired bool
	if err := tx.QueryRow(ctx, `SELECT run_id, enqueue_required FROM start_identity_admin_reconciliation($1, $2, $3, $4, $5)`,
		trigger, requestedBy, reason, scheduledFor, onlyIfOverdue,
	).Scan(&runID, &enqueueRequired); err != nil {
		return nil, false, fmt.Errorf("identity admin reconciliation: create run: %w", err)
	}
	return runID, enqueueRequired, nil
}

// getRun loads one run by ID.
func (s *Store) getRun(ctx context.Context, runID int64) (Run, error) {
	pool := s.dbWithContext(ctx)
	if pool == nil || runID <= 0 {
		return Run{}, errInvalidReconciliationCommand
	}
	row := pool.QueryRow(ctx, `
SELECT id, status, trigger_type, requested_by, reason, scheduled_for, finding_count,
       attempt_count, error_message, requested_at, started_at, completed_at
FROM identity_admin_reconciliation_runs WHERE id = $1`, runID)
	run, err := scanRun(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrRunNotFound
	}
	if err != nil {
		return Run{}, fmt.Errorf("identity admin reconciliation: get run: %w", err)
	}
	return run, nil
}

// markRunning flips an unfinished run to running and counts the attempt.
// Completed runs are never re-entered here; ExecuteReconciliation returns
// early for them so a finished run stays immutable.
func (s *Store) markRunning(ctx context.Context, runID int64) error {
	pool := s.dbWithContext(ctx)
	if pool == nil || runID <= 0 {
		return errInvalidReconciliationCommand
	}
	var id int64
	err := pool.QueryRow(ctx, `
UPDATE identity_admin_reconciliation_runs
SET status = 'running', started_at = clock_timestamp(), completed_at = NULL,
    error_message = '', attempt_count = attempt_count + 1
WHERE id = $1 AND status IN ('queued', 'running', 'failed')
RETURNING id`, runID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRunNotExecutable
	}
	if err != nil {
		return fmt.Errorf("identity admin reconciliation: mark running: %w", err)
	}
	return nil
}

// persistScan upserts every finding (deduplicated by fingerprint), links
// them to the run, resolves findings the full scan no longer observes, and
// completes the run, all in one transaction. Re-running the same run
// re-upserts the same fingerprints and re-links the same pairs
// (ON CONFLICT DO NOTHING), so findings never duplicate.
func (s *Store) persistScan(ctx context.Context, runID int64, drafts []findingDraft) error {
	pool := s.dbWithContext(ctx)
	if pool == nil || runID <= 0 {
		return errInvalidReconciliationCommand
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("identity admin reconciliation: begin persist: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	for _, draft := range drafts {
		details, err := json.Marshal(draft.Details)
		if err != nil {
			return fmt.Errorf("identity admin reconciliation: encode finding details: %w", err)
		}
		var findingID int64
		err = tx.QueryRow(ctx, `
INSERT INTO identity_admin_reconciliation_findings (
  fingerprint, finding_type, severity, risk, user_id, subject_type, subject_id, details,
  first_seen_run_id, last_seen_run_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
ON CONFLICT (fingerprint) DO UPDATE SET
  severity = EXCLUDED.severity, risk = EXCLUDED.risk, user_id = EXCLUDED.user_id,
  details = EXCLUDED.details, status = 'open', last_seen_run_id = EXCLUDED.last_seen_run_id,
  last_seen_at = clock_timestamp(), occurrence_count = identity_admin_reconciliation_findings.occurrence_count + 1,
  resolved_at = NULL
RETURNING id`,
			fingerprintForDraft(draft), draft.FindingType, draft.Severity, draft.Risk,
			draft.UserID, draft.SubjectType, draft.SubjectID, details, runID,
		).Scan(&findingID)
		if err != nil {
			return fmt.Errorf("identity admin reconciliation: upsert finding: %w", err)
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO identity_admin_reconciliation_run_findings (
  run_id, finding_id, observed_severity, observed_risk, observed_details, observed_status,
  observed_occurrence_count, observed_first_seen_at, observed_last_seen_at, observed_resolved_at
) SELECT $1, findings.id, findings.severity, findings.risk, findings.details, findings.status,
    findings.occurrence_count, findings.first_seen_at, findings.last_seen_at, findings.resolved_at
  FROM identity_admin_reconciliation_findings findings WHERE findings.id = $2
ON CONFLICT DO NOTHING`, runID, findingID); err != nil {
			return fmt.Errorf("identity admin reconciliation: link run finding: %w", err)
		}
	}
	// Every run is a full scan, so an open finding this run did not observe
	// is converged drift (or a deleted local user): resolve it.
	if _, err := tx.Exec(ctx, `
UPDATE identity_admin_reconciliation_findings
SET status = 'resolved', resolved_at = clock_timestamp()
WHERE status = 'open' AND last_seen_run_id <> $1`, runID); err != nil {
		return fmt.Errorf("identity admin reconciliation: resolve stale findings: %w", err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE identity_admin_reconciliation_runs
SET status = 'completed', completed_at = clock_timestamp(),
    finding_count = (SELECT COUNT(*) FROM identity_admin_reconciliation_run_findings WHERE run_id = $1)
WHERE id = $1`, runID); err != nil {
		return fmt.Errorf("identity admin reconciliation: complete run: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("identity admin reconciliation: commit scan: %w", err)
	}
	return nil
}

// failRun marks a run failed after the River job exhausts its attempts.
func (s *Store) failRun(ctx context.Context, runID int64, cause error) error {
	pool := s.dbWithContext(ctx)
	if pool == nil || runID <= 0 || cause == nil {
		return errInvalidReconciliationCommand
	}
	if _, err := pool.Exec(ctx, `SELECT fail_identity_admin_reconciliation($1, $2)`, runID, cause.Error()); err != nil {
		return fmt.Errorf("identity admin reconciliation: mark run failed: %w", err)
	}
	return nil
}

// ListOpenFindings returns every unresolved finding, newest first.
func (s *Store) ListOpenFindings(ctx context.Context) ([]Finding, error) {
	pool := s.dbWithContext(ctx)
	if pool == nil {
		return nil, errInvalidReconciliationCommand
	}
	rows, err := pool.Query(ctx, `
SELECT id, fingerprint, finding_type, severity, risk, user_id, subject_type, subject_id,
       details, status, first_seen_run_id, last_seen_run_id, occurrence_count,
       first_seen_at, last_seen_at, resolved_at
FROM identity_admin_reconciliation_findings
WHERE status = 'open' ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("identity admin reconciliation: list open findings: %w", err)
	}
	defer rows.Close()
	return scanFindings(rows)
}

// ListRunFindings returns the findings observed by one run (run-id
// isolation: re-running the same run never grows this set).
func (s *Store) ListRunFindings(ctx context.Context, runID int64) ([]Finding, error) {
	pool := s.dbWithContext(ctx)
	if pool == nil || runID <= 0 {
		return nil, errInvalidReconciliationCommand
	}
	rows, err := pool.Query(ctx, `
SELECT findings.id, findings.fingerprint, findings.finding_type, findings.severity, findings.risk,
       findings.user_id, findings.subject_type, findings.subject_id, findings.details, findings.status,
       findings.first_seen_run_id, findings.last_seen_run_id, findings.occurrence_count,
       findings.first_seen_at, findings.last_seen_at, findings.resolved_at
FROM identity_admin_reconciliation_findings findings
JOIN identity_admin_reconciliation_run_findings links
  ON links.finding_id = findings.id AND links.run_id = $1
ORDER BY findings.id DESC`, runID)
	if err != nil {
		return nil, fmt.Errorf("identity admin reconciliation: list run findings: %w", err)
	}
	defer rows.Close()
	return scanFindings(rows)
}

// CountFindings returns the total finding rows (idempotency assertion seam:
// re-runs must not grow this count for unchanged drift).
func (s *Store) CountFindings(ctx context.Context) (int64, error) {
	pool := s.dbWithContext(ctx)
	if pool == nil {
		return 0, errInvalidReconciliationCommand
	}
	var count int64
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM identity_admin_reconciliation_findings`).Scan(&count); err != nil {
		return 0, fmt.Errorf("identity admin reconciliation: count findings: %w", err)
	}
	return count, nil
}

type findingRows interface {
	Next() bool
	Scan(...any) error
	Err() error
}

func scanRun(scanner interface{ Scan(...any) error }) (Run, error) {
	var run Run
	var reason *string
	if err := scanner.Scan(
		&run.ID,
		&run.Status,
		&run.Trigger,
		&run.RequestedBy,
		&reason,
		&run.ScheduledFor,
		&run.FindingCount,
		&run.AttemptCount,
		&run.ErrorMessage,
		&run.RequestedAt,
		&run.StartedAt,
		&run.CompletedAt,
	); err != nil {
		return Run{}, err
	}
	if reason != nil {
		run.Reason = *reason
	}
	return run, nil
}

func scanFindings(rows findingRows) ([]Finding, error) {
	findings := make([]Finding, 0)
	for rows.Next() {
		var finding Finding
		var details []byte
		if err := rows.Scan(
			&finding.ID, &finding.Fingerprint, &finding.FindingType, &finding.Severity,
			&finding.Risk, &finding.UserID, &finding.SubjectType, &finding.SubjectID,
			&details, &finding.Status, &finding.FirstSeenRunID, &finding.LastSeenRunID,
			&finding.OccurrenceCount, &finding.FirstSeenAt, &finding.LastSeenAt, &finding.ResolvedAt,
		); err != nil {
			return nil, fmt.Errorf("identity admin reconciliation: scan finding: %w", err)
		}
		finding.Details = make(map[string]any)
		if len(details) > 0 {
			if err := json.Unmarshal(details, &finding.Details); err != nil {
				return nil, fmt.Errorf("identity admin reconciliation: decode finding details: %w", err)
			}
		}
		findings = append(findings, finding)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("identity admin reconciliation: read findings: %w", err)
	}
	return findings, nil
}
