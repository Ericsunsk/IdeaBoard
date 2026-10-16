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

// int4Pair projects a nullable media dimension pair onto the Library
// read shape: both set and positive, else both nil (unknown). Media rows
// outside images may carry half-known dimensions; the wire never does.
func int4Pair(width, height pgtype.Int4) (*int32, *int32) {
	if !width.Valid || !height.Valid {
		return nil, nil
	}
	w, h := width.Int32, height.Int32
	return domain.PairDimensions(&w, &h)
}

func assetFromRow(workspacePublicID string, view dbsqlc.GetLibraryAssetByPublicIDRow) *domain.V1Asset {
	row := view.LibraryAsset
	asset := &domain.V1Asset{
		WorkspacePublicID: workspacePublicID,
		PublicID:          row.PublicID,
		FolderPublicID:    view.FolderPublicID,
		Title:             row.Title,
		Author:            row.Author,
		CreatedAt:         dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:         dbutil.TimeValue(row.UpdatedAt),
	}
	if row.MediaAssetID.Valid {
		asset.Source = domain.AssetSourceMedia
		asset.MediaAssetID = view.MediaAssetPublicID.String
		asset.Width, asset.Height = int4Pair(view.MediaWidth, view.MediaHeight)
		if row.TaskID.Valid && row.TaskResultIndex.Valid {
			asset.Lineage = &domain.TaskLineage{TaskID: row.TaskID.String, ResultIndex: row.TaskResultIndex.Int32}
		}
		return asset
	}
	asset.Source = domain.AssetSourceExternal
	asset.Provider = row.Provider.String
	asset.SourceID = row.SourceID.String
	asset.SourceURL = row.SourceUrl.String
	asset.PreviewURL = row.PreviewUrl.String
	asset.OriginalURL = row.OriginalUrl.String
	asset.Width, asset.Height = &row.Width.Int32, &row.Height.Int32
	return asset
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

// resolveAssetFolder resolves an explicit folder or ensures Quick Saves.
func resolveAssetFolder(ctx context.Context, q *dbsqlc.Queries, workspaceInternalID int64, folderPublicID string) (dbsqlc.LibraryFolder, error) {
	trimmed := strings.TrimSpace(folderPublicID)
	if trimmed == "" {
		return ensureQuickSavesFolderRow(ctx, q, workspaceInternalID)
	}
	if !domain.IsV1FolderID(trimmed) {
		return dbsqlc.LibraryFolder{}, domain.ErrV1FolderNotFound
	}
	folder, err := q.GetLibraryFolderByPublicID(ctx, dbsqlc.GetLibraryFolderByPublicIDParams{
		WorkspaceID: workspaceInternalID, PublicID: trimmed,
	})
	if database.IsNotFound(err) {
		return dbsqlc.LibraryFolder{}, domain.ErrV1FolderNotFound
	}
	return folder, err
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

// textParam lifts a validated non-empty value into a nullable column.
func textParam(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: true}
}

// int4Param lifts a validated dimension into a nullable column.
func int4Param(value int32) pgtype.Int4 {
	return pgtype.Int4{Int32: value, Valid: true}
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
