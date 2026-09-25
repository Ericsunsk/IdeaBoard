package library

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Service is the postgres WorkspaceLibraryStore (#818, Phase 4-01 of
// #809). Repository methods take context.Context first and use the
// dbWithContext pattern. It must never be replaced by the test-only
// InMemoryWorkspaceLibraryStore in production wiring.
//
// Workspace scoping: every lookup resolves the workspace public id to
// its internal id first and keys all folder/asset statements by
// (workspace_id, public_id). Cross-workspace ids miss by construction
// and read as not found; unknown workspaces list empty and mutate as
// not found. Membership itself is enforced by the v1 handlers
// (RequireWorkspaceCapability) before the store is reached; the store
// guarantees no cross-workspace reference escapes.
//
// Quick Saves: the workspace tables require folder_id NOT NULL, so the
// seam's unfiled ("") state maps to the workspace Quick Saves system
// folder (is_system + is_default, name 'Quick Saves'), ensured on
// demand. Deleting a user folder moves its assets to Quick Saves first
// (same tx), mirroring the in-memory "become unfiled" semantics.
// System folders are immutable: rename/delete statements exclude them.
//
// Title/author have no durable columns (workspace assets keep provider
// URL + source metadata only); inputs are validated by the seam and
// reads return empty strings.
//
// Materializations are durable jobs (pending -> running -> completed|
// failed; terminal states immutable). The durable status word for
// running is 'processing' (chk_library_materializations_status); the
// store maps it at the boundary and enforces the machine in SQL
// guards. Completion links a workspace-scoped Media Asset (the
// caller's asset_... id resolves to media_assets.internal_id; unknown
// links fail closed with the job left non-terminal). Failures persist
// error_message only (no durable error_code column exists; reads
// report an empty code) and rows carry no updated_at (reads report
// completed_at, else created_at, as updated_at).
type Service struct {
	db *pgxpool.Pool
	q  *dbsqlc.Queries
}

// Compile-time convergence with the v1 persistence seam.
var _ WorkspaceLibraryStore = (*Service)(nil)

// errWorkspaceNotFound stays unexported: callers map it to the
// folder/asset not-found error of their operation so hidden workspaces
// never leak a distinct status.
var errWorkspaceNotFound = errors.New("library workspace not found")

// errNotConfigured reports a Service built without a database pool.
var errNotConfigured = errors.New("library service is not configured")

// List caps: every list query carries a LIMIT so no caller can trigger
// an unbounded scan. Keyset pagination is a follow-up; the handler
// slices pages in memory like the in-memory store.
const (
	maxLibraryFolders          = int32(500)
	maxLibraryAssets           = int32(2000)
	maxLibraryMaterializations = int32(2000)
)

// Durable materialization status words (chk_library_materializations_
// status). The public API exposes running; the table stores
// processing. The store maps at the boundary in both directions.
const (
	libraryMaterializationDBPending    = "pending"
	libraryMaterializationDBProcessing = "processing"
	libraryMaterializationDBCompleted  = "completed"
	libraryMaterializationDBFailed     = "failed"
)

// mintAttempts bounds public_id regeneration after a random collision.
const mintAttempts = 3

// NewService builds a postgres library Service over pool.
func NewService(db *pgxpool.Pool) *Service {
	if db == nil {
		return &Service{}
	}
	return &Service{db: db, q: dbsqlc.New(db)}
}

type serviceQueryer struct {
	ctx context.Context
	db  *pgxpool.Pool
	q   *dbsqlc.Queries
}

func (s *Service) dbWithContext(ctx context.Context) serviceQueryer {
	return serviceQueryer{ctx: ctx, db: s.db, q: s.q}
}

func (db serviceQueryer) withTx(run func(*dbsqlc.Queries) error) error {
	return dbutil.WithTx(db.ctx, db.db, run)
}

func (db serviceQueryer) queries() (*dbsqlc.Queries, error) {
	if db.q == nil {
		return nil, errNotConfigured
	}
	return db.q, nil
}

// resolveWorkspaceInternal maps an opaque workspace public id to its
// internal id. Unknown workspaces report errWorkspaceNotFound so
// callers converge to their operation's not-found error.
func resolveWorkspaceInternal(ctx context.Context, q *dbsqlc.Queries, workspacePublicID string) (int64, error) {
	row, err := q.GetWorkspaceByPublicID(ctx, strings.TrimSpace(workspacePublicID))
	if database.IsNotFound(err) {
		return 0, errWorkspaceNotFound
	}
	if err != nil {
		return 0, err
	}
	return row.InternalID, nil
}

func folderFromRow(workspacePublicID string, row dbsqlc.LibraryFolder) *V1Folder {
	return &V1Folder{
		WorkspacePublicID: workspacePublicID,
		PublicID:          row.PublicID,
		Name:              row.Name,
		CreatedAt:         dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:         dbutil.TimeValue(row.UpdatedAt),
	}
}

func assetCore(workspacePublicID, folderPublicID, publicID, provider, sourceID, sourceURL, previewURL, originalURL string, width, height int32, createdAt, updatedAt pgtype.Timestamptz) *V1Asset {
	return &V1Asset{
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

func assetFromLibraryAsset(workspacePublicID, folderPublicID string, row dbsqlc.LibraryAsset) *V1Asset {
	return assetCore(workspacePublicID, folderPublicID, row.PublicID, row.Provider, row.SourceID,
		row.SourceUrl, row.PreviewUrl, row.OriginalUrl, row.Width, row.Height, row.CreatedAt, row.UpdatedAt)
}

func assetFromPublicIDRow(workspacePublicID string, row dbsqlc.GetLibraryAssetByPublicIDRow) *V1Asset {
	return assetCore(workspacePublicID, row.FolderPublicID, row.PublicID, row.Provider, row.SourceID,
		row.SourceUrl, row.PreviewUrl, row.OriginalUrl, row.Width, row.Height, row.CreatedAt, row.UpdatedAt)
}

func assetFromWorkspaceSourceRow(workspacePublicID string, row dbsqlc.GetLibraryAssetByWorkspaceSourceRow) *V1Asset {
	return assetCore(workspacePublicID, row.FolderPublicID, row.PublicID, row.Provider, row.SourceID,
		row.SourceUrl, row.PreviewUrl, row.OriginalUrl, row.Width, row.Height, row.CreatedAt, row.UpdatedAt)
}

func assetFromListRow(workspacePublicID string, row dbsqlc.ListLibraryAssetsRow) *V1Asset {
	return assetCore(workspacePublicID, row.FolderPublicID, row.PublicID, row.Provider, row.SourceID,
		row.SourceUrl, row.PreviewUrl, row.OriginalUrl, row.Width, row.Height, row.CreatedAt, row.UpdatedAt)
}

func assetFromProviderRow(workspacePublicID string, row dbsqlc.ListLibraryAssetsByWorkspaceProviderRow) *V1Asset {
	return assetCore(workspacePublicID, row.FolderPublicID, row.PublicID, row.Provider, row.SourceID,
		row.SourceUrl, row.PreviewUrl, row.OriginalUrl, row.Width, row.Height, row.CreatedAt, row.UpdatedAt)
}

// ensureQuickSavesFolderRow ensures the workspace Quick Saves system
// folder (INSERT ... ON CONFLICT DO NOTHING + read back the winner, so
// concurrent callers converge). A user folder squatting the exact
// 'Quick Saves' name surfaces as a name conflict: the operator must
// rename it before Quick Saves can exist.
func ensureQuickSavesFolderRow(ctx context.Context, q *dbsqlc.Queries, workspaceInternalID int64) (dbsqlc.LibraryFolder, error) {
	publicID, err := NewV1FolderPublicID()
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
		return dbsqlc.LibraryFolder{}, ErrV1FolderNameConflict
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
	if !IsV1FolderID(trimmed) {
		return 0, ErrV1FolderNotFound
	}
	folder, err := q.GetLibraryFolderByPublicID(ctx, dbsqlc.GetLibraryFolderByPublicIDParams{
		WorkspaceID: workspaceInternalID, PublicID: trimmed,
	})
	if database.IsNotFound(err) {
		return 0, ErrV1FolderNotFound
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

// v1MaterializationStatusFromDB maps the durable status word to the
// public API word (processing -> running).
func v1MaterializationStatusFromDB(status string) (V1MaterializationStatus, error) {
	switch status {
	case libraryMaterializationDBPending:
		return V1MaterializationPending, nil
	case libraryMaterializationDBProcessing:
		return V1MaterializationRunning, nil
	case libraryMaterializationDBCompleted:
		return V1MaterializationCompleted, nil
	case libraryMaterializationDBFailed:
		return V1MaterializationFailed, nil
	default:
		return "", fmt.Errorf("library: unknown materialization status %q", status)
	}
}

// materializationFromDetail builds the API snapshot from one joined
// materialization row. Rows carry no updated_at: terminal records
// report completed_at, non-terminal records created_at. error_code
// has no durable column: reads always report an empty code.
func materializationFromDetail(workspacePublicID, publicID, assetPublicID, dbStatus string, mediaAssetPublicID pgtype.Text, errorMessage string, createdAt, completedAt pgtype.Timestamptz) (*V1Materialization, error) {
	status, err := v1MaterializationStatusFromDB(dbStatus)
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
	return &V1Materialization{
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

// CreateFolder validates the display name (1..100 runes) and enforces
// per-workspace name uniqueness (case-insensitive pre-check plus the
// uq_library_folders_workspace_name backstop).
func (s *Service) CreateFolder(ctx context.Context, workspacePublicID, name, publicID string) (*V1Folder, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	trimmed, err := normalizeFolderName(name)
	if err != nil {
		return nil, err
	}
	if !IsV1FolderID(publicID) {
		return nil, ErrV1FolderNotFound
	}
	workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, workspacePublicID)
	if err != nil {
		if errors.Is(err, errWorkspaceNotFound) {
			return nil, ErrV1FolderNotFound
		}
		return nil, err
	}
	if _, err := q.GetLibraryFolderByWorkspaceAndNameCI(ctx, dbsqlc.GetLibraryFolderByWorkspaceAndNameCIParams{
		WorkspaceID: workspaceInternalID, Name: trimmed, ExcludePublicID: pgtype.Text{},
	}); err == nil {
		return nil, ErrV1FolderNameConflict
	} else if !database.IsNotFound(err) {
		return nil, err
	}
	minted := publicID
	for attempt := 0; attempt < mintAttempts; attempt++ {
		row, err := q.CreateLibraryFolder(ctx, dbsqlc.CreateLibraryFolderParams{
			PublicID: minted, WorkspaceID: workspaceInternalID, Name: trimmed,
		})
		if isFolderNameConflict(err) {
			return nil, ErrV1FolderNameConflict
		}
		if isFolderPublicIDConflict(err) {
			minted, err = NewV1FolderPublicID()
			if err != nil {
				return nil, err
			}
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("create library folder: %w", err)
		}
		return folderFromRow(strings.TrimSpace(workspacePublicID), row), nil
	}
	return nil, errors.New("library: folder public id mint collision, retry the request")
}

// ListFolders returns folders in creation order (stable for cursor
// pagination at the handler). Unknown workspaces list empty, matching
// the in-memory store.
func (s *Service) ListFolders(ctx context.Context, workspacePublicID string) ([]*V1Folder, error) {
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
			return []*V1Folder{}, nil
		}
		return nil, err
	}
	rows, err := q.ListLibraryFolders(ctx, dbsqlc.ListLibraryFoldersParams{
		WorkspaceID: workspaceInternalID, RowLimit: maxLibraryFolders,
	})
	if err != nil {
		return nil, fmt.Errorf("list library folders: %w", err)
	}
	out := make([]*V1Folder, 0, len(rows))
	for _, row := range rows {
		out = append(out, folderFromRow(strings.TrimSpace(workspacePublicID), row))
	}
	return out, nil
}

// RenameFolder keeps name uniqueness; unknown folders 404 and system
// folders report immutable.
func (s *Service) RenameFolder(ctx context.Context, workspacePublicID, folderPublicID, name string) (*V1Folder, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	trimmed, err := normalizeFolderName(name)
	if err != nil {
		return nil, err
	}
	if !IsV1FolderID(strings.TrimSpace(folderPublicID)) {
		return nil, ErrV1FolderNotFound
	}
	workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, workspacePublicID)
	if err != nil {
		if errors.Is(err, errWorkspaceNotFound) {
			return nil, ErrV1FolderNotFound
		}
		return nil, err
	}
	folderID := strings.TrimSpace(folderPublicID)
	if _, err := q.GetLibraryFolderByWorkspaceAndNameCI(ctx, dbsqlc.GetLibraryFolderByWorkspaceAndNameCIParams{
		WorkspaceID: workspaceInternalID, Name: trimmed, ExcludePublicID: nullText(folderID),
	}); err == nil {
		return nil, ErrV1FolderNameConflict
	} else if !database.IsNotFound(err) {
		return nil, err
	}
	row, err := q.RenameLibraryFolder(ctx, dbsqlc.RenameLibraryFolderParams{
		WorkspaceID: workspaceInternalID, PublicID: folderID, Name: trimmed,
	})
	if err == nil {
		return folderFromRow(strings.TrimSpace(workspacePublicID), row), nil
	}
	if isFolderNameConflict(err) {
		return nil, ErrV1FolderNameConflict
	}
	if !database.IsNotFound(err) {
		return nil, fmt.Errorf("rename library folder: %w", err)
	}
	existing, getErr := q.GetLibraryFolderByPublicID(ctx, dbsqlc.GetLibraryFolderByPublicIDParams{
		WorkspaceID: workspaceInternalID, PublicID: folderID,
	})
	if database.IsNotFound(getErr) {
		return nil, ErrV1FolderNotFound
	}
	if getErr != nil {
		return nil, getErr
	}
	if existing.IsSystem {
		return nil, ErrSystemFolderImmutable
	}
	return nil, ErrV1FolderNotFound
}

// DeleteFolder drops a user folder; contained assets move to Quick
// Saves in the same transaction rather than deleting (documented
// postgres behavior). System folders are immutable.
func (s *Service) DeleteFolder(ctx context.Context, workspacePublicID, folderPublicID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	db := s.dbWithContext(ctx)
	if _, err := db.queries(); err != nil {
		return err
	}
	folderID := strings.TrimSpace(folderPublicID)
	if !IsV1FolderID(folderID) {
		return ErrV1FolderNotFound
	}
	return db.withTx(func(q *dbsqlc.Queries) error {
		workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, workspacePublicID)
		if err != nil {
			if errors.Is(err, errWorkspaceNotFound) {
				return ErrV1FolderNotFound
			}
			return err
		}
		folder, err := q.GetLibraryFolderByPublicID(ctx, dbsqlc.GetLibraryFolderByPublicIDParams{
			WorkspaceID: workspaceInternalID, PublicID: folderID,
		})
		if database.IsNotFound(err) {
			return ErrV1FolderNotFound
		}
		if err != nil {
			return err
		}
		if folder.IsSystem {
			return ErrSystemFolderImmutable
		}
		quickSaves, err := ensureQuickSavesFolderRow(ctx, q, workspaceInternalID)
		if err != nil {
			return err
		}
		if _, err := q.MoveLibraryAssetsToFolder(ctx, dbsqlc.MoveLibraryAssetsToFolderParams{
			WorkspaceID: workspaceInternalID, SourceFolderID: folder.InternalID, TargetFolderID: quickSaves.InternalID,
		}); err != nil {
			return fmt.Errorf("move library assets to Quick Saves: %w", err)
		}
		affected, err := q.DeleteLibraryFolder(ctx, dbsqlc.DeleteLibraryFolderParams{
			WorkspaceID: workspaceInternalID, PublicID: folderID,
		})
		if err != nil {
			return fmt.Errorf("delete library folder: %w", err)
		}
		if affected == 0 {
			return ErrV1FolderNotFound
		}
		return nil
	})
}

// SaveAsset validates the provider reference (provider-allowlisted
// https URLs; never bytes) and dedups on
// (workspace, provider, source_id): re-saving returns the existing
// asset with created=false. An unknown folder binding 404s. An empty
// folder binding saves into Quick Saves.
func (s *Service) SaveAsset(ctx context.Context, workspacePublicID string, input V1SaveAssetInput, publicID string) (*V1Asset, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, false, err
	}
	normalized, err := normalizeAndValidateAsset(input.Asset)
	if err != nil {
		return nil, false, err
	}
	if !IsV1AssetID(strings.TrimSpace(publicID)) {
		return nil, false, ErrV1AssetNotFound
	}
	workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, workspacePublicID)
	if err != nil {
		if errors.Is(err, errWorkspaceNotFound) {
			return nil, false, ErrV1AssetNotFound
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
			minted, err = NewV1AssetPublicID()
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
func (s *Service) SaveManagedMediaAsset(ctx context.Context, workspacePublicID, sourceID, mediaAssetID, publicID string) (*V1Asset, bool, error) {
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
	if sourceID == "" || len(sourceID) > 255 || !contract.IsAssetID(mediaAssetID) || !IsV1AssetID(publicID) {
		return nil, false, ErrV1AssetNotFound
	}
	workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, workspacePublicID)
	if err != nil {
		if errors.Is(err, errWorkspaceNotFound) {
			return nil, false, ErrV1AssetNotFound
		}
		return nil, false, err
	}
	if existing, err := q.GetLibraryAssetByWorkspaceSource(ctx, dbsqlc.GetLibraryAssetByWorkspaceSourceParams{
		WorkspaceID: workspaceInternalID, Provider: ProviderWorkspaceMedia, SourceID: sourceID,
	}); err == nil {
		return assetFromWorkspaceSourceRow(strings.TrimSpace(workspacePublicID), existing), false, nil
	} else if !database.IsNotFound(err) {
		return nil, false, err
	}
	media, err := q.GetMediaAssetByPublicID(ctx, dbsqlc.GetMediaAssetByPublicIDParams{
		WorkspacePublicID: strings.TrimSpace(workspacePublicID), PublicID: mediaAssetID,
	})
	if database.IsNotFound(err) {
		return nil, false, ErrV1AssetNotFound
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
			WorkspaceID: workspaceInternalID, Provider: ProviderWorkspaceMedia, SourceID: sourceID,
		})
		if getErr == nil {
			return assetFromWorkspaceSourceRow(strings.TrimSpace(workspacePublicID), existing), false, nil
		}
	}
	return nil, false, fmt.Errorf("save managed library asset: %w", err)
}

// ListAssets returns every workspace asset in creation order; the
// handler slices the stable page. Unknown workspaces list empty.
func (s *Service) ListAssets(ctx context.Context, workspacePublicID string) ([]*V1Asset, error) {
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
			return []*V1Asset{}, nil
		}
		return nil, err
	}
	rows, err := q.ListLibraryAssets(ctx, dbsqlc.ListLibraryAssetsParams{
		WorkspaceID: workspaceInternalID, RowLimit: maxLibraryAssets,
	})
	if err != nil {
		return nil, fmt.Errorf("list library assets: %w", err)
	}
	out := make([]*V1Asset, 0, len(rows))
	for _, row := range rows {
		out = append(out, assetFromListRow(strings.TrimSpace(workspacePublicID), row))
	}
	return out, nil
}

// GetAsset converges unknown ids to ErrV1AssetNotFound (cross-workspace
// ids miss the workspace-keyed lookup by construction).
func (s *Service) GetAsset(ctx context.Context, workspacePublicID, assetPublicID string) (*V1Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	if !IsV1AssetID(strings.TrimSpace(assetPublicID)) {
		return nil, ErrV1AssetNotFound
	}
	workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, workspacePublicID)
	if err != nil {
		if errors.Is(err, errWorkspaceNotFound) {
			return nil, ErrV1AssetNotFound
		}
		return nil, err
	}
	row, err := q.GetLibraryAssetByPublicID(ctx, dbsqlc.GetLibraryAssetByPublicIDParams{
		WorkspaceID: workspaceInternalID, PublicID: strings.TrimSpace(assetPublicID),
	})
	if database.IsNotFound(err) {
		return nil, ErrV1AssetNotFound
	}
	if err != nil {
		return nil, err
	}
	return assetFromPublicIDRow(strings.TrimSpace(workspacePublicID), row), nil
}

// MoveAsset rebinds the folder ("" moves to Quick Saves); unknown
// assets or folders 404 with their own codes.
func (s *Service) MoveAsset(ctx context.Context, workspacePublicID, assetPublicID, folderPublicID string) (*V1Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	if !IsV1AssetID(strings.TrimSpace(assetPublicID)) {
		return nil, ErrV1AssetNotFound
	}
	workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, workspacePublicID)
	if err != nil {
		if errors.Is(err, errWorkspaceNotFound) {
			return nil, ErrV1AssetNotFound
		}
		return nil, err
	}
	if _, err := q.GetLibraryAssetByPublicID(ctx, dbsqlc.GetLibraryAssetByPublicIDParams{
		WorkspaceID: workspaceInternalID, PublicID: strings.TrimSpace(assetPublicID),
	}); database.IsNotFound(err) {
		return nil, ErrV1AssetNotFound
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
		return nil, ErrV1AssetNotFound
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
	if !IsV1AssetID(strings.TrimSpace(assetPublicID)) {
		return ErrV1AssetNotFound
	}
	workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, workspacePublicID)
	if err != nil {
		if errors.Is(err, errWorkspaceNotFound) {
			return ErrV1AssetNotFound
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
		return ErrV1AssetNotFound
	}
	return nil
}

// ResolveAssets performs the typed batch resolve (bounded to
// V1MaxResolveItems): provider + source_id matches a saved workspace
// asset; a bare provider URL must be an allowlisted https URL and match
// a saved asset's known URL exactly. Nothing is fetched: unknown
// references report unresolved per item. NOT a general proxy.
func (s *Service) ResolveAssets(ctx context.Context, workspacePublicID string, items []V1ResolveItem) ([]V1ResolveResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	if len(items) > V1MaxResolveItems {
		return nil, ErrV1ResolveTooMany
	}
	workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, workspacePublicID)
	if err != nil && !errors.Is(err, errWorkspaceNotFound) {
		return nil, err
	}
	if errors.Is(err, errWorkspaceNotFound) {
		workspaceInternalID = -1
	}
	trimmedWorkspace := strings.TrimSpace(workspacePublicID)
	results := make([]V1ResolveResult, 0, len(items))
	for _, item := range items {
		provider := strings.ToLower(strings.TrimSpace(item.Provider))
		sourceID := strings.TrimSpace(item.SourceID)
		sourceURL := strings.TrimSpace(item.SourceURL)
		result := V1ResolveResult{Provider: provider, SourceID: sourceID, Status: "unresolved"}
		if provider != ProviderUnsplash && provider != ProviderPexels {
			results = append(results, result)
			continue
		}
		if sourceID != "" {
			if workspaceInternalID > 0 {
				row, err := q.GetLibraryAssetByWorkspaceSource(ctx, dbsqlc.GetLibraryAssetByWorkspaceSourceParams{
					WorkspaceID: workspaceInternalID, Provider: provider, SourceID: sourceID,
				})
				if err == nil {
					result.Status = "resolved"
					result.Asset = assetFromWorkspaceSourceRow(trimmedWorkspace, row)
				} else if !database.IsNotFound(err) {
					return nil, err
				}
			}
			results = append(results, result)
			continue
		}
		if sourceURL == "" || !isAllowedProviderURL(provider, sourceURL) {
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
					result.Status = "resolved"
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

// CreateMaterialization opens a pending job for an owned workspace
// asset. Unknown workspaces and unknown/malformed assets converge to
// ErrV1AssetNotFound (parity with the in-memory store, where both
// miss the same map); malformed job ids report
// ErrV1MaterializationNotFound. No dedup key exists: one asset may
// back many jobs and request idempotency stays handler-side
// (library:materialize scope).
func (s *Service) CreateMaterialization(ctx context.Context, workspacePublicID, assetPublicID, publicID string) (*V1Materialization, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	if !IsV1MaterializationID(strings.TrimSpace(publicID)) {
		return nil, ErrV1MaterializationNotFound
	}
	if !IsV1AssetID(strings.TrimSpace(assetPublicID)) {
		return nil, ErrV1AssetNotFound
	}
	workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, workspacePublicID)
	if err != nil {
		if errors.Is(err, errWorkspaceNotFound) {
			return nil, ErrV1AssetNotFound
		}
		return nil, err
	}
	asset, err := q.GetLibraryAssetByPublicID(ctx, dbsqlc.GetLibraryAssetByPublicIDParams{
		WorkspaceID: workspaceInternalID, PublicID: strings.TrimSpace(assetPublicID),
	})
	if database.IsNotFound(err) {
		return nil, ErrV1AssetNotFound
	}
	if err != nil {
		return nil, err
	}
	trimmedWorkspace := strings.TrimSpace(workspacePublicID)
	minted := strings.TrimSpace(publicID)
	for attempt := 0; attempt < mintAttempts; attempt++ {
		row, err := q.CreateLibraryMaterialization(ctx, dbsqlc.CreateLibraryMaterializationParams{
			PublicID: minted, WorkspaceID: workspaceInternalID, LibraryAssetID: asset.InternalID,
		})
		if isMaterializationPublicIDConflict(err) {
			minted, err = NewV1MaterializationPublicID()
			if err != nil {
				return nil, err
			}
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("create library materialization: %w", err)
		}
		return materializationFromDetail(trimmedWorkspace, row.PublicID, asset.PublicID,
			row.Status, pgtype.Text{}, row.ErrorMessage, row.CreatedAt, row.CompletedAt)
	}
	return nil, errors.New("library: materialization public id mint collision, retry the request")
}

// GetMaterialization returns the status-addressable job snapshot.
// Malformed, unknown, and cross-workspace ids converge to
// ErrV1MaterializationNotFound.
func (s *Service) GetMaterialization(ctx context.Context, workspacePublicID, materializationPublicID string) (*V1Materialization, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	if !IsV1MaterializationID(strings.TrimSpace(materializationPublicID)) {
		return nil, ErrV1MaterializationNotFound
	}
	workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, workspacePublicID)
	if err != nil {
		if errors.Is(err, errWorkspaceNotFound) {
			return nil, ErrV1MaterializationNotFound
		}
		return nil, err
	}
	row, err := q.GetLibraryMaterializationByPublicID(ctx, dbsqlc.GetLibraryMaterializationByPublicIDParams{
		WorkspaceID: workspaceInternalID, PublicID: strings.TrimSpace(materializationPublicID),
	})
	if database.IsNotFound(err) {
		return nil, ErrV1MaterializationNotFound
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
func (s *Service) ListMaterializations(ctx context.Context, workspacePublicID string) ([]*V1Materialization, error) {
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
			return []*V1Materialization{}, nil
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
	out := make([]*V1Materialization, 0, len(rows))
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
func materializationSnapshot(ctx context.Context, q *dbsqlc.Queries, workspacePublicID string, workspaceInternalID int64, materializationPublicID string) (*V1Materialization, error) {
	row, err := q.GetLibraryMaterializationByPublicID(ctx, dbsqlc.GetLibraryMaterializationByPublicIDParams{
		WorkspaceID: workspaceInternalID, PublicID: materializationPublicID,
	})
	if database.IsNotFound(err) {
		return nil, ErrV1MaterializationNotFound
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
func (s *Service) SetMaterializationResult(ctx context.Context, workspacePublicID, materializationPublicID string, status V1MaterializationStatus, mediaAssetID, errorCode, errorMessage string) (*V1Materialization, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	if !IsV1MaterializationID(strings.TrimSpace(materializationPublicID)) {
		return nil, ErrV1MaterializationNotFound
	}
	workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, workspacePublicID)
	if err != nil {
		if errors.Is(err, errWorkspaceNotFound) {
			return nil, ErrV1MaterializationNotFound
		}
		return nil, err
	}
	jobID := strings.TrimSpace(materializationPublicID)
	switch status {
	case V1MaterializationRunning, V1MaterializationCompleted, V1MaterializationFailed:
		// Proceed below.
	default:
		// Unknown words never advance a job: terminal records still
		// report their snapshot (in-memory parity) while anything
		// else reports not found.
		snapshot, err := materializationSnapshot(ctx, q, workspacePublicID, workspaceInternalID, jobID)
		if err != nil {
			return nil, err
		}
		if snapshot.Status == V1MaterializationCompleted || snapshot.Status == V1MaterializationFailed {
			return snapshot, nil
		}
		return nil, ErrV1MaterializationNotFound
	}
	switch status {
	case V1MaterializationRunning:
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
	case V1MaterializationCompleted:
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
