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
// Task insert + River enqueue in ONE PostgreSQL transaction, and terminal
// Complete/Cancel/Fail to settle Capture/Release atomically with the state
// flip. The seams below expose both halves: the plain Quoter/Holder/
// Enqueuer methods (compensating fallback for test backends) and the
// TxQuoter/TxHolder/TxEnqueuer variants that join the Service-owned
// settlement transaction (opened on the task store pool via
// workspacetasks.SettlementTransactor). Credits converge through
// credits.Service.InTx(tx) — the sole ledger writer — the billing catalog
// quotes through QuoteAtResolutionInTx, and River enqueues through
// InsertTx, so the river_job row commits atomically with the business tx.
// Events still stage and flush only after commit.
//
// #809 hard cut: legacy_task_store_stub.go is DELETED (it covered the
// legacy taskstore.Repository seams — submission/execution/statistics/
// retention, all keyed by the pre-workspace task model — which the
// workspace store does not implement). The v1 workspace seams below are
// the only task path.

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
	"github.com/riverqueue/river/rivertype"
)

// workspaceTaskQuoter adapts the billing catalog to workspacetasks.Quoter.
// SKU is server-derived (ai-image:<canonical feature>); no client price
// input crosses this seam. Quoter failures map to ErrQuoteUnavailable so
// the handler renders 503 billing_quote_unavailable.
type workspaceTaskQuoter struct {
	catalog *billingapplication.BillableItemCatalog
}

var _ workspacetasks.Quoter = (*workspaceTaskQuoter)(nil)
var _ workspacetasks.TxQuoter = (*workspaceTaskQuoter)(nil)

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
		SKU:              string(quote.SKU),
		DisplayName:      quote.DisplayName,
		PriceVersionID:   quote.PriceVersionID,
		PriceVersion:     quote.PriceVersion,
		OutputResolution: string(quote.OutputResolution),
		UnitCredits:      quote.UnitCredits,
		Quantity:         quote.Quantity,
		TotalCredits:     quote.TotalCredits,
		QuotedAt:         quote.QuotedAt,
		SettlementState:  "quoted",
	}, nil
}

// QuoteInTx mints the same snapshot through the settlement transaction so
// the price read shares the repeatable-read snapshot with the Hold that
// follows. It satisfies workspacetasks.TxQuoter.
func (q *workspaceTaskQuoter) QuoteInTx(ctx context.Context, tx pgx.Tx, workflowKind, resolution string, quantity int64) (workspacetasks.BillingSnapshot, error) {
	if q == nil || q.catalog == nil {
		return workspacetasks.BillingSnapshot{}, fmt.Errorf("workspace task quoter is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if tx == nil {
		return workspacetasks.BillingSnapshot{}, fmt.Errorf("workspace task quoter: settlement transaction is nil: %w", workspacetasks.ErrQuoteUnavailable)
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
	quote, err := q.catalog.QuoteAtResolutionInTx(ctx, tx, sku, outputResolution, quantity)
	if err != nil {
		return workspacetasks.BillingSnapshot{}, fmt.Errorf("workspace task quoter: %w", workspacetasks.ErrQuoteUnavailable)
	}
	return workspacetasks.BillingSnapshot{
		SKU:              string(quote.SKU),
		DisplayName:      quote.DisplayName,
		PriceVersionID:   quote.PriceVersionID,
		PriceVersion:     quote.PriceVersion,
		OutputResolution: string(quote.OutputResolution),
		UnitCredits:      quote.UnitCredits,
		Quantity:         quote.Quantity,
		TotalCredits:     quote.TotalCredits,
		QuotedAt:         quote.QuotedAt,
		SettlementState:  "quoted",
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
//
// The TxHolder variants (HoldInTx/CaptureInTx/ReleaseInTx) take the userID
// explicitly from the settled task row and run through
// credits.Service.InTx(tx), so terminal settlement never depends on the
// process-local map.
type workspaceTaskHolder struct {
	credits *credits.Service

	mu    sync.Mutex
	users map[string]int64
}

var _ workspacetasks.Holder = (*workspaceTaskHolder)(nil)
var _ workspacetasks.TxHolder = (*workspaceTaskHolder)(nil)

func newWorkspaceTaskHolder(creditService *credits.Service) *workspaceTaskHolder {
	return &workspaceTaskHolder{credits: creditService, users: make(map[string]int64)}
}

func taskCommandMetadata(idempotencyKey, taskID string, userID int64) domain.CommandMetadata {
	return domain.CommandMetadata{
		IdempotencyKey:    idempotencyKey,
		Actor:             domain.Actor{Type: domain.ActorUser, ID: fmt.Sprint(userID)},
		Source:            "task_submission",
		Reason:            "AI 生图任务 Idea 预授权",
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

// HoldInTx reserves Ideas inside the settlement transaction through the
// transaction-scoped Commands (credits.Service.InTx): the hold row commits
// atomically with the task insert. It satisfies workspacetasks.TxHolder.
func (h *workspaceTaskHolder) HoldInTx(ctx context.Context, tx pgx.Tx, userID int64, amount int64, idempotencyKey, taskID string) (string, error) {
	if h == nil || h.credits == nil {
		return "", fmt.Errorf("workspace task holder is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if tx == nil {
		return "", fmt.Errorf("workspace task holder: settlement transaction is nil: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if userID <= 0 {
		return "", fmt.Errorf("workspace task holder: user required: %w", workspacetasks.ErrWalletRestricted)
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return "", fmt.Errorf("workspace task holder: %w", workspacetasks.ErrQuoteUnavailable)
	}
	result, err := h.credits.InTx(tx).Hold(ctx, ports.HoldCommand{
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

// CaptureInTx converges a hold to revenue inside the settlement
// transaction. The userID comes from the settled task row, never the
// process-local map. It satisfies workspacetasks.TxHolder.
func (h *workspaceTaskHolder) CaptureInTx(ctx context.Context, tx pgx.Tx, userID int64, holdTransactionID string, amount int64, taskID string) (string, error) {
	if h == nil || h.credits == nil {
		return "", fmt.Errorf("workspace task holder is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if tx == nil {
		return "", fmt.Errorf("workspace task holder: settlement transaction is nil: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if userID <= 0 {
		return "", fmt.Errorf("workspace task holder: user required: %w", workspacetasks.ErrWalletRestricted)
	}
	if amount <= 0 {
		return "", nil
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return "", fmt.Errorf("workspace task holder: %w", workspacetasks.ErrQuoteUnavailable)
	}
	result, err := h.credits.InTx(tx).Capture(ctx, ports.CaptureCommand{
		UserID:            userID,
		HoldTransactionID: holdTransactionID,
		Amount:            creditAmount,
		Metadata:          taskCommandMetadata("task-capture:"+taskID, taskID, userID),
	})
	if err != nil {
		return "", mapHolderError(err)
	}
	h.forgetHold(holdTransactionID)
	return result.TransactionID, nil
}

// ReleaseInTx returns reserved Ideas inside the settlement transaction. It
// satisfies workspacetasks.TxHolder.
func (h *workspaceTaskHolder) ReleaseInTx(ctx context.Context, tx pgx.Tx, userID int64, holdTransactionID string, amount int64, taskID string) (string, error) {
	if h == nil || h.credits == nil {
		return "", fmt.Errorf("workspace task holder is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if tx == nil {
		return "", fmt.Errorf("workspace task holder: settlement transaction is nil: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if userID <= 0 {
		return "", fmt.Errorf("workspace task holder: user required: %w", workspacetasks.ErrWalletRestricted)
	}
	if amount <= 0 {
		return "", nil
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return "", fmt.Errorf("workspace task holder: %w", workspacetasks.ErrQuoteUnavailable)
	}
	result, err := h.credits.InTx(tx).Release(ctx, ports.ReleaseCommand{
		UserID:            userID,
		HoldTransactionID: holdTransactionID,
		Amount:            creditAmount,
		Metadata:          taskCommandMetadata("task-release:"+taskID, taskID, userID),
	})
	if err != nil {
		return "", mapHolderError(err)
	}
	h.forgetHold(holdTransactionID)
	return result.TransactionID, nil
}

// WorkspaceTaskArgs is the River job payload for workspace-scoped task
// execution. The workspace worker (#825, workspace_task_worker.go) consumes
// this kind on the shared drawing queue. The payload carries only opaque
// identifiers (no bytes, no secrets).
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
// (#825 Phase-4b) consumes the workspace_task_execute kind on the same
// workspaceRiverClient is the narrow River seam the workspace enqueuer
// needs: plain inserts plus transaction-joined inserts. The production
// *river.Client[pgx.Tx] satisfies it; tests inject a stub to prove the
// ambient settlement tx is forwarded untouched.
type workspaceRiverClient interface {
	Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
	InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// queue via the shared River client.
type workspaceTaskEnqueuer struct {
	mu          sync.Mutex
	client      workspaceRiverClient
	queue       string
	maxAttempts int
}

var _ workspacetasks.Enqueuer = (*workspaceTaskEnqueuer)(nil)
var _ workspacetasks.TxEnqueuer = (*workspaceTaskEnqueuer)(nil)

func newWorkspaceTaskEnqueuer(client workspaceRiverClient, queue string, maxAttempts int) *workspaceTaskEnqueuer {
	if strings.TrimSpace(queue) == "" {
		queue = "drawing_default"
	}
	if maxAttempts <= 0 {
		maxAttempts = 25
	}
	return &workspaceTaskEnqueuer{client: client, queue: queue, maxAttempts: maxAttempts}
}

// SetClient attaches the River client once it exists. The enqueuer is built
// before the client (the settlement Service needs it at construction, the
// client needs the worker at registration); jobs only flow after Start, by
// which time the client is set. Enqueue before then fails closed.
func (e *workspaceTaskEnqueuer) SetClient(client workspaceRiverClient) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.client = client
}

func (e *workspaceTaskEnqueuer) Enqueue(ctx context.Context, task *workspacetasks.WorkspaceTask) error {
	if e == nil {
		return fmt.Errorf("workspace task enqueuer is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	e.mu.Lock()
	client := e.client
	e.mu.Unlock()
	if client == nil {
		return fmt.Errorf("workspace task enqueuer is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if task == nil || strings.TrimSpace(task.ID) == "" {
		return fmt.Errorf("workspace task enqueuer: task identity required: %w", workspacetasks.ErrNotFound)
	}
	_, err := client.Insert(ctx, WorkspaceTaskArgs{
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

// EnqueueInTx inserts the River job through the settlement transaction
// (client.InsertTx joins the ambient pgx.Tx): the river_job row commits
// atomically with the task insert, retry flip, or not at all. It satisfies
// workspacetasks.TxEnqueuer.
func (e *workspaceTaskEnqueuer) EnqueueInTx(ctx context.Context, tx pgx.Tx, task *workspacetasks.WorkspaceTask) error {
	if e == nil {
		return fmt.Errorf("workspace task enqueuer is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if tx == nil {
		return fmt.Errorf("workspace task enqueuer: settlement transaction is nil: %w", workspacetasks.ErrQuoteUnavailable)
	}
	e.mu.Lock()
	client := e.client
	e.mu.Unlock()
	if client == nil {
		return fmt.Errorf("workspace task enqueuer is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if task == nil || strings.TrimSpace(task.ID) == "" {
		return fmt.Errorf("workspace task enqueuer: task identity required: %w", workspacetasks.ErrNotFound)
	}
	_, err := client.InsertTx(ctx, tx, WorkspaceTaskArgs{
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
