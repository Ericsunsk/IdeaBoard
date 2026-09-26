// Package library is the workspace-scoped library bounded context (v1 API).
//
// Layout after the #840 god-service split:
//   - domain: V1 value types, validation, status mapping, public-id helpers.
//   - ports: the frozen WorkspaceLibraryStore persistence seam.
//   - application: per-aggregate orchestration (FolderService, AssetService,
//     MaterializationService) over the seam.
//   - infrastructure/postgres: the postgres Service, one file per aggregate,
//     sharing the resolve/mapping helpers.
//
// This file keeps every previously exported identifier byte-identical so
// runtime composition, the v1 handlers, search, and the tasking
// materializer compile untouched. New code should import the subpackage
// that owns the symbol; the aliases below exist only for the migration
// window.
package library

import (
	"ideaboard/api/internal/library/domain"
)

// Value-type aliases (canonical definitions live in domain).
type (
	FolderSelectionKind     = domain.FolderSelectionKind
	FolderSelection         = domain.FolderSelection
	Folder                  = domain.Folder
	Asset                   = domain.Asset
	AssetInput              = domain.AssetInput
	SaveAssetCommand        = domain.SaveAssetCommand
	Cursor                  = domain.Cursor
	ListAssetsResult        = domain.ListAssetsResult
	AssetKey                = domain.AssetKey
	MaterializeInput        = domain.MaterializeInput
	MaterializedAsset       = domain.MaterializedAsset
	AssetMaterializer       = domain.AssetMaterializer
	V1Folder                = domain.V1Folder
	V1Asset                 = domain.V1Asset
	V1MaterializationStatus = domain.V1MaterializationStatus
	V1Materialization       = domain.V1Materialization
	V1SaveAssetInput        = domain.V1SaveAssetInput
	V1ResolveItem           = domain.V1ResolveItem
	V1ResolveResult         = domain.V1ResolveResult
)

// Provider and folder constants (canonical values live in domain).
const (
	ProviderUnsplash       = domain.ProviderUnsplash
	ProviderPexels         = domain.ProviderPexels
	ProviderWorkspaceMedia = domain.ProviderWorkspaceMedia
	AllAssetsFolder        = domain.AllAssetsFolder
	QuickSavesFolder       = domain.QuickSavesFolder
	DefaultPageSize        = domain.DefaultPageSize
	MaxPageSize            = domain.MaxPageSize
)

// V1 id prefixes, resolve bound, and lifecycle words.
const (
	V1FolderPrefix             = domain.V1FolderPrefix
	V1AssetPrefix              = domain.V1AssetPrefix
	V1MaterializationPrefix    = domain.V1MaterializationPrefix
	V1MaxResolveItems          = domain.V1MaxResolveItems
	V1MaterializationPending   = domain.V1MaterializationPending
	V1MaterializationRunning   = domain.V1MaterializationRunning
	V1MaterializationCompleted = domain.V1MaterializationCompleted
	V1MaterializationFailed    = domain.V1MaterializationFailed
	FolderSelectionAll         = domain.FolderSelectionAll
	FolderSelectionQuickSaves  = domain.FolderSelectionQuickSaves
	FolderSelectionUser        = domain.FolderSelectionUser
)

// Error aliases (canonical sentinels live in domain).
var (
	ErrInvalidFolderName            = domain.ErrInvalidFolderName
	ErrFolderNameConflict           = domain.ErrFolderNameConflict
	ErrSystemFolderImmutable        = domain.ErrSystemFolderImmutable
	ErrFolderNotFound               = domain.ErrFolderNotFound
	ErrInvalidProvider              = domain.ErrInvalidProvider
	ErrInvalidSource                = domain.ErrInvalidSource
	ErrAssetNotFound                = domain.ErrAssetNotFound
	ErrInvalidTargetFolder          = domain.ErrInvalidTargetFolder
	ErrInvalidCursor                = domain.ErrInvalidCursor
	ErrInvalidPageSize              = domain.ErrInvalidPageSize
	ErrInvalidOwner                 = domain.ErrInvalidOwner
	ErrMaterializerUnavailable      = domain.ErrMaterializerUnavailable
	ErrV1FolderNotFound             = domain.ErrV1FolderNotFound
	ErrV1AssetNotFound              = domain.ErrV1AssetNotFound
	ErrV1MaterializationNotFound    = domain.ErrV1MaterializationNotFound
	ErrV1FolderNameConflict         = domain.ErrV1FolderNameConflict
	ErrV1ResolveTooMany             = domain.ErrV1ResolveTooMany
	ErrV1MaterializationUnavailable = domain.ErrV1MaterializationUnavailable
)

// Public-id helpers and folder-selection parsing (canonical in domain).
func NewV1FolderPublicID() (string, error)          { return domain.NewV1FolderPublicID() }
func NewV1AssetPublicID() (string, error)           { return domain.NewV1AssetPublicID() }
func NewV1MaterializationPublicID() (string, error) { return domain.NewV1MaterializationPublicID() }
func IsV1FolderID(id string) bool                   { return domain.IsV1FolderID(id) }
func IsV1AssetID(id string) bool                    { return domain.IsV1AssetID(id) }
func IsV1MaterializationID(id string) bool          { return domain.IsV1MaterializationID(id) }
func ParseFolderSelection(raw string) (FolderSelection, error) {
	return domain.ParseFolderSelection(raw)
}

// normalizeFolderName trims and bounds the display name (canonical in domain).
func normalizeFolderName(raw string) (string, error) { return domain.NormalizeFolderName(raw) }

// normalizeAndValidateAsset trims and gates the provider reference
// (canonical in domain).
func normalizeAndValidateAsset(input AssetInput) (AssetInput, error) {
	return domain.NormalizeAndValidateAsset(input)
}

// isAllowedProviderURL reports the provider-allowlisted https gate
// (canonical in domain).
func isAllowedProviderURL(provider, raw string) bool {
	return domain.IsAllowedProviderURL(provider, raw)
}

// validateOwner rejects non-positive owner ids (canonical in domain).
func validateOwner(ownerUserID int64) error { return domain.ValidateOwner(ownerUserID) }
