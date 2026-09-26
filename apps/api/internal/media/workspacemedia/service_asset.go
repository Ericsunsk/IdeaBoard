package workspacemedia

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"
)

func assetFromRow(workspacePublicID string, row dbsqlc.MediaAsset) *Asset {
	return &Asset{
		InternalID:     row.InternalID,
		ID:             row.PublicID,
		WorkspaceID:    strings.TrimSpace(workspacePublicID),
		OwnerUserID:    row.OwnerUserID,
		Status:         AssetStatusReady,
		MediaKind:      row.MediaKind,
		MIMEType:       row.MimeType,
		Width:          int4Value(row.Width),
		Height:         int4Value(row.Height),
		SizeBytes:      row.SizeBytes,
		ChecksumSHA256: row.Checksum,
		StorageKey:     row.StorageKey,
		ArchivedAt:     dbutil.TimePtr(row.ArchivedAt),
		CreatedAt:      dbutil.TimeValue(row.CreatedAt),
	}
}

func isAssetPublicIDConflict(err error) bool {
	if !database.IsDuplicateConstraintError(err) {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "uq_media_assets_public_id")
}

// GetAsset resolves a workspace-scoped asset. Cross-workspace and
// malformed ids read as ErrNotFound; archived assets stay readable.
func (s *Service) GetAsset(ctx context.Context, workspaceID, assetID string) (*Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	if !IsAssetID(strings.TrimSpace(assetID)) {
		return nil, ErrNotFound
	}
	row, err := q.GetMediaAssetByPublicID(ctx, dbsqlc.GetMediaAssetByPublicIDParams{
		PublicID:          strings.TrimSpace(assetID),
		WorkspacePublicID: strings.TrimSpace(workspaceID),
	})
	if database.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return assetFromRow(strings.TrimSpace(workspaceID), row), nil
}

// ListAssets pages live workspace assets newest-first. Unknown
// workspaces list empty, matching the in-memory store.
func (s *Service) ListAssets(ctx context.Context, workspaceID string, limit, offset int) ([]*Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	if limit < 1 {
		limit = 1
	}
	if limit > int(maxMediaAssetRows) {
		limit = int(maxMediaAssetRows)
	}
	if offset < 0 {
		offset = 0
	}
	trimmedWorkspace := strings.TrimSpace(workspaceID)
	if _, err := resolveWorkspaceInternal(ctx, q, trimmedWorkspace); err != nil {
		if errors.Is(err, errWorkspaceNotFound) {
			return []*Asset{}, nil
		}
		return nil, err
	}
	rows, err := q.ListMediaAssets(ctx, dbsqlc.ListMediaAssetsParams{
		WorkspacePublicID: trimmedWorkspace,
		RowLimit:          int32(limit),
		RowOffset:         int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("workspacemedia: list media assets: %w", err)
	}
	out := make([]*Asset, 0, len(rows))
	for _, row := range rows {
		out = append(out, assetFromRow(trimmedWorkspace, row))
	}
	return out, nil
}

// ArchiveAsset stamps the archive lifecycle marker. Repeat archives
// converge on the stored row so idempotent replays never 404.
func (s *Service) ArchiveAsset(ctx context.Context, workspaceID, assetID string) (*Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	trimmedWorkspace := strings.TrimSpace(workspaceID)
	if !IsAssetID(strings.TrimSpace(assetID)) {
		return nil, ErrNotFound
	}
	row, err := q.ArchiveMediaAsset(ctx, dbsqlc.ArchiveMediaAssetParams{
		PublicID:          strings.TrimSpace(assetID),
		WorkspacePublicID: trimmedWorkspace,
	})
	if database.IsNotFound(err) {
		current, getErr := getSessionRowAsset(ctx, q, trimmedWorkspace, assetID)
		if getErr != nil {
			return nil, getErr
		}
		return assetFromRow(trimmedWorkspace, current), nil
	}
	if err != nil {
		return nil, err
	}
	return assetFromRow(trimmedWorkspace, row), nil
}

func getSessionRowAsset(ctx context.Context, q *dbsqlc.Queries, workspacePublicID, assetID string) (dbsqlc.MediaAsset, error) {
	row, err := q.GetMediaAssetByPublicID(ctx, dbsqlc.GetMediaAssetByPublicIDParams{
		PublicID:          strings.TrimSpace(assetID),
		WorkspacePublicID: strings.TrimSpace(workspacePublicID),
	})
	if database.IsNotFound(err) {
		return dbsqlc.MediaAsset{}, ErrNotFound
	}
	if err != nil {
		return dbsqlc.MediaAsset{}, err
	}
	return row, nil
}

// UpdateAssetMetadata refreshes dimensions and merges metadata
// (unknown keys from other writers survive the merge).
func (s *Service) UpdateAssetMetadata(ctx context.Context, workspaceID, assetID string, params UpdateMetadataParams) (*Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	trimmedWorkspace := strings.TrimSpace(workspaceID)
	if !IsAssetID(strings.TrimSpace(assetID)) {
		return nil, ErrNotFound
	}
	merge := params.Metadata
	if merge == nil {
		merge = map[string]any{}
	}
	raw, err := json.Marshal(merge)
	if err != nil {
		return nil, fmt.Errorf("workspacemedia: encode asset metadata: %w", err)
	}
	row, err := q.UpdateMediaAssetMetadata(ctx, dbsqlc.UpdateMediaAssetMetadataParams{
		// Zero/negative dimensions normalize to NULL (unknown):
		// the image CHECK requires width/height > 0 when present.
		Width:             nullInt4(params.Width, params.HasDims && params.Width > 0),
		Height:            nullInt4(params.Height, params.HasDims && params.Height > 0),
		DurationMs:        nullInt4(params.DurationMs, params.HasDur),
		Metadata:          raw,
		PublicID:          strings.TrimSpace(assetID),
		WorkspacePublicID: trimmedWorkspace,
	})
	if database.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return assetFromRow(trimmedWorkspace, row), nil
}
