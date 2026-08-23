package derivatives

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/riverqueue/river"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/media/maintenance"
	"ideaboard/api/internal/media/storage"
)

const QueueName = "media_derivatives"

const (
	reconcileBatchSize = maintenance.BatchSize
	processingLease    = maintenance.ProcessingLease
	uploadingLease     = maintenance.UploadingLease
	orphanScanInterval = maintenance.OrphanScanInterval
)

var (
	mediaDerivativeReconcileRuns = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ideaboard_media_derivative_reconcile_runs_total",
		Help: "Total media derivative reconciler runs.",
	})
	mediaDerivativeOrphanObjects = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ideaboard_media_derivative_orphan_objects_total",
		Help: "Total physical media derivative objects without a database record.",
	})
	mediaDerivativeOrphanScanFailures = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ideaboard_media_derivative_orphan_scan_failures_total",
		Help: "Total storage profile failures encountered while scanning for orphan media derivatives.",
	})
)

func init() {
	prometheus.MustRegister(
		mediaDerivativeReconcileRuns,
		mediaDerivativeOrphanObjects,
		mediaDerivativeOrphanScanFailures,
	)
}

type JobArgs struct {
	DerivativeID int64 `json:"derivative_id"`
}

func (JobArgs) Kind() string { return "media_derivative" }

type Worker struct {
	river.WorkerDefaults[JobArgs]
	service *Service
}

type OutboxDispatcher struct {
	river.WorkerDefaults[DispatchScheduleArgs]
	q              *dbsqlc.Queries
	storage        *storage.StorageService
	orphanScanMu   sync.Mutex
	lastOrphanScan time.Time
}

func NewOutboxDispatcher(db *sql.DB, storageService *storage.StorageService) *OutboxDispatcher {
	return &OutboxDispatcher{q: dbsqlc.New(db), storage: storageService}
}

type DispatchScheduleArgs struct{}

func (DispatchScheduleArgs) Kind() string { return "media_derivative_outbox_schedule" }

type derivativeOrphanFinding struct {
	storageProfileID string
	count            int
}

type derivativeOrphanScanFailure struct {
	storageProfileID string
	err              error
}

type derivativeOrphanScanResult struct {
	findings []derivativeOrphanFinding
	failures []derivativeOrphanScanFailure
}

func (d *OutboxDispatcher) Work(ctx context.Context, job *river.Job[DispatchScheduleArgs]) error {
	if d == nil || d.q == nil || job == nil {
		return fmt.Errorf("invalid media derivative outbox job")
	}
	client, err := river.ClientFromContextSafely[pgx.Tx](ctx)
	if err != nil {
		return fmt.Errorf("get River client for media derivative outbox: %w", err)
	}
	if err := d.reconcile(ctx); err != nil {
		return err
	}
	ids, err := d.q.ListPendingMediaDerivativeIDs(ctx, dbsqlc.ListPendingMediaDerivativeIDsParams{MaxAttemptCount: MaxAttempts, LimitCount: reconcileBatchSize})
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := client.Insert(ctx, JobArgs{DerivativeID: id}, &river.InsertOpts{Queue: QueueName, MaxAttempts: 5, Priority: 4, UniqueOpts: river.UniqueOpts{ByArgs: true}}); err != nil {
			return err
		}
	}
	return nil
}

func (d *OutboxDispatcher) reconcile(ctx context.Context) error {
	mediaDerivativeReconcileRuns.Inc()
	if err := d.reconcileStaleMediaUploads(ctx); err != nil {
		return err
	}
	if err := d.reconcileReadyAssets(ctx); err != nil {
		return err
	}
	staleBefore := time.Now().Add(-processingLease)
	staleIDs, err := d.q.ListStaleProcessingMediaDerivativeIDs(ctx, dbsqlc.ListStaleProcessingMediaDerivativeIDsParams{
		StaleBefore: staleBefore,
		RowLimit:    reconcileBatchSize,
	})
	if err != nil {
		return fmt.Errorf("list stale media derivatives: %w", err)
	}
	if len(staleIDs) > 0 {
		if _, err := d.q.ResetStaleProcessingMediaDerivatives(ctx, dbsqlc.ResetStaleProcessingMediaDerivativesParams{
			Ids:             staleIDs,
			StaleBefore:     staleBefore,
			MaxAttemptCount: MaxAttempts,
		}); err != nil {
			return fmt.Errorf("reset stale media derivatives: %w", err)
		}
	}
	missingIDs, err := d.q.ListMediaDerivativeIDsMissingOutbox(ctx, dbsqlc.ListMediaDerivativeIDsMissingOutboxParams{
		MaxAttemptCount: MaxAttempts,
		RowLimit:        reconcileBatchSize,
	})
	if err != nil {
		return fmt.Errorf("list media derivatives missing outbox: %w", err)
	}
	for _, id := range missingIDs {
		if _, err := d.q.EnqueueMediaDerivativeOutbox(ctx, id); err != nil {
			return fmt.Errorf("restore media derivative outbox id=%d: %w", id, err)
		}
	}
	if err := d.reconcileOrphanObjects(ctx); err != nil {
		slog.Warn("media derivative orphan scan failed", "err", err)
	}
	return nil
}

func (d *OutboxDispatcher) reconcileReadyAssets(ctx context.Context) error {
	assets, err := d.q.ListReadyMediaAssetIDsMissingDerivatives(ctx, reconcileBatchSize)
	if err != nil {
		return fmt.Errorf("list ready assets missing derivatives: %w", err)
	}
	for _, asset := range assets {
		if _, err := ScheduleDefault(ctx, d.q, asset.ID, asset.OwnerUserID); err != nil {
			return fmt.Errorf("restore default derivatives for asset %d: %w", asset.ID, err)
		}
	}
	return nil
}

func (d *OutboxDispatcher) reconcileOrphanObjects(ctx context.Context) error {
	if d == nil || d.storage == nil {
		return nil
	}
	now := time.Now()
	d.orphanScanMu.Lock()
	if !d.lastOrphanScan.IsZero() && now.Sub(d.lastOrphanScan) < orphanScanInterval {
		d.orphanScanMu.Unlock()
		return nil
	}
	d.lastOrphanScan = now
	d.orphanScanMu.Unlock()

	rows, err := d.q.ListMediaDerivativeStorageKeys(ctx)
	if err != nil {
		return fmt.Errorf("list derivative storage keys: %w", err)
	}
	expected := make(map[string]map[string]struct{})
	for _, row := range rows {
		if _, ok := expected[row.StorageProfileID]; !ok {
			expected[row.StorageProfileID] = make(map[string]struct{})
		}
		expected[row.StorageProfileID][row.StorageKey] = struct{}{}
	}
	profiles, err := d.storage.ListProfiles(ctx)
	if err != nil {
		return fmt.Errorf("list storage profiles for derivative scan: %w", err)
	}
	result := scanDerivativeOrphanObjects(
		ctx,
		profiles,
		expected,
		d.storage.ListObjectKeysByStorageProfile,
	)
	for _, failure := range result.failures {
		mediaDerivativeOrphanScanFailures.Inc()
		slog.Warn(
			"storage profile derivative orphan scan failed",
			"storage_profile_id", failure.storageProfileID,
			"err", failure.err,
		)
	}
	for _, finding := range result.findings {
		mediaDerivativeOrphanObjects.Add(float64(finding.count))
		slog.Warn(
			"orphan media derivative objects detected",
			"storage_profile_id", finding.storageProfileID,
			"count", finding.count,
		)
	}
	return nil
}

func scanDerivativeOrphanObjects(
	ctx context.Context,
	profiles []storage.StorageProfile,
	expected map[string]map[string]struct{},
	listObjectKeys func(context.Context, string, string) ([]string, error),
) derivativeOrphanScanResult {
	result := derivativeOrphanScanResult{}
	for _, profile := range profiles {
		if !profile.Enabled {
			continue
		}
		keys, err := listObjectKeys(ctx, profile.ID, "uploads/")
		if err != nil {
			result.failures = append(result.failures, derivativeOrphanScanFailure{
				storageProfileID: profile.ID,
				err:              err,
			})
			continue
		}
		orphanCount := 0
		for _, key := range keys {
			if !storage.IsImageDerivativeObjectKey(key) {
				continue
			}
			if _, ok := expected[profile.ID][key]; ok {
				continue
			}
			orphanCount++
		}
		if orphanCount > 0 {
			result.findings = append(result.findings, derivativeOrphanFinding{
				storageProfileID: profile.ID,
				count:            orphanCount,
			})
		}
	}
	return result
}

func (d *OutboxDispatcher) reconcileStaleMediaUploads(ctx context.Context) error {
	staleBefore := time.Now().Add(-uploadingLease)
	ids, err := d.q.ListStaleUploadingMediaAssetIDs(ctx, dbsqlc.ListStaleUploadingMediaAssetIDsParams{
		StaleBefore: staleBefore, RowLimit: reconcileBatchSize,
	})
	if err != nil {
		return fmt.Errorf("list stale media uploads: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}
	if _, err := d.q.MarkStaleUploadingMediaAssetsFailed(ctx, ids); err != nil {
		return fmt.Errorf("mark stale media uploads failed: %w", err)
	}
	return nil
}

func PeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{river.NewPeriodicJob(
		river.PeriodicInterval(30*time.Second),
		func() (river.JobArgs, *river.InsertOpts) {
			return DispatchScheduleArgs{}, &river.InsertOpts{Queue: QueueName, MaxAttempts: 5, Priority: 4}
		},
		&river.PeriodicJobOpts{ID: "media-derivative-outbox-schedule", RunOnStart: true},
	)}
}

func NewWorker(service *Service) *Worker { return &Worker{service: service} }

func (*Worker) Timeout(*river.Job[JobArgs]) time.Duration { return 5 * time.Minute }

func (w *Worker) Work(ctx context.Context, job *river.Job[JobArgs]) error {
	if w == nil || w.service == nil || job == nil || job.Args.DerivativeID <= 0 {
		return context.Canceled
	}
	err := w.service.Process(ctx, job.Args.DerivativeID)
	var recorded *failureRecordedError
	if errors.As(err, &recorded) {
		slog.Warn("media derivative processing failed", "derivative_id", job.Args.DerivativeID, "err", err)
		return nil
	}
	return err
}
