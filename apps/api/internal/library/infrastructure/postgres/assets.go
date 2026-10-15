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

// SaveAsset validates the provider reference (provider-allowlisted
// https URLs; never bytes) and dedups on
// (workspace, provider, source_id): re-saving returns the existing
// asset with created=false. An unknown folder binding 404s. An empty
// folder binding saves into Quick Saves.
func (s *Service) SaveAsset(ctx context.Context, workspacePublicID string, input domain.V1SaveAssetInput, publicID string) (*domain.V1Asset, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, false, err
	}
	normalized, err := domain.NormalizeAndValidateAsset(input.Asset)
	if err != nil {
		return nil, false, err
	}
	if !domain.IsV1AssetID(strings.TrimSpace(publicID)) {
		return nil, false, domain.ErrV1AssetNotFound
	}
	workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, workspacePublicID)
	if err != nil {
		if errors.Is(err, errWorkspaceNotFound) {
			return nil, false, domain.ErrV1AssetNotFound
		}
		return nil, false, err
	}
	folderInternalID, err := resolveAssetFolderInternal(ctx, q, workspaceInternalID, input.FolderPublicID)
	if err != nil {
		return nil, false, err
	}
	if existing, err := q.GetLibraryAssetByWorkspaceSource(ctx, dbsqlc.GetLibraryAssetByWorkspaceSourceParams{
		WorkspaceID: workspaceInternalID, Provider: normalized.Provider, SourceID: normalized.SourceID,
	}); err == nil {
		return assetFromWorkspaceSourceRow(strings.TrimSpace(workspacePublicID), existing), false, nil
	} else if !database.IsNotFound(err) {
		return nil, false, err
	}
	minted := strings.TrimSpace(publicID)
	for attempt := 0; attempt < mintAttempts; attempt++ {
		row, err := q.CreateLibraryAsset(ctx, dbsqlc.CreateLibraryAssetParams{
			PublicID: minted, WorkspaceID: workspaceInternalID, FolderID: folderInternalID,
			Provider: normalized.Provider, SourceID: normalized.SourceID,
			SourceUrl: normalized.SourceURL, PreviewUrl: normalized.PreviewURL, OriginalUrl: normalized.OriginalURL,
			Width: normalized.Width, Height: normalized.Height,
		})
		if err == nil {
			folderPublicID := strings.TrimSpace(input.FolderPublicID)
			if folderPublicID == "" {
				quickSaves, qsErr := q.GetQuickSavesFolder(ctx, workspaceInternalID)
				if qsErr != nil {
					return nil, false, qsErr
				}
				folderPublicID = quickSaves.PublicID
			}
			return assetFromLibraryAsset(strings.TrimSpace(workspacePublicID), folderPublicID, row), true, nil
		}
		if !database.IsDuplicateConstraintError(err) {
			return nil, false, fmt.Errorf("save library asset: %w", err)
		}
		if isAssetPublicIDConflict(err) {
			minted, err = domain.NewV1AssetPublicID()
			if err != nil {
				return nil, false, err
			}
			continue
		}
		// (workspace, provider, source_id) lost a concurrent save:
		// read back the winner so replays converge.
		existing, getErr := q.GetLibraryAssetByWorkspaceSource(ctx, dbsqlc.GetLibraryAssetByWorkspaceSourceParams{
			WorkspaceID: workspaceInternalID, Provider: normalized.Provider, SourceID: normalized.SourceID,
		})
		if getErr != nil {
			return nil, false, fmt.Errorf("save library asset: %w", err)
		}
		return assetFromWorkspaceSourceRow(strings.TrimSpace(workspacePublicID), existing), false, nil
	}
	return nil, false, errors.New("library: asset public id mint collision, retry the request")
}

// SaveManagedMediaAsset creates the workspace library reference for a task
// output. The reference is keyed by (workspace, ProviderWorkspaceMedia,
// sourceID), lives in Quick Saves, and points to an existing ready media asset.
// Replays return the existing row with created=false.
func (s *Service) SaveManagedMediaAsset(ctx context.Context, workspacePublicID, sourceID, mediaAssetID, publicID string) (*domain.V1Asset, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, false, err
	}
	sourceID = strings.TrimSpace(sourceID)
	mediaAssetID = strings.TrimSpace(mediaAssetID)
	publicID = strings.TrimSpace(publicID)
	if sourceID == "" || len(sourceID) > 255 || !contract.IsAssetID(mediaAssetID) || !domain.IsV1AssetID(publicID) {
		return nil, false, domain.ErrV1AssetNotFound
	}
	workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, workspacePublicID)
	if err != nil {
		if errors.Is(err, errWorkspaceNotFound) {
			return nil, false, domain.ErrV1AssetNotFound
		}
		return nil, false, err
	}
	if existing, err := q.GetLibraryAssetByWorkspaceSource(ctx, dbsqlc.GetLibraryAssetByWorkspaceSourceParams{
		WorkspaceID: workspaceInternalID, Provider: domain.ProviderWorkspaceMedia, SourceID: sourceID,
	}); err == nil {
		return assetFromWorkspaceSourceRow(strings.TrimSpace(workspacePublicID), existing), false, nil
	} else if !database.IsNotFound(err) {
		return nil, false, err
	}
	media, err := q.GetMediaAssetByPublicID(ctx, dbsqlc.GetMediaAssetByPublicIDParams{
		WorkspacePublicID: strings.TrimSpace(workspacePublicID), PublicID: mediaAssetID,
	})
	if database.IsNotFound(err) {
		return nil, false, domain.ErrV1AssetNotFound
	}
	if err != nil {
		return nil, false, err
	}
	folder, err := ensureQuickSavesFolderRow(ctx, q, workspaceInternalID)
	if err != nil {
		return nil, false, err
	}
	row, err := q.CreateManagedLibraryAsset(ctx, dbsqlc.CreateManagedLibraryAssetParams{
		PublicID: publicID, WorkspaceID: workspaceInternalID, FolderID: folder.InternalID,
		SourceID: sourceID, MediaAssetID: pgtype.Int8{Int64: media.InternalID, Valid: true},
	})
	if err == nil {
		return assetFromLibraryAsset(strings.TrimSpace(workspacePublicID), folder.PublicID, row), true, nil
	}
	if isAssetPublicIDConflict(err) {
		return nil, false, fmt.Errorf("save managed library asset: public id collision: %w", err)
	}
	if database.IsDuplicateConstraintError(err) {
		existing, getErr := q.GetLibraryAssetByWorkspaceSource(ctx, dbsqlc.GetLibraryAssetByWorkspaceSourceParams{
			WorkspaceID: workspaceInternalID, Provider: domain.ProviderWorkspaceMedia, SourceID: sourceID,
		})
		if getErr == nil {
			return assetFromWorkspaceSourceRow(strings.TrimSpace(workspacePublicID), existing), false, nil
		}
	}
	return nil, false, fmt.Errorf("save managed library asset: %w", err)
}

// ListAssets returns every workspace asset in creation order; the
// handler slices the stable page. Unknown workspaces list empty.
func (s *Service) ListAssets(ctx context.Context, workspacePublicID string) ([]*domain.V1Asset, error) {
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
			return []*domain.V1Asset{}, nil
		}
		return nil, err
	}
	rows, err := q.ListLibraryAssets(ctx, dbsqlc.ListLibraryAssetsParams{
		WorkspaceID: workspaceInternalID, RowLimit: maxLibraryAssets,
	})
	if err != nil {
		return nil, fmt.Errorf("list library assets: %w", err)
	}
	out := make([]*domain.V1Asset, 0, len(rows))
	for _, row := range rows {
		out = append(out, assetFromListRow(strings.TrimSpace(workspacePublicID), row))
	}
	return out, nil
}

// GetAsset converges unknown ids to ErrV1AssetNotFound (cross-workspace
// ids miss the workspace-keyed lookup by construction).
func (s *Service) GetAsset(ctx context.Context, workspacePublicID, assetPublicID string) (*domain.V1Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
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
	row, err := q.GetLibraryAssetByPublicID(ctx, dbsqlc.GetLibraryAssetByPublicIDParams{
		WorkspaceID: workspaceInternalID, PublicID: strings.TrimSpace(assetPublicID),
	})
	if database.IsNotFound(err) {
		return nil, domain.ErrV1AssetNotFound
	}
	if err != nil {
		return nil, err
	}
	return assetFromPublicIDRow(strings.TrimSpace(workspacePublicID), row), nil
}

// MoveAsset rebinds the folder ("" moves to Quick Saves); unknown
// assets or folders 404 with their own codes.
func (s *Service) MoveAsset(ctx context.Context, workspacePublicID, assetPublicID, folderPublicID string) (*domain.V1Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
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
	if _, err := q.GetLibraryAssetByPublicID(ctx, dbsqlc.GetLibraryAssetByPublicIDParams{
		WorkspaceID: workspaceInternalID, PublicID: strings.TrimSpace(assetPublicID),
	}); database.IsNotFound(err) {
		return nil, domain.ErrV1AssetNotFound
	} else if err != nil {
		return nil, err
	}
	targetInternalID, err := resolveAssetFolderInternal(ctx, q, workspaceInternalID, folderPublicID)
	if err != nil {
		return nil, err
	}
	row, err := q.MoveLibraryAsset(ctx, dbsqlc.MoveLibraryAssetParams{
		WorkspaceID: workspaceInternalID, PublicID: strings.TrimSpace(assetPublicID), FolderID: targetInternalID,
	})
	if database.IsNotFound(err) {
		return nil, domain.ErrV1AssetNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("move library asset: %w", err)
	}
	targetPublicID := strings.TrimSpace(folderPublicID)
	if targetPublicID == "" {
		quickSaves, qsErr := q.GetQuickSavesFolder(ctx, workspaceInternalID)
		if qsErr != nil {
			return nil, qsErr
		}
		targetPublicID = quickSaves.PublicID
	}
	return assetFromLibraryAsset(strings.TrimSpace(workspacePublicID), targetPublicID, row), nil
}

// DeleteAsset drops the saved reference (the provider original is
// untouched; materialized Media Assets are owned by Media, not here).
func (s *Service) DeleteAsset(ctx context.Context, workspacePublicID, assetPublicID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return err
	}
	if !domain.IsV1AssetID(strings.TrimSpace(assetPublicID)) {
		return domain.ErrV1AssetNotFound
	}
	workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, workspacePublicID)
	if err != nil {
		if errors.Is(err, errWorkspaceNotFound) {
			return domain.ErrV1AssetNotFound
		}
		return err
	}
	affected, err := q.DeleteLibraryAsset(ctx, dbsqlc.DeleteLibraryAssetParams{
		WorkspaceID: workspaceInternalID, PublicID: strings.TrimSpace(assetPublicID),
	})
	if err != nil {
		return fmt.Errorf("delete library asset: %w", err)
	}
	if affected == 0 {
		return domain.ErrV1AssetNotFound
	}
	return nil
}

// ResolveAssets performs the typed batch resolve (bounded to
// V1MaxResolveItems): provider + source_id matches a saved workspace
// asset; a bare provider URL must be an allowlisted https URL and match
// a saved asset's known URL exactly. Nothing is fetched: unknown
// references report unresolved per item. NOT a general proxy.
func (s *Service) ResolveAssets(ctx context.Context, workspacePublicID string, items []domain.V1ResolveItem) ([]domain.V1ResolveResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	if len(items) > domain.V1MaxResolveItems {
		return nil, domain.ErrV1ResolveTooMany
	}
	workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, workspacePublicID)
	if err != nil && !errors.Is(err, errWorkspaceNotFound) {
		return nil, err
	}
	if errors.Is(err, errWorkspaceNotFound) {
		workspaceInternalID = -1
	}
	trimmedWorkspace := strings.TrimSpace(workspacePublicID)
	results := make([]domain.V1ResolveResult, 0, len(items))
	for _, item := range items {
		provider := strings.ToLower(strings.TrimSpace(item.Provider))
		sourceID := strings.TrimSpace(item.SourceID)
		sourceURL := strings.TrimSpace(item.SourceURL)
		result := domain.V1ResolveResult{Provider: provider, SourceID: sourceID, Status: domain.V1ResolveUnresolved}
		if provider != domain.ProviderUnsplash && provider != domain.ProviderPexels {
			results = append(results, result)
			continue
		}
		if sourceID != "" {
			if workspaceInternalID > 0 {
				row, err := q.GetLibraryAssetByWorkspaceSource(ctx, dbsqlc.GetLibraryAssetByWorkspaceSourceParams{
					WorkspaceID: workspaceInternalID, Provider: provider, SourceID: sourceID,
				})
				if err == nil {
					result.Status = domain.V1ResolveResolved
					result.Asset = assetFromWorkspaceSourceRow(trimmedWorkspace, row)
				} else if !database.IsNotFound(err) {
					return nil, err
				}
			}
			results = append(results, result)
			continue
		}
		if sourceURL == "" || !domain.IsAllowedProviderURL(provider, sourceURL) {
			results = append(results, result)
			continue
		}
		if workspaceInternalID > 0 {
			rows, err := q.ListLibraryAssetsByWorkspaceProvider(ctx, dbsqlc.ListLibraryAssetsByWorkspaceProviderParams{
				WorkspaceID: workspaceInternalID, Provider: provider, RowLimit: maxLibraryAssets,
			})
			if err != nil {
				return nil, err
			}
			for _, row := range rows {
				if row.SourceUrl == sourceURL || row.PreviewUrl == sourceURL || row.OriginalUrl == sourceURL {
					result.Status = domain.V1ResolveResolved
					result.SourceID = row.SourceID
					result.Asset = assetFromProviderRow(trimmedWorkspace, row)
					break
				}
			}
		}
		results = append(results, result)
	}
	return results, nil
}

// FindManagedMedia resolves the stored canonical media link without materialization.
func (s *Service) FindManagedMedia(ctx context.Context, workspacePublicID, assetPublicID string) (string, error) {
	q, err := s.dbWithContext(ctx).queries()
	if err != nil {
		return "", err
	}
	id, err := q.GetLibraryManagedMediaPublicID(ctx, dbsqlc.GetLibraryManagedMediaPublicIDParams{WorkspacePublicID: workspacePublicID, AssetPublicID: assetPublicID})
	if database.IsNotFound(err) {
		return "", domain.ErrV1AssetNotFound
	}
	return id, err
}
