package reclamation

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	identitydomain "ideaboard/api/internal/identity/users/domain"
	identitypostgres "ideaboard/api/internal/identity/users/infrastructure/postgres"
	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"
	"ideaboard/api/internal/testdb"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
)

func TestReclaimWorkerDeletesFromRecordedProfileAndRetiresURL(t *testing.T) {
	db := testdb.Open(t)
	q := dbsqlc.New(db)
	assetID, generation := seedDueMediaAsset(t, db, q)
	storage := &recordingObjectDeleter{}
	worker := NewReclaimWorker(db, storage)
	worker.now = func() time.Time { return time.Now() }

	if err := worker.Work(t.Context(), &river.Job[ReclaimJobArgs]{Args: ReclaimJobArgs{MediaAssetID: assetID, ReclaimGeneration: generation}}); err != nil {
		t.Fatalf("reclaim worker failed: %v", err)
	}
	if storage.profileID != "builtin-local" || storage.objectKey != "uploads/tasks/1/task/output.png" {
		t.Fatalf("storage delete = profile:%q key:%q", storage.profileID, storage.objectKey)
	}
	asset, err := q.GetMediaAssetByID(t.Context(), assetID)
	if err != nil {
		t.Fatal(err)
	}
	if asset.Status != "deleted" || asset.Url != "" || !asset.DeletedAt.Valid {
		t.Fatalf("asset after reclamation = %#v, want deleted with cleared URL", asset)
	}
}

func TestReclaimWorkerTreatsObjectNotFoundAsSuccess(t *testing.T) {
	db := testdb.Open(t)
	q := dbsqlc.New(db)
	assetID, generation := seedDueMediaAsset(t, db, q)
	worker := NewReclaimWorker(db, objectDeleterFunc(func(context.Context, string, string) error { return fs.ErrNotExist }))

	if err := worker.Work(t.Context(), &river.Job[ReclaimJobArgs]{Args: ReclaimJobArgs{MediaAssetID: assetID, ReclaimGeneration: generation}}); err != nil {
		t.Fatalf("not found should converge as success: %v", err)
	}
	asset, err := q.GetMediaAssetByID(t.Context(), assetID)
	if err != nil || asset.Status != "deleted" {
		t.Fatalf("asset after not-found reclamation = %#v err=%v", asset, err)
	}
}

func TestReclaimWorkerDoesNotClaimAssetOwnedByActiveTask(t *testing.T) {
	db := testdb.Open(t)
	q := dbsqlc.New(db)
	assetID, generation := seedDueMediaAsset(t, db, q)
	assetBefore, err := q.GetMediaAssetByID(t.Context(), assetID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpsertMediaRef(t.Context(), dbsqlc.UpsertMediaRefParams{
		MediaAssetID: assetID,
		OwnerUserID:  assetBefore.OwnerUserID,
		RefType:      "task_input",
		RefID:        "task-active-media",
		TaskID:       "task-active-media",
		Role:         "input",
		Metadata:     []byte(`{}`),
	}); err != nil {
		t.Fatalf("seed active task media ref: %v", err)
	}
	storage := &recordingObjectDeleter{}
	worker := NewReclaimWorker(db, storage)
	if err := worker.Work(t.Context(), &river.Job[ReclaimJobArgs]{Args: ReclaimJobArgs{MediaAssetID: assetID, ReclaimGeneration: generation}}); err != nil {
		t.Fatalf("active task ref should make reclamation a no-op: %v", err)
	}
	asset, err := q.GetMediaAssetByID(t.Context(), assetID)
	if err != nil || asset.Status != "ready" || asset.DeletedAt.Valid {
		t.Fatalf("asset after protected reclamation = %#v err=%v", asset, err)
	}
	if storage.objectKey != "" {
		t.Fatalf("active task asset was deleted: %q", storage.objectKey)
	}
}

func TestReclaimWorkerReleasesClaimForRiverRetryAfterStorageFailure(t *testing.T) {
	db := testdb.Open(t)
	q := dbsqlc.New(db)
	assetID, generation := seedDueMediaAsset(t, db, q)
	storage := &recordingObjectDeleter{err: errors.New("temporary storage outage")}
	worker := NewReclaimWorker(db, storage)
	job := &river.Job[ReclaimJobArgs]{Args: ReclaimJobArgs{MediaAssetID: assetID, ReclaimGeneration: generation}}

	if err := worker.Work(t.Context(), job); err == nil {
		t.Fatal("storage failure should be retried by River")
	}
	asset, err := q.GetMediaAssetByID(t.Context(), assetID)
	if err != nil || asset.ReclaimStartedAt.Valid || asset.Status != "ready" {
		t.Fatalf("asset after retryable failure = %#v err=%v", asset, err)
	}
	storage.err = nil
	if err := worker.Work(t.Context(), job); err != nil {
		t.Fatalf("retry should reclaim asset: %v", err)
	}
}

func TestReclaimWorkerRecoversAStaleClaim(t *testing.T) {
	db := testdb.Open(t)
	q := dbsqlc.New(db)
	assetID, generation := seedDueMediaAsset(t, db, q)
	now := time.Now()
	staleClaimedAt := now.Add(-ClaimLeaseDuration - time.Second)
	if _, err := q.ClaimDueMediaAssetReclamation(t.Context(), dbsqlc.ClaimDueMediaAssetReclamationParams{
		ID:                assetID,
		ReclaimGeneration: generation,
		ClaimedAt:         pgtype.Timestamptz{Time: staleClaimedAt, Valid: true},
		ClaimToken:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
		NowAt:             pgtype.Timestamptz{Time: now, Valid: true},
		StaleClaimBefore:  pgtype.Timestamptz{Time: now.Add(-ClaimLeaseDuration), Valid: true},
	}); err != nil {
		t.Fatalf("seed stale reclamation claim: %v", err)
	}

	storage := &recordingObjectDeleter{}
	worker := NewReclaimWorker(db, storage)
	worker.now = func() time.Time { return now }
	if err := worker.Work(t.Context(), &river.Job[ReclaimJobArgs]{Args: ReclaimJobArgs{MediaAssetID: assetID, ReclaimGeneration: generation}}); err != nil {
		t.Fatalf("stale claim recovery failed: %v", err)
	}
	asset, err := q.GetMediaAssetByID(t.Context(), assetID)
	if err != nil || asset.Status != "deleted" || !asset.DeletedAt.Valid {
		t.Fatalf("asset after stale claim recovery = %#v err=%v", asset, err)
	}
	if storage.objectKey == "" {
		t.Fatal("stale claim recovery did not delete the object")
	}
}

func TestReclaimWorkerDoesNotStealFreshClaim(t *testing.T) {
	db := testdb.Open(t)
	q := dbsqlc.New(db)
	assetID, generation := seedDueMediaAsset(t, db, q)
	now := time.Now()
	claimToken := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	if _, err := q.ClaimDueMediaAssetReclamation(t.Context(), dbsqlc.ClaimDueMediaAssetReclamationParams{
		ID:                assetID,
		ReclaimGeneration: generation,
		ClaimedAt:         pgtype.Timestamptz{Time: now, Valid: true},
		ClaimToken:        claimToken,
		NowAt:             pgtype.Timestamptz{Time: now, Valid: true},
		StaleClaimBefore:  pgtype.Timestamptz{Time: now.Add(-ClaimLeaseDuration), Valid: true},
	}); err != nil {
		t.Fatalf("seed fresh reclamation claim: %v", err)
	}

	storage := &recordingObjectDeleter{}
	worker := NewReclaimWorker(db, storage)
	worker.now = func() time.Time { return now.Add(ClaimLeaseDuration - time.Minute) }
	err := worker.Work(t.Context(), &river.Job[ReclaimJobArgs]{Args: ReclaimJobArgs{MediaAssetID: assetID, ReclaimGeneration: generation}})
	var snoozeErr *river.JobSnoozeError
	if !errors.As(err, &snoozeErr) {
		t.Fatalf("fresh claim should snooze, got %v", err)
	}
	asset, err := q.GetMediaAssetByID(t.Context(), assetID)
	if err != nil || !asset.ReclaimStartedAt.Valid || asset.ReclaimClaimToken != claimToken || asset.Status != "ready" {
		t.Fatalf("asset after fresh claim contention = %#v err=%v", asset, err)
	}
	if storage.objectKey != "" {
		t.Fatalf("fresh claim was stolen and deleted object %q", storage.objectKey)
	}
}

func TestReclaimWorkerRetriesAfterObjectDeleteLosesItsClaim(t *testing.T) {
	db := testdb.Open(t)
	q := dbsqlc.New(db)
	assetID, generation := seedDueMediaAsset(t, db, q)
	now := time.Now()
	replacementToken := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	deleteCalls := 0
	storage := objectDeleterFunc(func(context.Context, string, string) error {
		deleteCalls++
		if deleteCalls == 1 {
			if _, err := db.Exec(t.Context(), `
UPDATE media_assets
SET reclaim_started_at = $1,
    reclaim_claim_token = $2
WHERE id = $3`, now, replacementToken, assetID); err != nil {
				t.Fatalf("replace claim after object delete: %v", err)
			}
		}
		return nil
	})
	worker := NewReclaimWorker(db, storage)
	worker.now = func() time.Time { return now }
	job := &river.Job[ReclaimJobArgs]{Args: ReclaimJobArgs{MediaAssetID: assetID, ReclaimGeneration: generation}}
	err := worker.Work(t.Context(), job)
	var snoozeErr *river.JobSnoozeError
	if !errors.As(err, &snoozeErr) {
		t.Fatalf("lost claim should snooze for recovery, got %v", err)
	}
	asset, err := q.GetMediaAssetByID(t.Context(), assetID)
	if err != nil || asset.Status != "ready" || asset.ReclaimClaimToken != replacementToken {
		t.Fatalf("asset after lost mark = %#v err=%v", asset, err)
	}

	worker.now = func() time.Time { return now.Add(ClaimLeaseDuration + time.Minute) }
	if err := worker.Work(t.Context(), job); err != nil {
		t.Fatalf("stale replacement claim retry failed: %v", err)
	}
	asset, err = q.GetMediaAssetByID(t.Context(), assetID)
	if err != nil || asset.Status != "deleted" || deleteCalls != 2 {
		t.Fatalf("asset after retry = %#v deleteCalls=%d err=%v", asset, deleteCalls, err)
	}
}

func TestReclaimWorkerIgnoresCompletedGenerationAfterUndoAndReschedule(t *testing.T) {
	db := testdb.Open(t)
	q := dbsqlc.New(db)
	assetID, firstGeneration := seedDueMediaAsset(t, db, q)
	storage := &recordingObjectDeleter{}
	worker := NewReclaimWorker(db, storage)

	cancelled, err := q.CancelMediaAssetReclamation(t.Context(), dbsqlc.CancelMediaAssetReclamationParams{
		MediaAssetIds:    []int64{assetID},
		StaleClaimBefore: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil || len(cancelled) != 1 {
		t.Fatalf("cancel first generation = %v err=%v", cancelled, err)
	}
	if err := q.ScheduleUnreferencedMediaAssetsForReclamation(t.Context(), dbsqlc.ScheduleUnreferencedMediaAssetsForReclamationParams{
		ReclaimAfter:  pgtype.Timestamptz{Time: time.Now().Add(-time.Second), Valid: true},
		MediaAssetIds: []int64{assetID},
	}); err != nil {
		t.Fatalf("schedule second generation: %v", err)
	}
	asset, err := q.GetMediaAssetByID(t.Context(), assetID)
	if err != nil {
		t.Fatal(err)
	}
	if asset.ReclaimGeneration != firstGeneration+1 {
		t.Fatalf("second generation = %d, want %d", asset.ReclaimGeneration, firstGeneration+1)
	}

	if err := worker.Work(t.Context(), &river.Job[ReclaimJobArgs]{Args: ReclaimJobArgs{
		MediaAssetID:      assetID,
		ReclaimGeneration: firstGeneration,
	}}); err != nil {
		t.Fatalf("stale generation should no-op: %v", err)
	}
	if storage.objectKey != "" {
		t.Fatalf("stale generation deleted object %q", storage.objectKey)
	}

	if err := worker.Work(t.Context(), &river.Job[ReclaimJobArgs]{Args: ReclaimJobArgs{
		MediaAssetID:      assetID,
		ReclaimGeneration: asset.ReclaimGeneration,
	}}); err != nil {
		t.Fatalf("current generation reclaim failed: %v", err)
	}
	if storage.objectKey == "" {
		t.Fatal("current generation did not delete object")
	}
}

func seedDueMediaAsset(t *testing.T, db *pgxpool.Pool, q *dbsqlc.Queries) (int64, int64) {
	t.Helper()
	owner := &identitydomain.User{DisplayName: "reclaim-owner", Status: 1, Role: "user"}
	if err := identitypostgres.NewRepository(db).Create(context.Background(), owner); err != nil {
		t.Fatalf("create owner: %v", err)
	}
	assetID, err := q.UpsertMediaAsset(context.Background(), dbsqlc.UpsertMediaAssetParams{
		AssetID:          "asset-reclaim-" + uuid.NewString(),
		OwnerUserID:      owner.ID,
		StorageProfileID: "builtin-local",
		StorageKey:       "uploads/tasks/1/task/output.png",
		Url:              "/uploads/tasks/1/task/output.png",
		MediaKind:        "image",
		MimeType:         "image/png",
		Status:           "ready",
		Metadata:         []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("create media asset: %v", err)
	}
	if err := q.ScheduleUnreferencedMediaAssetsForReclamation(context.Background(), dbsqlc.ScheduleUnreferencedMediaAssetsForReclamationParams{
		ReclaimAfter:  pgtype.Timestamptz{Time: time.Now().Add(-time.Second), Valid: true},
		MediaAssetIds: []int64{assetID},
	}); err != nil {
		t.Fatalf("schedule media asset: %v", err)
	}
	asset, err := q.GetMediaAssetByID(context.Background(), assetID)
	if err != nil {
		t.Fatalf("load scheduled media asset: %v", err)
	}
	return assetID, asset.ReclaimGeneration
}

type recordingObjectDeleter struct {
	profileID string
	objectKey string
	err       error
}

func (d *recordingObjectDeleter) DeleteObjectByStorageProfile(_ context.Context, profileID, objectKey string) error {
	d.profileID = profileID
	d.objectKey = objectKey
	return d.err
}

type objectDeleterFunc func(context.Context, string, string) error

func (f objectDeleterFunc) DeleteObjectByStorageProfile(ctx context.Context, profileID, objectKey string) error {
	return f(ctx, profileID, objectKey)
}

func TestReclaimWorkerTimeoutStaysBelowClaimLease(t *testing.T) {
	worker := &ReclaimWorker{}
	timeout := worker.Timeout(&river.Job[ReclaimJobArgs]{})
	if timeout <= 0 || timeout >= ClaimLeaseDuration {
		t.Fatalf("reclaim timeout=%s, claim lease=%s", timeout, ClaimLeaseDuration)
	}
	if timeout != taskqueue.RiverJobTimeoutMediaReclamation {
		t.Fatalf("reclaim timeout=%s, want matrix %s", timeout, taskqueue.RiverJobTimeoutMediaReclamation)
	}
}

func TestScheduleWorkerPinsTimeoutMatrix(t *testing.T) {
	worker := &ScheduleWorker{}
	if timeout := worker.Timeout(&river.Job[ScheduleJobArgs]{}); timeout != taskqueue.RiverJobTimeoutMediaReclamationSchedule {
		t.Fatalf("reclamation schedule timeout=%s, want matrix %s", timeout, taskqueue.RiverJobTimeoutMediaReclamationSchedule)
	}
}
