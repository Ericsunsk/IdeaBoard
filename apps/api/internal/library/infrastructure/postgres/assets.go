package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/library/domain"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// savedBy maps a server-side operator identity to the durable column:
// unknown operators persist NULL (never a zero id).
func savedBy(operatorUserID int64) pgtype.Int8 {
	if operatorUserID <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: operatorUserID, Valid: true}
}

// SaveAsset validates the external provider reference
// (provider-allowlisted https URLs; never bytes) and dedups on
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
	folder, err := resolveAssetFolder(ctx, q, workspaceInternalID, input.FolderPublicID)
	if err != nil {
		return nil, false, err
	}
	if existing, err := q.GetLibraryAssetByWorkspaceSource(ctx, dbsqlc.GetLibraryAssetByWorkspaceSourceParams{
		WorkspaceID: workspaceInternalID, Provider: textParam(normalized.Provider), SourceID: textParam(normalized.SourceID),
	}); err == nil {
		return assetFromRow(strings.TrimSpace(workspacePublicID), dbsqlc.GetLibraryAssetByPublicIDRow(existing)), false, nil
	} else if !database.IsNotFound(err) {
		return nil, false, err
	}
	minted := strings.TrimSpace(publicID)
	for attempt := 0; attempt < mintAttempts; attempt++ {
		row, err := q.CreateLibraryAsset(ctx, dbsqlc.CreateLibraryAssetParams{
			PublicID: minted, WorkspaceID: workspaceInternalID, FolderID: folder.InternalID,
			Provider: textParam(normalized.Provider), SourceID: textParam(normalized.SourceID),
			SourceUrl: textParam(normalized.SourceURL), PreviewUrl: textParam(normalized.PreviewURL), OriginalUrl: textParam(normalized.OriginalURL),
			Title: normalized.Title, Author: normalized.Author,
			Width: int4Param(normalized.Width), Height: int4Param(normalized.Height),
			SavedByUserID: savedBy(input.OperatorUserID),
		})
		if err == nil {
			return assetFromRow(strings.TrimSpace(workspacePublicID), dbsqlc.GetLibraryAssetByPublicIDRow{
				LibraryAsset: row, FolderPublicID: folder.PublicID,
			}), true, nil
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
			WorkspaceID: workspaceInternalID, Provider: textParam(normalized.Provider), SourceID: textParam(normalized.SourceID),
		})
		if getErr != nil {
			return nil, false, fmt.Errorf("save library asset: %w", err)
		}
		return assetFromRow(strings.TrimSpace(workspacePublicID), dbsqlc.GetLibraryAssetByPublicIDRow(existing)), false, nil
	}
	return nil, false, errors.New("library: asset public id mint collision, retry the request")
}

// SaveManagedMediaAsset atomically authorizes and publishes readable Media.
func (s *Service) SaveManagedMediaAsset(ctx context.Context, workspacePublicID string, input domain.V1SaveManagedAssetInput, publicID string) (*domain.V1Asset, bool, error) {
	return s.acceptPublication(ctx, workspacePublicID, input, publicID, s.authorizeManual)
}

// SaveTaskOutput accepts one automatic publication from a completed Task.
func (s *Service) SaveTaskOutput(ctx context.Context, workspacePublicID string, input domain.V1SaveManagedAssetInput, publicID string) (*domain.V1Asset, bool, error) {
	return s.acceptPublication(ctx, workspacePublicID, input, publicID, s.authorizeAutomatic)
}

func (s *Service) authorizeManual(ctx context.Context, tx pgx.Tx, workspaceID string, input domain.V1SaveManagedAssetInput) (bool, error) {
	return true, s.publication.AuthorizeManual(ctx, tx, workspaceID, input)
}

func (s *Service) authorizeAutomatic(ctx context.Context, tx pgx.Tx, workspaceID string, input domain.V1SaveManagedAssetInput) (bool, error) {
	return s.publication.AuthorizeAutomatic(ctx, tx, workspaceID, input)
}

func (s *Service) acceptPublication(ctx context.Context, workspaceID string, input domain.V1SaveManagedAssetInput, publicID string, authorize func(context.Context, pgx.Tx, string, domain.V1SaveManagedAssetInput) (bool, error)) (*domain.V1Asset, bool, error) {
	if s.publication == nil {
		return nil, false, ErrNotConfigured
	}
	var asset *domain.V1Asset
	var created bool
	err := s.dbWithContext(ctx).withTxDBTX(func(q *dbsqlc.Queries, tx pgx.Tx) error {
		ctx := dbutil.ContextWithAmbientTx(ctx, tx)
		allowed, err := authorize(ctx, tx, workspaceID, input)
		if err != nil || !allowed {
			return err
		}
		asset, created, err = s.saveManagedMediaAsset(ctx, q, workspaceID, input, publicID)
		return err
	})
	return asset, created, err
}

func (s *Service) saveManagedMediaAsset(ctx context.Context, q *dbsqlc.Queries, workspacePublicID string, input domain.V1SaveManagedAssetInput, publicID string) (*domain.V1Asset, bool, error) {
	normalized, err := domain.NormalizeAndValidateManagedAsset(input)
	if err != nil {
		return nil, false, err
	}
	if input.OperatorUserID <= 0 {
		return nil, false, fmt.Errorf("save managed library asset: server-side operator required")
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
	media, err := q.GetMediaAssetByPublicID(ctx, dbsqlc.GetMediaAssetByPublicIDParams{
		WorkspacePublicID: strings.TrimSpace(workspacePublicID), PublicID: normalized.MediaAssetID,
	})
	if database.IsNotFound(err) {
		return nil, false, domain.ErrV1AssetNotFound
	}
	if err != nil {
		return nil, false, err
	}
	if media.ArchivedAt.Valid {
		return nil, false, domain.ErrV1AssetNotFound
	}
	if existing, err := q.GetLibraryAssetByWorkspaceMedia(ctx, dbsqlc.GetLibraryAssetByWorkspaceMediaParams{
		WorkspaceID: workspaceInternalID, MediaAssetID: pgtype.Int8{Int64: media.InternalID, Valid: true},
	}); err == nil {
		return assetFromRow(strings.TrimSpace(workspacePublicID), dbsqlc.GetLibraryAssetByPublicIDRow(existing)), false, nil
	} else if !database.IsNotFound(err) {
		return nil, false, err
	}
	folder, err := resolveAssetFolder(ctx, q, workspaceInternalID, input.FolderPublicID)
	if err != nil {
		return nil, false, err
	}
	var taskID pgtype.Text
	var taskResultIndex pgtype.Int4
	if normalized.Lineage != nil {
		taskID = pgtype.Text{String: normalized.Lineage.TaskID, Valid: true}
		taskResultIndex = pgtype.Int4{Int32: normalized.Lineage.ResultIndex, Valid: true}
	}
	row, err := q.CreateMediaLibraryAsset(ctx, dbsqlc.CreateMediaLibraryAssetParams{
		PublicID: publicID, WorkspaceID: workspaceInternalID, FolderID: folder.InternalID,
		MediaAssetID: pgtype.Int8{Int64: media.InternalID, Valid: true},
		Title:        normalized.Title, Author: normalized.Author,
		TaskID: taskID, TaskResultIndex: taskResultIndex,
		SavedByUserID: savedBy(input.OperatorUserID),
	})
	if err == nil {
		return assetFromRow(strings.TrimSpace(workspacePublicID), dbsqlc.GetLibraryAssetByPublicIDRow{
			LibraryAsset: row, FolderPublicID: folder.PublicID,
			MediaAssetPublicID: pgtype.Text{String: media.PublicID, Valid: true},
			MediaWidth:         media.Width, MediaHeight: media.Height,
		}), true, nil
	}
	if isAssetPublicIDConflict(err) {
		return nil, false, fmt.Errorf("save managed library asset: public id collision: %w", err)
	}
	if database.IsDuplicateConstraintError(err) {
		// (workspace, media_asset_id) lost a concurrent save: read
		// back the winner so replays converge on one row.
		existing, getErr := q.GetLibraryAssetByWorkspaceMedia(ctx, dbsqlc.GetLibraryAssetByWorkspaceMediaParams{
			WorkspaceID: workspaceInternalID, MediaAssetID: pgtype.Int8{Int64: media.InternalID, Valid: true},
		})
		if getErr == nil {
			return assetFromRow(strings.TrimSpace(workspacePublicID), dbsqlc.GetLibraryAssetByPublicIDRow(existing)), false, nil
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
		out = append(out, assetFromRow(strings.TrimSpace(workspacePublicID), dbsqlc.GetLibraryAssetByPublicIDRow(row)))
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
	return assetFromRow(strings.TrimSpace(workspacePublicID), row), nil
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
	targetFolder, err := resolveAssetFolder(ctx, q, workspaceInternalID, folderPublicID)
	if err != nil {
		return nil, err
	}
	row, err := q.MoveLibraryAsset(ctx, dbsqlc.MoveLibraryAssetParams{
		WorkspaceID: workspaceInternalID, PublicID: strings.TrimSpace(assetPublicID), FolderID: targetFolder.InternalID,
	})
	if database.IsNotFound(err) {
		return nil, domain.ErrV1AssetNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("move library asset: %w", err)
	}
	return assetFromRow(strings.TrimSpace(workspacePublicID), dbsqlc.GetLibraryAssetByPublicIDRow(row)), nil
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
					WorkspaceID: workspaceInternalID, Provider: textParam(provider), SourceID: textParam(sourceID),
				})
				if err == nil {
					result.Status = domain.V1ResolveResolved
					result.Asset = assetFromRow(trimmedWorkspace, dbsqlc.GetLibraryAssetByPublicIDRow(row))
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
				WorkspaceID: workspaceInternalID, Provider: textParam(provider), RowLimit: maxLibraryAssets,
			})
			if err != nil {
				return nil, err
			}
			for _, row := range rows {
				if row.LibraryAsset.SourceUrl.String == sourceURL || row.LibraryAsset.PreviewUrl.String == sourceURL || row.LibraryAsset.OriginalUrl.String == sourceURL {
					result.Status = domain.V1ResolveResolved
					result.SourceID = row.LibraryAsset.SourceID.String
					result.Asset = assetFromRow(trimmedWorkspace, dbsqlc.GetLibraryAssetByPublicIDRow(row))
					break
				}
			}
		}
		results = append(results, result)
	}
	return results, nil
}
