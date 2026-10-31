package worker

// Model-fallback Prometheus observations (#852): the workspace worker
// reports exactly one IncModelFallback per completed fallback send,
// with the send outcome, through the narrow ModelFallbackObserver seam.
// Nil metrics keeps unit paths uninstrumented (covered by the pre-#852
// fallback tests, which never wire Metrics).

import (
	"context"
	"sync"
	"testing"

	tasks "ideaboard/api/internal/tasking/domain"
)

type fallbackObservation struct {
	service string
	failed  bool
}

type recordingFallbackObserver struct {
	mu    sync.Mutex
	calls []fallbackObservation
}

func (o *recordingFallbackObserver) IncModelFallback(service string, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.calls = append(o.calls, fallbackObservation{service: service, failed: err != nil})
}

func (o *recordingFallbackObserver) snapshot() []fallbackObservation {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]fallbackObservation(nil), o.calls...)
}

func newObservedWorker(env *workerTestEnv, provider WorkspaceTaskProvider, observer ModelFallbackObserver) *WorkspaceTaskWorker {
	worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: env.store, Settler: env.service,
		Assets:   &workerTestAssets{ready: map[string]bool{}, archived: map[string]bool{}},
		Provider: provider, Attempts: env.attempts, Metrics: observer,
	})
	if err != nil {
		panic(err)
	}
	return worker
}

func TestWorkspaceTaskWorkerObservesSuccessfulFallback(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	snap := workerTestSnapshot(t, "render", []string{"openrouter/model-a", "openrouter/model-b"})
	created = pinWorkerSnapshot(t, env.store, created, snap)

	observer := &recordingFallbackObserver{}
	var seen []string
	worker := newObservedWorker(env, modelObservingProvider(&seen, func(model string) ([]tasks.OutputRef, error) {
		if model == snap.SelectedModel {
			return nil, workerUpstreamError(503)
		}
		return workerTestRemoteOutput(), nil
	}), observer)
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	calls := observer.snapshot()
	if len(calls) != 1 {
		t.Fatalf("fallback observations = %d, want exactly 1", len(calls))
	}
	if calls[0].service != "render" || calls[0].failed {
		t.Fatalf("observation = %+v, want {render, success}", calls[0])
	}
}

func TestWorkspaceTaskWorkerObservesFailedFallback(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	snap := workerTestSnapshot(t, "render", []string{"openrouter/model-a", "openrouter/model-b"})
	created = pinWorkerSnapshot(t, env.store, created, snap)

	observer := &recordingFallbackObserver{}
	var seen []string
	worker := newObservedWorker(env, modelObservingProvider(&seen, func(string) ([]tasks.OutputRef, error) {
		return nil, workerUpstreamError(503)
	}), observer)
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work must converge (nil), got: %v", err)
	}
	calls := observer.snapshot()
	if len(calls) != 1 {
		t.Fatalf("fallback observations = %d, want exactly 1", len(calls))
	}
	if calls[0].service != "render" || !calls[0].failed {
		t.Fatalf("observation = %+v, want {render, error}", calls[0])
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != tasks.StatusPending || got.RetryCount != 1 {
		t.Fatalf("task = (%q, retry %d), want (pending, 1) river requeue", got.Status, got.RetryCount)
	}
}

func TestWorkspaceTaskWorkerSkipsObservationWithoutFallback(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	snap := workerTestSnapshot(t, "render", []string{"openrouter/model-a", "openrouter/model-b"})
	created = pinWorkerSnapshot(t, env.store, created, snap)

	observer := &recordingFallbackObserver{}
	var seen []string
	worker := newObservedWorker(env, modelObservingProvider(&seen, func(string) ([]tasks.OutputRef, error) {
		return nil, workerUpstreamError(400)
	}), observer)
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work must converge (nil), got: %v", err)
	}
	if calls := observer.snapshot(); len(calls) != 0 {
		t.Fatalf("fallback observations = %+v, want none for deterministic failure", calls)
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != tasks.StatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
}
