package worker

// Phase-boundary tests (#860): Claim, Execution, and Retry+Settlement
// state transitions verified independently of the full Work loop. The
// loop-level twins live in workspace_task_worker_test.go (success,
// deterministic failure, requeue, exhaustion) and
// workspace_task_snapshot_fallback_test.go (fallback at-most-once,
// taxonomy); these tests pin the seams between phases.

import (
	"context"
	"fmt"
	"testing"

	"ideaboard/api/internal/generation/routing"
	"ideaboard/api/internal/tasking/workspacetasks"
)

// A job carrying no execution token still claims (the claim phase mints a
// fresh lease) and executes to completion.
func TestWorkspaceTaskClaimMintsLeaseWhenJobTokenEmpty(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	created = pinWorkerSnapshot(t, env.store, created, workerTestSnapshot(t, "render", []string{"openrouter/only-model"}))

	called := false
	worker := env.newWorker(WorkspaceTaskGenerateFunc(func(ctx context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
		called = true
		return workerTestRemoteOutput(), nil
	}), nil)

	job := workerJobFor(created)
	job.Args.ExecutionToken = ""
	if err := worker.Work(ctx, job); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if !called {
		t.Fatal("provider not called, want claim with a minted lease to proceed")
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != workspacetasks.StatusCompleted {
		t.Fatalf("status = %q, want completed", got.Status)
	}
}

// A provider send yielding more than MaxOutputsPerTask refs is a
// deterministic execution failure: terminal fail, full hold released, no
// retry burn.
func TestWorkspaceTaskGenerateRejectsOversizeOutputs(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	created = pinWorkerSnapshot(t, env.store, created, workerTestSnapshot(t, "render", []string{"openrouter/only-model"}))

	refs := make([]workspacetasks.OutputRef, 0, workspacetasks.MaxOutputsPerTask+1)
	for i := 0; i <= workspacetasks.MaxOutputsPerTask; i++ {
		refs = append(refs, workspacetasks.OutputRef{
			Kind: workspacetasks.OutputKindRemote, URL: fmt.Sprintf("https://cdn.example.com/oversize-%d.png", i),
			ChecksumSHA256: workerTestRemoteChecksum, Role: "result", ResultIndex: i,
		})
	}
	worker := env.newWorker(WorkspaceTaskGenerateFunc(func(ctx context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
		return refs, nil
	}), nil)

	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != workspacetasks.StatusFailed {
		t.Fatalf("status = %q, want failed for oversize outputs", got.Status)
	}
	env.holder.mu.Lock()
	released := env.holder.releases["hold_"+created.ID]
	env.holder.mu.Unlock()
	if released != 10 {
		t.Fatalf("released = %d, want 10 (full hold back, no retry burn)", released)
	}
	if n := env.queue.count(); n != 1 {
		t.Fatalf("enqueues = %d, want 1 (submit only, no requeue)", n)
	}
}

// phaseStubSettler records terminal-settlement calls for the
// retry-vs-terminal matrix below.
type phaseStubSettler struct {
	completeCalls int
	failCalls     int
	retryCalls    int
}

func (s *phaseStubSettler) Complete(_ context.Context, _, _ string, _ []workspacetasks.OutputRef) (*workspacetasks.WorkspaceTask, error) {
	s.completeCalls++
	return &workspacetasks.WorkspaceTask{}, nil
}

func (s *phaseStubSettler) Fail(_ context.Context, _, _, _ string) (*workspacetasks.WorkspaceTask, error) {
	s.failCalls++
	return &workspacetasks.WorkspaceTask{}, nil
}

func (s *phaseStubSettler) Retry(_ context.Context, _, _ string, _ bool) (*workspacetasks.WorkspaceTask, error) {
	s.retryCalls++
	return &workspacetasks.WorkspaceTask{}, nil
}

// phaseStubStore satisfies the constructor; the converge path never reads.
type phaseStubStore struct{}

func (phaseStubStore) GetTask(_ context.Context, _, _ string) (*workspacetasks.WorkspaceTask, error) {
	return nil, workspacetasks.ErrNotFound
}

func (phaseStubStore) ClaimTask(_ context.Context, _, _, _ string) (*workspacetasks.WorkspaceTask, error) {
	return nil, workspacetasks.ErrStateConflict
}

// convergeSendFailure routes purely on (retryable, budget): retryable with
// budget requeues through Retry, exhausted budget or deterministic failures
// converge through Fail. No store, no provider, no River involved.
func TestConvergeSendFailureRetryVsTerminalMatrix(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name      string
		failure   routing.AttemptFailure
		retryCnt  int
		wantRetry int
		wantFail  int
	}{
		{"transient with budget requeues", routing.AttemptFailure{HTTPStatus: 503}, 0, 1, 0},
		{"network error with budget requeues", routing.AttemptFailure{NetworkError: true}, 1, 1, 0},
		{"transient with exhausted budget fails", routing.AttemptFailure{HTTPStatus: 503}, workspacetasks.MaxProviderRequeues, 0, 1},
		{"deterministic with budget fails", routing.AttemptFailure{InvalidParams: true}, 0, 0, 1},
		{"unmarked failure fails closed", routing.AttemptFailure{}, 0, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settler := &phaseStubSettler{}
			worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{Store: phaseStubStore{}, Settler: settler})
			if err != nil {
				t.Fatalf("NewWorkspaceTaskWorker: %v", err)
			}
			claimed := &workspacetasks.WorkspaceTask{
				ID: "task_phase1", WorkspaceID: "ws_test", Service: "render", RetryCount: tc.retryCnt,
			}
			send := newWorkspaceSendTracker(nil, claimed.ID, 0, "render").
				startSend(ctx, "openrouter/model-a", workspacetasks.AttemptRetryLayerGateway)
			genErr := fmt.Errorf("provider send failed: %s", tc.name)
			if err := worker.convergeSendFailure(ctx, send, claimed, tc.failure, genErr, false); err != nil {
				t.Fatalf("convergeSendFailure: %v", err)
			}
			if settler.retryCalls != tc.wantRetry || settler.failCalls != tc.wantFail {
				t.Fatalf("retry/fail = (%d, %d), want (%d, %d)",
					settler.retryCalls, settler.failCalls, tc.wantRetry, tc.wantFail)
			}
			if settler.completeCalls != 0 {
				t.Fatalf("complete calls = %d, want 0 (failure path never completes)", settler.completeCalls)
			}
		})
	}
}

// failTerminal on an already-converged task drops the job (nil) instead of
// erroring: a cancel landing mid-flight must not ride River backoff.
func TestFailTerminalDropsConvergedTask(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	if _, err := env.service.Fail(ctx, "ws_test", created.ID, "first failure"); err != nil {
		t.Fatalf("service Fail: %v", err)
	}
	worker := env.newWorker(nil, nil)
	current, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if err := worker.failTerminal(ctx, current, "late failure after converge"); err != nil {
		t.Fatalf("failTerminal on converged task = %v, want drop nil", err)
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != workspacetasks.StatusFailed {
		t.Fatalf("status = %q, want failed (unchanged)", got.Status)
	}
}
