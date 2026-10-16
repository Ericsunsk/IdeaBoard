package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	librarydomain "ideaboard/api/internal/library/domain"
	"ideaboard/api/internal/media/workspacemedia"
	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	"ideaboard/api/internal/tasking/workspacetasks"

	"github.com/riverqueue/river"
)

func TestWorkerAutoSaveEndToEndPostgres(t *testing.T) {
	env := openWorkspaceIntegrationEnv(t)
	ctx := context.Background()
	service, _, _ := newWorkerIntegrationService(env)

	asset := completeUploadForIntegration(t, ctx, env.media, env.wsA, env.userA, "trigger-input.png")
	created, err := service.Create(ctx, workspacetasks.CreateInput{
		WorkspaceID: env.wsA, OwnerUserID: env.userA,
		Service: "render", MediaAssetIDs: []string{asset.ID}, Quantity: 1, Resolution: "",
	}, fmt.Sprintf("worker-trigger-%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatalf("settlement Create: %v", err)
	}
	pinIntegrationSnapshot(t, env, created, "openrouter/fake-test-image")

	trigger, err := taskruntime.NewWorkspaceTaskAutoSaver(taskruntime.NewWorkspaceTaskLibrarySaver(env.lib))
	if err != nil {
		t.Fatal(err)
	}
	worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: env.tasks, Settler: service,
		Assets: &workspacemedia.TaskAssetAdapter{Store: env.media},
		Provider: WorkspaceTaskGenerateFunc(func(context.Context, *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
			return []workspacetasks.OutputRef{{Kind: workspacetasks.OutputKindAsset, AssetID: asset.ID, Role: "result", ResultIndex: 0}}, nil
		}),
		LibrarySaver: trigger,
		Attempts:     env.tasks,
	})
	if err != nil {
		t.Fatal(err)
	}
	job := &river.Job[workspacetasks.WorkspaceTaskArgs]{Args: workspacetasks.WorkspaceTaskArgs{
		TaskID: created.ID, WorkspaceID: env.wsA,
		Service: "render", ExecutionToken: created.ExecutionToken,
	}}
	for range 2 {
		if err := worker.Work(ctx, job); err != nil {
			t.Fatalf("Work: %v", err)
		}
	}

	stored, err := env.tasks.GetTask(ctx, env.wsA, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.Status != workspacetasks.StatusCompleted {
		t.Fatalf("status = %q, want completed", stored.Status)
	}
	assets, err := env.lib.ListAssets(ctx, env.wsA)
	if err != nil || len(assets) != 1 {
		t.Fatalf("saved assets = (%d, %v), want 1", len(assets), err)
	}
	saved := assets[0]
	if saved.Source != librarydomain.AssetSourceMedia || saved.MediaAssetID != asset.ID || saved.Lineage == nil || saved.Lineage.TaskID != created.ID {
		t.Fatalf("saved asset lost managed identity or Task lineage: %+v", saved)
	}
	if !librarydomain.IsV1FolderID(strings.TrimSpace(saved.FolderPublicID)) {
		t.Fatalf("asset folder = %q, want the Quick Saves lfolder_ id", saved.FolderPublicID)
	}
}
