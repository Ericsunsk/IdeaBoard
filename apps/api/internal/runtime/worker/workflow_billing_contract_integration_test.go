package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/prompts"
	"ideaboard/api/internal/runtime/billingregistry"
	"ideaboard/api/internal/tasking/taskbilling"
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/taskruntime"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskservice"
	"ideaboard/api/internal/tasking/taskstore"
	"ideaboard/api/internal/tasking/tasksubmission"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/workflow"

	"github.com/jackc/pgx/v5"
)

func TestAIImageWorkflowBillingContractUsesCurrentProviderPoolAndConvergesFacts(t *testing.T) {
	ctx := t.Context()
	runID := time.Now().UnixNano()
	queueName := fmt.Sprintf("workflow_billing_contract_%d", runID)

	db, pool := testdb.OpenWithPool(t)
	if err := MigrateRiver(ctx, pool); err != nil {
		t.Fatalf("migrate River schema: %v", err)
	}

	creditService := credits.NewService(pool)
	billingService := billing.NewService(pool)
	taskBilling := taskbilling.NewService(billingService, creditService)
	billingRequest, err := taskbilling.AIImageRequest(workflow.FeatureSlugPatternGen, 1)
	if err != nil {
		t.Fatal(err)
	}
	sku, err := taskbilling.ResolveSKU(billingRequest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := billingService.PublishPrice(ctx, workerPriceInput(sku, "Pattern Generation", 5)); err != nil {
		t.Fatalf("publish price: %v", err)
	}
	seedWorkflowContractWallet(t, pool, creditService, 42, 20, runID)

	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := taskstore.NewRepository(db)

	assigner := taskqueue.NewPostgresTaskAssigner(db)
	taskRuntime := taskruntime.NewService(accountRepo, taskRepo, assigner, newFakeCancelStoreForRiver())
	var providerCalls atomic.Int32
	var failedProviderCalls atomic.Int32
	var successfulProviderCalls atomic.Int32
	var failedProviderID atomic.Int64
	var successfulProviderID atomic.Int64
	riverQueue := taskqueue.NewRiverQueueWithClient(nil, db, nil, queueName, 1)
	orchestrator := newTaskExecutionOrchestrator(
		accountRepo,
		riverQueue,
		taskRuntime,
		func(_ context.Context, runtimeAccount *accounts.ApiAccount, _ *tasks.TaskPayload, assignment taskqueue.TaskAssignment) (providerCallOutput, error) {
			providerCalls.Add(1)
			switch runtimeAccount.ID {
			case failedProviderID.Load():
				failedProviderCalls.Add(1)
				if assignment.ResolvedModel != "contract-failed-model" {
					return providerCallOutput{}, fmt.Errorf("first resolved model=%q, want contract-failed-model", assignment.ResolvedModel)
				}
				if _, err := db.ExecContext(context.Background(), `UPDATE api_accounts SET status = 0 WHERE id = $1`, runtimeAccount.ID); err != nil {
					return providerCallOutput{}, fmt.Errorf("disable failed Provider: %w", err)
				}
				return providerCallOutput{}, newUpstreamHTTPError(429, "contract failover")
			case successfulProviderID.Load():
				successfulProviderCalls.Add(1)
				if assignment.ResolvedModel != "contract-success-model" {
					return providerCallOutput{}, fmt.Errorf("second resolved model=%q, want contract-success-model", assignment.ResolvedModel)
				}
			default:
				return providerCallOutput{}, fmt.Errorf("unexpected execution Provider %d", runtimeAccount.ID)
			}
			return providerOutputJSON(`{"status":"success","image_url":"https://example.com/workflow-contract.png"}`), nil
		},
	)
	riverWorker := newRiverTaskWorkerWithProcessor(newTaskProcessor(assigner, taskRuntime, orchestrator.execute))
	riverClient, err := NewRiverClient(pool, riverWorker, RiverServerOptions{
		QueueName: queueName, Concurrency: 1, MaxRetry: 1, JobTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("create River client: %v", err)
	}
	riverQueue.SetClient(riverClient)

	registry, err := billing.NewBillableItemRegistry(billingregistry.WorkflowItems())
	if err != nil {
		t.Fatalf("build Billable Item registry: %v", err)
	}
	submissionStore := tasksubmission.NewStore(pool, taskRepo, riverClient, queueName, 1, taskBilling)
	taskService := taskservice.NewService(taskservice.ServiceDeps{
		TaskIntake: taskservice.NewTaskIntake(taskservice.TaskIntakeDeps{
			Accounts: accountRepo,
			Prompts:  prompts.NewFeaturePromptRepository(db),
			Media:    workflowContractMediaValidator{},
		}),
		TaskRepo:            taskRepo,
		SubmissionStore:     submissionStore,
		TaskBilling:         taskBilling,
		AdminBillingHistory: taskRepo,
		AdminTaskTrace:      taskRepo,
		AdminTaskListRead:   taskRepo,
		BillableItems:       registry,
	}, taskservice.ServiceOptions{})

	request := &tasks.SubmitTaskRequest{
		Prompt:         "repeatable geometric pattern",
		Feature:        workflow.FeatureSlugPatternGen,
		ClientActionID: fmt.Sprintf("workflow-contract-%d", runID),
	}
	if _, err := taskService.CheckSubmitTaskReadiness(ctx, 42, request, 1); err == nil {
		t.Fatal("readiness accepted a Workflow with no enabled Provider")
	} else {
		var submissionErr *taskservice.SubmissionError
		if !errors.As(err, &submissionErr) || submissionErr.Code != "provider_unavailable" {
			t.Fatalf("no-Provider readiness error=%v, want provider_unavailable", err)
		}
	}

	submissionProvider := createRiverTestAccount(t, db, accountRepo, fmt.Sprintf("contract-before-submit-%d", runID))
	saturateWorkflowContractProvider(t, db, taskRepo, assigner, submissionProvider, runID)

	readiness, err := taskService.CheckSubmitTaskReadiness(ctx, 42, request, 1)
	if err != nil {
		t.Fatalf("readiness: %v", err)
	}
	if readiness.Billing == nil || readiness.Billing.TotalCredits != 5 || !readiness.Billing.Sufficient {
		t.Fatalf("readiness Billing=%+v, want sufficient 5 Ideas quote", readiness.Billing)
	}
	taskID, err := taskService.SubmitTask(ctx, 42, request)
	if err != nil {
		t.Fatalf("submit billed Workflow: %v", err)
	}
	if taskID == "" {
		t.Fatal("submitted task ID is empty")
	}
	assertWorkflowContractSubmissionFacts(t, db, taskID)
	assertRiverTaskBillingState(t, db, taskID, tasks.StatusPending, "held", 15, 5)

	if _, err := db.ExecContext(ctx, `UPDATE api_accounts SET status = 0 WHERE id = $1`, submissionProvider.ID); err != nil {
		t.Fatalf("disable submission-time Provider: %v", err)
	}
	failedProvider := createRiverTestAccount(t, db, accountRepo, fmt.Sprintf("contract-failover-first-%d", runID))
	setWorkflowContractProviderModel(t, db, failedProvider.ID, "contract-failed-model")
	successfulProvider := createRiverTestAccount(t, db, accountRepo, fmt.Sprintf("contract-failover-second-%d", runID))
	setWorkflowContractProviderModel(t, db, successfulProvider.ID, "contract-success-model")
	failedProviderID.Store(failedProvider.ID)
	successfulProviderID.Store(successfulProvider.ID)

	if err := riverClient.Start(ctx); err != nil {
		t.Fatalf("start River client: %v", err)
	}
	t.Cleanup(func() { stopRiverClientForSubmitToWorkerTest(t, riverClient) })

	waitForSubmitToRiverWorkerTaskStatus(t, taskRepo, taskID, tasks.StatusCompleted, 10*time.Second)
	waitForWorkflowContractRiverJobs(t, db, taskID, 2, 5*time.Second)
	if got := providerCalls.Load(); got != 2 {
		t.Fatalf("Provider calls=%d, want exactly 2", got)
	}
	if failedProviderCalls.Load() != 1 || successfulProviderCalls.Load() != 1 {
		t.Fatalf(
			"failed/successful Provider calls=%d/%d, want 1/1",
			failedProviderCalls.Load(),
			successfulProviderCalls.Load(),
		)
	}

	adminTask, err := taskService.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("load Admin Task detail: %v", err)
	}
	coreTask, err := taskRepo.GetByTaskID(ctx, taskID)
	if err != nil {
		t.Fatalf("load core Task: %v", err)
	}
	assertWorkflowContractTerminalFacts(t, db, coreTask, adminTask, failedProvider, successfulProvider)
}

type workflowContractMediaValidator struct{}

func (workflowContractMediaValidator) ValidateSubmitMediaRef(context.Context, int64, string) error {
	return nil
}

func saturateWorkflowContractProvider(
	t testing.TB,
	db *sql.DB,
	taskRepo *taskstore.Repository,
	assigner *taskqueue.PostgresTaskAssigner,
	provider *accounts.ApiAccount,
	runID int64,
) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), `UPDATE api_accounts SET max_concurrent = 1 WHERE id = $1`, provider.ID); err != nil {
		t.Fatalf("limit submission-time Provider capacity: %v", err)
	}
	taskID := fmt.Sprintf("workflow-contract-capacity-%d", runID)
	if err := taskRepo.Create(t.Context(), &tasks.Task{
		TaskID:      taskID,
		UserID:      1,
		Prompt:      "occupy Provider capacity",
		UserPrompt:  "occupy Provider capacity",
		Params:      `{"schema_version":1,"feature":"pattern-gen"}`,
		FeatureSlug: workflow.FeatureSlugPatternGen,
		Priority:    tasks.TaskPriorityDefault,
		Status:      tasks.StatusPending,
	}); err != nil {
		t.Fatalf("create capacity fixture Task: %v", err)
	}
	assignment, err := assigner.Assign(t.Context(), &tasks.TaskPayload{
		TaskID:     taskID,
		UserID:     1,
		Priority:   tasks.TaskPriorityDefault,
		RiverJobID: runID,
	})
	if err != nil {
		t.Fatalf("occupy Provider capacity: %v", err)
	}
	if assignment.Status != taskqueue.AssignStatusAssigned || assignment.AccountID != provider.ID {
		t.Fatalf("capacity fixture assignment=%+v, want Provider %d", assignment, provider.ID)
	}
}

func setWorkflowContractProviderModel(t testing.TB, db *sql.DB, accountID int64, model string) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), `
UPDATE api_account_models
SET model_name = $2
WHERE account_id = $1`, accountID, model); err != nil {
		t.Fatalf("set Provider %d model: %v", accountID, err)
	}
}

func waitForWorkflowContractRiverJobs(t testing.TB, db *sql.DB, taskID string, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var total, completed int
	for time.Now().Before(deadline) {
		err := db.QueryRowContext(t.Context(), `
SELECT COUNT(*), COUNT(*) FILTER (WHERE state = 'completed')
FROM river_job
WHERE args->'task'->>'task_id' = $1`, taskID).Scan(&total, &completed)
		if err == nil && total == want && completed == want {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("River jobs total/completed=%d/%d, want %d/%d", total, completed, want, want)
}

func seedWorkflowContractWallet(
	t testing.TB,
	pool interface {
		Begin(context.Context) (pgx.Tx, error)
	},
	service *credits.Service,
	userID, amount, runID int64,
) {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	if err := service.LifecycleInTx(tx).ProvisionWallet(t.Context(), userID); err != nil {
		t.Fatal(err)
	}
	creditAmount, err := credits.NewCreditAmount(amount)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.InTx(tx).Grant(t.Context(), credits.GrantCommand{
		UserID: userID,
		Amount: creditAmount,
		Metadata: credits.CommandMetadata{
			IdempotencyKey: fmt.Sprintf("workflow-contract-grant-%d", runID),
			Actor:          credits.Actor{Type: credits.ActorSystem, ID: "contract-test"},
			Source:         "test",
			Reason:         "seed Workflow billing contract",
			BusinessReference: credits.BusinessReference{
				Type: "test", ID: fmt.Sprintf("workflow-contract-%d", runID),
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func assertWorkflowContractSubmissionFacts(t testing.TB, db *sql.DB, taskID string) {
	t.Helper()
	var params string
	var accountID sql.NullInt64
	if err := db.QueryRowContext(t.Context(), `
SELECT params, account_id
FROM tasks
WHERE task_id = $1`, taskID).Scan(&params, &accountID); err != nil {
		t.Fatal(err)
	}
	if accountID.Valid {
		t.Fatalf("pending task snapshots account_id=%d", accountID.Int64)
	}
	var canonicalParams map[string]any
	if err := json.Unmarshal([]byte(params), &canonicalParams); err != nil {
		t.Fatalf("decode canonical params: %v", err)
	}
	for _, forbidden := range []string{"model", "requested_model", "canonical_model", "account_ids"} {
		if _, exists := canonicalParams[forbidden]; exists {
			t.Fatalf("canonical params snapshot forbidden %q: %#v", forbidden, canonicalParams)
		}
	}

	var riverArgs string
	if err := db.QueryRowContext(t.Context(), `
SELECT args::text
FROM river_job
WHERE args->'task'->>'task_id' = $1`, taskID).Scan(&riverArgs); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"model"`, `"requested_model"`, `"canonical_model"`, `"account_ids"`} {
		if strings.Contains(riverArgs, forbidden) {
			t.Fatalf("River payload snapshots %s: %s", forbidden, riverArgs)
		}
	}
}

func assertWorkflowContractTerminalFacts(
	t testing.TB,
	db *sql.DB,
	coreTask *tasks.Task,
	adminTask *taskservice.AdminTaskMonitorDetail,
	failedProvider *accounts.ApiAccount,
	successfulProvider *accounts.ApiAccount,
) {
	t.Helper()
	if coreTask.Status != tasks.StatusCompleted ||
		coreTask.RetryCount != 1 ||
		coreTask.ResolvedModel != "" ||
		coreTask.AccountID == nil ||
		*coreTask.AccountID != successfulProvider.ID {
		t.Fatalf("terminal Task=%+v, want completed with execution Account fact and no active model lease", coreTask)
	}
	if adminTask.Billing == nil ||
		adminTask.Billing.SettlementState != "captured" ||
		adminTask.Billing.TotalCredits != 5 ||
		!adminTask.Billing.FeatureExists {
		t.Fatalf("Admin Billing snapshot=%+v", adminTask.Billing)
	}
	if len(adminTask.ExecutionAttempts) != 2 {
		t.Fatalf("Admin execution attempts=%d, want 2: %+v", len(adminTask.ExecutionAttempts), adminTask.ExecutionAttempts)
	}
	failedAttempt := adminTask.ExecutionAttempts[0]
	if failedAttempt.AttemptNo != 1 ||
		failedAttempt.Status != tasks.TaskAttemptStatusRequeued ||
		failedAttempt.ProviderEndpointID != failedProvider.ProviderEndpointID ||
		failedAttempt.ResolvedModel != "contract-failed-model" ||
		failedAttempt.FailureCategory != tasks.FailureCategoryTimeout {
		t.Fatalf("first Admin execution attempt=%+v", failedAttempt)
	}
	successfulAttempt := adminTask.ExecutionAttempts[1]
	if successfulAttempt.AttemptNo != 2 ||
		successfulAttempt.Status != tasks.TaskAttemptStatusCompleted ||
		successfulAttempt.ProviderEndpointID != successfulProvider.ProviderEndpointID ||
		successfulAttempt.ResolvedModel != "contract-success-model" {
		t.Fatalf("second Admin execution attempt=%+v", successfulAttempt)
	}

	var holdCount, captureCount, releaseCount int
	if err := db.QueryRowContext(t.Context(), `
SELECT
  COUNT(*) FILTER (WHERE command_type = 'hold'),
  COUNT(*) FILTER (WHERE command_type = 'capture'),
  COUNT(*) FILTER (WHERE command_type = 'release')
FROM credit_transactions
WHERE business_reference_id = $1`, coreTask.TaskID).Scan(&holdCount, &captureCount, &releaseCount); err != nil {
		t.Fatal(err)
	}
	if holdCount != 1 || captureCount != 1 || releaseCount != 0 {
		t.Fatalf("Ledger hold/capture/release=%d/%d/%d, want 1/1/0", holdCount, captureCount, releaseCount)
	}
	assertRiverTaskBillingState(t, db, coreTask.TaskID, tasks.StatusCompleted, "captured", 15, 0)

	var snapshotTotal int64
	var settlementState string
	var attemptCount int
	var failedAttemptProviderID, successfulAttemptProviderID int64
	var failedAttemptModel, successfulAttemptModel string
	if err := db.QueryRowContext(t.Context(), `
SELECT snapshots.total_credits, settlements.state,
       COUNT(*) AS attempt_count,
       MIN(leases.provider_endpoint_id) FILTER (WHERE attempts.attempt_no = 1),
       MIN(leases.resolved_model) FILTER (WHERE attempts.attempt_no = 1),
       MIN(leases.provider_endpoint_id) FILTER (WHERE attempts.attempt_no = 2),
       MIN(leases.resolved_model) FILTER (WHERE attempts.attempt_no = 2)
FROM task_billing_snapshots snapshots
JOIN task_billing_settlements settlements USING (task_id)
JOIN task_attempts attempts USING (task_id)
JOIN task_leases leases ON leases.id = attempts.lease_id
WHERE snapshots.task_id = $1
GROUP BY snapshots.total_credits, settlements.state`, coreTask.TaskID).Scan(
		&snapshotTotal,
		&settlementState,
		&attemptCount,
		&failedAttemptProviderID,
		&failedAttemptModel,
		&successfulAttemptProviderID,
		&successfulAttemptModel,
	); err != nil {
		t.Fatal(err)
	}
	if snapshotTotal != adminTask.Billing.TotalCredits ||
		settlementState != adminTask.Billing.SettlementState ||
		attemptCount != 2 ||
		failedAttemptProviderID != failedAttempt.ProviderEndpointID ||
		failedAttemptModel != failedAttempt.ResolvedModel ||
		successfulAttemptProviderID != successfulAttempt.ProviderEndpointID ||
		successfulAttemptModel != successfulAttempt.ResolvedModel {
		t.Fatalf(
			"database/Admin facts snapshot=%d/%d settlement=%s/%s attempts=%d failed=%d/%d/%s/%s successful=%d/%d/%s/%s",
			snapshotTotal,
			adminTask.Billing.TotalCredits,
			settlementState,
			adminTask.Billing.SettlementState,
			attemptCount,
			failedAttemptProviderID,
			failedAttempt.ProviderEndpointID,
			failedAttemptModel,
			failedAttempt.ResolvedModel,
			successfulAttemptProviderID,
			successfulAttempt.ProviderEndpointID,
			successfulAttemptModel,
			successfulAttempt.ResolvedModel,
		)
	}
}
