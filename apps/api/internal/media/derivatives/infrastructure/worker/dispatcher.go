package worker

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/media/derivatives/application/processing"
	"ideaboard/api/internal/media/derivatives/application/reconciliation"
	"ideaboard/api/internal/media/derivatives/application/repair"
	"ideaboard/api/internal/media/derivatives/domain"
	"ideaboard/api/internal/media/derivatives/ports"
	"ideaboard/api/internal/media/maintenance"
)

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
	mediaDerivativeReconcileMissing = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ideaboard_media_derivative_reconcile_missing_total",
		Help: "Total media derivatives the reconciler flipped ready → missing.",
	})
	mediaDerivativeReconcileInvalid = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ideaboard_media_derivative_reconcile_invalid_total",
		Help: "Total media derivatives the reconciler flipped ready → invalid.",
	})
	mediaDerivativeReconcileErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ideaboard_media_derivative_reconcile_errors_total",
		Help: "Total per-row reconcile probe errors. Persistent growth means storage is degraded.",
	})
	mediaDerivativeRepairTerminals = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ideaboard_media_derivative_repair_terminals_total",
		Help: "Total media derivatives that hit the bounded repair attempt cap and were marked repair_not_possible.",
	})
)

func init() {
	prometheus.MustRegister(
		mediaDerivativeReconcileRuns,
		mediaDerivativeOrphanObjects,
		mediaDerivativeOrphanScanFailures,
		mediaDerivativeReconcileMissing,
		mediaDerivativeReconcileInvalid,
		mediaDerivativeReconcileErrors,
		mediaDerivativeRepairTerminals,
	)
}

type DispatchScheduleArgs struct{}

func (DispatchScheduleArgs) Kind() string { return "media_derivative_outbox_schedule" }

type OutboxDispatcher struct {
	river.WorkerDefaults[DispatchScheduleArgs]
	db             *sql.DB
	q              *dbsqlc.Queries
	storage        ports.Storage
	verifier       ports.Verifier
	orphanScanMu   sync.Mutex
	lastOrphanScan time.Time
	reconciler     *reconciliation.Reconciler
	repair         *repair.Repair
}

func NewOutboxDispatcher(db *sql.DB, storage ports.Storage, verifier ports.Verifier) *OutboxDispatcher {
	d := &OutboxDispatcher{
		db:       db,
		q:        dbsqlc.New(db),
		storage:  storage,
		verifier: verifier,
	}
	d.reconciler = reconciliation.NewReconciler(d.q, verifier, nil, reconciliation.ReconcilerConfig{})
	d.repair = repair.NewRepair(db, nil, repair.RepairConfig{})
	return d
}

func (d *OutboxDispatcher) SetReconciler(events ports.EventPublisher) {
	if d == nil {
		return
	}
	d.reconciler = reconciliation.NewReconciler(d.q, d.verifier, events, reconciliation.ReconcilerConfig{})
	d.repair = repair.NewRepair(d.db, events, repair.RepairConfig{})
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
	ids, err := d.q.ListPendingMediaDerivativeIDs(ctx, dbsqlc.ListPendingMediaDerivativeIDsParams{
		MaxAttemptCount: domain.MaxAttempts,
		LimitCount:      reconcileBatchSize,
	})
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := client.Insert(ctx, JobArgs{DerivativeID: id}, DerivativeJobInsertOpts()); err != nil {
			return err
		}
	}
	return nil
}

func DerivativeJobUniqueOpts() river.UniqueOpts {
	return river.UniqueOpts{
		ByArgs: true,
		ByState: []rivertype.JobState{
			rivertype.JobStateAvailable,
			rivertype.JobStatePending,
			rivertype.JobStateRunning,
			rivertype.JobStateRetryable,
			rivertype.JobStateScheduled,
		},
	}
}

func DerivativeJobInsertOpts() *river.InsertOpts {
	return &river.InsertOpts{
		Queue:       QueueName,
		MaxAttempts: domain.MaxAttempts,
		Priority:    4,
		UniqueOpts:  DerivativeJobUniqueOpts(),
	}
}

func (d *OutboxDispatcher) reconcile(ctx context.Context) error {
	mediaDerivativeReconcileRuns.Inc()
	if err := d.reconcileStaleMediaUploads(ctx); err != nil {
		return err
	}
	if err := d.reconcileReadyAssets(ctx); err != nil {
		return err
	}
	d.reconcileReadyObjects(ctx)
	d.repairMissingObjects(ctx)
	if err := d.restoreStaleProcessing(ctx); err != nil {
		return err
	}
	if err := d.restoreMissingOutbox(ctx); err != nil {
		return err
	}
	if err := d.reconcileOrphanObjects(ctx); err != nil {
		slog.Warn("media derivative orphan scan failed", "err", err)
	}
	return nil
}

func (d *OutboxDispatcher) reconcileReadyObjects(ctx context.Context) {
	if d.reconciler == nil {
		return
	}
	result, err := d.reconciler.Reconcile(ctx)
	if err != nil {
		slog.Warn("media derivative ready-object reconcile failed", "err", err)
		return
	}
	if result.Missing > 0 {
		mediaDerivativeReconcileMissing.Add(float64(result.Missing))
	}
	if result.Invalid > 0 {
		mediaDerivativeReconcileInvalid.Add(float64(result.Invalid))
	}
	if result.Errors > 0 {
		mediaDerivativeReconcileErrors.Add(float64(result.Errors))
	}
}

func (d *OutboxDispatcher) repairMissingObjects(ctx context.Context) {
	if d.repair == nil {
		return
	}
	_, terminals, err := d.repair.RunOnce(ctx, reconcileBatchSize)
	if err != nil {
		slog.Warn("media derivative repair run failed", "err", err)
		return
	}
	if len(terminals) > 0 {
		mediaDerivativeRepairTerminals.Add(float64(len(terminals)))
	}
}

func (d *OutboxDispatcher) restoreStaleProcessing(ctx context.Context) error {
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
			MaxAttemptCount: domain.MaxAttempts,
		}); err != nil {
			return fmt.Errorf("reset stale media derivatives: %w", err)
		}
	}
	return nil
}

func (d *OutboxDispatcher) restoreMissingOutbox(ctx context.Context) error {
	missingIDs, err := d.q.ListMediaDerivativeIDsMissingOutbox(ctx, dbsqlc.ListMediaDerivativeIDsMissingOutboxParams{
		MaxAttemptCount: domain.MaxAttempts,
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
	return nil
}

func (d *OutboxDispatcher) reconcileReadyAssets(ctx context.Context) error {
	assets, err := d.q.ListReadyMediaAssetIDsMissingDerivatives(ctx, reconcileBatchSize)
	if err != nil {
		return fmt.Errorf("list ready assets missing derivatives: %w", err)
	}
	for _, asset := range assets {
		if _, err := processing.ScheduleDefault(ctx, d.q, asset.ID, asset.OwnerUserID); err != nil {
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
	result := reconciliation.ScanDerivativeOrphanObjects(
		ctx,
		profiles,
		expected,
		d.storage.ListObjectKeysByStorageProfile,
	)
	for _, failure := range result.Failures {
		mediaDerivativeOrphanScanFailures.Inc()
		slog.Warn(
			"storage profile derivative orphan scan failed",
			"storage_profile_id", failure.StorageProfileID,
			"err", failure.Err,
		)
	}
	for _, finding := range result.Findings {
		mediaDerivativeOrphanObjects.Add(float64(finding.Count))
		slog.Warn(
			"orphan media derivative objects detected",
			"storage_profile_id", finding.StorageProfileID,
			"count", finding.Count,
		)
	}
	return nil
}

func (d *OutboxDispatcher) reconcileStaleMediaUploads(ctx context.Context) error {
	staleBefore := time.Now().Add(-uploadingLease)
	ids, err := d.q.ListStaleUploadingMediaAssetIDs(ctx, dbsqlc.ListStaleUploadingMediaAssetIDsParams{
		StaleBefore: staleBefore,
		RowLimit:    reconcileBatchSize,
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
			return DispatchScheduleArgs{}, &river.InsertOpts{
				Queue:       QueueName,
				MaxAttempts: domain.MaxAttempts,
				Priority:    4,
			}
		},
		&river.PeriodicJobOpts{ID: "media-derivative-outbox-schedule", RunOnStart: true},
	)}
}
