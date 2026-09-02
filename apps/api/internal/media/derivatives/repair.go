package derivatives

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	mediaerrors "ideaboard/api/internal/media/errors"
)

const (
	defaultRepairMaxAttempts = int32(3)
	defaultRepairLease       = 30 * time.Second
)

var ErrRepairNotPossible = errors.New("media derivative repair is not possible")

type RepairConfig struct {
	MaxAttempts int32
	Lease       time.Duration
}

// Repair owns the bounded missing/invalid -> pending transition. The state
// change and durable outbox insert share one PostgreSQL transaction; events
// are observability signals emitted only after commit.
type Repair struct {
	db          *sql.DB
	events      EventPublisher
	maxAttempts int32
	lease       time.Duration
}

func NewRepair(db *sql.DB, events EventPublisher, cfg RepairConfig) *Repair {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = defaultRepairMaxAttempts
	}
	if cfg.Lease <= 0 {
		cfg.Lease = defaultRepairLease
	}
	return &Repair{db: db, events: events, maxAttempts: cfg.MaxAttempts, lease: cfg.Lease}
}

func (r *Repair) RunOnce(ctx context.Context, limit int32) (requeued, terminal []int64, err error) {
	if r == nil || r.db == nil {
		return nil, nil, fmt.Errorf("media derivative repair is unavailable")
	}
	if limit <= 0 {
		return nil, nil, fmt.Errorf("media derivative repair limit must be positive")
	}

	q := dbsqlc.New(r.db)
	candidates, err := q.ListMediaDerivativeRepairCandidates(ctx, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("list media derivative repair candidates: %w", err)
	}
	for _, candidate := range candidates {
		row, outcome, repairErr := r.repairOne(ctx, candidate.ID)
		if repairErr != nil {
			return requeued, terminal, repairErr
		}
		switch outcome {
		case repairOutcomeRequeued:
			requeued = append(requeued, candidate.ID)
			r.publishRepairRequested(row)
		case repairOutcomeTerminal:
			terminal = append(terminal, candidate.ID)
			slog.WarnContext(ctx, "media derivative repair attempt cap reached",
				"operation", "media.derivative.repair",
				"derivative_id", row.DerivativeID,
				"media_asset_id", row.MediaAssetID,
				"result", "terminal",
				"error_class", mediaerrors.CodeRepairNotPossible,
			)
		}
	}
	return requeued, terminal, nil
}

type repairOutcome uint8

const (
	repairOutcomeNoop repairOutcome = iota
	repairOutcomeRequeued
	repairOutcomeTerminal
)

func (r *Repair) repairOne(ctx context.Context, derivativeID int64) (dbsqlc.MediaDerivative, repairOutcome, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return dbsqlc.MediaDerivative{}, repairOutcomeNoop, fmt.Errorf("begin media derivative repair: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	q := dbsqlc.New(tx)
	row, err := q.GetMediaDerivativeByID(ctx, derivativeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dbsqlc.MediaDerivative{}, repairOutcomeNoop, nil
		}
		return dbsqlc.MediaDerivative{}, repairOutcomeNoop, fmt.Errorf("load media derivative repair candidate %d: %w", derivativeID, err)
	}

	if row.AttemptCount >= r.maxAttempts {
		updated, markErr := q.MarkMediaDerivativeRepairTerminal(ctx, dbsqlc.MarkMediaDerivativeRepairTerminalParams{
			ErrorMessage:    ErrRepairNotPossible.Error(),
			ID:              derivativeID,
			MaxAttemptCount: r.maxAttempts,
		})
		if markErr != nil {
			return row, repairOutcomeNoop, fmt.Errorf("mark media derivative repair terminal %d: %w", derivativeID, markErr)
		}
		if updated == 0 {
			return row, repairOutcomeNoop, nil
		}
		if err := tx.Commit(); err != nil {
			return row, repairOutcomeNoop, fmt.Errorf("commit terminal media derivative repair %d: %w", derivativeID, err)
		}
		return row, repairOutcomeTerminal, nil
	}

	updated, err := q.RequeueMediaDerivativeRepair(ctx, dbsqlc.RequeueMediaDerivativeRepairParams{
		ID: derivativeID, MaxAttemptCount: r.maxAttempts,
	})
	if err != nil {
		return row, repairOutcomeNoop, fmt.Errorf("requeue media derivative repair %d: %w", derivativeID, err)
	}
	if updated == 0 {
		return row, repairOutcomeNoop, nil
	}
	if _, err := q.EnqueueMediaDerivativeOutbox(ctx, derivativeID); err != nil {
		return row, repairOutcomeNoop, fmt.Errorf("enqueue media derivative repair %d: %w", derivativeID, err)
	}
	if err := tx.Commit(); err != nil {
		return row, repairOutcomeNoop, fmt.Errorf("commit media derivative repair %d: %w", derivativeID, err)
	}
	return row, repairOutcomeRequeued, nil
}

func (r *Repair) publishRepairRequested(row dbsqlc.MediaDerivative) {
	if r.events == nil {
		return
	}
	r.events.Publish(row.OwnerUserID, Event{
		SchemaVersion:  EventSchemaVersion,
		Type:           EventTypeRepairRequested,
		MediaAssetID:   row.MediaAssetID,
		DerivativeID:   row.DerivativeID,
		Tier:           row.Tier,
		Format:         row.Format,
		Status:         "pending",
		StorageProfile: row.StorageProfileID,
	})
}

func (r *Repair) PublishRepaired(userID int64, event Event) {
	if r == nil || r.events == nil {
		return
	}
	event.SchemaVersion = EventSchemaVersion
	event.Type = EventTypeRepaired
	r.events.Publish(userID, event)
}
