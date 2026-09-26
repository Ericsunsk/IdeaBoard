package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/library/domain"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5/pgtype"
)

// CreateFolder validates the display name (1..100 runes) and enforces
// per-workspace name uniqueness (case-insensitive pre-check plus the
// uq_library_folders_workspace_name backstop).
func (s *Service) CreateFolder(ctx context.Context, workspacePublicID, name, publicID string) (*domain.V1Folder, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	trimmed, err := domain.NormalizeFolderName(name)
	if err != nil {
		return nil, err
	}
	if !domain.IsV1FolderID(publicID) {
		return nil, domain.ErrV1FolderNotFound
	}
	workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, workspacePublicID)
	if err != nil {
		if errors.Is(err, errWorkspaceNotFound) {
			return nil, domain.ErrV1FolderNotFound
		}
		return nil, err
	}
	if _, err := q.GetLibraryFolderByWorkspaceAndNameCI(ctx, dbsqlc.GetLibraryFolderByWorkspaceAndNameCIParams{
		WorkspaceID: workspaceInternalID, Name: trimmed, ExcludePublicID: pgtype.Text{},
	}); err == nil {
		return nil, domain.ErrV1FolderNameConflict
	} else if !database.IsNotFound(err) {
		return nil, err
	}
	minted := publicID
	for attempt := 0; attempt < mintAttempts; attempt++ {
		row, err := q.CreateLibraryFolder(ctx, dbsqlc.CreateLibraryFolderParams{
			PublicID: minted, WorkspaceID: workspaceInternalID, Name: trimmed,
		})
		if isFolderNameConflict(err) {
			return nil, domain.ErrV1FolderNameConflict
		}
		if isFolderPublicIDConflict(err) {
			minted, err = domain.NewV1FolderPublicID()
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
func (s *Service) ListFolders(ctx context.Context, workspacePublicID string) ([]*domain.V1Folder, error) {
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
			return []*domain.V1Folder{}, nil
		}
		return nil, err
	}
	rows, err := q.ListLibraryFolders(ctx, dbsqlc.ListLibraryFoldersParams{
		WorkspaceID: workspaceInternalID, RowLimit: maxLibraryFolders,
	})
	if err != nil {
		return nil, fmt.Errorf("list library folders: %w", err)
	}
	out := make([]*domain.V1Folder, 0, len(rows))
	for _, row := range rows {
		out = append(out, folderFromRow(strings.TrimSpace(workspacePublicID), row))
	}
	return out, nil
}

// RenameFolder keeps name uniqueness; unknown folders 404 and system
// folders report immutable.
func (s *Service) RenameFolder(ctx context.Context, workspacePublicID, folderPublicID, name string) (*domain.V1Folder, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	trimmed, err := domain.NormalizeFolderName(name)
	if err != nil {
		return nil, err
	}
	if !domain.IsV1FolderID(strings.TrimSpace(folderPublicID)) {
		return nil, domain.ErrV1FolderNotFound
	}
	workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, workspacePublicID)
	if err != nil {
		if errors.Is(err, errWorkspaceNotFound) {
			return nil, domain.ErrV1FolderNotFound
		}
		return nil, err
	}
	folderID := strings.TrimSpace(folderPublicID)
	if _, err := q.GetLibraryFolderByWorkspaceAndNameCI(ctx, dbsqlc.GetLibraryFolderByWorkspaceAndNameCIParams{
		WorkspaceID: workspaceInternalID, Name: trimmed, ExcludePublicID: nullText(folderID),
	}); err == nil {
		return nil, domain.ErrV1FolderNameConflict
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
		return nil, domain.ErrV1FolderNameConflict
	}
	if !database.IsNotFound(err) {
		return nil, fmt.Errorf("rename library folder: %w", err)
	}
	existing, getErr := q.GetLibraryFolderByPublicID(ctx, dbsqlc.GetLibraryFolderByPublicIDParams{
		WorkspaceID: workspaceInternalID, PublicID: folderID,
	})
	if database.IsNotFound(getErr) {
		return nil, domain.ErrV1FolderNotFound
	}
	if getErr != nil {
		return nil, getErr
	}
	if existing.IsSystem {
		return nil, domain.ErrSystemFolderImmutable
	}
	return nil, domain.ErrV1FolderNotFound
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
	if !domain.IsV1FolderID(folderID) {
		return domain.ErrV1FolderNotFound
	}
	return db.withTx(func(q *dbsqlc.Queries) error {
		workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, workspacePublicID)
		if err != nil {
			if errors.Is(err, errWorkspaceNotFound) {
				return domain.ErrV1FolderNotFound
			}
			return err
		}
		folder, err := q.GetLibraryFolderByPublicID(ctx, dbsqlc.GetLibraryFolderByPublicIDParams{
			WorkspaceID: workspaceInternalID, PublicID: folderID,
		})
		if database.IsNotFound(err) {
			return domain.ErrV1FolderNotFound
		}
		if err != nil {
			return err
		}
		if folder.IsSystem {
			return domain.ErrSystemFolderImmutable
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
			return domain.ErrV1FolderNotFound
		}
		return nil
	})
}
