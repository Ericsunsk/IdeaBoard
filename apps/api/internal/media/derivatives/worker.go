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
	"github.com/riverqueue/river/rivertype"
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
	// ready-object existence reconciler (issue #661/#663).
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
	// bounded repair (issue #663).
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
	db             *sql.DB
	q              *dbsqlc.Queries
	storage        *storage.StorageService
	orphanScanMu   sync.Mutex
	lastOrphanScan time.Time
	// reconciler is the canonical media derivative ready-object
	// reconciler (issue #661/#663). nil until NewOutboxDispatcher
	// is called; tests can override it via SetReconciler.
	reconciler *Reconciler
	// repair is the bounded retry target for missing/invalid
	// derivatives (issue #663). nil until SetRepair is called.
	repair *Repair
}

func NewOutboxDispatcher(db *sql.DB, storageService *storage.StorageService) *OutboxDispatcher {
	d := &OutboxDispatcher{db: db, q: dbsqlc.New(db), storage: storageService}
	d.reconciler = NewReconciler(d.q, storageService, nil, ReconcilerConfig{})
	d.repair = NewRepair(db, nil, RepairConfig{})
	return d
}

// SetReconciler overrides the default reconciler. The publisher is
// passed through here so the dispatcher can fan the canonical event
// types to the same UserEventHub that publishes updated/ready.
func (d *OutboxDispatcher) SetReconciler(events EventPublisher) {
	if d == nil {
		return
	}
	d.reconciler = NewReconciler(d.q, d.storage, events, ReconcilerConfig{})
	d.repair = NewRepair(d.db, events, RepairConfig{})
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
		if _, err := client.Insert(ctx, JobArgs{DerivativeID: id}, DerivativeJobInsertOpts()); err != nil {
			return err
		}
	}
	return nil
}

// Completed jobs are historical facts, not active work. They must not block
// a retry after a derivative is reset from failed/obsolete to pending.
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
		MaxAttempts: MaxAttempts,
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

// reconcileReadyObjects observes ready derivatives without stopping the
// scheduler when object storage is transiently unavailable.
func (d *OutboxDispatcher) reconcileReadyObjects(ctx context.Context) {
	// Reconcile ready-but-object-missing or invalid derivatives
	// (issue #661/#663). Errors are logged but do not abort the
	// rest of the reconcile cycle; the runbook triages a slow
	// reconciler via the metrics counters below.
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

// repairMissingObjects has the same best-effort policy as ready-object
// reconciliation: a repair outage must not block queue recovery.
func (d *OutboxDispatcher) repairMissingObjects(ctx context.Context) {
	// Bounded repair for missing/invalid derivatives (issue
	// #663). Errors are logged but do not abort the cycle; the
	// runbook triages a stalled repair via the metrics
	// counters below.
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
			MaxAttemptCount: MaxAttempts,
		}); err != nil {
			return fmt.Errorf("reset stale media derivatives: %w", err)
		}
	}
	return nil
}

func (d *OutboxDispatcher) restoreMissingOutbox(ctx context.Context) error {
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
