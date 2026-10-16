package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/library/domain"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5/pgtype"
)

// CreateMaterialization opens a pending job for an owned workspace
// asset. Unknown workspaces and unknown/malformed assets converge to
// ErrV1AssetNotFound (parity with the in-memory store, where both
// miss the same map); malformed job ids report
// ErrV1MaterializationNotFound. No dedup key exists: one asset may
// back many jobs and request idempotency stays handler-side
// (library:materialize scope). Internal triggers that need atomic
// workspace-asset deduplication use ClaimMaterialization below.
func (s *Service) CreateMaterialization(ctx context.Context, workspacePublicID, assetPublicID, publicID string) (*domain.V1Materialization, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	if !domain.IsV1MaterializationID(strings.TrimSpace(publicID)) {
		return nil, domain.ErrV1MaterializationNotFound
	}
	if !domain.IsV1AssetID(strings.TrimSpace(assetPublicID)) {
		return nil, domain.ErrV1AssetNotFound
	}
	workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, workspacePublicID)
	if err != nil {
		if errors.Is(err, errWorkspaceNotFound) {
			return nil, domain.ErrV1AssetNotFound
		}
		return nil, err
	}
	asset, err := q.GetLibraryAssetByPublicID(ctx, dbsqlc.GetLibraryAssetByPublicIDParams{
		WorkspaceID: workspaceInternalID, PublicID: strings.TrimSpace(assetPublicID),
	})
	if database.IsNotFound(err) {
		return nil, domain.ErrV1AssetNotFound
	}
	if err != nil {
		return nil, err
	}
	trimmedWorkspace := strings.TrimSpace(workspacePublicID)
	minted := strings.TrimSpace(publicID)
	for attempt := 0; attempt < mintAttempts; attempt++ {
		row, err := q.CreateLibraryMaterialization(ctx, dbsqlc.CreateLibraryMaterializationParams{
			PublicID: minted, WorkspaceID: workspaceInternalID, LibraryAssetID: asset.LibraryAsset.InternalID,
		})
		if isMaterializationPublicIDConflict(err) {
			minted, err = domain.NewV1MaterializationPublicID()
			if err != nil {
				return nil, err
			}
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("create library materialization: %w", err)
		}
		return materializationFromDetail(trimmedWorkspace, row.PublicID, asset.LibraryAsset.PublicID,
			row.Status, pgtype.Text{}, row.ErrorMessage, row.CreatedAt, row.CompletedAt)
	}
	return nil, errors.New("library: materialization public id mint collision, retry the request")
}

// ClaimMaterialization atomically returns an active/completed job for the
// asset, or creates one when no such job exists. The advisory lock is held
// for the transaction, closing the list-then-create race between concurrent
// read-through and task-completion triggers. Failed jobs remain retryable.
func (s *Service) ClaimMaterialization(ctx context.Context, workspacePublicID, assetPublicID, publicID string) (*domain.V1Materialization, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, false, err
	}
	if !domain.IsV1MaterializationID(strings.TrimSpace(publicID)) {
		return nil, false, domain.ErrV1MaterializationNotFound
	}
	if !domain.IsV1AssetID(strings.TrimSpace(assetPublicID)) {
		return nil, false, domain.ErrV1AssetNotFound
	}
	workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, workspacePublicID)
	if err != nil {
		if errors.Is(err, errWorkspaceNotFound) {
			return nil, false, domain.ErrV1AssetNotFound
		}
		return nil, false, err
	}
	asset, err := q.GetLibraryAssetByPublicID(ctx, dbsqlc.GetLibraryAssetByPublicIDParams{
		WorkspaceID: workspaceInternalID, PublicID: strings.TrimSpace(assetPublicID),
	})
	if database.IsNotFound(err) {
		return nil, false, domain.ErrV1AssetNotFound
	}
	if err != nil {
		return nil, false, err
	}
	var result *domain.V1Materialization
	created := false
	err = db.withTx(func(txq *dbsqlc.Queries) error {
		if err := txq.LockLibraryMaterializationAsset(ctx, dbsqlc.LockLibraryMaterializationAssetParams{
			WorkspaceID: workspaceInternalID, LibraryAssetID: asset.LibraryAsset.InternalID,
		}); err != nil {
			return err
		}
		existing, err := txq.GetActiveLibraryMaterializationByAsset(ctx, dbsqlc.GetActiveLibraryMaterializationByAssetParams{
			WorkspaceID: workspaceInternalID, LibraryAssetID: asset.LibraryAsset.InternalID,
		})
		if err == nil {
			result, err = materializationFromDetail(strings.TrimSpace(workspacePublicID), existing.PublicID, existing.AssetPublicID,
				existing.Status, existing.MediaAssetPublicID, existing.ErrorMessage, existing.CreatedAt, existing.CompletedAt)
			return err
		}
		if !database.IsNotFound(err) {
			return err
		}
		row, err := txq.CreateLibraryMaterialization(ctx, dbsqlc.CreateLibraryMaterializationParams{
			PublicID: strings.TrimSpace(publicID), WorkspaceID: workspaceInternalID, LibraryAssetID: asset.LibraryAsset.InternalID,
		})
		if err != nil {
			return fmt.Errorf("create library materialization: %w", err)
		}
		result, err = materializationFromDetail(strings.TrimSpace(workspacePublicID), row.PublicID, asset.LibraryAsset.PublicID,
			row.Status, pgtype.Text{}, row.ErrorMessage, row.CreatedAt, row.CompletedAt)
		created = err == nil
		return err
	})
	if err != nil {
		return nil, false, err
	}
	return result, created, nil
}

// GetMaterialization returns the status-addressable job snapshot.
// Malformed, unknown, and cross-workspace ids converge to
// ErrV1MaterializationNotFound.
func (s *Service) GetMaterialization(ctx context.Context, workspacePublicID, materializationPublicID string) (*domain.V1Materialization, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	if !domain.IsV1MaterializationID(strings.TrimSpace(materializationPublicID)) {
		return nil, domain.ErrV1MaterializationNotFound
	}
	workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, workspacePublicID)
	if err != nil {
		if errors.Is(err, errWorkspaceNotFound) {
			return nil, domain.ErrV1MaterializationNotFound
		}
		return nil, err
	}
	row, err := q.GetLibraryMaterializationByPublicID(ctx, dbsqlc.GetLibraryMaterializationByPublicIDParams{
		WorkspaceID: workspaceInternalID, PublicID: strings.TrimSpace(materializationPublicID),
	})
	if database.IsNotFound(err) {
		return nil, domain.ErrV1MaterializationNotFound
	}
	if err != nil {
		return nil, err
	}
	return materializationFromDetail(strings.TrimSpace(workspacePublicID), row.PublicID, row.AssetPublicID,
		row.Status, row.MediaAssetPublicID, row.ErrorMessage, row.CreatedAt, row.CompletedAt)
}

// ListMaterializations returns every workspace job in creation order
// (concrete Service method only: the WorkspaceLibraryStore seam has no
// list; the handler exposes status-addressable GETs). Unknown
// workspaces list empty, matching the asset posture.
func (s *Service) ListMaterializations(ctx context.Context, workspacePublicID string) ([]*domain.V1Materialization, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, workspacePublicID)
	if err != nil {
		if errors.Is(err, errWorkspaceNotFound) {
			return []*domain.V1Materialization{}, nil
		}
		return nil, err
	}
	rows, err := q.ListLibraryMaterializations(ctx, dbsqlc.ListLibraryMaterializationsParams{
		WorkspaceID: workspaceInternalID, RowLimit: maxLibraryMaterializations,
	})
	if err != nil {
		return nil, fmt.Errorf("list library materializations: %w", err)
	}
	trimmedWorkspace := strings.TrimSpace(workspacePublicID)
	out := make([]*domain.V1Materialization, 0, len(rows))
	for _, row := range rows {
		record, err := materializationFromDetail(trimmedWorkspace, row.PublicID, row.AssetPublicID,
			row.Status, row.MediaAssetPublicID, row.ErrorMessage, row.CreatedAt, row.CompletedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, nil
}

// materializationSnapshot reads back the stored record, mapping a
// miss to ErrV1MaterializationNotFound. Transition misses land here:
// terminal records report their snapshot so idempotent replays
// converge instead of resurrecting jobs.
func materializationSnapshot(ctx context.Context, q *dbsqlc.Queries, workspacePublicID string, workspaceInternalID int64, materializationPublicID string) (*domain.V1Materialization, error) {
	row, err := q.GetLibraryMaterializationByPublicID(ctx, dbsqlc.GetLibraryMaterializationByPublicIDParams{
		WorkspaceID: workspaceInternalID, PublicID: materializationPublicID,
	})
	if database.IsNotFound(err) {
		return nil, domain.ErrV1MaterializationNotFound
	}
	if err != nil {
		return nil, err
	}
	return materializationFromDetail(strings.TrimSpace(workspacePublicID), row.PublicID, row.AssetPublicID,
		row.Status, row.MediaAssetPublicID, row.ErrorMessage, row.CreatedAt, row.CompletedAt)
}

// resolveMaterializationMediaInternal maps the caller's asset_...
// result link to its workspace-scoped media_assets.internal_id.
// Unknown or cross-workspace links fail closed with the job left
// non-terminal (safe to retry once Media settles).
func resolveMaterializationMediaInternal(ctx context.Context, q *dbsqlc.Queries, workspacePublicID, mediaAssetID string) (int64, error) {
	trimmed := strings.TrimSpace(mediaAssetID)
	if !contract.IsAssetID(trimmed) {
		return 0, fmt.Errorf("library: invalid media asset id %q", trimmed)
	}
	row, err := q.GetMediaAssetByPublicID(ctx, dbsqlc.GetMediaAssetByPublicIDParams{
		PublicID: trimmed, WorkspacePublicID: strings.TrimSpace(workspacePublicID),
	})
	if database.IsNotFound(err) {
		return 0, fmt.Errorf("library: media asset %q not found in workspace", trimmed)
	}
	if err != nil {
		return 0, err
	}
	return row.InternalID, nil
}

// SetMaterializationResult advances pending jobs through running to a
// terminal completed|failed state. The machine is enforced in SQL
// guards (pending -> processing -> completed|failed, with direct
// pending -> completed|failed allowed): guarded misses read back the
// stored snapshot, so terminal records are immutable and concurrent
// settlers converge on the winner. Unknown statuses report
// ErrV1MaterializationNotFound, except against terminal records,
// which still report their snapshot (in-memory parity). errorCode is
// accepted for seam stability but not persisted (no durable column);
// completion requires a resolvable workspace-scoped asset_... link.
func (s *Service) SetMaterializationResult(ctx context.Context, workspacePublicID, materializationPublicID string, status domain.V1MaterializationStatus, mediaAssetID, errorCode, errorMessage string) (*domain.V1Materialization, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	if !domain.IsV1MaterializationID(strings.TrimSpace(materializationPublicID)) {
		return nil, domain.ErrV1MaterializationNotFound
	}
	workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, workspacePublicID)
	if err != nil {
		if errors.Is(err, errWorkspaceNotFound) {
			return nil, domain.ErrV1MaterializationNotFound
		}
		return nil, err
	}
	jobID := strings.TrimSpace(materializationPublicID)
	switch status {
	case domain.V1MaterializationRunning, domain.V1MaterializationCompleted, domain.V1MaterializationFailed:
		// Proceed below.
	default:
		// Unknown words never advance a job: terminal records still
		// report their snapshot (in-memory parity) while anything
		// else reports not found.
		snapshot, err := materializationSnapshot(ctx, q, workspacePublicID, workspaceInternalID, jobID)
		if err != nil {
			return nil, err
		}
		if snapshot.Status == domain.V1MaterializationCompleted || snapshot.Status == domain.V1MaterializationFailed {
			return snapshot, nil
		}
		return nil, domain.ErrV1MaterializationNotFound
	}
	switch status {
	case domain.V1MaterializationRunning:
		row, err := q.StartLibraryMaterialization(ctx, dbsqlc.StartLibraryMaterializationParams{
			WorkspaceID: workspaceInternalID, PublicID: jobID,
		})
		if err == nil {
			return materializationFromDetail(strings.TrimSpace(workspacePublicID), row.PublicID, row.AssetPublicID,
				row.Status, row.MediaAssetPublicID, row.ErrorMessage, row.CreatedAt, row.CompletedAt)
		}
		if !database.IsNotFound(err) {
			return nil, fmt.Errorf("start library materialization: %w", err)
		}
		return materializationSnapshot(ctx, q, workspacePublicID, workspaceInternalID, jobID)
	case domain.V1MaterializationCompleted:
		mediaInternalID, err := resolveMaterializationMediaInternal(ctx, q, workspacePublicID, mediaAssetID)
		if err != nil {
			return nil, err
		}
		row, err := q.CompleteLibraryMaterialization(ctx, dbsqlc.CompleteLibraryMaterializationParams{
			WorkspaceID: workspaceInternalID, PublicID: jobID, MediaAssetID: pgtype.Int8{Int64: mediaInternalID, Valid: true},
		})
		if err == nil {
			return materializationFromDetail(strings.TrimSpace(workspacePublicID), row.PublicID, row.AssetPublicID,
				row.Status, row.MediaAssetPublicID, row.ErrorMessage, row.CreatedAt, row.CompletedAt)
		}
		if !database.IsNotFound(err) {
			return nil, fmt.Errorf("complete library materialization: %w", err)
		}
		return materializationSnapshot(ctx, q, workspacePublicID, workspaceInternalID, jobID)
	default: // V1MaterializationFailed
		row, err := q.FailLibraryMaterialization(ctx, dbsqlc.FailLibraryMaterializationParams{
			WorkspaceID: workspaceInternalID, PublicID: jobID, ErrorMessage: strings.TrimSpace(errorMessage),
		})
		if err == nil {
			return materializationFromDetail(strings.TrimSpace(workspacePublicID), row.PublicID, row.AssetPublicID,
				row.Status, row.MediaAssetPublicID, row.ErrorMessage, row.CreatedAt, row.CompletedAt)
		}
		if !database.IsNotFound(err) {
			return nil, fmt.Errorf("fail library materialization: %w", err)
		}
		return materializationSnapshot(ctx, q, workspacePublicID, workspaceInternalID, jobID)
	}
}
