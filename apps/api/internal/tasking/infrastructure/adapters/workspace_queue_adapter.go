package adapters

// Workspace task River bridge (#821, Phase 4-04 of #809).
//
// This enqueuer adapts the River client to workspacetasks.TxEnqueuer.
// Production enqueues into the default drawing queue; the workspace worker
// (#825 Phase-4b) consumes the workspace_task_execute kind on the shared
// River client.
//
// EnqueueInTx inserts the River job through the settlement transaction
// (client.InsertTx joins the ambient pgx.Tx): the river_job row commits
// atomically with the task insert, retry flip, or not at all. It satisfies
// workspacetasks.TxEnqueuer.
//
// Trace propagation (telemetry.Inject) lives here, at the enqueue edge —
// application services never touch telemetry.

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"ideaboard/api/internal/platform/riverpolicy"
	"ideaboard/api/internal/platform/telemetry"
	tasks "ideaboard/api/internal/tasking/domain"
	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"
	taskports "ideaboard/api/internal/tasking/ports/tasks"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// workspaceRiverClient is the narrow River seam the workspace enqueuer
// needs: transaction-joined inserts. The production
// *river.Client[pgx.Tx] satisfies it; tests inject a stub to prove the
// ambient settlement tx is forwarded untouched.
type workspaceRiverClient interface {
	InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// WorkspaceTaskEnqueuer adapts the River client to workspacetasks.TxEnqueuer.
// Production enqueues into the default drawing queue; the workspace worker
// (#825 Phase-4b) consumes the workspace_task_execute kind on the same
// queue via the shared River client.
type WorkspaceTaskEnqueuer struct {
	mu          sync.Mutex
	client      workspaceRiverClient
	queue       string
	maxAttempts int
}

var _ taskports.TxEnqueuer = (*WorkspaceTaskEnqueuer)(nil)

func NewWorkspaceTaskEnqueuer(client workspaceRiverClient, queue string, maxAttempts int) *WorkspaceTaskEnqueuer {
	queue, maxAttempts = riverpolicy.WithDefaults(queue, maxAttempts)
	return &WorkspaceTaskEnqueuer{client: client, queue: queue, maxAttempts: maxAttempts}
}

// SetClient attaches the River client once it exists. The enqueuer is built
// before the client (the settlement Service needs it at construction, the
// client needs the worker at registration); jobs only flow after Start, by
// which time the client is set. EnqueueInTx before then fails closed.
func (e *WorkspaceTaskEnqueuer) SetClient(client workspaceRiverClient) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.client = client
}

// EnqueueInTx inserts the River job through the settlement transaction
// (client.InsertTx joins the ambient pgx.Tx): the river_job row commits
// atomically with the task insert, retry flip, or not at all. It satisfies
// taskports.TxEnqueuer.
func (e *WorkspaceTaskEnqueuer) EnqueueInTx(ctx context.Context, tx pgx.Tx, task *tasks.WorkspaceTask) error {
	if e == nil {
		return fmt.Errorf("workspace task enqueuer is not configured: %w", tasks.ErrQuoteUnavailable)
	}
	if tx == nil {
		return fmt.Errorf("workspace task enqueuer: settlement transaction is nil: %w", tasks.ErrQuoteUnavailable)
	}
	e.mu.Lock()
	client := e.client
	e.mu.Unlock()
	if client == nil {
		return fmt.Errorf("workspace task enqueuer is not configured: %w", tasks.ErrQuoteUnavailable)
	}
	if task == nil || strings.TrimSpace(task.ID) == "" {
		return fmt.Errorf("workspace task enqueuer: task identity required: %w", tasks.ErrNotFound)
	}
	_, err := client.InsertTx(ctx, tx, taskqueue.WorkspaceTaskArgs{
		TaskID:         strings.TrimSpace(task.ID),
		WorkspaceID:    strings.TrimSpace(task.WorkspaceID),
		Service:        strings.TrimSpace(task.Service),
		ExecutionToken: strings.TrimSpace(task.ExecutionToken),
		TraceParent:    telemetry.Inject(ctx),
	}, &river.InsertOpts{
		Queue:       e.queue,
		MaxAttempts: e.maxAttempts,
		Tags:        []string{strings.TrimSpace(task.ID)},
	})
	if err != nil {
		return fmt.Errorf("workspace task enqueue: %w", err)
	}
	return nil
}
