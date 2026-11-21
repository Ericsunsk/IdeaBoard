package worker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ideaboard/api/internal/prompts"
	runtimeports "ideaboard/api/internal/runtime/ports"
	taskservice "ideaboard/api/internal/tasking/application/task"
	tasks "ideaboard/api/internal/tasking/domain"
)

type deadlineDispatchWorkerClientFunc func(context.Context, runtimeports.WorkerRequest) (runtimeports.WorkerResponse, error)

func (f deadlineDispatchWorkerClientFunc) Do(ctx context.Context, req runtimeports.WorkerRequest) (runtimeports.WorkerResponse, error) {
	return f(ctx, req)
}

// Exercise the real executor, byte acceptance, workspace materializer, renderer,
// and settlement. Only the external Worker and the short test budget are faked.
func TestWorkspaceBatchDeadlineSettlesAcceptedOutputs(t *testing.T) {
	for accepted := 0; accepted < 4; accepted++ {
		t.Run(fmt.Sprint(accepted), func(t *testing.T) {
			allowLiveDispatchLoopback(t)
			env := newLiveDispatchEnv()
			canonical, err := taskservice.CompileWorkspaceParams("generation", "a fake test sunset", map[string]interface{}{"output_count": 4}, prompts.FeaturePromptRuntimeConfig{})
			if err != nil {
				t.Fatal(err)
			}
			created, err := env.service.Create(t.Context(), tasks.CreateInput{
				WorkspaceID: "ws_test", OwnerUserID: 7, Service: "generation",
				CanonicalParams: canonical,
			}, "deadline-"+t.Name())
			if err != nil {
				t.Fatal(err)
			}
			created = pinLiveDispatchSnapshot(t, env, created, "openai/fake-test-image")
			transfer := newLiveDispatchTransfer()
			server := liveDispatchWorkerServer(t, transfer)
			defer server.Close()
			factory := liveDispatchWorkerFactory(server.URL)
			calls := 0
			worker := env.liveWorker(t, func(ctx context.Context) (runtimeports.WorkerClient, error) {
				client, err := factory(ctx)
				if err != nil {
					return nil, err
				}
				return deadlineDispatchWorkerClientFunc(func(ctx context.Context, req runtimeports.WorkerRequest) (runtimeports.WorkerResponse, error) {
					calls++
					if calls > accepted {
						<-ctx.Done()
						return runtimeports.WorkerResponse{}, ctx.Err()
					}
					return client.Do(ctx, req)
				}), nil
			}, transfer)
			provider := worker.provider
			worker.provider = WorkspaceTaskGenerateFunc(func(parent, executionCtx context.Context, task *tasks.WorkspaceTask) ([]tasks.OutputRef, error) {
				// Shorten the real claim budget in the test without changing
				// the production timeout contract or detaching parent cancellation.
				budget, cancel := context.WithTimeout(executionCtx, 500*time.Millisecond)
				defer cancel()
				return provider.Generate(parent, budget, task)
			})
			if err := worker.Work(t.Context(), workerJobFor(created)); err != nil {
				t.Fatal(err)
			}
			stored, err := env.store.GetTask(t.Context(), created.WorkspaceID, created.ID)
			if err != nil {
				t.Fatal(err)
			}
			if calls != accepted+1 {
				t.Fatalf("calls=%d, want %d without model fallback", calls, accepted+1)
			}
			if accepted == 0 {
				if stored.Status != tasks.StatusPending || stored.RetryCount != 1 || env.queue.count() != 2 {
					t.Fatalf("task=%+v enqueues=%d, want timeout requeue", stored, env.queue.count())
				}
				return
			}
			if stored.Status != tasks.StatusCompleted || stored.RetryCount != 0 || len(stored.OutputRefs) != accepted || env.queue.count() != 1 {
				t.Fatalf("task=%+v enqueues=%d, want partial completion without requeue", stored, env.queue.count())
			}
			env.holder.mu.Lock()
			captured := env.holder.captures["hold_"+created.ID]
			released := env.holder.releases["hold_"+created.ID]
			env.holder.mu.Unlock()
			if captured != int64(10*accepted) || released != int64(10*(4-accepted)) {
				t.Fatalf("captured=%d released=%d, want %d/%d", captured, released, 10*accepted, 10*(4-accepted))
			}
			for i, output := range stored.OutputRefs {
				if output.Kind != tasks.OutputKindAsset || output.ResultIndex != i {
					t.Fatalf("output[%d]=%+v, want ordered ingested asset", i, output)
				}
				if _, err := env.media.GetAsset(t.Context(), created.WorkspaceID, output.AssetID); err != nil {
					t.Fatalf("accepted asset not durable: %v", err)
				}
			}
			if err := worker.Work(t.Context(), workerJobFor(created)); err != nil || calls != accepted+1 {
				t.Fatalf("redelivery calls=%d err=%v; completed task must not regenerate", calls, err)
			}
		})
	}
}
