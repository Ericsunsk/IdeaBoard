package tasks

import (
	"context"
	"time"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/generation/routing"
	tasks "ideaboard/api/internal/tasking/domain"

	"github.com/jackc/pgx/v5"
)

// RoutingSnapshotSaver persists the immutable submit-time RoutingSnapshot.
// Production joins the settlement transaction (tx non-nil) via
// catalog.SaveRoutingSnapshot so the snapshot commits atomically with the
// Credit Hold, task insert, and River enqueue. Tests stub it to capture the
// pinned fact. A nil saver is only valid for test fixtures; production wiring
// always supplies the saver and the worker fails closed on a missing snapshot.
type RoutingSnapshotSaver func(ctx context.Context, tx pgx.Tx, taskID string, snap routing.RoutingSnapshot) error

// Quoter mints immutable server-side price snapshots. Production adapts
// the Billing catalog; tests stub this interface.
type Quoter interface {
	Quote(ctx context.Context, service, resolution string, quantity int64) (tasks.BillingSnapshot, error)
	QuoteInTx(ctx context.Context, tx pgx.Tx, service, resolution string, quantity int64) (tasks.BillingSnapshot, error)
}

// AssetReader checks byte-verified readiness. Production adapts the
// workspace media asset store; tests stub it.
type AssetReader interface {
	GetAsset(ctx context.Context, workspaceID, assetID string) (*tasks.AssetInfo, error)
}

// BoardAuthorizer resolves one caller's Board posture without touching
// Boards storage internals or HTTP handlers. Production adapts the Boards
// WorkspaceBoardStore; tests stub this interface. A nil authorizer fails
// closed: Board-bound submits are rejected and Board-bound reads are
// invisible.
type BoardAuthorizer interface {
	ResolveBoardAccess(ctx context.Context, workspacePublicID, boardPublicID string, userID int64, workspaceRole string) (*tasks.BoardAccess, error)
}

// AttemptRecorder is the durable attempt-fact seam. The postgres store
// implements it over task_execution_attempts; the in-memory sibling
// is the test backend. Repository methods take context.Context
// first and use the dbWithContext pattern in the postgres sibling.
type AttemptRecorder interface {
	StartAttempt(ctx context.Context, start tasks.AttemptStart) (int32, error)
	FinishAttempt(ctx context.Context, finish tasks.AttemptFinish) (int64, error)
	// HasModelFallbackAttempt reports whether any model_fallback row
	// exists for the task, in any status. True consumes the single
	// bounded fallback: later executions run the retry candidate
	// directly and never fall back again.
	HasModelFallbackAttempt(ctx context.Context, taskID string) (bool, error)
}

// AttemptStore is an alias for AttemptRecorder.
type AttemptStore = AttemptRecorder

// SubmissionAuthorizer accepts current membership, Board capability and all input
// permissions on the settlement transaction before any Quote or Hold.
type SubmissionAuthorizer interface {
	AuthorizeSubmission(context.Context, pgx.Tx, tasks.CreateInput) (tasks.TaskBoardBinding, error)
}

// TxSubmissionClaimStore persists the durable submit idempotency fact inside
// the settlement transaction.
type TxSubmissionClaimStore interface {
	ClaimSubmissionInTx(ctx context.Context, tx pgx.Tx, claim tasks.SubmissionClaim) (existing *tasks.SubmissionClaim, claimed bool, err error)
}

// SettlementTransactor demarcates the settlement transaction. The postgres
// workspace task Store implements it over its pool. In-memory stores are
// test fixtures and are intentionally outside this production seam.
type SettlementTransactor interface {
	BeginSettlementScope(ctx context.Context) (*dbutil.Scope, error)
}

// TxHolder reserves and converges Ideas inside the settlement transaction.
// It keeps credits.Commands as the sole ledger writer: implementations must
// delegate to the transaction-scoped Commands (credits.Service.InTx) and
// perform no ledger SQL of their own.
type TxHolder interface {
	HoldInTx(ctx context.Context, tx pgx.Tx, billingAccountID, actorUserID int64, amount int64, idempotencyKey, taskID string) (string, error)
	CaptureInTx(ctx context.Context, tx pgx.Tx, billingAccountID, actorUserID int64, holdTransactionID string, amount int64, taskID string) (string, error)
	ReleaseInTx(ctx context.Context, tx pgx.Tx, billingAccountID, actorUserID int64, holdTransactionID string, amount int64, taskID string) (string, error)
}

// TxBillingAccountResolver resolves the settlement payer inside the
// settlement transaction (#915 resolution, #916 funds identity, payer =
// actor for Personal scopes per #973/#974). Production implements it on
// the postgres task Store; tests stub it.
//
// The actor rides explicitly: Personal scopes resolve the actor's own
// deterministic Personal account (fail-closed, never the workspace owner's
// or another wallet); Organization scopes resolve the shared pool.
// ownerType reports the Organization-pool fact (organization) versus the
// Personal actor account (workspace) so the settlement skips the member
// quota check on Personal scopes while keeping the Organization check.
type TxBillingAccountResolver interface {
	ResolveBillingAccountInTx(ctx context.Context, tx pgx.Tx, workspacePublicID string, actorUserID int64) (billingAccountID int64, ownerType string, err error)
}

// TxMemberQuotaChecker enforces the member cross-workspace consumption
// quota inside the settlement transaction (#946). Production implements
// it on the memberquotas Store; tests stub it. The settlement calls it
// for Organization scopes only; Personal scopes skip it (no quota
// concept, #973/#974).
type TxMemberQuotaChecker interface {
	CheckQuotaInTx(ctx context.Context, tx pgx.Tx, billingAccountID, userID int64, amount int64, reservedAt time.Time) error
}

// TxEnqueuer enqueues River jobs inside the settlement transaction.
type TxEnqueuer interface {
	EnqueueInTx(ctx context.Context, tx pgx.Tx, task *tasks.WorkspaceTask) error
}

// TaskReadStore reads workspace tasks.
type TaskReadStore interface {
	GetTask(ctx context.Context, workspaceID, taskID string) (*tasks.WorkspaceTask, error)
	ListTasks(ctx context.Context, workspaceID string, limit, offset int) ([]*tasks.WorkspaceTask, error)
}

// TxTaskLifecycleStore persists task state transitions inside the settlement transaction.
type TxTaskLifecycleStore interface {
	CreateTaskInTx(ctx context.Context, tx pgx.Tx, task *tasks.WorkspaceTask) (*tasks.WorkspaceTask, error)
	CompleteTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID string, outputs []tasks.OutputRef) (*tasks.WorkspaceTask, error)
	FailTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID, errorMsg string) (*tasks.WorkspaceTask, error)
	CancelTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID string, actorUserID int64, reason string) (*tasks.WorkspaceTask, bool, error)
	TransitionForRetryInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID, newToken string) (*tasks.WorkspaceTask, error)
	GetTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID string) (*tasks.WorkspaceTask, error)
}

// TaskSettlementStore requires lifecycle writes and their transaction owner together.
type TaskSettlementStore interface {
	SettlementTransactor
	TxTaskLifecycleStore
}

// TaskLifecycleStore updates workspace task state outside a settlement transaction.
type TaskLifecycleStore interface {
	CreateTask(ctx context.Context, task *tasks.WorkspaceTask) (*tasks.WorkspaceTask, error)
	CancelTask(ctx context.Context, workspaceID, taskID string, cancelledBy int64, reason string) (*tasks.WorkspaceTask, bool, error)
	TransitionForRetry(ctx context.Context, workspaceID, taskID, executionToken string) (*tasks.WorkspaceTask, error)
}

// TaskClaimStore claims tasks for worker execution.
type TaskClaimStore interface {
	ClaimTask(ctx context.Context, workspaceID, taskID, executionToken string) (*tasks.WorkspaceTask, error)
}

// TaskOutputStore updates task completion or failure without transactional settlement.
type TaskOutputStore interface {
	CompleteTask(ctx context.Context, workspaceID, taskID string, outputs []tasks.OutputRef) (*tasks.WorkspaceTask, error)
	FailTask(ctx context.Context, workspaceID, taskID, errorMsg string) (*tasks.WorkspaceTask, error)
}

// TaskBillingStore persists immutable billing facts and the settlement read
// model in the same transaction as task state and Credits.
type TaskBillingStore interface {
	CreateBillingFactsInTx(context.Context, pgx.Tx, *tasks.WorkspaceTask) error
	SetBillingSettlementInTx(context.Context, pgx.Tx, string, string, int64, int64, int64, string, string) error
}

// TaskEventBridge delivers terminal events to consumers.
type TaskEventBridge interface {
	Complete(ctx context.Context, workspaceID, taskID string, outputs []tasks.OutputRef) (*tasks.WorkspaceTask, error)
	Fail(ctx context.Context, workspaceID, taskID, errorMsg string) (*tasks.WorkspaceTask, error)
	Retry(ctx context.Context, workspaceID, taskID string, retryable bool) (*tasks.WorkspaceTask, error)
}

// SettlementTracer is the port for tracing spans during settlement,
// allowing application services to avoid importing platform/telemetry.
type SettlementTracer interface {
	Start(ctx context.Context, name string) (context.Context, func(err error))
}

// StalledSettlementScanner is the read-only discovery seam for stalled tasks.
type StalledSettlementScanner interface {
	ListStalledHeldSettlements(ctx context.Context, stalledBefore time.Time, limit int) ([]tasks.StalledSettlement, error)
}

// StalledSettlementFailer is the terminal settlement seam the sweep converges through.
type StalledSettlementFailer interface {
	Fail(ctx context.Context, workspaceID, taskID, errorMsg string) (*tasks.WorkspaceTask, error)
}
