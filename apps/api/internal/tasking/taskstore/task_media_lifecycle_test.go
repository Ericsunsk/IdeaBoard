package taskstore

import (
	"testing"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tasking/tasks"
)

func TestTerminalTransitionReleasesTaskInputMediaRefs(t *testing.T) {
	repo, db := newRepositoryTestDB(t)
	task := &tasks.Task{TaskID: "task-terminal-media-release", UserID: 1, FeatureSlug: "test-non-billable", Status: tasks.StatusPending}
	if err := createRepositoryTestTask(t, repo, t.Context(), task); err != nil {
		t.Fatal(err)
	}
	q := dbsqlc.New(db)
	assetID, err := q.UpsertMediaAsset(t.Context(), dbsqlc.UpsertMediaAssetParams{
		AssetID: "asset-terminal-media-release", OwnerUserID: task.UserID, StorageProfileID: "builtin-local",
		StorageKey: "uploads/tasks/1/task-terminal-media-release/input.png", Url: "/uploads/tasks/1/task-terminal-media-release/input.png",
		MediaKind: "image", MimeType: "image/png", Status: "ready", Metadata: []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpsertMediaRef(t.Context(), dbsqlc.UpsertMediaRefParams{
		MediaAssetID: assetID, OwnerUserID: task.UserID, RefType: "task_input", RefID: task.TaskID,
		TaskID: task.TaskID, Role: tasks.MediaRoleInput, Metadata: []byte(`{}`),
	}); err != nil {
		t.Fatal(err)
	}

	rows, err := repo.MarkTaskCancelled(t.Context(), tasks.TaskCancellationSnapshot{
		TaskID: task.TaskID, CancelledAt: time.Now(), Reason: "cancel media task",
	}, nil)
	if err != nil || rows != 1 {
		t.Fatalf("cancel task rows=%d err=%v", rows, err)
	}
	var refCount int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM media_refs WHERE task_id=$1 AND ref_type='task_input'`, task.TaskID).Scan(&refCount); err != nil {
		t.Fatal(err)
	}
	if refCount != 0 {
		t.Fatalf("task input refs=%d, want released", refCount)
	}
	asset, err := q.GetMediaAssetByID(t.Context(), assetID)
	if err != nil {
		t.Fatal(err)
	}
	if !asset.ReclaimAfter.Valid || asset.ReclaimGeneration != 1 {
		t.Fatalf("released asset reclamation=%#v generation=%d", asset.ReclaimAfter, asset.ReclaimGeneration)
	}
}
