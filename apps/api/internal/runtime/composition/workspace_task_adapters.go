package composition

// Workspace task production adapters (#821, Phase 4-04 of #809).
//
// The v1 task surface (contract_v1_tasks.go) settles through the
// workspacetasks.Service seams (Quoter/Holder/Enqueuer/AssetReader) with
// compensating rollback (Quote -> Hold -> Insert -> Enqueue). This file
// adapts the production billing catalog, credits Commands, River client,
// and workspace media store to those seams so http_wiring.go can take the
// Tasks surface live (replacing the 501 fail-closed).
//
// Transactional note: DESIGN.md requires Quote snapshot + Credit Hold +
// Task insert + River enqueue in ONE PostgreSQL transaction. The Service
// orchestrates compensating actions (Release on insert failure, Delete +
// Release on enqueue failure) rather than a single shared tx: the billing
// catalog quoter, credits Commands, postgres task store, and River client
// each own their connection/transaction today, and threading one pgx.Tx
// through all four is Phase-4b work. The compensation order guarantees no
// stranded Hold and no orphan River job; events still stage and flush only
// after commit. See the residual note in the #821 report.
//
// legacy_task_store_stub.go is UNTOUCHED: it covers the legacy
// taskstore.Repository seams (submission/execution/statistics/retention,
// all keyed by the pre-workspace task model) which the workspace store
// does not implement. The v1 workspace seams below are additive and live
// beside it.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"ideaboard/api/internal/billing"
	billingapplication "ideaboard/api/internal/billing/application"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/workflow"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

// workspaceTaskQuoter adapts the billing catalog to workspacetasks.Quoter.
// SKU is server-derived (ai-image:<canonical feature>); no client price
// input crosses this seam. Quoter failures map to ErrQuoteUnavailable so
// the handler renders 503 billing_quote_unavailable.
type workspaceTaskQuoter struct {
	catalog *billingapplication.BillableItemCatalog
}

var _ workspacetasks.Quoter = (*workspaceTaskQuoter)(nil)

func (q *workspaceTaskQuoter) Quote(ctx context.Context, workflowKind, resolution string, quantity int64) (workspacetasks.BillingSnapshot, error) {
	if q == nil || q.catalog == nil {
		return workspacetasks.BillingSnapshot{}, fmt.Errorf("workspace task quoter is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	canonical, ok := workflow.ParseAIImageFeatureSlug(strings.TrimSpace(workflowKind))
	if !ok {
		return workspacetasks.BillingSnapshot{}, fmt.Errorf("workspace task quoter: unsupported kind %q: %w", workflowKind, workspacetasks.ErrQuoteUnavailable)
	}
	if quantity <= 0 {
		quantity = 1
	}
	outputResolution, err := billing.NormalizeOutputResolution(resolution)
	if err != nil {
		outputResolution = billing.OutputResolutionDefault
	}
	sku := "ai-image:" + canonical
	quote, err := q.catalog.QuoteAtResolution(ctx, sku, outputResolution, quantity)
	if err != nil {
		return workspacetasks.BillingSnapshot{}, fmt.Errorf("workspace task quoter: %w", workspacetasks.ErrQuoteUnavailable)
	}
	return workspacetasks.BillingSnapshot{
		SKU:               string(quote.SKU),
		DisplayName:       quote.DisplayName,
		PriceVersionID:    quote.PriceVersionID,
		PriceVersion:      quote.PriceVersion,
		OutputResolution:  string(quote.OutputResolution),
		UnitCredits:       quote.UnitCredits,
		Quantity:          quote.Quantity,
		TotalCredits:      quote.TotalCredits,
		QuotedAt:          quote.QuotedAt,
		SettlementState:   "quoted",
	}, nil
}

// workspaceTaskHolder adapts credits.Commands to workspacetasks.Holder.
// Only Credits owns ledger writes; this seam performs none directly.
//
// The workspacetasks.Holder Capture/Release signature carries no userID,
// but credits Commands require one for wallet resolution. The adapter
// records hold_transaction_id -> userID at Hold time (process-local,
// deleted on Capture/Release) so settlement converges without an
// interface change. A process restart between Hold and terminal settle
// leaves the hold to the 24h stale-hold reconciler (credits ownership),
// never to a second Hold.
type workspaceTaskHolder struct {
	credits *credits.Service

	mu    sync.Mutex
	users map[string]int64
}

var _ workspacetasks.Holder = (*workspaceTaskHolder)(nil)

func newWorkspaceTaskHolder(creditService *credits.Service) *workspaceTaskHolder {
	return &workspaceTaskHolder{credits: creditService, users: make(map[string]int64)}
}

func taskCommandMetadata(idempotencyKey, taskID string, userID int64) domain.CommandMetadata {
	return domain.CommandMetadata{
		IdempotencyKey: idempotencyKey,
		Actor:          domain.Actor{Type: domain.ActorUser, ID: fmt.Sprint(userID)},
		Source:         "task_submission",
		Reason:         "AI 生图任务 Idea 预授权",
		BusinessReference: domain.BusinessReference{Type: "task", ID: taskID},
	}
}

func mapHolderError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrInsufficientCredits) {
		return fmt.Errorf("%w: %w", workspacetasks.ErrInsufficientCredits, err)
	}
	if errors.Is(err, domain.ErrWalletRestricted) {
		return fmt.Errorf("%w: %w", workspacetasks.ErrWalletRestricted, err)
	}
	return err
}

func (h *workspaceTaskHolder) Hold(ctx context.Context, userID int64, amount int64, idempotencyKey, taskID string) (string, error) {
	if h == nil || h.credits == nil {
		return "", fmt.Errorf("workspace task holder is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if userID <= 0 {
		return "", fmt.Errorf("workspace task holder: user required: %w", workspacetasks.ErrWalletRestricted)
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return "", fmt.Errorf("workspace task holder: %w", workspacetasks.ErrQuoteUnavailable)
	}
	result, err := h.credits.Hold(ctx, ports.HoldCommand{
		UserID:   userID,
		Amount:   creditAmount,
		Metadata: taskCommandMetadata(idempotencyKey, taskID, userID),
	})
	if err != nil {
		return "", mapHolderError(err)
	}
	h.mu.Lock()
	h.users[result.TransactionID] = userID
	h.mu.Unlock()
	return result.TransactionID, nil
}

func (h *workspaceTaskHolder) holderUserID(holdTransactionID string) (int64, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	userID, ok := h.users[holdTransactionID]
	return userID, ok
}

func (h *workspaceTaskHolder) forgetHold(holdTransactionID string) {
	h.mu.Lock()
	delete(h.users, holdTransactionID)
	h.mu.Unlock()
}

func (h *workspaceTaskHolder) Capture(ctx context.Context, holdTransactionID string, amount int64, taskID string) error {
	if h == nil || h.credits == nil {
		return fmt.Errorf("workspace task holder is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if amount <= 0 {
		h.forgetHold(holdTransactionID)
		return nil
	}
	userID, ok := h.holderUserID(holdTransactionID)
	if !ok {
		return fmt.Errorf("workspace task holder: unknown hold %s: %w", holdTransactionID, workspacetasks.ErrQuoteUnavailable)
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return fmt.Errorf("workspace task holder: %w", workspacetasks.ErrQuoteUnavailable)
	}
	_, err = h.credits.Capture(ctx, ports.CaptureCommand{
		UserID:            userID,
		HoldTransactionID: holdTransactionID,
		Amount:            creditAmount,
		Metadata:          taskCommandMetadata("task-capture:"+taskID, taskID, userID),
	})
	if err != nil {
		return mapHolderError(err)
	}
	h.forgetHold(holdTransactionID)
	return nil
}

func (h *workspaceTaskHolder) Release(ctx context.Context, holdTransactionID string, amount int64, taskID string) error {
	if h == nil || h.credits == nil {
		return fmt.Errorf("workspace task holder is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if amount <= 0 {
		h.forgetHold(holdTransactionID)
		return nil
	}
	userID, ok := h.holderUserID(holdTransactionID)
	if !ok {
		return fmt.Errorf("workspace task holder: unknown hold %s: %w", holdTransactionID, workspacetasks.ErrQuoteUnavailable)
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return fmt.Errorf("workspace task holder: %w", workspacetasks.ErrQuoteUnavailable)
	}
	_, err = h.credits.Release(ctx, ports.ReleaseCommand{
		UserID:            userID,
		HoldTransactionID: holdTransactionID,
		Amount:            creditAmount,
		Metadata:          taskCommandMetadata("task-release:"+taskID, taskID, userID),
	})
	if err != nil {
		return mapHolderError(err)
	}
	h.forgetHold(holdTransactionID)
	return nil
}

// WorkspaceTaskArgs is the River job payload for workspace-scoped task
// execution. The worker that consumes it lands in Phase-4b (alongside the
// vision/materialization billing adapters and the derivative renderer):
// until then jobs wait as available and never dispatch to a provider.
// The payload carries only opaque identifiers (no bytes, no secrets).
type WorkspaceTaskArgs struct {
	TaskID         string `json:"task_id"`
	WorkspaceID    string `json:"workspace_id"`
	WorkflowKind   string `json:"workflow_kind"`
	ExecutionToken string `json:"execution_token"`
}

// Kind implements river.JobArgs.
func (WorkspaceTaskArgs) Kind() string { return "workspace_task_execute" }

// workspaceTaskEnqueuer adapts the River client to workspacetasks.Enqueuer.
// Production enqueues into the default drawing queue; the workspace worker
// (Phase-4b) picks up the workspace_task_execute kind.
type workspaceTaskEnqueuer struct {
	client      *river.Client[pgx.Tx]
	queue       string
	maxAttempts int
}

var _ workspacetasks.Enqueuer = (*workspaceTaskEnqueuer)(nil)

func newWorkspaceTaskEnqueuer(client *river.Client[pgx.Tx], queue string, maxAttempts int) *workspaceTaskEnqueuer {
	if strings.TrimSpace(queue) == "" {
		queue = "drawing_default"
	}
	if maxAttempts <= 0 {
		maxAttempts = 25
	}
	return &workspaceTaskEnqueuer{client: client, queue: queue, maxAttempts: maxAttempts}
}

func (e *workspaceTaskEnqueuer) Enqueue(ctx context.Context, task *workspacetasks.WorkspaceTask) error {
	if e == nil || e.client == nil {
		return fmt.Errorf("workspace task enqueuer is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if task == nil || strings.TrimSpace(task.ID) == "" {
		return fmt.Errorf("workspace task enqueuer: task identity required: %w", workspacetasks.ErrNotFound)
	}
	_, err := e.client.Insert(ctx, WorkspaceTaskArgs{
		TaskID:         strings.TrimSpace(task.ID),
		WorkspaceID:    strings.TrimSpace(task.WorkspaceID),
		WorkflowKind:   strings.TrimSpace(task.WorkflowKind),
		ExecutionToken: strings.TrimSpace(task.ExecutionToken),
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
