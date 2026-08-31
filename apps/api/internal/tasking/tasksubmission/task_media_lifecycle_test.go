package tasksubmission

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/identity/users"
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskstore"
	"ideaboard/api/internal/testdb"
)

func TestStoreSubmitTaskClaimsMediaRefInTaskRiverTransaction(t *testing.T) {
	for _, test := range []struct {
		name       string
		failRiver  bool
		wantInputs int
		wantPrompt int
	}{
		{name: "commit", wantInputs: 1},
		{name: "rollback", failRiver: true, wantPrompt: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, pool := testdb.OpenWithPool(t)
			owner := &users.User{DisplayName: "task-media-owner", Status: 1, Role: "user"}
			if err := users.NewUserRepository(db).Create(t.Context(), owner); err != nil {
				t.Fatal(err)
			}
			q := dbsqlc.New(db)
			assetID, err := q.UpsertMediaAsset(t.Context(), dbsqlc.UpsertMediaAssetParams{
				AssetID: "asset-task-submit-" + test.name, OwnerUserID: owner.ID, StorageProfileID: "builtin-local",
				StorageKey: "uploads/tasks/task-submit/" + test.name + ".png", Url: "/uploads/tasks/task-submit/" + test.name + ".png",
				MediaKind: "image", MimeType: "image/png", Status: "ready", Metadata: []byte(`{}`),
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := q.ScheduleUnreferencedMediaAssetsForReclamation(t.Context(), dbsqlc.ScheduleUnreferencedMediaAssetsForReclamationParams{
				ReclaimAfter: sql.NullTime{Time: time.Now().Add(-time.Second), Valid: true}, MediaAssetIds: []int64{assetID},
			}); err != nil {
				t.Fatal(err)
			}
			const clientActionID = "task-media-action"
			if _, err := q.UpsertMediaRef(t.Context(), dbsqlc.UpsertMediaRefParams{
				MediaAssetID: assetID, OwnerUserID: owner.ID, RefType: "prompt_reference", RefID: clientActionID,
				Role: tasks.MediaRoleInput, Metadata: []byte(`{}`),
			}); err != nil {
				t.Fatal(err)
			}

			client := newTaskSubmissionRiverClient(t, pool, submissionRiverTestQueue, submissionRiverTestMaxAttempts)
			var riverClient taskqueue.RiverTxClient = client
			if test.failRiver {
				riverClient = insertThenFailRiverTxClient{inner: client, err: errors.New("forced River failure")}
			}
			store := NewStore(pool, taskstore.NewRepository(db), riverClient, submissionRiverTestQueue, submissionRiverTestMaxAttempts, nil)
			canonical, err := NewCanonicalRequest(CanonicalRequestInput{
				UserID: owner.ID, UserPrompt: "media", Prompt: "media", Params: `{"schema_version":5}`,
				ClientActionID: clientActionID, Feature: "test-non-billable",
				Generation: &tasks.GenerationRequestSnapshot{
					SchemaVersion: tasks.CurrentParamsSchemaVersion, Feature: "test-non-billable", Primitive: "image-to-image",
					TemplateFingerprint: "task-media", RequestedOutputCount: 1,
					Media: []tasks.GenerationMediaIntent{{InputIndex: 0, Role: tasks.MediaRoleInput, AssetID: assetID}},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			_, submitErr := store.SubmitTask(context.Background(), canonical)
			if test.failRiver && submitErr == nil {
				t.Fatal("expected River failure")
			}
			if !test.failRiver && submitErr != nil {
				t.Fatal(submitErr)
			}
			assertMediaRefCount(t, db, assetID, "task_input", test.wantInputs)
			assertMediaRefCount(t, db, assetID, "prompt_reference", test.wantPrompt)
			asset, err := q.GetMediaAssetByID(t.Context(), assetID)
			if err != nil {
				t.Fatal(err)
			}
			if test.failRiver && !asset.ReclaimAfter.Valid {
				t.Fatal("rolled-back submission cleared reclamation schedule")
			}
			if !test.failRiver && asset.ReclaimAfter.Valid {
				t.Fatalf("committed task ownership kept reclamation scheduled at %v", asset.ReclaimAfter.Time)
			}
		})
	}
}

func assertMediaRefCount(t *testing.T, db *sql.DB, assetID int64, refType string, want int) {
	t.Helper()
	var count int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM media_refs WHERE media_asset_id=$1 AND ref_type=$2`, assetID, refType).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("%s ref count=%d want=%d", refType, count, want)
	}
}
