package credits

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"ideaboard/api/internal/platform/adminaction"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

const ReconciliationQueue = "credit_reconciliation"

const (
	reconciliationTriggerLookupTimeout = 2 * time.Second
	reconciliationFinalFailureTimeout  = 5 * time.Second
)

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
	Trigger             ReconciliationTrigger   `json:"trigger"`
	RequestedBy         *int64                  `json:"requested_by,omitempty"`
	Reason              string                  `json:"reason,omitempty"`
	ScheduledFor        *time.Time              `json:"scheduled_for,omitempty"`
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
	FindingType     ReconciliationFindingType   `json:"finding_type"`
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
type ReconciliationTrigger string
type ReconciliationFindingStatus string
type ReconciliationFindingType string
type ReconciliationSeverity string
type ReconciliationRisk string

const (
	ReconciliationTriggerManual                         ReconciliationTrigger       = "manual"
	ReconciliationTriggerScheduled                      ReconciliationTrigger       = "scheduled"
	ReconciliationRunQueued                             ReconciliationRunStatus     = "queued"
	ReconciliationRunRunning                            ReconciliationRunStatus     = "running"
	ReconciliationRunCompleted                          ReconciliationRunStatus     = "completed"
	ReconciliationRunFailed                             ReconciliationRunStatus     = "failed"
	ReconciliationFindingOpen                           ReconciliationFindingStatus = "open"
	ReconciliationFindingResolved                       ReconciliationFindingStatus = "resolved"
	ReconciliationFindingLedgerPostingsInvalid          ReconciliationFindingType   = "ledger_postings_invalid"
	ReconciliationFindingCommandShapeInvalid            ReconciliationFindingType   = "command_shape_invalid"
	ReconciliationFindingWalletProjectionMismatch       ReconciliationFindingType   = "wallet_projection_mismatch"
	ReconciliationFindingStaleHold                      ReconciliationFindingType   = "stale_hold"
	ReconciliationFindingTaskHoldMissingTask            ReconciliationFindingType   = "task_hold_missing_task"
	ReconciliationFindingTaskBillingFactMissing         ReconciliationFindingType   = "task_billing_fact_missing"
	ReconciliationFindingActiveTaskNotHeld              ReconciliationFindingType   = "active_task_not_held"
	ReconciliationFindingCompletedTaskNotCaptured       ReconciliationFindingType   = "completed_task_not_captured"
	ReconciliationFindingFailedTaskNotReleased          ReconciliationFindingType   = "failed_task_not_released"
	ReconciliationFindingTaskTerminalTransactionCount   ReconciliationFindingType   = "task_terminal_transaction_count"
	ReconciliationFindingIdempotencyRequestHashConflict ReconciliationFindingType   = "idempotency_request_hash_conflict"
	ReconciliationFindingTaskSettlementReferenceInvalid ReconciliationFindingType   = "task_settlement_reference_invalid"
	ReconciliationFindingDuplicateBusinessReference     ReconciliationFindingType   = "duplicate_business_reference"
	ReconciliationFindingCorrectionExceedsOriginal      ReconciliationFindingType   = "correction_amount_exceeds_original"
	ReconciliationFindingRefundAmountInvalid            ReconciliationFindingType   = "refund_amount_invalid"
	ReconciliationFindingIllegalReferenceChain          ReconciliationFindingType   = "illegal_reference_chain"
	ReconciliationFindingHoldStateInvalid               ReconciliationFindingType   = "hold_state_invalid"
	ReconciliationFindingHoldTerminalTransactionCount   ReconciliationFindingType   = "hold_terminal_transaction_count"
	ReconciliationFindingChargeStateInvalid             ReconciliationFindingType   = "charge_state_invalid"
	ReconciliationSeverityWarning                       ReconciliationSeverity      = "warning"
	ReconciliationSeverityError                         ReconciliationSeverity      = "error"
	ReconciliationSeverityCritical                      ReconciliationSeverity      = "critical"
	ReconciliationRiskLow                               ReconciliationRisk          = "low"
	ReconciliationRiskHigh                              ReconciliationRisk          = "high"
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

type StartManualReconciliationInput struct {
	Action adminaction.Metadata
}

type startReconciliationInput struct {
	trigger       ReconciliationTrigger
	action        adminaction.Metadata
	scheduledFor  *time.Time
	onlyIfOverdue bool
}

func (s *Service) StartReconciliation(
	ctx context.Context,
	input StartManualReconciliationInput,
) (ReconciliationRun, error) {
	run, err := s.startReconciliation(ctx, startReconciliationInput{
		trigger: ReconciliationTriggerManual,
		action:  input.Action,
	})
	if err != nil {
		return ReconciliationRun{}, err
	}
	if run == nil {
		return ReconciliationRun{}, ErrInvalidCommand
	}
	return *run, nil
}

func (s *Service) ScheduleReconciliation(
	ctx context.Context,
	scheduledFor time.Time,
	onlyIfOverdue bool,
) (*ReconciliationRun, error) {
	return s.startReconciliation(ctx, startReconciliationInput{
		trigger:       ReconciliationTriggerScheduled,
		scheduledFor:  &scheduledFor,
		onlyIfOverdue: onlyIfOverdue,
	})
}

func (s *Service) startReconciliation(
	ctx context.Context,
	input startReconciliationInput,
) (*ReconciliationRun, error) {
	if s == nil || s.pool == nil || s.reconciliationEnqueuer == nil {
		return nil, ErrReconciliationUnavailable
	}
	var requestedBy any
	var reason any
	var scheduledFor any
	switch input.trigger {
	case ReconciliationTriggerManual:
		if !input.action.IsValid() {
			return nil, ErrInvalidCommand
		}
		requestedBy = input.action.ActorID()
		reason = input.action.Reason()
	case ReconciliationTriggerScheduled:
		if input.scheduledFor == nil || input.scheduledFor.IsZero() {
			return nil, ErrInvalidCommand
		}
		normalized := input.scheduledFor.UTC().Truncate(time.Second)
		scheduledFor = normalized
	default:
		return nil, ErrInvalidCommand
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("credits reconciliation: begin: %w", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
		defer cancel()
		if rollbackErr := tx.Rollback(rollbackCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			slog.Warn("failed to roll back Credits reconciliation transaction", "err", rollbackErr)
		}
	}()
	var runID *int64
	var enqueueRequired bool
	if err := tx.QueryRow(ctx, `SELECT run_id, enqueue_required FROM start_credit_reconciliation($1, $2, $3, $4, $5)`,
		string(input.trigger), requestedBy, reason, scheduledFor, input.onlyIfOverdue,
	).Scan(&runID, &enqueueRequired); err != nil {
		return nil, fmt.Errorf("credits reconciliation: create run: %w", err)
	}
	if enqueueRequired {
		if runID == nil || *runID <= 0 {
			return nil, fmt.Errorf("credits reconciliation: enqueue-required run has invalid ID")
		}
		if _, err := s.reconciliationEnqueuer.InsertTx(ctx, tx, ReconciliationJobArgs{RunID: *runID}, &river.InsertOpts{
			Queue: ReconciliationQueue, MaxAttempts: 5, Priority: 4,
		}); err != nil {
			return nil, fmt.Errorf("credits reconciliation: enqueue run: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("credits reconciliation: commit: %w", err)
	}
	if runID == nil {
		return nil, nil
	}
	run, err := s.GetReconciliationRun(ctx, *runID)
	if err != nil {
		return nil, err
	}
	return &run, nil
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
	row := s.pool.QueryRow(ctx, `
SELECT id, status, trigger_type, requested_by, reason, scheduled_for, EXTRACT(EPOCH FROM stale_hold_after), finding_count, critical_count,
       isolated_wallet_count, attempt_count, error_message, requested_at, started_at, completed_at
FROM credit_reconciliation_runs WHERE id = $1`, runID)
	run, err := scanReconciliationRun(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReconciliationRun{}, ErrReconciliationNotFound
	}
	if err != nil {
		return ReconciliationRun{}, fmt.Errorf("credits reconciliation: get run: %w", err)
	}
	return run, nil
}

func (s *Service) ListReconciliationRuns(ctx context.Context, input ListReconciliationInput) (ReconciliationRunPage, error) {
	limit, err := normalizeReconciliationLimit(input.Limit)
	if err != nil || input.BeforeID < 0 || s == nil || s.pool == nil {
		return ReconciliationRunPage{}, ErrInvalidCommand
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, status, trigger_type, requested_by, reason, scheduled_for, EXTRACT(EPOCH FROM stale_hold_after), finding_count, critical_count,
       isolated_wallet_count, attempt_count, error_message, requested_at, started_at, completed_at
FROM credit_reconciliation_runs WHERE ($1::BIGINT = 0 OR id < $1) ORDER BY id DESC LIMIT $2`, input.BeforeID, limit+1)
	if err != nil {
		return ReconciliationRunPage{}, fmt.Errorf("credits reconciliation: list runs: %w", err)
	}
	defer rows.Close()
	page := ReconciliationRunPage{Runs: make([]ReconciliationRun, 0, limit)}
	for rows.Next() {
		run, err := scanReconciliationRun(rows)
		if err != nil {
			return ReconciliationRunPage{}, fmt.Errorf("credits reconciliation: scan run: %w", err)
		}
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

type reconciliationRunScanner interface {
	Scan(...any) error
}

func scanReconciliationRun(scanner reconciliationRunScanner) (ReconciliationRun, error) {
	var run ReconciliationRun
	var reason *string
	var staleSeconds float64
	if err := scanner.Scan(
		&run.ID,
		&run.Status,
		&run.Trigger,
		&run.RequestedBy,
		&reason,
		&run.ScheduledFor,
		&staleSeconds,
		&run.FindingCount,
		&run.CriticalCount,
		&run.IsolatedWalletCount,
		&run.AttemptCount,
		&run.ErrorMessage,
		&run.RequestedAt,
		&run.StartedAt,
		&run.CompletedAt,
	); err != nil {
		return ReconciliationRun{}, err
	}
	if reason != nil {
		run.Reason = *reason
	}
	run.StaleHoldAfterSecs = int64(staleSeconds)
	return run, nil
}

func (s *Service) ListGlobalReconciliationFindings(ctx context.Context, status ReconciliationFindingStatus, input ListReconciliationInput) (ReconciliationFindingPage, error) {
	limit, err := normalizeReconciliationLimit(input.Limit)
	if err != nil || input.BeforeID < 0 || (status != ReconciliationFindingOpen && status != ReconciliationFindingResolved) || s == nil || s.pool == nil {
		return ReconciliationFindingPage{}, ErrInvalidCommand
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, fingerprint, finding_type, severity, risk, wallet_user_id, subject_type, subject_id, details, status,
       isolated, occurrence_count, first_seen_at, last_seen_at, resolved_at
FROM credit_reconciliation_findings
WHERE status = $1 AND ($2::BIGINT = 0 OR id < $2)
ORDER BY id DESC LIMIT $3`, string(status), input.BeforeID, limit+1)
	if err != nil {
		return ReconciliationFindingPage{}, fmt.Errorf("credits reconciliation: list global findings: %w", err)
	}
	defer rows.Close()
	return scanReconciliationFindingPage(rows, limit)
}

type reconciliationFindingRows interface {
	Next() bool
	Scan(...any) error
	Err() error
}

func scanReconciliationFindingPage(rows reconciliationFindingRows, limit int) (ReconciliationFindingPage, error) {
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
	GetReconciliationRun(context.Context, int64) (ReconciliationRun, error)
}

func NewReconciliationWorker(service reconciliationWorkerService) *ReconciliationWorker {
	return &ReconciliationWorker{service: service}
}

func (w *ReconciliationWorker) Work(ctx context.Context, job *river.Job[ReconciliationJobArgs]) error {
	if w == nil || w.service == nil || job == nil || job.Args.RunID <= 0 {
		return ErrInvalidCommand
	}
	err := w.service.ExecuteReconciliation(ctx, job.Args.RunID)
	if err == nil {
		return nil
	}
	lookupCtx, cancelLookup := context.WithTimeout(context.WithoutCancel(ctx), reconciliationTriggerLookupTimeout)
	run, runErr := w.service.GetReconciliationRun(lookupCtx, job.Args.RunID)
	cancelLookup()
	if runErr == nil && run.Trigger == ReconciliationTriggerScheduled {
		slog.Error("scheduled Credit reconciliation execution failed",
			"run_id", job.Args.RunID,
			"attempt", job.Attempt,
			"max_attempts", job.MaxAttempts,
			"err", err,
		)
	} else if runErr != nil {
		slog.Error("Credit reconciliation trigger lookup failed",
			"run_id", job.Args.RunID,
			"attempt", job.Attempt,
			"max_attempts", job.MaxAttempts,
			"err", err,
			"trigger_lookup_err", runErr,
		)
	}
	if job.Attempt < job.MaxAttempts {
		return err
	}
	failCtx, cancelFail := context.WithTimeout(context.WithoutCancel(ctx), reconciliationFinalFailureTimeout)
	defer cancelFail()
	if failErr := w.service.FailReconciliation(failCtx, job.Args.RunID, err); failErr != nil {
		slog.Error("Credit reconciliation final failure persistence failed",
			"run_id", job.Args.RunID,
			"attempt", job.Attempt,
			"max_attempts", job.MaxAttempts,
			"err", err,
			"mark_failed_err", failErr,
		)
		return errors.Join(err, failErr)
	}
	return nil
}
