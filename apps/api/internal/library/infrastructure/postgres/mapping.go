package postgres

import (
	"context"
	"fmt"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/library/domain"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5/pgtype"
)

func folderFromRow(workspacePublicID string, row dbsqlc.LibraryFolder) *domain.V1Folder {
	return &domain.V1Folder{
		WorkspacePublicID: workspacePublicID,
		PublicID:          row.PublicID,
		Name:              row.Name,
		CreatedAt:         dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:         dbutil.TimeValue(row.UpdatedAt),
	}
}

func assetCore(workspacePublicID, folderPublicID, publicID, provider, sourceID, sourceURL, previewURL, originalURL string, width, height int32, createdAt, updatedAt pgtype.Timestamptz) *domain.V1Asset {
	return &domain.V1Asset{
		WorkspacePublicID: workspacePublicID,
		PublicID:          publicID,
		FolderPublicID:    folderPublicID,
		Provider:          provider,
		SourceID:          sourceID,
		SourceURL:         sourceURL,
		PreviewURL:        previewURL,
		OriginalURL:       originalURL,
		Width:             width,
		Height:            height,
		CreatedAt:         dbutil.TimeValue(createdAt),
		UpdatedAt:         dbutil.TimeValue(updatedAt),
	}
}

func assetFromLibraryAsset(workspacePublicID, folderPublicID string, row dbsqlc.LibraryAsset) *domain.V1Asset {
	return assetCore(workspacePublicID, folderPublicID, row.PublicID, row.Provider, row.SourceID,
		row.SourceUrl, row.PreviewUrl, row.OriginalUrl, row.Width, row.Height, row.CreatedAt, row.UpdatedAt)
}

func assetFromPublicIDRow(workspacePublicID string, row dbsqlc.GetLibraryAssetByPublicIDRow) *domain.V1Asset {
	return assetCore(workspacePublicID, row.FolderPublicID, row.PublicID, row.Provider, row.SourceID,
		row.SourceUrl, row.PreviewUrl, row.OriginalUrl, row.Width, row.Height, row.CreatedAt, row.UpdatedAt)
}

func assetFromWorkspaceSourceRow(workspacePublicID string, row dbsqlc.GetLibraryAssetByWorkspaceSourceRow) *domain.V1Asset {
	return assetCore(workspacePublicID, row.FolderPublicID, row.PublicID, row.Provider, row.SourceID,
		row.SourceUrl, row.PreviewUrl, row.OriginalUrl, row.Width, row.Height, row.CreatedAt, row.UpdatedAt)
}

func assetFromListRow(workspacePublicID string, row dbsqlc.ListLibraryAssetsRow) *domain.V1Asset {
	return assetCore(workspacePublicID, row.FolderPublicID, row.PublicID, row.Provider, row.SourceID,
		row.SourceUrl, row.PreviewUrl, row.OriginalUrl, row.Width, row.Height, row.CreatedAt, row.UpdatedAt)
}

func assetFromProviderRow(workspacePublicID string, row dbsqlc.ListLibraryAssetsByWorkspaceProviderRow) *domain.V1Asset {
	return assetCore(workspacePublicID, row.FolderPublicID, row.PublicID, row.Provider, row.SourceID,
		row.SourceUrl, row.PreviewUrl, row.OriginalUrl, row.Width, row.Height, row.CreatedAt, row.UpdatedAt)
}

// ensureQuickSavesFolderRow ensures the workspace Quick Saves system
// folder (INSERT ... ON CONFLICT DO NOTHING + read back the winner, so
// concurrent callers converge). A user folder squatting the exact
// 'Quick Saves' name surfaces as a name conflict: the operator must
// rename it before Quick Saves can exist.
func ensureQuickSavesFolderRow(ctx context.Context, q *dbsqlc.Queries, workspaceInternalID int64) (dbsqlc.LibraryFolder, error) {
	publicID, err := domain.NewV1FolderPublicID()
	if err != nil {
		return dbsqlc.LibraryFolder{}, err
	}
	if err := q.EnsureQuickSavesFolder(ctx, dbsqlc.EnsureQuickSavesFolderParams{
		PublicID: publicID, WorkspaceID: workspaceInternalID,
	}); err != nil {
		return dbsqlc.LibraryFolder{}, fmt.Errorf("ensure Quick Saves folder: %w", err)
	}
	row, err := q.GetQuickSavesFolder(ctx, workspaceInternalID)
	if database.IsNotFound(err) {
		return dbsqlc.LibraryFolder{}, domain.ErrV1FolderNameConflict
	}
	if err != nil {
		return dbsqlc.LibraryFolder{}, fmt.Errorf("get Quick Saves folder: %w", err)
	}
	return row, nil
}

// resolveAssetFolderInternal maps a folder binding to its internal id:
// "" means Quick Saves (ensured), otherwise the folder must exist in
// the workspace.
func resolveAssetFolderInternal(ctx context.Context, q *dbsqlc.Queries, workspaceInternalID int64, folderPublicID string) (int64, error) {
	trimmed := strings.TrimSpace(folderPublicID)
	if trimmed == "" {
		quickSaves, err := ensureQuickSavesFolderRow(ctx, q, workspaceInternalID)
		if err != nil {
			return 0, err
		}
		return quickSaves.InternalID, nil
	}
	if !domain.IsV1FolderID(trimmed) {
		return 0, domain.ErrV1FolderNotFound
	}
	folder, err := q.GetLibraryFolderByPublicID(ctx, dbsqlc.GetLibraryFolderByPublicIDParams{
		WorkspaceID: workspaceInternalID, PublicID: trimmed,
	})
	if database.IsNotFound(err) {
		return 0, domain.ErrV1FolderNotFound
	}
	if err != nil {
		return 0, err
	}
	return folder.InternalID, nil
}

func isFolderNameConflict(err error) bool {
	if !database.IsDuplicateConstraintError(err) {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "uq_library_folders_workspace_name")
}

func isFolderPublicIDConflict(err error) bool {
	if !database.IsDuplicateConstraintError(err) {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "uq_library_folders_public_id")
}

func isAssetPublicIDConflict(err error) bool {
	if !database.IsDuplicateConstraintError(err) {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "uq_library_assets_public_id")
}

func nullText(value string) pgtype.Text {
	if strings.TrimSpace(value) == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func isMaterializationPublicIDConflict(err error) bool {
	if !database.IsDuplicateConstraintError(err) {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "uq_library_materializations_public_id")
}

// materializationFromDetail builds the API snapshot from one joined
// materialization row. Rows carry no updated_at: terminal records
// report completed_at, non-terminal records created_at. error_code
// has no durable column: reads always report an empty code.
func materializationFromDetail(workspacePublicID, publicID, assetPublicID, dbStatus string, mediaAssetPublicID pgtype.Text, errorMessage string, createdAt, completedAt pgtype.Timestamptz) (*domain.V1Materialization, error) {
	status, err := domain.V1MaterializationStatusFromDB(dbStatus)
	if err != nil {
		return nil, err
	}
	mediaAssetID := ""
	if mediaAssetPublicID.Valid {
		mediaAssetID = mediaAssetPublicID.String
	}
	updatedAt := dbutil.TimeValue(createdAt)
	if completedAt.Valid {
		updatedAt = completedAt.Time
	}
	return &domain.V1Materialization{
		WorkspacePublicID: workspacePublicID,
		PublicID:          publicID,
		AssetPublicID:     assetPublicID,
		Status:            status,
		MediaAssetID:      mediaAssetID,
		ErrorMessage:      errorMessage,
		CreatedAt:         dbutil.TimeValue(createdAt),
		UpdatedAt:         updatedAt,
	}, nil
}
