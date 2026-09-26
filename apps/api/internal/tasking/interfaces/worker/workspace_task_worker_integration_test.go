package worker

// Worker loop over fixture PostgreSQL (#825, Phase 4b-01 of #809):
// submit -> claim -> process -> complete against the REAL postgres task
// store, the REAL media store (TaskAssets adapter), and the REAL
// workspacetasks settlement Service. SKIPs gracefully without
// API_TEST_DATABASE_DSN (same harness as workspace_integration_test.go).
//
// What this proves: the River job payload the enqueuer writes
// (taskruntime.WorkspaceTaskArgs with the submit-time execution token) drives the
// worker through ClaimTask (lease-guarded), read-only media resolution,
// the provider seam, and terminal Complete with task_outputs mapping
// (ListTaskAssets reads the output back). The provider itself is an
// injected fake: live gateway dispatch awaits credentials + execution-plan
// minting (#827); the deterministic-failure twin below proves the loop
// converges without them.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/media/workspacemedia"
	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	"ideaboard/api/internal/tasking/workspacetasks"

	"github.com/riverqueue/river"
)

func newWorkerIntegrationService(env *workspaceIntegrationEnv) (*workspacetasks.Service, *integrationHolder, *integrationQueue) {
	holder := newIntegrationHolder()
	queue := &integrationQueue{}
	service := workspacetasks.NewService(workspacetasks.ServiceDeps{
		Tasks: env.tasks, Quoter: &integrationQuoter{}, Holder: holder, Queue: queue,
		Assets: &workspacemedia.TaskAssetAdapter{Store: env.media}, Models: workspaceTestModelResolver{},
	})
	return service, holder, queue
}

func newWorkerIntegrationWorker(
	env *workspaceIntegrationEnv,
	service *workspacetasks.Service,
	provider WorkspaceTaskProvider,
) *WorkspaceTaskWorker {
	worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: env.tasks, Settler: service,
		Assets:   &workspacemedia.TaskAssetAdapter{Store: env.media},
		Provider: provider,
	})
	if err != nil {
		panic(err)
	}
	return worker
}

// Success: submit (with a real media input, verified through the adapter)
// -> worker claims, resolves, generates an asset output -> completed with
// the output mapped through task_outputs (ListTaskAssets reads it back).
func TestWorkspaceWorkerLoopPostgres(t *testing.T) {
	env := openWorkspaceIntegrationEnv(t)
	ctx := context.Background()
	service, _, _ := newWorkerIntegrationService(env)

	asset := completeUploadForIntegration(t, ctx, env.media, env.wsA, env.userA, "worker-input.png")

	created, err := service.Create(ctx, workspacetasks.CreateInput{
		WorkspaceID: env.wsA, OwnerUserID: env.userA,
		WorkflowKind: "render", MediaAssetIDs: []string{asset.ID}, Quantity: 1, Resolution: "default",
	}, fmt.Sprintf("worker-loop-%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatalf("settlement Create with media input: %v", err)
	}

	// The args mirror exactly what workspaceTaskEnqueuer writes at submit.
	worker := newWorkerIntegrationWorker(env, service, WorkspaceTaskGenerateFunc(
		func(ctx context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
			if task.Status != workspacetasks.StatusRunning {
				t.Errorf("provider saw status %q, want running (claimed)", task.Status)
			}
			if len(task.MediaAssetIDs) != 1 || task.MediaAssetIDs[0] != asset.ID {
				t.Errorf("provider saw inputs %+v, want the submitted asset", task.MediaAssetIDs)
			}
			return []workspacetasks.OutputRef{
				{Kind: workspacetasks.OutputKindAsset, AssetID: asset.ID, Role: "result", ResultIndex: 0},
			}, nil
		}))
	if err := worker.Work(ctx, &river.Job[taskruntime.WorkspaceTaskArgs]{Args: taskruntime.WorkspaceTaskArgs{
		TaskID: created.ID, WorkspaceID: env.wsA,
		WorkflowKind: "render", ExecutionToken: created.ExecutionToken,
	}}); err != nil {
		t.Fatalf("Work: %v", err)
	}

	stored, err := env.tasks.GetTask(ctx, env.wsA, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.Status != workspacetasks.StatusCompleted {
		t.Fatalf("status = %q, want completed", stored.Status)
	}
	if len(stored.OutputRefs) != 1 || stored.OutputRefs[0].AssetID != asset.ID {
		t.Fatalf("outputs = %+v, want the asset ref through task_outputs", stored.OutputRefs)
	}
	items, err := env.media.ListTaskAssets(ctx, env.wsA, created.ID)
	if err != nil {
		t.Fatalf("ListTaskAssets: %v", err)
	}
	if len(items) != 1 || items[0].Asset.ID != asset.ID || items[0].Role != "result" {
		t.Fatalf("task assets = %+v, want the completed asset with role", items)
	}
}

// Deterministic provider failure on PG: the loop converges to failed with
// the terminal error instead of stranding the lease.
func TestWorkspaceWorkerDeterministicFailurePostgres(t *testing.T) {
	env := openWorkspaceIntegrationEnv(t)
	ctx := context.Background()
	service, _, _ := newWorkerIntegrationService(env)

	created, err := service.Create(ctx, workspacetasks.CreateInput{
		WorkspaceID: env.wsA, OwnerUserID: env.userA,
		WorkflowKind: "render", Quantity: 1, Resolution: "default",
	}, fmt.Sprintf("worker-fail-%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatalf("settlement Create: %v", err)
	}

	worker := newWorkerIntegrationWorker(env, service, WorkspaceTaskGenerateFunc(
		func(ctx context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
			return nil, fmt.Errorf("上游图像服务未配置，请联系管理员")
		}))
	if err := worker.Work(ctx, &river.Job[taskruntime.WorkspaceTaskArgs]{Args: taskruntime.WorkspaceTaskArgs{
		TaskID: created.ID, WorkspaceID: env.wsA,
		WorkflowKind: "render", ExecutionToken: created.ExecutionToken,
	}}); err != nil {
		t.Fatalf("Work: %v", err)
	}

	stored, err := env.tasks.GetTask(ctx, env.wsA, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.Status != workspacetasks.StatusFailed {
		t.Fatalf("status = %q, want failed", stored.Status)
	}
	if !strings.Contains(stored.ErrorMsg, "上游图像服务未配置") {
		t.Fatalf("error_msg = %q, want the deterministic provider message", stored.ErrorMsg)
	}
}
