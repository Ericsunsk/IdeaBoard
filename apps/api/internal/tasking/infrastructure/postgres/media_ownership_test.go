package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/identity/users"
	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/testdb"
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
	if err := createRepositoryTestTask(t, repo, ctx, task); err != nil {
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

func TestCreateTaskOutputEnforcesOutputKindShape(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	q := dbsqlc.New(db)
	repo := NewRepository(db)
	owner := createMediaOwnerForTest(t, db, "task-output-kind-owner", "TOKIND")

	task := &tasks.Task{
		TaskID: "task-output-kind-boundary",
		UserID: owner.ID,
		Status: tasks.StatusPending,
	}
	if err := createRepositoryTestTask(t, repo, ctx, task); err != nil {
		t.Fatalf("create task failed: %v", err)
	}
	assetID, err := q.UpsertMediaAsset(ctx, mediaAssetParamsForTest(owner.ID, "task-output-kind-asset", "owner-kind/output.png"))
	if err != nil {
		t.Fatalf("UpsertMediaAsset returned error: %v", err)
	}

	if _, err := q.CreateTaskOutput(ctx, taskOutputParamsForTest(task.TaskID, assetID, "owner-kind/output.png")); err != nil {
		t.Fatalf("asset output CreateTaskOutput returned error: %v", err)
	}

	remoteParams := taskOutputParamsForTest(task.TaskID, assetID, "owner-kind/remote.png")
	remoteParams.MediaAssetID = sql.NullInt64{}
	remoteParams.OutputKind = "remote"
	remoteParams.Url = "https://provider.example/result.png"
	remoteParams.ResultIndex = 1
	if _, err := q.CreateTaskOutput(ctx, remoteParams); err != nil {
		t.Fatalf("remote output CreateTaskOutput returned error: %v", err)
	}

	assetWithoutMedia := taskOutputParamsForTest(task.TaskID, assetID, "owner-kind/missing.png")
	assetWithoutMedia.MediaAssetID = sql.NullInt64{}
	if _, err := q.CreateTaskOutput(ctx, assetWithoutMedia); err == nil {
		t.Fatal("asset output without media_asset_id should fail")
	}

	remoteWithAsset := taskOutputParamsForTest(task.TaskID, assetID, "owner-kind/bad-remote.png")
	remoteWithAsset.OutputKind = "remote"
	remoteWithAsset.Url = "https://provider.example/bad.png"
	if _, err := q.CreateTaskOutput(ctx, remoteWithAsset); err == nil {
		t.Fatal("remote output with media_asset_id should fail")
	}
}

func TestInsertTaskOutputsReusesReadyMaterializedAssetAndPersistsLineage(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	q := dbsqlc.New(db)
	owner := createMediaOwnerForTest(t, db, "task-result-convergence-owner", "TRESULT")
	repo := NewRepository(db)
	if err := createRepositoryTestTask(t, repo, ctx, &tasks.Task{
		TaskID: "task-result-convergence", UserID: owner.ID, Status: tasks.StatusPending,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	sourceID, err := q.UpsertMediaAsset(ctx, mediaAssetParamsForTest(owner.ID, "task-result-source", "inputs/source.png"))
	if err != nil {
		t.Fatalf("create source asset: %v", err)
	}
	if _, err := q.UpsertMediaRef(ctx, dbsqlc.UpsertMediaRefParams{
		MediaAssetID: sourceID, OwnerUserID: owner.ID, RefType: "task_input", RefID: "task-result-convergence",
		TaskID: "task-result-convergence", Role: "input", InputIndex: 0, Metadata: []byte(`{}`),
	}); err != nil {
		t.Fatalf("create task input ref: %v", err)
	}
	outputID, err := q.UpsertMediaAsset(ctx, mediaAssetParamsForTest(owner.ID, "task-result-output", "outputs/result.png"))
	if err != nil {
		t.Fatalf("create output asset: %v", err)
	}
	if err := insertTaskOutputs(ctx, q, "task-result-convergence", owner.ID, []tasks.TaskOutput{{
		MediaAssetID: &outputID, OutputKind: "asset", Role: "image", StorageKey: "outputs/result.png", ResultIndex: 0,
	}}); err != nil {
		t.Fatalf("insert task outputs: %v", err)
	}
	var taskOutputAssetID int64
	if err := db.QueryRowContext(ctx, `SELECT media_asset_id FROM task_outputs WHERE task_id = $1`, "task-result-convergence").Scan(&taskOutputAssetID); err != nil {
		t.Fatalf("load task output asset: %v", err)
	}
	if taskOutputAssetID != outputID {
		t.Fatalf("task output asset=%d, want materialized asset %d", taskOutputAssetID, outputID)
	}
	var taskOutputRefs, lineageRows int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_refs WHERE media_asset_id = $1 AND ref_type = 'task_output'`, outputID).Scan(&taskOutputRefs); err != nil {
		t.Fatalf("count task output refs: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_asset_sources WHERE media_asset_id = $1`, outputID).Scan(&lineageRows); err != nil {
		t.Fatalf("count task output lineage: %v", err)
	}
	if taskOutputRefs != 1 || lineageRows != 1 {
		t.Fatalf("task output refs/lineage=%d/%d, want 1/1", taskOutputRefs, lineageRows)
	}
	var assetCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_assets WHERE owner_user_id = $1 AND storage_key = $2`, owner.ID, "outputs/result.png").Scan(&assetCount); err != nil {
		t.Fatalf("count output assets: %v", err)
	}
	if assetCount != 1 {
		t.Fatalf("output asset count=%d, want 1", assetCount)
	}
}

func createMediaOwnerForTest(t *testing.T, db *sql.DB, displayName string, storageUID string) *users.User {
	t.Helper()
	repo := users.NewUserRepository(db)
	user := &users.User{
		DisplayName: displayName,
		StorageUID:  storageUID,
		Role:        "user",
		Status:      1,
	}
	if err := repo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user %q failed: %v", displayName, err)
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
		OutputKind:   "asset",
		Role:         "image",
		Url:          "",
		Metadata:     []byte(`{}`),
	}
}
