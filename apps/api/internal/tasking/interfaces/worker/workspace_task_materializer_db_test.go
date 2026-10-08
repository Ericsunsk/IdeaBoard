package worker

// Fixture-DB contract test for the #828 slice B trigger: the worker
// completes a task on fixture PostgreSQL while the trigger records
// against the REAL postgres library store with the fake saver. SKIPs
// gracefully without API_TEST_DATABASE_DSN.
//
// What this proves end to end: task completes -> one completed
// materialization job -> media link -> asset in Quick Saves. The fixture
// DDL intentionally excludes library_materializations, so this file
// applies that table from the canonical baseline migration in the
// isolated test schema (no shared-file edits, no conflicts).

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

// materializationsFixtureDDL mirrors the canonical baseline migration
// for library_materializations only (workspaceIntegrationDDL excludes
// it). Kept local so this contract test never edits shared fixtures.
const materializationsFixtureDDL = `
CREATE TABLE library_materializations (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id TEXT NOT NULL,
  workspace_id BIGINT NOT NULL REFERENCES workspaces(internal_id) ON DELETE CASCADE,
  library_asset_id BIGINT NOT NULL REFERENCES library_assets(internal_id) ON DELETE CASCADE,
  status TEXT NOT NULL DEFAULT 'pending',
  media_asset_id BIGINT NULL REFERENCES media_assets(internal_id) ON DELETE RESTRICT,
  error_message TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ NULL,
  CONSTRAINT uq_library_materializations_public_id UNIQUE (public_id),
  CONSTRAINT chk_library_materializations_public_id CHECK (public_id LIKE 'lmat\_%'),
  CONSTRAINT chk_library_materializations_status CHECK (status IN ('pending','processing','completed','failed')),
  CONSTRAINT chk_library_materializations_completion CHECK (
    (status = 'completed' AND media_asset_id IS NOT NULL AND completed_at IS NOT NULL)
    OR (status = 'failed' AND completed_at IS NOT NULL)
    OR (status IN ('pending','processing'))
  )
);
CREATE INDEX idx_library_materializations_workspace ON library_materializations (workspace_id, created_at DESC);
CREATE INDEX idx_library_materializations_library_asset ON library_materializations (library_asset_id);
CREATE INDEX idx_library_materializations_status ON library_materializations (status, created_at);
`

func TestWorkerAutoMaterializeEndToEndPostgres(t *testing.T) {
	env := openWorkspaceIntegrationEnv(t)
	ctx := context.Background()
	if _, err := env.pool.Exec(ctx, materializationsFixtureDDL); err != nil {
		t.Fatalf("apply materializations fixture ddl: %v", err)
	}
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

	store := &countingLibraryStore{WorkspaceLibraryStore: env.lib}
	trigger, err := taskruntime.NewWorkspaceTaskAutoMaterializer(taskruntime.WorkspaceTaskAutoMaterializerDeps{
		Store: store,
		Saver: &fakeTaskOutputSaver{store: env.lib},
	})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: env.tasks, Settler: service,
		Assets: &workspacemedia.TaskAssetAdapter{Store: env.media},
		Provider: WorkspaceTaskGenerateFunc(func(context.Context, *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
			return []workspacetasks.OutputRef{{Kind: workspacetasks.OutputKindAsset, AssetID: asset.ID, Role: "result", ResultIndex: 0}}, nil
		}),
		Materializer: trigger,
		Attempts:     env.tasks,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Work(ctx, &river.Job[workspacetasks.WorkspaceTaskArgs]{Args: workspacetasks.WorkspaceTaskArgs{
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
	// Job: exactly one, completed, media linked to the task output.
	if got := store.jobCount(); got != 1 {
		t.Fatalf("jobs = %d, want 1", got)
	}
	record, err := env.lib.GetMaterialization(ctx, env.wsA, store.created[0])
	if err != nil {
		t.Fatalf("GetMaterialization: %v", err)
	}
	if record.Status != librarydomain.V1MaterializationCompleted {
		t.Fatalf("job status = %q, want completed", record.Status)
	}
	if record.MediaAssetID != asset.ID {
		t.Fatalf("job media = %q, want %q", record.MediaAssetID, asset.ID)
	}
	// Quick Saves: the saved asset resolves to the system folder.
	saved, err := env.lib.GetAsset(ctx, env.wsA, record.AssetPublicID)
	if err != nil {
		t.Fatalf("GetAsset: %v", err)
	}
	if !librarydomain.IsV1FolderID(strings.TrimSpace(saved.FolderPublicID)) {
		t.Fatalf("asset folder = %q, want the Quick Saves lfolder_ id", saved.FolderPublicID)
	}
}
