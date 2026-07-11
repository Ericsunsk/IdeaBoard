package credits

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

const ReconciliationQueue = "credit_reconciliation"

const reconciliationFinalFailureTimeout = 5 * time.Second

var (
	ErrReconciliationUnavailable = errors.New("credit reconciliation unavailable")
	ErrReconciliationNotFound    = errors.New("credit reconciliation run not found")
)

type ReconciliationJobArgs struct {
	RunID int64 `json:"run_id"`
}

func (ReconciliationJobArgs) Kind() string { return "credit_reconciliation" }

type ReconciliationEnqueuer interface {
	InsertTx(context.Context, pgx.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

type ReconciliationRun struct {
	ID                  int64                   `json:"id"`
	Status              ReconciliationRunStatus `json:"status"`
	RequestedBy         int64                   `json:"requested_by"`
	StaleHoldAfterSecs  int64                   `json:"stale_hold_after_seconds"`
	FindingCount        int64                   `json:"finding_count"`
	CriticalCount       int64                   `json:"critical_count"`
	IsolatedWalletCount int64                   `json:"isolated_wallet_count"`
	AttemptCount        int                     `json:"attempt_count"`
	ErrorMessage        string                  `json:"error_message,omitempty"`
	RequestedAt         time.Time               `json:"requested_at"`
	StartedAt           *time.Time              `json:"started_at,omitempty"`
	CompletedAt         *time.Time              `json:"completed_at,omitempty"`
}

type ReconciliationFinding struct {
	ID              int64                       `json:"id"`
	Fingerprint     string                      `json:"fingerprint"`
	FindingType     string                      `json:"finding_type"`
	Severity        ReconciliationSeverity      `json:"severity"`
	Risk            ReconciliationRisk          `json:"risk"`
	WalletUserID    *int64                      `json:"wallet_user_id,omitempty"`
	SubjectType     string                      `json:"subject_type"`
	SubjectID       string                      `json:"subject_id"`
	Details         map[string]any              `json:"details"`
	Status          ReconciliationFindingStatus `json:"status"`
	Isolated        bool                        `json:"isolated"`
	OccurrenceCount int64                       `json:"occurrence_count"`
	FirstSeenAt     time.Time                   `json:"first_seen_at"`
	LastSeenAt      time.Time                   `json:"last_seen_at"`
	ResolvedAt      *time.Time                  `json:"resolved_at,omitempty"`
}

type ReconciliationRunStatus string
type ReconciliationFindingStatus string
type ReconciliationSeverity string
type ReconciliationRisk string

const (
	ReconciliationRunQueued        ReconciliationRunStatus     = "queued"
	ReconciliationRunRunning       ReconciliationRunStatus     = "running"
	ReconciliationRunCompleted     ReconciliationRunStatus     = "completed"
	ReconciliationRunFailed        ReconciliationRunStatus     = "failed"
	ReconciliationFindingOpen      ReconciliationFindingStatus = "open"
	ReconciliationFindingResolved  ReconciliationFindingStatus = "resolved"
	ReconciliationSeverityWarning  ReconciliationSeverity      = "warning"
	ReconciliationSeverityError    ReconciliationSeverity      = "error"
	ReconciliationSeverityCritical ReconciliationSeverity      = "critical"
	ReconciliationRiskLow          ReconciliationRisk          = "low"
	ReconciliationRiskHigh         ReconciliationRisk          = "high"
)

type ReconciliationRunPage struct {
	Runs         []ReconciliationRun `json:"runs"`
	NextBeforeID int64               `json:"next_before_id,omitempty"`
}

type ReconciliationFindingPage struct {
	Findings     []ReconciliationFinding `json:"findings"`
	NextBeforeID int64                   `json:"next_before_id,omitempty"`
}

type ListReconciliationInput struct {
	BeforeID int64
	Limit    int
}

func (s *Service) StartReconciliation(ctx context.Context, adminID int64, staleHoldAfter time.Duration) (ReconciliationRun, error) {
	if s == nil || s.pool == nil || s.reconciliationEnqueuer == nil {
		return ReconciliationRun{}, ErrReconciliationUnavailable
	}
	if adminID <= 0 || staleHoldAfter < time.Minute || staleHoldAfter > 365*24*time.Hour {
		return ReconciliationRun{}, ErrInvalidCommand
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ReconciliationRun{}, fmt.Errorf("credits reconciliation: begin: %w", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
		defer cancel()
		if rollbackErr := tx.Rollback(rollbackCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			slog.Warn("failed to roll back Credits reconciliation transaction", "err", rollbackErr)
		}
	}()
	var runID int64
	if err := tx.QueryRow(ctx, `SELECT start_credit_reconciliation($1, $2)`, adminID, int64(staleHoldAfter/time.Second)).Scan(&runID); err != nil {
		return ReconciliationRun{}, fmt.Errorf("credits reconciliation: create run: %w", err)
	}
	if _, err := s.reconciliationEnqueuer.InsertTx(ctx, tx, ReconciliationJobArgs{RunID: runID}, &river.InsertOpts{
		Queue: ReconciliationQueue, MaxAttempts: 5, Priority: 4,
		UniqueOpts: river.UniqueOpts{ByArgs: true},
	}); err != nil {
		return ReconciliationRun{}, fmt.Errorf("credits reconciliation: enqueue run: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ReconciliationRun{}, fmt.Errorf("credits reconciliation: commit: %w", err)
	}
	return s.GetReconciliationRun(ctx, runID)
}

func (s *Service) ExecuteReconciliation(ctx context.Context, runID int64) error {
	if s == nil || s.pool == nil || runID <= 0 {
		return ErrInvalidCommand
	}
	if _, err := s.pool.Exec(ctx, `SELECT execute_credit_reconciliation($1)`, runID); err != nil {
		return fmt.Errorf("credits reconciliation: execute: %w", err)
	}
	return nil
}

func (s *Service) FailReconciliation(ctx context.Context, runID int64, cause error) error {
	if s == nil || s.pool == nil || runID <= 0 || cause == nil {
		return ErrInvalidCommand
	}
	_, err := s.pool.Exec(ctx, `SELECT fail_credit_reconciliation($1, $2)`, runID, cause.Error())
	if err != nil {
		return fmt.Errorf("credits reconciliation: mark run failed: %w", err)
	}
	return nil
}

func (s *Service) GetReconciliationRun(ctx context.Context, runID int64) (ReconciliationRun, error) {
	if s == nil || s.pool == nil || runID <= 0 {
		return ReconciliationRun{}, ErrInvalidCommand
	}
	var run ReconciliationRun
	var staleSeconds float64
	err := s.pool.QueryRow(ctx, `
SELECT id, status, requested_by, EXTRACT(EPOCH FROM stale_hold_after), finding_count, critical_count,
       isolated_wallet_count, attempt_count, error_message, requested_at, started_at, completed_at
FROM credit_reconciliation_runs WHERE id = $1`, runID).Scan(
		&run.ID, &run.Status, &run.RequestedBy, &staleSeconds, &run.FindingCount, &run.CriticalCount,
		&run.IsolatedWalletCount, &run.AttemptCount, &run.ErrorMessage, &run.RequestedAt, &run.StartedAt, &run.CompletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReconciliationRun{}, ErrReconciliationNotFound
	}
	if err != nil {
		return ReconciliationRun{}, fmt.Errorf("credits reconciliation: get run: %w", err)
	}
	run.StaleHoldAfterSecs = int64(staleSeconds)
	return run, nil
}

func (s *Service) ListReconciliationRuns(ctx context.Context, input ListReconciliationInput) (ReconciliationRunPage, error) {
	limit, err := normalizeReconciliationLimit(input.Limit)
	if err != nil || input.BeforeID < 0 || s == nil || s.pool == nil {
		return ReconciliationRunPage{}, ErrInvalidCommand
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, status, requested_by, EXTRACT(EPOCH FROM stale_hold_after), finding_count, critical_count,
       isolated_wallet_count, attempt_count, error_message, requested_at, started_at, completed_at
FROM credit_reconciliation_runs WHERE ($1::BIGINT = 0 OR id < $1) ORDER BY id DESC LIMIT $2`, input.BeforeID, limit+1)
	if err != nil {
		return ReconciliationRunPage{}, fmt.Errorf("credits reconciliation: list runs: %w", err)
	}
	defer rows.Close()
	page := ReconciliationRunPage{Runs: make([]ReconciliationRun, 0, limit)}
	for rows.Next() {
		var run ReconciliationRun
		var staleSeconds float64
		if err := rows.Scan(&run.ID, &run.Status, &run.RequestedBy, &staleSeconds, &run.FindingCount, &run.CriticalCount,
			&run.IsolatedWalletCount, &run.AttemptCount, &run.ErrorMessage, &run.RequestedAt, &run.StartedAt, &run.CompletedAt); err != nil {
			return ReconciliationRunPage{}, fmt.Errorf("credits reconciliation: scan run: %w", err)
		}
		run.StaleHoldAfterSecs = int64(staleSeconds)
		page.Runs = append(page.Runs, run)
	}
	if err := rows.Err(); err != nil {
		return ReconciliationRunPage{}, fmt.Errorf("credits reconciliation: read runs: %w", err)
	}
	if len(page.Runs) > limit {
		page.NextBeforeID = page.Runs[limit-1].ID
		page.Runs = page.Runs[:limit]
	}
	return page, nil
}

func (s *Service) ListReconciliationFindings(ctx context.Context, runID int64, status ReconciliationFindingStatus, input ListReconciliationInput) (ReconciliationFindingPage, error) {
	limit, err := normalizeReconciliationLimit(input.Limit)
	if err != nil || runID <= 0 || input.BeforeID < 0 || (status != "" && status != ReconciliationFindingOpen && status != ReconciliationFindingResolved) || s == nil || s.pool == nil {
		return ReconciliationFindingPage{}, ErrInvalidCommand
	}
	rows, err := s.pool.Query(ctx, `
SELECT findings.id, findings.fingerprint, findings.finding_type, links.observed_severity, links.observed_risk,
       findings.wallet_user_id, findings.subject_type, findings.subject_id, links.observed_details, links.observed_status,
       links.observed_isolated, links.observed_occurrence_count, links.observed_first_seen_at, links.observed_last_seen_at, links.observed_resolved_at
FROM credit_reconciliation_findings findings
JOIN credit_reconciliation_run_findings links ON links.finding_id = findings.id
WHERE links.run_id = $1 AND ($2 = '' OR links.observed_status = $2) AND ($3::BIGINT = 0 OR findings.id < $3)
ORDER BY findings.id DESC LIMIT $4`, runID, string(status), input.BeforeID, limit+1)
	if err != nil {
		return ReconciliationFindingPage{}, fmt.Errorf("credits reconciliation: list findings: %w", err)
	}
	defer rows.Close()
	page := ReconciliationFindingPage{Findings: make([]ReconciliationFinding, 0, limit)}
	for rows.Next() {
		var finding ReconciliationFinding
		if err := rows.Scan(&finding.ID, &finding.Fingerprint, &finding.FindingType, &finding.Severity, &finding.Risk,
			&finding.WalletUserID, &finding.SubjectType, &finding.SubjectID, &finding.Details, &finding.Status,
			&finding.Isolated, &finding.OccurrenceCount, &finding.FirstSeenAt, &finding.LastSeenAt, &finding.ResolvedAt); err != nil {
			return ReconciliationFindingPage{}, fmt.Errorf("credits reconciliation: scan finding: %w", err)
		}
		page.Findings = append(page.Findings, finding)
	}
	if err := rows.Err(); err != nil {
		return ReconciliationFindingPage{}, fmt.Errorf("credits reconciliation: read findings: %w", err)
	}
	if len(page.Findings) > limit {
		page.NextBeforeID = page.Findings[limit-1].ID
		page.Findings = page.Findings[:limit]
	}
	return page, nil
}

func normalizeReconciliationLimit(limit int) (int, error) {
	if limit == 0 {
		return 50, nil
	}
	if limit < 1 || limit > 200 {
		return 0, ErrInvalidCommand
	}
	return limit, nil
}

type ReconciliationWorker struct {
	river.WorkerDefaults[ReconciliationJobArgs]
	service reconciliationWorkerService
}

type reconciliationWorkerService interface {
	ExecuteReconciliation(context.Context, int64) error
	FailReconciliation(context.Context, int64, error) error
}

func NewReconciliationWorker(service reconciliationWorkerService) *ReconciliationWorker {
	return &ReconciliationWorker{service: service}
}

func (w *ReconciliationWorker) Work(ctx context.Context, job *river.Job[ReconciliationJobArgs]) error {
	if w == nil || w.service == nil || job == nil || job.Args.RunID <= 0 {
		return ErrInvalidCommand
	}
	err := w.service.ExecuteReconciliation(ctx, job.Args.RunID)
	if err != nil && job.Attempt >= job.MaxAttempts {
		failCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), reconciliationFinalFailureTimeout)
		defer cancel()
		if failErr := w.service.FailReconciliation(failCtx, job.Args.RunID, err); failErr != nil {
			return errors.Join(err, failErr)
		}
		return nil
	}
	return err
}
