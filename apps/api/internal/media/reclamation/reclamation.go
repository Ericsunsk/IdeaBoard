package reclamation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

const (
	QueueName         = "media_reclamation"
	ReclaimJobKind    = "media_asset_reclaim"
	ScheduleJobKind   = "media_reclamation_schedule"
	scheduleInterval  = 15 * time.Minute
	GracePeriod       = 24 * time.Hour
	reclaimJobTimeout = 2 * time.Minute
	// The lease outlives this worker's dedicated timeout while still allowing a
	// crashed process to recover independently of the configurable task timeout.
	ClaimLeaseDuration       = 5 * time.Minute
	dueAssetBatchSize  int32 = 100
)

type ReclaimJobArgs struct {
	MediaAssetID      int64 `json:"media_asset_id"`
	ReclaimGeneration int64 `json:"reclaim_generation"`
}

func (ReclaimJobArgs) Kind() string { return ReclaimJobKind }

type ScheduleJobArgs struct{}

func (ScheduleJobArgs) Kind() string { return ScheduleJobKind }

type ObjectDeleter interface {
	DeleteObjectByStorageProfile(context.Context, string, string) error
}

type ReclaimWorker struct {
	river.WorkerDefaults[ReclaimJobArgs]
	q       *dbsqlc.Queries
	storage ObjectDeleter
	now     func() time.Time
}

func NewReclaimWorker(db *sql.DB, storage ObjectDeleter) *ReclaimWorker {
	return &ReclaimWorker{q: dbsqlc.New(db), storage: storage, now: time.Now}
}

func (*ReclaimWorker) Timeout(*river.Job[ReclaimJobArgs]) time.Duration {
	return reclaimJobTimeout
}

func (w *ReclaimWorker) Work(ctx context.Context, job *river.Job[ReclaimJobArgs]) error {
	if !validReclaimJob(w, job) {
		return fmt.Errorf("invalid media reclamation job")
	}
	args := job.Args
	now := w.now()
	claimToken := uuid.NullUUID{UUID: uuid.New(), Valid: true}
	asset, claimed, err := w.claimAsset(ctx, args, now, claimToken)
	if !claimed {
		return w.resolveSkippedClaim(ctx, args, now)
	}
	if err != nil {
		return err
	}
	if err := w.deleteClaimedObjects(ctx, asset, args, claimToken); err != nil {
		return err
	}
	return w.finalizeReclamation(ctx, asset, args, claimToken)
}

func validReclaimJob(w *ReclaimWorker, job *river.Job[ReclaimJobArgs]) bool {
	return w != nil && w.q != nil && w.storage != nil && job != nil &&
		job.Args.MediaAssetID > 0 && job.Args.ReclaimGeneration > 0
}

func (w *ReclaimWorker) claimAsset(ctx context.Context, args ReclaimJobArgs, now time.Time, claimToken uuid.NullUUID) (dbsqlc.MediaAsset, bool, error) {
	asset, err := w.q.ClaimDueMediaAssetReclamation(ctx, dbsqlc.ClaimDueMediaAssetReclamationParams{
		ID:                args.MediaAssetID,
		ReclaimGeneration: args.ReclaimGeneration,
		ClaimedAt:         sql.NullTime{Time: now, Valid: true},
		ClaimToken:        claimToken,
		NowAt:             sql.NullTime{Time: now, Valid: true},
		StaleClaimBefore:  sql.NullTime{Time: now.Add(-ClaimLeaseDuration), Valid: true},
	})
	if database.IsNotFound(err) || errors.Is(err, sql.ErrNoRows) {
		return dbsqlc.MediaAsset{}, false, nil
	}
	if err != nil {
		return dbsqlc.MediaAsset{}, true, fmt.Errorf("claim media asset %d: %w", args.MediaAssetID, err)
	}
	return asset, true, nil
}

func (w *ReclaimWorker) deleteClaimedObjects(ctx context.Context, asset dbsqlc.MediaAsset, args ReclaimJobArgs, claimToken uuid.NullUUID) error {
	if err := w.storage.DeleteObjectByStorageProfile(ctx, asset.StorageProfileID, asset.StorageKey); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return w.releaseClaimOnError(ctx, asset.ID, args.ReclaimGeneration, claimToken, fmt.Errorf("delete media asset %d: %w", asset.ID, err))
	}
	derivatives, err := w.q.ListMediaDerivativesByAsset(ctx, dbsqlc.ListMediaDerivativesByAssetParams{
		MediaAssetID: asset.ID, OwnerUserID: asset.OwnerUserID,
	})
	if err != nil {
		return fmt.Errorf("list media asset %d derivatives: %w", asset.ID, err)
	}
	for _, derivative := range derivatives {
		if derivative.StorageKey == "" || derivative.StorageProfileID == "" {
			continue
		}
		if err := w.storage.DeleteObjectByStorageProfile(ctx, derivative.StorageProfileID, derivative.StorageKey); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return w.releaseClaimOnError(ctx, asset.ID, args.ReclaimGeneration, claimToken, fmt.Errorf("delete media asset %d derivative %s: %w", asset.ID, derivative.Tier, err))
		}
	}
	return nil
}

func (w *ReclaimWorker) releaseClaimOnError(ctx context.Context, assetID, generation int64, claimToken uuid.NullUUID, operationErr error) error {
	releaseCtx := context.WithoutCancel(ctx)
	if releaseErr := w.q.ReleaseMediaAssetReclamationClaim(releaseCtx, dbsqlc.ReleaseMediaAssetReclamationClaimParams{ID: assetID, ReclaimGeneration: generation, ClaimToken: claimToken}); releaseErr != nil {
		return errors.Join(operationErr, fmt.Errorf("release media reclamation claim: %w", releaseErr))
	}
	return operationErr
}

func (w *ReclaimWorker) finalizeReclamation(ctx context.Context, asset dbsqlc.MediaAsset, args ReclaimJobArgs, claimToken uuid.NullUUID) error {
	if err := w.q.DeleteMediaDerivativesByAsset(ctx, dbsqlc.DeleteMediaDerivativesByAssetParams{MediaAssetID: asset.ID, OwnerUserID: asset.OwnerUserID}); err != nil {
		return fmt.Errorf("delete media asset %d derivative records: %w", asset.ID, err)
	}
	rows, err := w.q.MarkMediaAssetReclaimed(ctx, dbsqlc.MarkMediaAssetReclaimedParams{
		ID:                asset.ID,
		ReclaimGeneration: args.ReclaimGeneration,
		ClaimToken:        claimToken,
		DeletedAt:         sql.NullTime{Time: w.now(), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("mark media asset %d reclaimed: %w", asset.ID, err)
	}
	if rows == 0 {
		return w.resolveSkippedClaim(ctx, args, w.now())
	}
	return nil
}

func (w *ReclaimWorker) resolveSkippedClaim(ctx context.Context, args ReclaimJobArgs, now time.Time) error {
	asset, err := w.q.GetMediaAssetByID(ctx, args.MediaAssetID)
	if database.IsNotFound(err) || errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect media asset %d reclamation claim: %w", args.MediaAssetID, err)
	}
	if asset.DeletedAt.Valid || asset.ReclaimGeneration != args.ReclaimGeneration || !asset.ReclaimAfter.Valid {
		return nil
	}
	if asset.ReclaimStartedAt.Valid {
		retryAt := asset.ReclaimStartedAt.Time.Add(ClaimLeaseDuration)
		if retryAt.After(now) {
			return river.JobSnooze(retryAt.Sub(now))
		}
	}
	return nil
}

type ScheduleWorker struct {
	river.WorkerDefaults[ScheduleJobArgs]
	q   *dbsqlc.Queries
	now func() time.Time
}

func NewScheduleWorker(db *sql.DB) *ScheduleWorker {
	return &ScheduleWorker{q: dbsqlc.New(db), now: time.Now}
}

func (w *ScheduleWorker) Work(ctx context.Context, job *river.Job[ScheduleJobArgs]) error {
	if w == nil || w.q == nil || job == nil {
		return fmt.Errorf("invalid media reclamation schedule job")
	}
	now := w.now()
	assetIDs, err := w.q.ListDueMediaAssetReclamationIDs(ctx, dbsqlc.ListDueMediaAssetReclamationIDsParams{
		NowAt:            sql.NullTime{Time: now, Valid: true},
		StaleClaimBefore: sql.NullTime{Time: now.Add(-ClaimLeaseDuration), Valid: true},
		RowLimit:         dueAssetBatchSize,
	})
	if err != nil {
		return fmt.Errorf("list due media reclamation assets: %w", err)
	}
	if len(assetIDs) == 0 {
		return nil
	}
	client, err := river.ClientFromContextSafely[pgx.Tx](ctx)
	if err != nil {
		return fmt.Errorf("get River client for media reclamation: %w", err)
	}
	for _, asset := range assetIDs {
		if _, err := client.Insert(ctx, ReclaimJobArgs{MediaAssetID: asset.ID, ReclaimGeneration: asset.ReclaimGeneration}, &river.InsertOpts{
			Queue:       QueueName,
			MaxAttempts: 10,
			Priority:    4,
			UniqueOpts:  river.UniqueOpts{ByArgs: true},
		}); err != nil {
			return fmt.Errorf("enqueue media reclamation asset %d: %w", asset.ID, err)
		}
	}
	return nil
}

func PeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{river.NewPeriodicJob(
		river.PeriodicInterval(scheduleInterval),
		func() (river.JobArgs, *river.InsertOpts) {
			return ScheduleJobArgs{}, &river.InsertOpts{Queue: QueueName, MaxAttempts: 10, Priority: 4}
		},
		&river.PeriodicJobOpts{ID: "media-reclamation-schedule", RunOnStart: true},
	)}
}
