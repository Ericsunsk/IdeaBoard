package providers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	platformworkermedia "ideaboard/api/internal/platform/workermedia"
	runtimeports "ideaboard/api/internal/runtime/ports"
	tasks "ideaboard/api/internal/tasking/domain"
	taskmedia "ideaboard/api/internal/tasking/ports/media"
	"ideaboard/api/internal/workflow"
)

func TestCallWorkerAPIBatchDeadlineKeepsAcceptedOutputs(t *testing.T) {
	for _, requested := range []int{2, 3, 4} {
		for deadlineAt := 1; deadlineAt <= requested; deadlineAt++ {
			t.Run(fmt.Sprintf("requested_%d/deadline_at_%d", requested, deadlineAt), func(t *testing.T) {
				fw := newFakeWorkerServer(t, buildTinyPNGBytes(t))
				defer fw.server.Close()
				parent := context.Background()
				execCtx, cancel := context.WithTimeout(parent, 500*time.Millisecond)
				defer cancel()
				stopped := make(chan struct{})
				fw.respond = func(taskID, _ string) (int, string) {
					if len(fw.capturedBodies) == deadlineAt {
						<-execCtx.Done()
						close(stopped)
						return 503, `{}`
					}
					return 0, ""
				}
				executor := workerTestExecutor(fw, nil)
				accepted := deadlineAt - 1
				items := make([]taskmedia.MaterializedResult, accepted)
				for i := range items {
					items[i] = taskmedia.MaterializedResult{ImageURL: fmt.Sprintf("/output/%d.png", i), Output: tasks.TaskOutput{AssetID: fmt.Sprintf("asset_%d", i), Role: "image", ResultIndex: i}}
				}
				materializer := &captureResultMaterializer{remoteResult: taskmedia.MaterializedResults{Items: items}}
				executor.resultMaterializer = materializer
				task := generationTaskForTest(t, "task-batch-deadline")
				task.Params = marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{SchemaVersion: tasks.CurrentParamsSchemaVersion, Service: workflow.FeatureSlugGeneration, Controls: &tasks.GenerationControls{OutputCount: requested}})
				output, err := executor.CallWorkerAPI(parent, runtimeports.WorkerCallInput{Task: task, Model: "openai/gpt-image-1", ExecutionContext: execCtx})
				<-stopped
				if accepted == 0 {
					if !errors.Is(err, context.DeadlineExceeded) || materializer.remoteCalls != 0 {
						t.Fatalf("outputs=%d ingests=%d err=%v; want timeout without materialization", len(output.Outputs), materializer.remoteCalls, err)
					}
				} else {
					if err != nil || len(output.Outputs) != accepted || len(materializer.remoteSourceURLs) != accepted {
						t.Fatalf("outputs=%d ingested=%d err=%v; want %d accepted outputs without retry", len(output.Outputs), len(materializer.remoteSourceURLs), err, accepted)
					}
					if materializer.remoteContext == nil || materializer.remoteContext.Err() != nil || !errors.Is(execCtx.Err(), context.DeadlineExceeded) {
						t.Fatal("materialization must retain the live parent after the execution deadline")
					}
				}
				if len(fw.capturedBodies) != deadlineAt {
					t.Fatalf("calls=%d, want %d", len(fw.capturedBodies), deadlineAt)
				}
				seen := map[string]bool{}
				for i, body := range fw.capturedBodies {
					key, err := platformworkermedia.ParseKey(body["upload_url"].(string))
					if err != nil || seen[key] || !strings.HasSuffix(key, fmt.Sprintf("/result-%d.png", i)) {
						t.Fatalf("slot[%d]=%q err=%v, want distinct ordered slot", i, key, err)
					}
					seen[key] = true
				}
			})
		}
	}
}

// The wrapper waits and interrupts synchronously before returning to the
// executor, avoiding a race between HTTP cancellation and server scheduling.
type deadlineWorkerClientFunc func(context.Context, runtimeports.WorkerRequest) (runtimeports.WorkerResponse, error)

func (f deadlineWorkerClientFunc) Do(ctx context.Context, req runtimeports.WorkerRequest) (runtimeports.WorkerResponse, error) {
	return f(ctx, req)
}

func TestCallWorkerAPIBatchDeadlineRespectsInterruption(t *testing.T) {
	for _, interruption := range []string{"parent_cancel", "parent_deadline", "operation_cancel"} {
		t.Run(interruption, func(t *testing.T) {
			fw := newFakeWorkerServer(t, buildTinyPNGBytes(t))
			defer fw.server.Close()
			parent, cancelParent := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancelParent()
			execCtx, cancel := context.WithTimeout(parent, 250*time.Millisecond)
			defer cancel()
			executor := workerTestExecutor(fw, nil)
			factory := executor.workerFactory
			calls := 0
			executor.workerFactory = func(ctx context.Context) (runtimeports.WorkerClient, error) {
				client, err := factory(ctx)
				if err != nil {
					return nil, err
				}
				return deadlineWorkerClientFunc(func(ctx context.Context, req runtimeports.WorkerRequest) (runtimeports.WorkerResponse, error) {
					calls++
					if calls == 1 {
						return client.Do(ctx, req)
					}
					<-execCtx.Done()
					switch interruption {
					case "parent_cancel":
						cancelParent()
					case "parent_deadline":
						<-parent.Done()
					case "operation_cancel":
						return runtimeports.WorkerResponse{}, context.Canceled
					}
					return runtimeports.WorkerResponse{}, context.DeadlineExceeded
				}), nil
			}
			materializer := &captureResultMaterializer{}
			executor.resultMaterializer = materializer
			task := generationTaskForTest(t, "task-interrupted-batch")
			task.Params = marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{SchemaVersion: tasks.CurrentParamsSchemaVersion, Service: workflow.FeatureSlugGeneration, Controls: &tasks.GenerationControls{OutputCount: 4}})
			output, err := executor.CallWorkerAPI(parent, runtimeports.WorkerCallInput{Task: task, Model: "openai/gpt-image-1", ExecutionContext: execCtx})
			want := context.Canceled
			if interruption == "parent_deadline" {
				want = context.DeadlineExceeded
			}
			if !errors.Is(err, want) || len(output.Outputs) != 0 || materializer.remoteCalls != 0 || calls != 2 {
				t.Fatalf("calls=%d outputs=%d ingests=%d err=%v; want %v without materialization", calls, len(output.Outputs), materializer.remoteCalls, err, want)
			}
		})
	}
}
