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

type revokeBetweenOutputSaver struct {
	saver      taskruntime.TaskOutputLibrarySaver
	afterFirst func() error
	calls      int
}

func (s *revokeBetweenOutputSaver) SaveTaskOutputAsset(ctx context.Context, workspaceID string, input taskruntime.TaskOutputSaveInput) (*librarydomain.V1Asset, bool, error) {
	asset, created, err := s.saver.SaveTaskOutputAsset(ctx, workspaceID, input)
	s.calls++
	if s.calls == 1 && err == nil {
		if revokeErr := s.afterFirst(); revokeErr != nil {
			return nil, false, revokeErr
		}
	}
	return asset, created, err
}

func TestAutoSaverRechecksMembershipForEachOutputPostgres(t *testing.T) {
	env := openWorkspaceIntegrationEnv(t)
	ctx := t.Context()
	service, _, _ := newWorkerIntegrationService(env)
	first := completeUploadForIntegration(t, ctx, env.media, env.wsA, env.userA, "batch-first.png")
	second := completeUploadForIntegration(t, ctx, env.media, env.wsA, env.userA, "batch-second.png")
	created, err := service.Create(ctx, workspacetasks.CreateInput{WorkspaceID: env.wsA, OwnerUserID: env.userA, Service: "render", Quantity: 1}, "batch-revoke")
	if err != nil {
		t.Fatal(err)
	}
	completed, err := service.Complete(ctx, env.wsA, created.ID, []workspacetasks.OutputRef{
		{Kind: workspacetasks.OutputKindAsset, AssetID: first.ID, Role: "result", ResultIndex: 0},
		{Kind: workspacetasks.OutputKindAsset, AssetID: second.ID, Role: "result", ResultIndex: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	saver := &revokeBetweenOutputSaver{saver: taskruntime.NewWorkspaceTaskLibrarySaver(env.lib), afterFirst: func() error {
		_, err := env.pool.Exec(ctx, `DELETE FROM workspace_memberships WHERE workspace_id=(SELECT internal_id FROM workspaces WHERE public_id=$1) AND user_id=$2`, env.wsA, env.userA)
		return err
	}}
	trigger, err := taskruntime.NewWorkspaceTaskAutoSaver(saver)
	if err != nil {
		t.Fatal(err)
	}
	if err := trigger.SaveCompletedTask(ctx, completed); err != nil {
		t.Fatal(err)
	}
	assets, err := env.lib.ListAssets(ctx, env.wsA)
	if err != nil {
		t.Fatal(err)
	}
	if saver.calls != 2 || len(assets) != 1 || assets[0].MediaAssetID != first.ID {
		t.Fatalf("batch reused stale membership: calls=%d assets=%+v", saver.calls, assets)
	}
}

func TestAutomaticPublicationRequiresPersistedOutputPostgres(t *testing.T) {
	env := openWorkspaceIntegrationEnv(t)
	ctx := t.Context()
	service, _, _ := newWorkerIntegrationService(env)
	output := completeUploadForIntegration(t, ctx, env.media, env.wsA, env.userA, "accepted-output.png")
	unrelated := completeUploadForIntegration(t, ctx, env.media, env.wsA, env.userA, "unrelated-upload.png")
	task, err := service.Create(ctx, workspacetasks.CreateInput{WorkspaceID: env.wsA, OwnerUserID: env.userA, Service: "render", Quantity: 1}, "exact-output")
	if err != nil {
		t.Fatal(err)
	}
	saver := taskruntime.NewWorkspaceTaskLibrarySaver(env.lib)
	input := taskruntime.TaskOutputSaveInput{TaskID: task.ID, ResultIndex: 0, MediaAssetID: output.ID, OperatorUserID: env.userA}
	if _, _, err := saver.SaveTaskOutputAsset(ctx, env.wsA, input); err == nil {
		t.Fatal("pending Task published an output")
	}
	if _, err := service.Complete(ctx, env.wsA, task.ID, []workspacetasks.OutputRef{{Kind: workspacetasks.OutputKindAsset, AssetID: output.ID, Role: "result", ResultIndex: 0}}); err != nil {
		t.Fatal(err)
	}
	for _, wrong := range []taskruntime.TaskOutputSaveInput{
		{TaskID: task.ID, ResultIndex: 1, MediaAssetID: output.ID, OperatorUserID: env.userA},
		{TaskID: task.ID, ResultIndex: 0, MediaAssetID: unrelated.ID, OperatorUserID: env.userA},
		{TaskID: task.ID, ResultIndex: 0, MediaAssetID: output.ID, OperatorUserID: env.userB},
	} {
		if _, _, err := saver.SaveTaskOutputAsset(ctx, env.wsA, wrong); err == nil {
			t.Fatalf("forged automatic output accepted: %+v", wrong)
		}
	}
	if assets, err := env.lib.ListAssets(ctx, env.wsA); err != nil || len(assets) != 0 {
		t.Fatalf("invalid callback published authority: %d,%v", len(assets), err)
	}
	if _, created, err := saver.SaveTaskOutputAsset(ctx, env.wsA, input); err != nil || !created {
		t.Fatalf("exact completed output not accepted: %v,%v", created, err)
	}
}
