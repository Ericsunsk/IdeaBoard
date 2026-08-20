package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"ideaboard/api/internal/db/sqlc"
)

type Repository struct {
	db *sql.DB
	q  *dbsqlc.Queries
}

func NewRepository(db *sql.DB) *Repository {
	if db == nil {
		return &Repository{}
	}
	return &Repository{db: db, q: dbsqlc.New(db)}
}

func (r *Repository) dbWithContext(ctx context.Context) *dbsqlc.Queries {
	if r == nil || r.q == nil {
		return nil
	}
	return r.q
}

func (r *Repository) withTx(ctx context.Context, run func(*dbsqlc.Queries) error) error {
	if r == nil || r.db == nil || r.q == nil {
		return errors.New("library repository is not configured")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin library transaction: %w", err)
	}
	if err := run(r.q.WithTx(tx)); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			return errors.Join(err, fmt.Errorf("rollback library transaction: %w", rollbackErr))
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit library transaction: %w", err)
	}
	return nil
}

func (r *Repository) EnsureQuickSaves(ctx context.Context, ownerUserID int64) (dbsqlc.LibraryFolder, error) {
	if err := validateOwner(ownerUserID); err != nil {
		return dbsqlc.LibraryFolder{}, err
	}
	q := r.dbWithContext(ctx)
	if q == nil {
		return dbsqlc.LibraryFolder{}, errors.New("library repository is not configured")
	}
	if err := q.EnsureQuickSavesFolder(ctx, ownerUserID); err != nil {
		return dbsqlc.LibraryFolder{}, fmt.Errorf("ensure Quick Saves folder: %w", err)
	}
	folder, err := q.GetQuickSavesFolder(ctx, ownerUserID)
	if err != nil {
		return dbsqlc.LibraryFolder{}, fmt.Errorf("get Quick Saves folder: %w", err)
	}
	return folder, nil
}

func (r *Repository) EnsureQuickSavesInTx(ctx context.Context, q *dbsqlc.Queries, ownerUserID int64) (dbsqlc.LibraryFolder, error) {
	if err := validateOwner(ownerUserID); err != nil {
		return dbsqlc.LibraryFolder{}, err
	}
	if q == nil {
		return dbsqlc.LibraryFolder{}, errors.New("library transaction queries are not configured")
	}
	if err := q.EnsureQuickSavesFolder(ctx, ownerUserID); err != nil {
		return dbsqlc.LibraryFolder{}, fmt.Errorf("ensure Quick Saves folder: %w", err)
	}
	folder, err := q.GetQuickSavesFolder(ctx, ownerUserID)
	if err != nil {
		return dbsqlc.LibraryFolder{}, fmt.Errorf("get Quick Saves folder: %w", err)
	}
	return folder, nil
}

func (r *Repository) ListFolders(ctx context.Context, ownerUserID int64) ([]dbsqlc.ListLibraryFoldersRow, error) {
	if err := validateOwner(ownerUserID); err != nil {
		return nil, err
	}
	if _, err := r.EnsureQuickSaves(ctx, ownerUserID); err != nil {
		return nil, err
	}
	q := r.dbWithContext(ctx)
	if q == nil {
		return nil, errors.New("library repository is not configured")
	}
	rows, err := q.ListLibraryFolders(ctx, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("list library folders: %w", err)
	}
	return rows, nil
}

func (r *Repository) CreateFolder(ctx context.Context, ownerUserID int64, name string) (dbsqlc.LibraryFolder, error) {
	q := r.dbWithContext(ctx)
	if q == nil {
		return dbsqlc.LibraryFolder{}, errors.New("library repository is not configured")
	}
	folder, err := q.CreateLibraryFolder(ctx, dbsqlc.CreateLibraryFolderParams{OwnerUserID: ownerUserID, Name: name})
	if err != nil {
		return dbsqlc.LibraryFolder{}, fmt.Errorf("create library folder: %w", err)
	}
	return folder, nil
}

func (r *Repository) EnsureFolderByNameInTx(ctx context.Context, q *dbsqlc.Queries, ownerUserID int64, name string) (dbsqlc.LibraryFolder, error) {
	if q == nil {
		return dbsqlc.LibraryFolder{}, errors.New("library transaction queries are not configured")
	}
	if err := q.EnsureLibraryFolderByName(ctx, dbsqlc.EnsureLibraryFolderByNameParams{OwnerUserID: ownerUserID, Name: name}); err != nil {
		return dbsqlc.LibraryFolder{}, fmt.Errorf("ensure library folder: %w", err)
	}
	folder, err := q.GetLibraryFolderByOwnerAndName(ctx, dbsqlc.GetLibraryFolderByOwnerAndNameParams{OwnerUserID: ownerUserID, Name: name})
	if err != nil {
		return dbsqlc.LibraryFolder{}, fmt.Errorf("get ensured library folder: %w", err)
	}
	return folder, nil
}

func (r *Repository) GetFolder(ctx context.Context, ownerUserID, folderID int64) (dbsqlc.LibraryFolder, error) {
	q := r.dbWithContext(ctx)
	if q == nil {
		return dbsqlc.LibraryFolder{}, errors.New("library repository is not configured")
	}
	folder, err := q.GetLibraryFolderByIDForOwner(ctx, dbsqlc.GetLibraryFolderByIDForOwnerParams{ID: folderID, OwnerUserID: ownerUserID})
	if errors.Is(err, sql.ErrNoRows) {
		return dbsqlc.LibraryFolder{}, ErrFolderNotFound
	}
	if err != nil {
		return dbsqlc.LibraryFolder{}, fmt.Errorf("get library folder: %w", err)
	}
	return folder, nil
}

func (r *Repository) RenameFolder(ctx context.Context, ownerUserID, folderID int64, name string) (dbsqlc.LibraryFolder, error) {
	q := r.dbWithContext(ctx)
	if q == nil {
		return dbsqlc.LibraryFolder{}, errors.New("library repository is not configured")
	}
	folder, err := q.RenameLibraryFolder(ctx, dbsqlc.RenameLibraryFolderParams{ID: folderID, OwnerUserID: ownerUserID, Name: name})
	if errors.Is(err, sql.ErrNoRows) {
		return dbsqlc.LibraryFolder{}, ErrFolderNotFound
	}
	if err != nil {
		return dbsqlc.LibraryFolder{}, fmt.Errorf("rename library folder: %w", err)
	}
	return folder, nil
}

func (r *Repository) DeleteFolder(ctx context.Context, ownerUserID, folderID int64) error {
	return r.withTx(ctx, func(q *dbsqlc.Queries) error {
		folder, err := q.GetLibraryFolderByIDForOwner(ctx, dbsqlc.GetLibraryFolderByIDForOwnerParams{ID: folderID, OwnerUserID: ownerUserID})
		if errors.Is(err, sql.ErrNoRows) {
			return ErrFolderNotFound
		}
		if err != nil {
			return fmt.Errorf("get folder before delete: %w", err)
		}
		if folder.IsSystem {
			return ErrSystemFolderImmutable
		}
		quickSaves, err := r.EnsureQuickSavesInTx(ctx, q, ownerUserID)
		if err != nil {
			return err
		}
		if _, err := q.MoveLibraryAssetsToFolder(ctx, dbsqlc.MoveLibraryAssetsToFolderParams{TargetFolderID: quickSaves.ID, OwnerUserID: ownerUserID, SourceFolderID: folderID}); err != nil {
			return fmt.Errorf("move assets to Quick Saves: %w", err)
		}
		deleted, err := q.DeleteLibraryFolder(ctx, dbsqlc.DeleteLibraryFolderParams{ID: folderID, OwnerUserID: ownerUserID})
		if err != nil {
			return fmt.Errorf("delete library folder: %w", err)
		}
		if deleted != 1 {
			return ErrFolderNotFound
		}
		return nil
	})
}

func (r *Repository) ListAssets(ctx context.Context, ownerUserID int64, folderID *int64, cursor *Cursor, limit int32) ([]dbsqlc.LibraryAsset, error) {
	q := r.dbWithContext(ctx)
	if q == nil {
		return nil, errors.New("library repository is not configured")
	}
	params := dbsqlc.ListLibraryAssetsParams{OwnerUserID: ownerUserID, RowLimit: limit}
	if folderID != nil {
		params.FolderID = sql.NullInt64{Int64: *folderID, Valid: true}
	}
	if cursor != nil {
		params.CursorCreatedAt = sql.NullTime{Time: cursor.CreatedAt, Valid: true}
		params.CursorID = sql.NullInt64{Int64: cursor.ID, Valid: true}
	}
	items, err := q.ListLibraryAssets(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list library assets: %w", err)
	}
	return items, nil
}

func (r *Repository) SaveAsset(ctx context.Context, command SaveAssetCommand) (dbsqlc.LibraryAsset, error) {
	returnValue := dbsqlc.LibraryAsset{}
	err := r.withTx(ctx, func(q *dbsqlc.Queries) error {
		var folder dbsqlc.LibraryFolder
		var err error
		if command.FolderSelection != nil {
			switch command.FolderSelection.Kind {
			case FolderSelectionQuickSaves:
				folder, err = r.EnsureQuickSavesInTx(ctx, q, command.OwnerUserID)
			case FolderSelectionUser:
				folder, err = q.GetLibraryFolderByIDForOwner(ctx, dbsqlc.GetLibraryFolderByIDForOwnerParams{ID: command.FolderSelection.ID, OwnerUserID: command.OwnerUserID})
				if errors.Is(err, sql.ErrNoRows) {
					return ErrFolderNotFound
				}
				if err == nil && folder.IsSystem {
					return ErrInvalidTargetFolder
				}
			default:
				return ErrInvalidTargetFolder
			}
		} else if command.NewFolderName != "" {
			folder, err = r.EnsureFolderByNameInTx(ctx, q, command.OwnerUserID, command.NewFolderName)
		} else {
			folder, err = r.EnsureQuickSavesInTx(ctx, q, command.OwnerUserID)
		}
		if err != nil {
			return err
		}
		asset, err := q.UpsertLibraryAsset(ctx, dbsqlc.UpsertLibraryAssetParams{
			OwnerUserID: command.OwnerUserID, FolderID: folder.ID, Provider: command.Asset.Provider,
			SourceID: command.Asset.SourceID, SourceUrl: command.Asset.SourceURL, PreviewUrl: command.Asset.PreviewURL,
			OriginalUrl: command.Asset.OriginalURL, Title: command.Asset.Title, Author: command.Asset.Author,
			Width: command.Asset.Width, Height: command.Asset.Height,
		})
		if err != nil {
			return fmt.Errorf("upsert library asset: %w", err)
		}
		returnValue = asset
		return nil
	})
	return returnValue, err
}

func (r *Repository) MoveAsset(ctx context.Context, ownerUserID, assetID, folderID int64) (dbsqlc.LibraryAsset, error) {
	q := r.dbWithContext(ctx)
	if q == nil {
		return dbsqlc.LibraryAsset{}, errors.New("library repository is not configured")
	}
	asset, err := q.MoveLibraryAsset(ctx, dbsqlc.MoveLibraryAssetParams{ID: assetID, OwnerUserID: ownerUserID, FolderID: folderID})
	if errors.Is(err, sql.ErrNoRows) {
		return dbsqlc.LibraryAsset{}, ErrAssetNotFound
	}
	if err != nil {
		return dbsqlc.LibraryAsset{}, fmt.Errorf("move library asset: %w", err)
	}
	return asset, nil
}

func (r *Repository) DeleteAsset(ctx context.Context, ownerUserID, assetID int64) error {
	q := r.dbWithContext(ctx)
	if q == nil {
		return errors.New("library repository is not configured")
	}
	deleted, err := q.DeleteLibraryAsset(ctx, dbsqlc.DeleteLibraryAssetParams{ID: assetID, OwnerUserID: ownerUserID})
	if err != nil {
		return fmt.Errorf("delete library asset: %w", err)
	}
	if deleted != 1 {
		return ErrAssetNotFound
	}
	return nil
}

func (r *Repository) GetAsset(ctx context.Context, ownerUserID, assetID int64) (dbsqlc.LibraryAsset, error) {
	q := r.dbWithContext(ctx)
	if q == nil {
		return dbsqlc.LibraryAsset{}, errors.New("library repository is not configured")
	}
	asset, err := q.GetLibraryAssetByIDForOwner(ctx, dbsqlc.GetLibraryAssetByIDForOwnerParams{ID: assetID, OwnerUserID: ownerUserID})
	if errors.Is(err, sql.ErrNoRows) {
		return dbsqlc.LibraryAsset{}, ErrAssetNotFound
	}
	if err != nil {
		return dbsqlc.LibraryAsset{}, fmt.Errorf("get library asset: %w", err)
	}
	return asset, nil
}

func (r *Repository) LookupAssets(ctx context.Context, ownerUserID int64, keys []AssetKey) ([]dbsqlc.LibraryAsset, error) {
	q := r.dbWithContext(ctx)
	if q == nil {
		return nil, errors.New("library repository is not configured")
	}
	raw, err := json.Marshal(keys)
	if err != nil {
		return nil, fmt.Errorf("encode library lookup keys: %w", err)
	}
	items, err := q.LookupLibraryAssets(ctx, dbsqlc.LookupLibraryAssetsParams{OwnerUserID: ownerUserID, Keys: raw})
	if err != nil {
		return nil, fmt.Errorf("lookup library assets: %w", err)
	}
	return items, nil
}
