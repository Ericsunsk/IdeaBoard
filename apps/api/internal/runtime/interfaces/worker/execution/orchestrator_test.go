package execution

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	runtimeports "ideaboard/api/internal/runtime/ports"
	taskexecution "ideaboard/api/internal/tasking/application/execution"
	tasks "ideaboard/api/internal/tasking/domain"
	runtimecontracts "ideaboard/api/internal/tasking/ports/runtime"
)

func newExecutionFailureError(stage, category string, err error) error {
	return runtimeports.NewExecutionFailureError(stage, category, err)
}

func TestUserFacingProviderErrorRecognizesWrappedDeadlineExceeded(t *testing.T) {
	err := fmt.Errorf("调用 API 失败 (服务端点: gptnb): %w",
		fmt.Errorf("HTTP 请求失败: %w", context.DeadlineExceeded),
	)

	got := userFacingProviderError(err)
	if got != "上游图像服务响应超时" {
		t.Fatalf("expected timeout message, got %q", got)
	}
}

func TestUserFacingProviderErrorFallsBackForNonTimeout(t *testing.T) {
	err := fmt.Errorf("调用 API 失败 (服务端点: gptnb): %w", fmt.Errorf("HTTP 请求失败: boom"))

	got := userFacingProviderError(err)
	if got != "上游图像服务调用失败" {
		t.Fatalf("expected generic provider message, got %q", got)
	}
}

func TestUserFacingProviderErrorPreservesInputInvalidDetails(t *testing.T) {
	err := newExecutionFailureError(
		tasks.ExecutionStageAssigned,
		tasks.FailureCategoryInputInvalid,
		errors.New("图案生成不需要输入图片，请移除图片输入后重试"),
	)

	got := userFacingProviderError(err)
	if got != "图案生成不需要输入图片，请移除图片输入后重试" {
		t.Fatalf("expected specific input validation message, got %q", got)
	}
}

type prepareFakeTaskStore struct {
	converged []runtimecontracts.TaskConvergenceIntent
	task      *tasks.Task
	claimErr  error
}

func (f *prepareFakeTaskStore) ConvergeTask(_ context.Context, intent runtimecontracts.TaskConvergenceIntent) (int64, error) {
	f.converged = append(f.converged, intent)
	return 1, nil
}

func (f *prepareFakeTaskStore) GetByTaskID(context.Context, string) (*tasks.Task, error) {
	if f.task == nil {
		return nil, errors.New("not implemented")
	}
	return f.task, nil
}

func (f *prepareFakeTaskStore) GetActiveExecutionAttemptByTaskID(context.Context, string) (*runtimecontracts.ActiveTaskExecution, error) {
	return nil, errors.New("not implemented")
}

func (f *prepareFakeTaskStore) ClaimAndStartExecutionAttempt(context.Context, tasks.ExecutionAttemptStart) (int32, error) {
	if f.claimErr != nil {
		return 0, f.claimErr
	}
	return 1, nil
}

func (f *prepareFakeTaskStore) PatchExecutionMetrics(context.Context, tasks.TaskExecutionMetricsPatch) error {
	return errors.New("not implemented")
}

func (f *prepareFakeTaskStore) PatchExecutionSnapshot(context.Context, tasks.TaskExecutionSnapshot) (int64, error) {
	return 0, errors.New("not implemented")
}

func (f *prepareFakeTaskStore) FinalizeExecutionAttempt(context.Context, tasks.ExecutionAttemptFinish) (int64, error) {
	return 0, errors.New("not implemented")
}

func (f *prepareFakeTaskStore) MarkTaskCancelled(context.Context, tasks.TaskCancellationSnapshot, *tasks.TaskAttemptFinish) (int64, error) {
	return 0, errors.New("not implemented")
}

type prepareFakeCancelStore struct {
	cancelled bool
	err       error
}

func (f *prepareFakeCancelStore) MarkCancelled(context.Context, string, time.Duration) error {
	return nil
}

func (f *prepareFakeCancelStore) IsCancelled(context.Context, string) (bool, error) {
	return f.cancelled, f.err
}

func (f *prepareFakeCancelStore) ClearCancelled(context.Context, string) error {
	return nil
}

type prepareFakeModelResolver struct {
	model string
	err   error
}

func (f *prepareFakeModelResolver) ResolveModel(context.Context, string) (string, error) {
	return f.model, f.err
}

type prepareFakeEnqueuer struct {
	calls int
}

func (f *prepareFakeEnqueuer) Enqueue(context.Context, *tasks.TaskPayload) error {
	f.calls++
	return nil
}

func newPrepareTestOrchestrator(store *prepareFakeTaskStore, cancel *prepareFakeCancelStore, resolver *prepareFakeModelResolver, enqueuer *prepareFakeEnqueuer) *Orchestrator {
	return NewOrchestrator(enqueuer, taskexecution.NewService(store, cancel), resolver, nil)
}

func TestPrepareUnparseableParamsFailsTaskWithoutEnqueue(t *testing.T) {
	store := &prepareFakeTaskStore{}
	enqueuer := &prepareFakeEnqueuer{}
	orchestrator := newPrepareTestOrchestrator(store, &prepareFakeCancelStore{}, &prepareFakeModelResolver{}, enqueuer)

	err := orchestrator.Execute(context.Background(), &tasks.TaskPayload{TaskID: "bad-params-task"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if enqueuer.calls != 0 {
		t.Fatalf("enqueue calls = %d, want 0 for deterministically bad params", enqueuer.calls)
	}
	if len(store.converged) != 1 || store.converged[0].Kind != runtimecontracts.TaskConvergenceFailed {
		t.Fatalf("converged intents = %+v, want one failed convergence", store.converged)
	}
}

func TestPrepareModelResolverErrorReturnsErrorWithoutEnqueue(t *testing.T) {
	store := &prepareFakeTaskStore{}
	enqueuer := &prepareFakeEnqueuer{}
	resolverErr := errors.New("settings DB unavailable")
	orchestrator := newPrepareTestOrchestrator(store, &prepareFakeCancelStore{}, &prepareFakeModelResolver{err: resolverErr}, enqueuer)

	err := orchestrator.Execute(context.Background(), &tasks.TaskPayload{
		TaskID: "model-error-task",
		Params: `{"schema_version":5,"feature":"generation"}`,
	})
	if !errors.Is(err, resolverErr) {
		t.Fatalf("Execute error = %v, want resolver error for River backoff", err)
	}
	if enqueuer.calls != 0 {
		t.Fatalf("enqueue calls = %d, want 0 when returning error", enqueuer.calls)
	}
	if len(store.converged) != 0 {
		t.Fatalf("converged intents = %+v, want none on transient prepare failure", store.converged)
	}
}

func TestPrepareCancelCheckErrorReturnsErrorWithoutEnqueue(t *testing.T) {
	store := &prepareFakeTaskStore{}
	enqueuer := &prepareFakeEnqueuer{}
	cancelErr := errors.New("cancel store unavailable")
	orchestrator := newPrepareTestOrchestrator(store, &prepareFakeCancelStore{err: cancelErr}, &prepareFakeModelResolver{}, enqueuer)

	err := orchestrator.Execute(context.Background(), &tasks.TaskPayload{TaskID: "cancel-check-task"})
	if !errors.Is(err, cancelErr) {
		t.Fatalf("Execute error = %v, want cancel-store error for River backoff", err)
	}
	if enqueuer.calls != 0 {
		t.Fatalf("enqueue calls = %d, want 0 when returning error", enqueuer.calls)
	}
}

func retryableProviderError() error {
	return fmt.Errorf("调用 API 失败: %w", context.DeadlineExceeded)
}

func TestRetryableProviderFailureRequeuesForRiverBackoff(t *testing.T) {
	store := &prepareFakeTaskStore{task: &tasks.Task{TaskID: "retry-task", RetryCount: 0}}
	orchestrator := newPrepareTestOrchestrator(store, &prepareFakeCancelStore{}, &prepareFakeModelResolver{}, &prepareFakeEnqueuer{})
	attempt := newAttemptContext(&tasks.TaskPayload{TaskID: "retry-task"}, "openai/gpt-image-1", "tok-retry", 1, time.Now())
	machine := newPhaseMachine("retry-task")
	for _, next := range []phase{phaseAssigned, phaseProvider} {
		if err := machine.advance(next); err != nil {
			t.Fatalf("advance to %s: %v", next, err)
		}
	}

	providerErr := retryableProviderError()
	err := orchestrator.handleProviderFailureConvergence(context.Background(), attempt, providerErr, machine)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want provider error for River backoff", err)
	}
	if len(store.converged) != 1 || store.converged[0].Kind != runtimecontracts.TaskConvergenceRequeued {
		t.Fatalf("converged intents = %+v, want one retry-counting requeue convergence", store.converged)
	}
	if store.converged[0].ExecutionToken != "tok-retry" {
		t.Fatalf("reset token = %q, want fenced token", store.converged[0].ExecutionToken)
	}
}

func TestExhaustedRetryableFailureFinalFails(t *testing.T) {
	store := &prepareFakeTaskStore{task: &tasks.Task{TaskID: "exhausted-task", RetryCount: maxProviderFailureRequeues}}
	orchestrator := newPrepareTestOrchestrator(store, &prepareFakeCancelStore{}, &prepareFakeModelResolver{}, &prepareFakeEnqueuer{})
	attempt := newAttemptContext(&tasks.TaskPayload{TaskID: "exhausted-task"}, "openai/gpt-image-1", "tok-done", 3, time.Now())
	machine := newPhaseMachine("exhausted-task")
	for _, next := range []phase{phaseAssigned, phaseProvider} {
		if err := machine.advance(next); err != nil {
			t.Fatalf("advance to %s: %v", next, err)
		}
	}

	err := orchestrator.handleProviderFailureConvergence(context.Background(), attempt, retryableProviderError(), machine)
	if err != nil {
		t.Fatalf("error = %v, want nil on terminal convergence", err)
	}
	if len(store.converged) != 1 || store.converged[0].Kind != runtimecontracts.TaskConvergenceFailed {
		t.Fatalf("converged intents = %+v, want one failed convergence", store.converged)
	}
}
