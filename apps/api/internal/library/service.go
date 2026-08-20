package library

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/lib/pq"
	dbsqlc "ideaboard/api/internal/db/sqlc"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) ListFolders(ctx context.Context, ownerUserID int64) ([]Folder, error) {
	if err := validateOwner(ownerUserID); err != nil {
		return nil, err
	}
	rows, err := s.repo.ListFolders(ctx, ownerUserID)
	if err != nil {
		return nil, err
	}
	result := make([]Folder, 0, len(rows)+1)
	var total int64
	for _, row := range rows {
		queryKey := strconv.FormatInt(row.ID, 10)
		if row.IsSystem && row.IsDefault {
			queryKey = QuickSavesFolder
		}
		folder := Folder{
			ID: row.ID, QueryKey: queryKey, Name: row.Name, IsSystem: row.IsSystem, IsDefault: row.IsDefault,
			AssetCount: row.AssetCount, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}
		total += row.AssetCount
		result = append(result, folder)
	}
	return append([]Folder{{QueryKey: AllAssetsFolder, Name: "All Assets", IsSystem: true, IsVirtual: true, AssetCount: total}}, result...), nil
}

func (s *Service) CreateFolder(ctx context.Context, ownerUserID int64, rawName string) (Folder, error) {
	if err := validateOwner(ownerUserID); err != nil {
		return Folder{}, err
	}
	name, err := normalizeFolderName(rawName)
	if err != nil {
		return Folder{}, err
	}
	row, err := s.repo.CreateFolder(ctx, ownerUserID, name)
	if isUniqueViolation(err) {
		return Folder{}, ErrFolderNameConflict
	}
	if err != nil {
		return Folder{}, err
	}
	return folderFromRow(row), nil
}

func (s *Service) RenameFolder(ctx context.Context, ownerUserID, folderID int64, rawName string) (Folder, error) {
	if err := validateOwner(ownerUserID); err != nil {
		return Folder{}, err
	}
	name, err := normalizeFolderName(rawName)
	if err != nil {
		return Folder{}, err
	}
	folder, err := s.repo.GetFolder(ctx, ownerUserID, folderID)
	if err != nil {
		return Folder{}, err
	}
	if folder.IsSystem {
		return Folder{}, ErrSystemFolderImmutable
	}
	row, err := s.repo.RenameFolder(ctx, ownerUserID, folderID, name)
	if isUniqueViolation(err) {
		return Folder{}, ErrFolderNameConflict
	}
	if err != nil {
		return Folder{}, err
	}
	return folderFromRow(row), nil
}

func (s *Service) DeleteFolder(ctx context.Context, ownerUserID, folderID int64) error {
	if err := validateOwner(ownerUserID); err != nil {
		return err
	}
	if folderID <= 0 {
		return ErrSystemFolderImmutable
	}
	return s.repo.DeleteFolder(ctx, ownerUserID, folderID)
}

func (s *Service) ListAssets(ctx context.Context, ownerUserID int64, selection FolderSelection, rawCursor string, rawLimit int) (ListAssetsResult, error) {
	if err := validateOwner(ownerUserID); err != nil {
		return ListAssetsResult{}, err
	}
	if rawLimit <= 0 {
		rawLimit = DefaultPageSize
	}
	if rawLimit > MaxPageSize {
		return ListAssetsResult{}, ErrInvalidPageSize
	}
	folderID, err := s.resolveFolderSelection(ctx, ownerUserID, selection)
	if err != nil {
		return ListAssetsResult{}, err
	}
	cursor, err := DecodeCursor(rawCursor)
	if err != nil {
		return ListAssetsResult{}, err
	}
	rows, err := s.repo.ListAssets(ctx, ownerUserID, folderID, cursor, int32(rawLimit+1))
	if err != nil {
		return ListAssetsResult{}, err
	}
	result := ListAssetsResult{Items: make([]Asset, 0, len(rows))}
	if len(rows) > rawLimit {
		result.HasMore = true
		rows = rows[:rawLimit]
	}
	for _, row := range rows {
		result.Items = append(result.Items, assetFromRow(row))
	}
	if result.HasMore && len(result.Items) > 0 {
		last := result.Items[len(result.Items)-1]
		result.NextCursor, err = EncodeCursor(Cursor{CreatedAt: last.CreatedAt, ID: last.ID})
		if err != nil {
			return ListAssetsResult{}, fmt.Errorf("encode library cursor: %w", err)
		}
	}
	return result, nil
}

func (s *Service) resolveFolderSelection(ctx context.Context, ownerUserID int64, selection FolderSelection) (*int64, error) {
	switch selection.Kind {
	case FolderSelectionAll:
		return nil, nil
	case FolderSelectionQuickSaves:
		id, err := s.QuickSavesFolderID(ctx, ownerUserID)
		if err != nil {
			return nil, err
		}
		return &id, nil
	case FolderSelectionUser:
		if selection.ID <= 0 {
			return nil, ErrFolderNotFound
		}
		folder, err := s.repo.GetFolder(ctx, ownerUserID, selection.ID)
		if err != nil {
			return nil, err
		}
		if folder.IsSystem {
			return nil, ErrFolderNotFound
		}
		id := selection.ID
		return &id, nil
	default:
		return nil, ErrFolderNotFound
	}
}

func (s *Service) QuickSavesFolderID(ctx context.Context, ownerUserID int64) (int64, error) {
	if err := validateOwner(ownerUserID); err != nil {
		return 0, err
	}
	folder, err := s.repo.EnsureQuickSaves(ctx, ownerUserID)
	if err != nil {
		return 0, err
	}
	return folder.ID, nil
}

func (s *Service) SaveAsset(ctx context.Context, command SaveAssetCommand) (Asset, error) {
	if err := validateOwner(command.OwnerUserID); err != nil {
		return Asset{}, err
	}
	asset, err := normalizeAndValidateAsset(command.Asset)
	if err != nil {
		return Asset{}, err
	}
	if command.FolderSelection != nil && strings.TrimSpace(command.NewFolderName) != "" {
		return Asset{}, ErrInvalidTargetFolder
	}
	if command.FolderSelection != nil {
		switch command.FolderSelection.Kind {
		case FolderSelectionQuickSaves:
		case FolderSelectionUser:
			if command.FolderSelection.ID <= 0 {
				return Asset{}, ErrInvalidTargetFolder
			}
		case FolderSelectionAll:
			return Asset{}, ErrInvalidTargetFolder
		default:
			return Asset{}, ErrInvalidTargetFolder
		}
	}
	command.Asset = asset
	command.NewFolderName, err = normalizeOptionalFolderName(command.NewFolderName)
	if err != nil {
		return Asset{}, err
	}
	row, err := s.repo.SaveAsset(ctx, command)
	if isUniqueViolation(err) {
		return Asset{}, ErrFolderNameConflict
	}
	if err != nil {
		return Asset{}, err
	}
	return assetFromRow(row), nil
}

func (s *Service) MoveAsset(ctx context.Context, ownerUserID, assetID, folderID int64) (Asset, error) {
	if err := validateOwner(ownerUserID); err != nil {
		return Asset{}, err
	}
	if folderID <= 0 {
		return Asset{}, ErrInvalidTargetFolder
	}
	if _, err := s.repo.GetFolder(ctx, ownerUserID, folderID); err != nil {
		if errors.Is(err, ErrFolderNotFound) {
			return Asset{}, ErrInvalidTargetFolder
		}
		return Asset{}, err
	}
	row, err := s.repo.MoveAsset(ctx, ownerUserID, assetID, folderID)
	if err != nil {
		return Asset{}, err
	}
	return assetFromRow(row), nil
}

func (s *Service) DeleteAsset(ctx context.Context, ownerUserID, assetID int64) error {
	if err := validateOwner(ownerUserID); err != nil {
		return err
	}
	return s.repo.DeleteAsset(ctx, ownerUserID, assetID)
}

func (s *Service) LookupAssets(ctx context.Context, ownerUserID int64, keys []AssetKey) ([]Asset, error) {
	if err := validateOwner(ownerUserID); err != nil {
		return nil, err
	}
	if len(keys) > MaxPageSize {
		return nil, ErrInvalidSource
	}
	validated := make([]AssetKey, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		provider := strings.ToLower(strings.TrimSpace(key.Provider))
		sourceID := strings.TrimSpace(key.SourceID)
		if err := validateAssetKey(AssetInput{Provider: provider, SourceID: sourceID}); err != nil {
			return nil, err
		}
		canonical := provider + "\x00" + sourceID
		if _, exists := seen[canonical]; exists {
			continue
		}
		seen[canonical] = struct{}{}
		validated = append(validated, AssetKey{Provider: provider, SourceID: sourceID})
	}
	rows, err := s.repo.LookupAssets(ctx, ownerUserID, validated)
	if err != nil {
		return nil, err
	}
	result := make([]Asset, 0, len(rows))
	for _, row := range rows {
		result = append(result, assetFromRow(row))
	}
	return result, nil
}

func normalizeOptionalFolderName(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	return normalizeFolderName(raw)
}

func folderFromRow(row dbsqlc.LibraryFolder) Folder {
	queryKey := strconv.FormatInt(row.ID, 10)
	if row.IsSystem && row.IsDefault {
		queryKey = QuickSavesFolder
	}
	return Folder{ID: row.ID, QueryKey: queryKey, Name: row.Name, IsSystem: row.IsSystem, IsDefault: row.IsDefault, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func assetFromRow(row dbsqlc.LibraryAsset) Asset {
	return Asset{ID: row.ID, FolderID: row.FolderID, Provider: row.Provider, SourceID: row.SourceID, SourceURL: row.SourceUrl, PreviewURL: row.PreviewUrl, OriginalURL: row.OriginalUrl, Title: row.Title, Author: row.Author, Width: row.Width, Height: row.Height, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func validateAssetKey(input AssetInput) error {
	if input.Provider != ProviderUnsplash && input.Provider != ProviderPexels {
		return ErrInvalidProvider
	}
	if input.SourceID == "" || len(input.SourceID) > 128 {
		return ErrInvalidSource
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}
