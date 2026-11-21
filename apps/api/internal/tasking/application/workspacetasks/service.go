package workspacetasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/generation/routing"
	"ideaboard/api/internal/platform/contract"
	tasks "ideaboard/api/internal/tasking/domain"
	taskports "ideaboard/api/internal/tasking/ports/tasks"
	"ideaboard/api/internal/workflow"
)

// Service orchestrates the workspace task settlement boundary:
//
//	Quote snapshot -> Credits Hold (via TxHolder, never direct ledger
//	writes) -> Task insert (pending, retry_count, execution_token,
//	output_refs, billing_snapshot) -> River enqueue (via TxEnqueuer).
//
// Production settlement commits in ONE shared PostgreSQL transaction: a
// billing failure rolls the state flip back and a state-guard failure rolls
// the billing writes back. Terminal settle (Capture on success / Release on
// fail/cancel/timeout) follows the same tx-first rule. Events stage during
// settlement and flush ONLY after commit; staging never publishes inside the
// txn. In-memory implementations are test-only fixtures for exercising the
// service contract.
type Service struct {
	taskRead         taskports.TaskReadStore
	taskLifecycle    taskports.TaskSettlementStore
	taskBilling      taskports.TaskBillingStore
	quoter           taskports.Quoter
	holder           taskports.TxHolder
	queue            taskports.TxEnqueuer
	assets           taskports.AssetReader
	acceptance       taskports.SubmissionAuthorizer
	resolver         *routing.RoutingResolver
	saveSnapshot     taskports.RoutingSnapshotSaver
	submissionClaims taskports.TxSubmissionClaimStore
	billingAccounts  taskports.TxBillingAccountResolver
	quotas           taskports.TxMemberQuotaChecker
	tracer           taskports.SettlementTracer
	events           []tasks.TaskEvent
	now              func() time.Time
}

// ServiceDeps wires the settlement seams.
type ServiceDeps struct {
	TaskRead         taskports.TaskReadStore
	TaskLifecycle    taskports.TaskSettlementStore
	TaskBilling      taskports.TaskBillingStore
	Quoter           taskports.Quoter
	Holder           taskports.TxHolder
	Queue            taskports.TxEnqueuer
	Assets           taskports.AssetReader
	Resolver         *routing.RoutingResolver
	SnapshotSaver    taskports.RoutingSnapshotSaver
	SubmissionClaims taskports.TxSubmissionClaimStore
	Acceptance       taskports.SubmissionAuthorizer
	BillingAccounts  taskports.TxBillingAccountResolver
	Quotas           taskports.TxMemberQuotaChecker
	Tracer           taskports.SettlementTracer
}

// NewService builds the settlement orchestrator.
func NewService(deps ServiceDeps) *Service {
	return &Service{
		taskRead:         deps.TaskRead,
		taskLifecycle:    deps.TaskLifecycle,
		taskBilling:      deps.TaskBilling,
		quoter:           deps.Quoter,
		holder:           deps.Holder,
		queue:            deps.Queue,
		assets:           deps.Assets,
		resolver:         deps.Resolver,
		saveSnapshot:     deps.SnapshotSaver,
		submissionClaims: deps.SubmissionClaims,
		acceptance:       deps.Acceptance,
		billingAccounts:  deps.BillingAccounts,
		quotas:           deps.Quotas,
		tracer:           deps.Tracer,
		now:              time.Now,
	}
}

// SetClockForTests injects a deterministic clock.
func (s *Service) SetClockForTests(now func() time.Time) {
	s.now = now
}

func (s *Service) clock() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

// StagedEvents drains staged status events (for #807 fanout). Callers flush
// only after the settlement commit succeeds.
func (s *Service) StagedEvents() []tasks.TaskEvent {
	staged := append([]tasks.TaskEvent(nil), s.events...)
	s.events = nil
	return staged
}

func (s *Service) stageWithTokenAt(taskID, workspaceID string, status tasks.Status, executionToken string, occurredAt time.Time, outputs []events.V1TaskCompletedOutput) {
	if occurredAt.IsZero() {
		occurredAt = s.clock()
	}
	s.events = append(s.events, tasks.TaskEvent{
		EventID:     events.StableEventID(taskID, string(status), executionToken),
		TaskID:      taskID,
		WorkspaceID: workspaceID,
		Status:      status,
		OccurredAt:  occurredAt.UTC(),
		Outputs:     outputs,
	})
}

// Create settles one task atomically: Quote -> Hold -> Insert -> Enqueue,
// fenced by a durable submit idempotency claim written in the same
// transaction.
func (s *Service) Create(ctx context.Context, input tasks.CreateInput, idempotencyKey string) (*tasks.WorkspaceTask, error) {
	if s.tracer != nil {
		var end func(err error)
		ctx, end = s.tracer.Start(ctx, "generation.submit.settlement")
		defer end(nil)
	}
	if s.taskLifecycle == nil {
		return nil, fmt.Errorf("workspacetasks: store unavailable: %w", tasks.ErrNotFound)
	}
	key, err := tasks.NormalizeSubmitIdempotencyKey(idempotencyKey)
	if err != nil {
		return nil, err
	}
	kind, err := tasks.ValidateCreate(input.Service, input.MediaAssetIDs, input.Params)
	if err != nil {
		return nil, err
	}
	if err := tasks.ValidateBoardID(input.BoardID); err != nil {
		return nil, err
	}
	if s.quoter == nil || s.holder == nil || s.queue == nil {
		return nil, fmt.Errorf("workspacetasks: settlement unavailable: %w", tasks.ErrQuoteUnavailable)
	}
	quantity := input.Quantity
	if input.CanonicalParams != "" {
		var requested *int64
		if quantity != 0 {
			requested = &quantity
		}
		quantity, err = ResolveSubmissionQuantity(input.CanonicalParams, requested)
		if err != nil {
			return nil, err
		}
	} else if quantity <= 0 {
		quantity = 1
	}
	resolution := strings.TrimSpace(input.Resolution)
	if resolution == "" {
		resolution = workflow.OutputResolution1K
	}
	workspaceID := strings.TrimSpace(input.WorkspaceID)
	taskID, err := contract.MintPublicID(contract.PrefixTask, 12)
	if err != nil {
		return nil, err
	}
	token, err := tasks.NewExecutionToken()
	if err != nil {
		return nil, err
	}
	claim := tasks.SubmissionClaim{
		WorkspaceID:    workspaceID,
		Operation:      tasks.TaskSubmitOperation,
		IdempotencyKey: key,
		TaskPublicID:   taskID,
		RequestHash:    s.submitRequestHash(input, kind, resolution, quantity),
	}
	return s.createInTx(ctx, input, kind, resolution, quantity, taskID, token, claim)
}

func pendingTaskWithBinding(input tasks.CreateInput, kind, taskID, token string, binding tasks.TaskBoardBinding, snapshot tasks.BillingSnapshot) *tasks.WorkspaceTask {
	return &tasks.WorkspaceTask{
		ID:                 taskID,
		WorkspaceID:        strings.TrimSpace(input.WorkspaceID),
		OwnerUserID:        input.OwnerUserID,
		BillingAccountID:   snapshot.BillingAccountID,
		Service:            kind,
		MediaAssetIDs:      append([]string(nil), input.MediaAssetIDs...),
		Params:             cloneParams(input.Params),
		CanonicalParams:    input.CanonicalParams,
		BoardID:            binding.BoardID,
		BoardAccessVersion: binding.AccessVersion,
		Status:             tasks.StatusPending,
		RetryCount:         0,
		ExecutionToken:     token,
		BillingSnapshot:    snapshot,
	}
}

func (s *Service) submitRequestHash(input tasks.CreateInput, kind, resolution string, quantity int64) string {
	if supplied := strings.TrimSpace(input.RequestHash); supplied != "" {
		return supplied
	}
	paramsJSON := ""
	if len(input.Params) > 0 {
		if encoded, err := json.Marshal(input.Params); err == nil {
			paramsJSON = string(encoded)
		}
	}
	return tasks.TaskSubmitRequestHash(input.WorkspaceID, kind, input.MediaAssetIDs, input.Prompt, paramsJSON, quantity, resolution, input.BoardID)
}

func cloneParams(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mapAssetGateError(err error) error {
	if errors.Is(err, tasks.ErrMediaArchived) {
		return tasks.ErrMediaArchived
	}
	if errors.Is(err, tasks.ErrMediaNotReady) {
		return tasks.ErrMediaNotReady
	}
	return tasks.ErrMediaNotFound
}

// Get returns one task by workspace and task public id.
func (s *Service) Get(ctx context.Context, workspaceID, taskID string) (*tasks.WorkspaceTask, error) {
	if s.taskRead == nil {
		return nil, tasks.ErrNotFound
	}
	return s.taskRead.GetTask(ctx, workspaceID, taskID)
}

// List returns tasks for a workspace with pagination.
func (s *Service) List(ctx context.Context, workspaceID string, limit, offset int) ([]*tasks.WorkspaceTask, error) {
	if s.taskRead == nil {
		return nil, tasks.ErrNotFound
	}
	return s.taskRead.ListTasks(ctx, workspaceID, limit, offset)
}

// Cancel flips pending/running to cancelled atomically with Release.
func (s *Service) Cancel(ctx context.Context, workspaceID, taskID string, actorUserID int64, reason string) (*tasks.WorkspaceTask, bool, error) {
	if s.tracer != nil {
		var end func(err error)
		ctx, end = s.tracer.Start(ctx, "generation.terminal.settlement.cancel")
		defer end(nil)
	}
	if s.taskRead == nil || s.taskLifecycle == nil {
		return nil, false, tasks.ErrNotFound
	}
	return s.cancelInTx(ctx, workspaceID, taskID, actorUserID, reason)
}

// Complete flips running to completed atomically with Capture.
func (s *Service) Complete(ctx context.Context, workspaceID, taskID string, outputs []tasks.OutputRef) (*tasks.WorkspaceTask, error) {
	if s.tracer != nil {
		var end func(err error)
		ctx, end = s.tracer.Start(ctx, "generation.terminal.settlement.complete")
		defer end(nil)
	}
	if s.taskLifecycle == nil {
		return nil, tasks.ErrNotFound
	}
	for _, ref := range outputs {
		if err := tasks.ValidateOutputRef(ref); err != nil {
			return nil, err
		}
	}
	if err := s.verifyOutputAssets(ctx, workspaceID, outputs); err != nil {
		return nil, err
	}
	return s.completeInTx(ctx, workspaceID, taskID, outputs)
}

// Fail flips running/pending to failed atomically with Release.
func (s *Service) Fail(ctx context.Context, workspaceID, taskID, errorMsg string) (*tasks.WorkspaceTask, error) {
	if s.tracer != nil {
		var end func(err error)
		ctx, end = s.tracer.Start(ctx, "generation.terminal.settlement.fail")
		defer end(nil)
	}
	if s.taskLifecycle == nil {
		return nil, tasks.ErrNotFound
	}
	return s.failInTx(ctx, workspaceID, taskID, errorMsg)
}

// Retry consumes one retry budget entry (reset + new execution token).
func (s *Service) Retry(ctx context.Context, workspaceID, taskID string, retryable bool) (*tasks.WorkspaceTask, error) {
	if s.tracer != nil {
		var end func(err error)
		ctx, end = s.tracer.Start(ctx, "generation.terminal.settlement.retry")
		defer end(nil)
	}
	if s.taskRead == nil || s.taskLifecycle == nil {
		return nil, tasks.ErrNotFound
	}
	current, err := s.taskRead.GetTask(ctx, workspaceID, taskID)
	if err != nil {
		return nil, err
	}
	if current.IsTerminal() {
		return nil, tasks.ErrStateConflict
	}
	if !tasks.ShouldRetry(current.RetryCount, retryable) {
		return nil, tasks.ErrStateConflict
	}
	token, err := tasks.NewExecutionToken()
	if err != nil {
		return nil, err
	}
	return s.retryInTx(ctx, workspaceID, taskID, token)
}

func (s *Service) verifyOutputAssets(ctx context.Context, workspaceID string, outputs []tasks.OutputRef) error {
	if s.assets == nil {
		return nil
	}
	for i, ref := range outputs {
		if ref.Kind != tasks.OutputKindAsset {
			continue
		}
		info, err := s.assets.GetAsset(ctx, workspaceID, ref.AssetID)
		if err != nil {
			return fmt.Errorf("workspacetasks: output asset: %w", mapAssetGateError(err))
		}
		if info != nil {
			if outputs[i].Width == 0 && info.Width > 0 {
				outputs[i].Width = info.Width
			}
			if outputs[i].Height == 0 && info.Height > 0 {
				outputs[i].Height = info.Height
			}
			if outputs[i].MIMEType == "" && info.MIMEType != "" {
				outputs[i].MIMEType = info.MIMEType
			}
		}
	}
	return nil
}
