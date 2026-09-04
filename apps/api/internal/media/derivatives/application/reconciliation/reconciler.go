package reconciliation

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/media/derivatives/domain"
	"ideaboard/api/internal/media/derivatives/ports"
	mediaerrors "ideaboard/api/internal/media/errors"
)

const (
	DefaultReconcilerProbeTimeout = 30 * time.Second
	DefaultReconcilerBatchSize    = int32(100)
	DefaultReconcilerLease        = 24 * time.Hour
)

type Reconciler struct {
	q            *dbsqlc.Queries
	verifier     ports.Verifier
	events       ports.EventPublisher
	probeTimeout time.Duration
	batchSize    int32
	lease        time.Duration
	now          func() time.Time
}

type ReconcilerConfig struct {
	ProbeTimeout time.Duration
	BatchSize    int32
	Lease        time.Duration
}

type Result struct {
	Inspected int
	Ready     int
	Missing   int
	Invalid   int
	Errors    int
}

type ReconcileOutcome uint8

const (
	OutcomeReady ReconcileOutcome = iota
	OutcomeMissing
	OutcomeInvalid
)

func NewReconciler(q *dbsqlc.Queries, verifier ports.Verifier, events ports.EventPublisher, cfg ReconcilerConfig) *Reconciler {
	if cfg.ProbeTimeout <= 0 {
		cfg.ProbeTimeout = DefaultReconcilerProbeTimeout
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = DefaultReconcilerBatchSize
	}
	if cfg.Lease <= 0 {
		cfg.Lease = DefaultReconcilerLease
	}
	return &Reconciler{
		q:            q,
		verifier:     verifier,
		events:       events,
		probeTimeout: cfg.ProbeTimeout,
		batchSize:    cfg.BatchSize,
		lease:        cfg.Lease,
		now:          time.Now,
	}
}

func (r *Reconciler) Reconcile(ctx context.Context) (Result, error) {
	if r == nil || r.q == nil || r.verifier == nil {
		return Result{}, fmt.Errorf("media derivative reconciler is unavailable")
	}
	rows, err := r.q.ListReadyDerivativesForReconcile(ctx, dbsqlc.ListReadyDerivativesForReconcileParams{
		ReconcileBefore: sql.NullTime{Time: r.now().Add(-r.lease), Valid: true},
		RowLimit:        r.batchSize,
	})
	if err != nil {
		return Result{}, fmt.Errorf("list ready derivatives for reconcile: %w", err)
	}
	result := Result{Inspected: len(rows)}
	for _, row := range rows {
		probeCtx, cancel := context.WithTimeout(ctx, r.probeTimeout)
		verifyErr := r.verifier.Verify(
			probeCtx,
			row.StorageProfileID,
			row.StorageKey,
			row.ChecksumSha256,
			row.MimeType,
			row.SizeBytes,
		)
		cancel()
		if verifyErr == nil {
			updated, touchErr := r.q.MarkMediaDerivativeVerified(ctx, dbsqlc.MarkMediaDerivativeVerifiedParams{
				ID: row.ID, ObjectGeneration: row.ObjectGeneration,
			})
			if touchErr != nil {
				result.Errors++
				r.logFailure(ctx, row, touchErr)
				continue
			}
			if updated > 0 {
				result.Ready++
			}
			continue
		}

		code := mediaerrors.CodeOf(verifyErr)
		var updated int64
		switch code {
		case mediaerrors.CodeObjectMissing:
			updated, err = r.q.MarkMediaDerivativeMissing(ctx, dbsqlc.MarkMediaDerivativeMissingParams{
				ID: row.ID, ObjectGeneration: row.ObjectGeneration, ErrorMessage: "storage object is missing",
			})
			if err == nil && updated > 0 {
				result.Missing++
				r.publishInvalidated(row, "missing")
			}
		case mediaerrors.CodeObjectInvalid:
			updated, err = r.q.MarkMediaDerivativeInvalid(ctx, dbsqlc.MarkMediaDerivativeInvalidParams{
				ID: row.ID, ObjectGeneration: row.ObjectGeneration, ErrorMessage: "storage object failed integrity verification",
			})
			if err == nil && updated > 0 {
				result.Invalid++
				r.publishInvalidated(row, "invalid")
			}
		default:
			err = verifyErr
		}
		if err != nil {
			result.Errors++
			r.logFailure(ctx, row, err)
		}
	}
	return result, nil
}

func (r *Reconciler) logFailure(ctx context.Context, row dbsqlc.MediaDerivative, err error) {
	slog.WarnContext(ctx, "media derivative reconcile failed",
		"operation", "media.derivative.reconcile",
		"derivative_id", row.DerivativeID,
		"media_asset_id", row.MediaAssetID,
		"storage_profile_id", row.StorageProfileID,
		"object_generation", row.ObjectGeneration,
		"result", "error",
		"error_class", mediaerrors.CodeOf(err),
	)
}

func (r *Reconciler) publishInvalidated(row dbsqlc.MediaDerivative, status string) {
	if r.events == nil {
		return
	}
	r.events.Publish(row.OwnerUserID, domain.Event{
		SchemaVersion:  domain.EventSchemaVersion,
		Type:           domain.EventTypeInvalidated,
		MediaAssetID:   row.MediaAssetID,
		DerivativeID:   row.DerivativeID,
		Tier:           row.Tier,
		Format:         row.Format,
		Status:         status,
		StorageProfile: row.StorageProfileID,
	})
}
