package taskstore

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tasks"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/users"
)

func TestUpsertMediaAssetRejectsOwnerTakeover(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	q := dbsqlc.New(db)
	ownerA := createMediaOwnerForTest(t, db, "media-owner-a", "MOWN1A")
	ownerB := createMediaOwnerForTest(t, db, "media-owner-b", "MOWN2B")

	firstID, err := q.UpsertMediaAsset(ctx, mediaAssetParamsForTest(ownerA.ID, "asset-a", "shared/key.png"))
	if err != nil {
		t.Fatalf("first UpsertMediaAsset returned error: %v", err)
	}
	if firstID == 0 {
		t.Fatal("first media asset id should be non-zero")
	}

	_, err = q.UpsertMediaAsset(ctx, mediaAssetParamsForTest(ownerB.ID, "asset-b", "shared/key.png"))
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-owner UpsertMediaAsset error = %v, want sql.ErrNoRows", err)
	}
}

func TestUpsertMediaRefRequiresAssetOwner(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	q := dbsqlc.New(db)
	ownerA := createMediaOwnerForTest(t, db, "media-ref-owner-a", "MREF1A")
	ownerB := createMediaOwnerForTest(t, db, "media-ref-owner-b", "MREF2B")

	assetID, err := q.UpsertMediaAsset(ctx, mediaAssetParamsForTest(ownerA.ID, "asset-ref", "owner-a/key.png"))
	if err != nil {
		t.Fatalf("UpsertMediaAsset returned error: %v", err)
	}

	if _, err := q.UpsertMediaRef(ctx, mediaRefParamsForTest(assetID, ownerA.ID, "ref-ok")); err != nil {
		t.Fatalf("owner UpsertMediaRef returned error: %v", err)
	}
	if _, err := q.UpsertMediaRef(ctx, mediaRefParamsForTest(assetID, ownerB.ID, "ref-bad")); err == nil {
		t.Fatal("cross-owner UpsertMediaRef should fail")
	}
}

func TestUpsertMediaRefRejectsOwnerTakeoverOnConflict(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	q := dbsqlc.New(db)
	ownerA := createMediaOwnerForTest(t, db, "media-ref-conflict-a", "MRFCA1")
	ownerB := createMediaOwnerForTest(t, db, "media-ref-conflict-b", "MRFCB2")

	assetID, err := q.UpsertMediaAsset(ctx, mediaAssetParamsForTest(ownerA.ID, "asset-conflict", "owner-a/conflict.png"))
	if err != nil {
		t.Fatalf("UpsertMediaAsset returned error: %v", err)
	}

	if _, err := q.UpsertMediaRef(ctx, mediaRefParamsForTest(assetID, ownerA.ID, "ref-conflict")); err != nil {
		t.Fatalf("owner UpsertMediaRef returned error: %v", err)
	}
	_, err = q.UpsertMediaRef(ctx, mediaRefParamsForTest(assetID, ownerB.ID, "ref-conflict"))
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-owner conflict UpsertMediaRef error = %v, want sql.ErrNoRows", err)
	}
}

func TestCreateTaskOutputRequiresMediaAssetOwner(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	q := dbsqlc.New(db)
	repo := NewRepository(db)
	ownerA := createMediaOwnerForTest(t, db, "task-output-owner-a", "TOUTA1")
	ownerB := createMediaOwnerForTest(t, db, "task-output-owner-b", "TOUTB2")

	task := &tasks.Task{
		TaskID: "task-output-owner-boundary",
		UserID: ownerA.ID,
		Status: tasks.StatusPending,
	}
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create task failed: %v", err)
	}

	ownedAssetID, err := q.UpsertMediaAsset(ctx, mediaAssetParamsForTest(ownerA.ID, "task-output-owned", "owner-a/output.png"))
	if err != nil {
		t.Fatalf("owner UpsertMediaAsset returned error: %v", err)
	}
	if _, err := q.CreateTaskOutput(ctx, taskOutputParamsForTest(task.TaskID, ownedAssetID, "owner-a/output.png")); err != nil {
		t.Fatalf("owner CreateTaskOutput returned error: %v", err)
	}

	foreignAssetID, err := q.UpsertMediaAsset(ctx, mediaAssetParamsForTest(ownerB.ID, "task-output-foreign", "owner-b/output.png"))
	if err != nil {
		t.Fatalf("foreign UpsertMediaAsset returned error: %v", err)
	}
	if _, err := q.CreateTaskOutput(ctx, taskOutputParamsForTest(task.TaskID, foreignAssetID, "owner-b/output.png")); err == nil {
		t.Fatal("cross-owner CreateTaskOutput should fail")
	}
}

func createMediaOwnerForTest(t *testing.T, db *sql.DB, username string, storageUID string) *users.User {
	t.Helper()
	repo := users.NewUserRepository(db)
	user := &users.User{
		Username:   username,
		StorageUID: storageUID,
		Role:       "user",
		Status:     1,
	}
	if err := repo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user %q failed: %v", username, err)
	}
	return user
}

func mediaAssetParamsForTest(ownerID int64, assetID string, storageKey string) dbsqlc.UpsertMediaAssetParams {
	return dbsqlc.UpsertMediaAssetParams{
		AssetID:          assetID,
		OwnerUserID:      ownerID,
		StorageProfileID: "builtin-local",
		StorageKey:       storageKey,
		Url:              "/uploads/" + storageKey,
		SourceUrl:        "",
		MediaKind:        "image",
		MimeType:         "image/png",
		Width:            64,
		Height:           64,
		SizeBytes:        1024,
		ChecksumSha256:   "",
		Status:           "ready",
		Metadata:         []byte(`{}`),
	}
}

func mediaRefParamsForTest(assetID int64, ownerID int64, refID string) dbsqlc.UpsertMediaRefParams {
	return dbsqlc.UpsertMediaRefParams{
		MediaAssetID: assetID,
		OwnerUserID:  ownerID,
		RefType:      "task_output",
		RefID:        refID,
		BoardID:      "",
		TaskID:       "task-" + refID,
		Role:         "image",
		Metadata:     []byte(`{}`),
	}
}

func taskOutputParamsForTest(taskID string, mediaAssetID int64, storageKey string) dbsqlc.CreateTaskOutputParams {
	return dbsqlc.CreateTaskOutputParams{
		TaskID:       taskID,
		MediaAssetID: sql.NullInt64{Int64: mediaAssetID, Valid: true},
		Role:         "image",
		Url:          "/uploads/" + storageKey,
		StorageKey:   storageKey,
		MimeType:     "image/png",
		Width:        64,
		Height:       64,
		SizeBytes:    1024,
		Metadata:     []byte(`{}`),
	}
}
