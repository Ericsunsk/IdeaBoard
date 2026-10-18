package worker

// Renderer over fixture PostgreSQL (#827, Phase 4b-03 of #809):
// task completes -> rendered output created as a workspace-scoped media
// asset row -> task_outputs ref persisted through the atomic Complete
// path. SKIPs gracefully without
// API_TEST_DATABASE_DSN (same harness as
// worker_integration_test.go).
//
// What this proves end to end: a fake gateway result (remote URL +
// checksum, standing in for provider bytes without credentials) flows
// through request shaping + result mapping + the #827 renderer into a NEW
// media_assets row (bytes via the objects seam, format validated via
// imageformat) and the Complete persists the asset ref through task_outputs
// (ListTaskAssets reads it back). Completion records no derivative rows:
// display tiers resolve on demand through signed imgproxy URLs (ADR-0007
// pull model).
// The deterministic-failure twin (TestWorkspaceWorkerDeterministicFailurePostgres)
// keeps proving the creds-gated path converges without them.

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/contract"
	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	"ideaboard/api/internal/tasking/workspacetasks"

	"github.com/riverqueue/river"
)

func rendererIntegrationPNG(t *testing.T) ([]byte, string) {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatalf("decode fixture png: %v", err)
	}
	return raw, fixtureChecksum(raw)
}

// Success: submit -> worker claims -> fake gateway yields a remote ref ->
// renderer ingests a new asset -> completed with the asset ref mapped
// through task_outputs.
func TestWorkspaceTaskRendererPostgres(t *testing.T) {
	env := openWorkspaceIntegrationEnv(t)
	ctx := context.Background()
	service, _, _ := newWorkerIntegrationService(env)

	created, err := service.Create(ctx, workspacetasks.CreateInput{
		WorkspaceID: env.wsA, OwnerUserID: env.userA,
		Service: "render", Quantity: 1, Resolution: "",
	}, fmt.Sprintf("renderer-loop-%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatalf("settlement Create: %v", err)
	}
	pinIntegrationSnapshot(t, env, created, "openrouter/fake-test-image")

	png, sum := rendererIntegrationPNG(t)
	objects := &fakeRendererObjects{}
	renderer, err := taskruntime.NewWorkspaceTaskRenderer(taskruntime.WorkspaceTaskRendererDeps{
		Media: env.media, Assets: &workspacemedia.TaskAssetAdapter{Store: env.media}, Objects: objects,
		Fetcher: taskruntime.WorkspaceResultBytesFetchFunc(func(_ context.Context, _ *workspacetasks.WorkspaceTask, _ workspacetasks.OutputRef) ([]byte, error) {
			return append([]byte(nil), png...), nil
		}),
	})
	if err != nil {
		t.Fatalf("taskruntime.NewWorkspaceTaskRenderer: %v", err)
	}
	worker := newWorkerIntegrationWorker(env, service, WorkspaceTaskGenerateFunc(
		func(_ context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
			if task.Status != workspacetasks.StatusRunning {
				t.Errorf("provider saw status %q, want running (claimed)", task.Status)
			}
			return []workspacetasks.OutputRef{{
				Kind:           workspacetasks.OutputKindRemote,
				URL:            "https://cdn.example.com/fake-gateway-result.png",
				ChecksumSHA256: sum, Role: "result", ResultIndex: 0,
			}}, nil
		}))
	// The shared test worker carries no renderer; rebuild with the #827
	// renderer so the loop exercises the production ordering
	// (generate -> render -> atomic Complete) through the same seams.
	renderingWorker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: env.tasks, Settler: service,
		Assets:   &workspacemedia.TaskAssetAdapter{Store: env.media},
		Provider: worker.provider,
		Renderer: renderer,
		Attempts: env.tasks,
	})
	if err != nil {
		t.Fatalf("NewWorkspaceTaskWorker: %v", err)
	}
	if err := renderingWorker.Work(ctx, &river.Job[workspacetasks.WorkspaceTaskArgs]{Args: workspacetasks.WorkspaceTaskArgs{
		TaskID: created.ID, WorkspaceID: env.wsA,
		Service: "render", ExecutionToken: created.ExecutionToken,
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
	if len(stored.OutputRefs) != 1 || stored.OutputRefs[0].Kind != workspacetasks.OutputKindAsset {
		t.Fatalf("outputs = %+v, want the rendered asset ref", stored.OutputRefs)
	}
	renderedID := stored.OutputRefs[0].AssetID
	if !contract.IsAssetID(renderedID) {
		t.Fatalf("asset id = %q, want asset_ shape", renderedID)
	}
	asset, err := env.media.GetAsset(ctx, env.wsA, renderedID)
	if err != nil {
		t.Fatalf("GetAsset: %v", err)
	}
	if asset.MIMEType != "image/png" || asset.Width != 1 || asset.Height != 1 {
		t.Fatalf("asset = %+v, want validated png facts", asset)
	}
	if asset.ChecksumSHA256 != sum || asset.SizeBytes != int64(len(png)) {
		t.Fatalf("asset = %+v, want byte facts echoed", asset)
	}
	if got := objects.objects[asset.StorageKey]; len(got) != len(png) {
		t.Fatalf("stored bytes = %d, want %d at the asset slot", len(got), len(png))
	}
	items, err := env.media.ListTaskAssets(ctx, env.wsA, created.ID)
	if err != nil {
		t.Fatalf("ListTaskAssets: %v", err)
	}
	if len(items) != 1 || items[0].Asset.ID != renderedID || items[0].Role != "result" {
		t.Fatalf("task assets = %+v, want the rendered asset with role", items)
	}
}
