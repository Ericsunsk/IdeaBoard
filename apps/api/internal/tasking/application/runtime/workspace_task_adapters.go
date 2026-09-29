package runtime

// Workspace task production adapters (#821, Phase 4-04 of #809).
//
// The v1 task surface (contract_v1_tasks.go) settles through the
// workspacetasks.Service seams. This file adapts the production billing
// catalog, credits Commands, River client, and workspace media store to
// those seams so http_wiring.go can take the Tasks surface live.
//
// Transactional note: DESIGN.md requires Quote snapshot + Credit Hold +
// Task insert + River enqueue in ONE PostgreSQL transaction, and terminal
// Complete/Cancel/Fail to settle Capture/Release atomically with the state
// flip. The seams below expose the plain Quote/Hold/Enqueue contracts used
// by preflight and terminal operations, plus the TxQuoter/TxHolder/TxEnqueuer
// variants that join the Service-owned
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
	"ideaboard/api/internal/platform/telemetry"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/workflow"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// WorkspaceTaskQuoter adapts the billing catalog to workspacetasks.Quoter.
// SKU is server-derived (ai-image:<canonical service>); no client price
// input crosses this seam. Quoter failures keep the ErrQuoteUnavailable
// routing identity while preserving the underlying catalog cause
// (multi-%w): pricing failures (ErrPricingUnavailable/ErrSKUNotFound)
// render 422 billing_pricing_unavailable via the first branch of
// renderTaskError, transient failures keep 503 billing_quote_unavailable.
type WorkspaceTaskQuoter struct {
	Catalog *billingapplication.BillableItemCatalog
}

var _ workspacetasks.Quoter = (*WorkspaceTaskQuoter)(nil)
var _ workspacetasks.TxQuoter = (*WorkspaceTaskQuoter)(nil)

func (q *WorkspaceTaskQuoter) Quote(ctx context.Context, service, resolution string, quantity int64) (workspacetasks.BillingSnapshot, error) {
	if q == nil || q.Catalog == nil {
		return workspacetasks.BillingSnapshot{}, fmt.Errorf("workspace task quoter is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	canonical, ok := workflow.ParseAIImageFeatureSlug(strings.TrimSpace(service))
	if !ok {
		return workspacetasks.BillingSnapshot{}, fmt.Errorf("workspace task quoter: unsupported service %q: %w", service, workspacetasks.ErrQuoteUnavailable)
	}
	if quantity <= 0 {
		quantity = 1
	}
	outputResolution, err := billing.NormalizeOutputResolution(resolution)
	if err != nil {
		return workspacetasks.BillingSnapshot{}, fmt.Errorf("workspace task quoter: invalid output resolution: %w", err)
	}
	sku := "ai-image:" + canonical
	quote, err := q.Catalog.QuoteAtResolution(ctx, sku, outputResolution, quantity)
	if err != nil {
		// Preserve the underlying billing cause alongside the
		// quote-unavailable identity (multi-%w): pricing failures
		// (ErrPricingUnavailable/ErrSKUNotFound) render 422 via the
		// first branch of renderTaskError, while transient failures
		// keep the 503 billing_quote_unavailable branch.
		return workspacetasks.BillingSnapshot{}, fmt.Errorf("workspace task quoter: %w: %w", err, workspacetasks.ErrQuoteUnavailable)
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
func (q *WorkspaceTaskQuoter) QuoteInTx(ctx context.Context, tx pgx.Tx, service, resolution string, quantity int64) (workspacetasks.BillingSnapshot, error) {
	if q == nil || q.Catalog == nil {
		return workspacetasks.BillingSnapshot{}, fmt.Errorf("workspace task quoter is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if tx == nil {
		return workspacetasks.BillingSnapshot{}, fmt.Errorf("workspace task quoter: settlement transaction is nil: %w", workspacetasks.ErrQuoteUnavailable)
	}
	canonical, ok := workflow.ParseAIImageFeatureSlug(strings.TrimSpace(service))
	if !ok {
		return workspacetasks.BillingSnapshot{}, fmt.Errorf("workspace task quoter: unsupported service %q: %w", service, workspacetasks.ErrQuoteUnavailable)
	}
	if quantity <= 0 {
		quantity = 1
	}
	outputResolution, err := billing.NormalizeOutputResolution(resolution)
	if err != nil {
		return workspacetasks.BillingSnapshot{}, fmt.Errorf("workspace task quoter: invalid output resolution: %w", err)
	}
	sku := "ai-image:" + canonical
	quote, err := q.Catalog.QuoteAtResolutionInTx(ctx, tx, sku, outputResolution, quantity)
	if err != nil {
		// Same multi-%w preservation as Quote: the price read shares
		// the repeatable-read snapshot with the Hold that follows, and
		// the pricing cause must survive to renderTaskError.
		return workspacetasks.BillingSnapshot{}, fmt.Errorf("workspace task quoter: %w: %w", err, workspacetasks.ErrQuoteUnavailable)
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

// WorkspaceTaskHolder adapts credits.Commands to workspacetasks.Holder.
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
type WorkspaceTaskHolder struct {
	credits *credits.Service

	mu    sync.Mutex
	users map[string]int64
}

var _ workspacetasks.Holder = (*WorkspaceTaskHolder)(nil)
var _ workspacetasks.TxHolder = (*WorkspaceTaskHolder)(nil)

func NewWorkspaceTaskHolder(creditService *credits.Service) *WorkspaceTaskHolder {
	return &WorkspaceTaskHolder{credits: creditService, users: make(map[string]int64)}
}

func taskCommandMetadata(idempotencyKey, taskID string, userID int64) domain.CommandMetadata {
	return domain.CommandMetadata{
		IdempotencyKey:    idempotencyKey,
		Actor:             domain.Actor{Type: domain.ActorUser, ID: fmt.Sprint(userID)},
		Source:            "task_submission",
		Reason:            msgTaskCommandMetadata,
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

func (h *WorkspaceTaskHolder) Hold(ctx context.Context, userID int64, amount int64, idempotencyKey, taskID string) (string, error) {
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

func (h *WorkspaceTaskHolder) holderUserID(holdTransactionID string) (int64, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	userID, ok := h.users[holdTransactionID]
	return userID, ok
}

func (h *WorkspaceTaskHolder) forgetHold(holdTransactionID string) {
	h.mu.Lock()
	delete(h.users, holdTransactionID)
	h.mu.Unlock()
}

func (h *WorkspaceTaskHolder) Capture(ctx context.Context, holdTransactionID string, amount int64, taskID string) error {
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

func (h *WorkspaceTaskHolder) Release(ctx context.Context, holdTransactionID string, amount int64, taskID string) error {
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
func (h *WorkspaceTaskHolder) HoldInTx(ctx context.Context, tx pgx.Tx, userID int64, amount int64, idempotencyKey, taskID string) (string, error) {
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
func (h *WorkspaceTaskHolder) CaptureInTx(ctx context.Context, tx pgx.Tx, userID int64, holdTransactionID string, amount int64, taskID string) (string, error) {
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
func (h *WorkspaceTaskHolder) ReleaseInTx(ctx context.Context, tx pgx.Tx, userID int64, holdTransactionID string, amount int64, taskID string) (string, error) {
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
	Service        string `json:"service"`
	ExecutionToken string `json:"execution_token"`
	TraceParent    string `json:"traceparent,omitempty"`
}

// Kind implements river.JobArgs.
func (WorkspaceTaskArgs) Kind() string { return "workspace_task_execute" }

// WorkspaceTaskEnqueuer adapts the River client to workspacetasks.Enqueuer.
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
type WorkspaceTaskEnqueuer struct {
	mu          sync.Mutex
	client      workspaceRiverClient
	queue       string
	maxAttempts int
}

var _ workspacetasks.Enqueuer = (*WorkspaceTaskEnqueuer)(nil)
var _ workspacetasks.TxEnqueuer = (*WorkspaceTaskEnqueuer)(nil)

func NewWorkspaceTaskEnqueuer(client workspaceRiverClient, queue string, maxAttempts int) *WorkspaceTaskEnqueuer {
	if strings.TrimSpace(queue) == "" {
		queue = "drawing_default"
	}
	if maxAttempts <= 0 {
		maxAttempts = 25
	}
	return &WorkspaceTaskEnqueuer{client: client, queue: queue, maxAttempts: maxAttempts}
}

// SetClient attaches the River client once it exists. The enqueuer is built
// before the client (the settlement Service needs it at construction, the
// client needs the worker at registration); jobs only flow after Start, by
// which time the client is set. Enqueue before then fails closed.
func (e *WorkspaceTaskEnqueuer) SetClient(client workspaceRiverClient) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.client = client
}

func (e *WorkspaceTaskEnqueuer) Enqueue(ctx context.Context, task *workspacetasks.WorkspaceTask) error {
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

// EnqueueInTx inserts the River job through the settlement transaction
// (client.InsertTx joins the ambient pgx.Tx): the river_job row commits
// atomically with the task insert, retry flip, or not at all. It satisfies
// workspacetasks.TxEnqueuer.
func (e *WorkspaceTaskEnqueuer) EnqueueInTx(ctx context.Context, tx pgx.Tx, task *workspacetasks.WorkspaceTask) error {
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
