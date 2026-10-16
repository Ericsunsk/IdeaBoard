// Package domain owns the library bounded-context language: value types,
// validation, public-id helpers, and status mapping. It has no database,
// SQL, or transport dependencies (only the contract id kernel and the
// shared pagination standard), so ports, application, and infrastructure
// all share one definition of what a folder, asset, or materialization is.
package domain

import (
	"context"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/pagination"
)

const (
	ProviderUnsplash = "unsplash"
	ProviderPexels   = "pexels"
	AllAssetsFolder  = "all"
	QuickSavesFolder = "quick-saves"
	// DefaultPageSize tracks the shared pagination standard.
	DefaultPageSize = pagination.DefaultLimit
	// MaxPageSize tracks the shared pagination standard.
	MaxPageSize = pagination.MaxLimit
)

// AssetSource is the Library Source discriminator:
// 'external' rows carry provider/source identity plus real URLs;
// 'media' rows carry a Media Asset identity and no provider/URL keys.
type AssetSource string

const (
	AssetSourceExternal AssetSource = "external"
	AssetSourceMedia    AssetSource = "media"
)

type FolderSelectionKind string

const (
	FolderSelectionAll        FolderSelectionKind = "all"
	FolderSelectionQuickSaves FolderSelectionKind = "quick-saves"
	FolderSelectionUser       FolderSelectionKind = "user"
)

type FolderSelection struct {
	Kind FolderSelectionKind
	ID   int64
}

func ParseFolderSelection(raw string) (FolderSelection, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == AllAssetsFolder {
		return FolderSelection{Kind: FolderSelectionAll}, nil
	}
	if raw == QuickSavesFolder {
		return FolderSelection{Kind: FolderSelectionQuickSaves}, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return FolderSelection{}, ErrFolderNotFound
	}
	return FolderSelection{Kind: FolderSelectionUser, ID: id}, nil
}

type Folder struct {
	ID         int64
	QueryKey   string
	Name       string
	IsSystem   bool
	IsDefault  bool
	IsVirtual  bool
	AssetCount int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Asset struct {
	ID          int64
	FolderID    int64
	Provider    string
	SourceID    string
	SourceURL   string
	PreviewURL  string
	OriginalURL string
	Title       string
	Author      string
	Width       int32
	Height      int32
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TaskLineage is task provenance for a managed save: which task output
// produced it. Provenance only, never a dedup key; nil means a manual
// (non-task) save. Always paired by construction.
type TaskLineage struct {
	TaskID      string
	ResultIndex int32
}

type AssetInput struct {
	Provider    string `json:"provider"`
	SourceID    string `json:"source_id"`
	SourceURL   string `json:"source_url"`
	PreviewURL  string `json:"preview_url"`
	OriginalURL string `json:"original_url"`
	Title       string `json:"title"`
	Author      string `json:"author"`
	Width       int32  `json:"width"`
	Height      int32  `json:"height"`
}

// NormalizeAndValidateManagedAsset trims the managed-save payload and
// enforces its shape: a well-formed asset_ Media identity, bounded
// display metadata, and well-formed task provenance when present.
// The store validates the operator and resolves the folder binding
// ("" means Quick Saves).
func NormalizeAndValidateManagedAsset(input V1SaveManagedAssetInput) (V1SaveManagedAssetInput, error) {
	input.MediaAssetID = strings.TrimSpace(input.MediaAssetID)
	input.Title = strings.TrimSpace(input.Title)
	input.Author = strings.TrimSpace(input.Author)
	if !contract.IsAssetID(input.MediaAssetID) {
		return V1SaveManagedAssetInput{}, ErrInvalidSource
	}
	if utf8.RuneCountInString(input.Title) > 255 || utf8.RuneCountInString(input.Author) > 255 {
		return V1SaveManagedAssetInput{}, ErrInvalidSource
	}
	if input.Lineage != nil {
		lineage := *input.Lineage
		lineage.TaskID = strings.TrimSpace(lineage.TaskID)
		if !contract.IsTaskID(lineage.TaskID) || lineage.ResultIndex < 0 {
			return V1SaveManagedAssetInput{}, ErrInvalidSource
		}
		input.Lineage = &lineage
	}
	return input, nil
}

// PairDimensions projects a media dimension pair onto the Library read
// shape: both set and positive, else both nil (unknown). Guards against
// half-known media rows so the wire never carries an unpaired pair.
func PairDimensions(width, height *int32) (*int32, *int32) {
	if width == nil || height == nil || *width <= 0 || *height <= 0 {
		return nil, nil
	}
	return width, height
}

type SaveAssetCommand struct {
	OwnerUserID     int64
	FolderSelection *FolderSelection
	NewFolderName   string
	Asset           AssetInput
}

type Cursor struct {
	CreatedAt time.Time
	ID        int64
}

type ListAssetsResult struct {
	Items      []Asset
	NextCursor string
	HasMore    bool
}

type AssetKey struct {
	Provider string `json:"provider"`
	SourceID string `json:"source_id"`
}

// MaterializeInput is the only cross-context handoff for turning an owned
// external Library asset into managed Media. The browser never supplies URLs
// to this command; Library resolves them from its own owner-scoped record.
type MaterializeInput struct {
	OwnerUserID    int64
	LibraryAssetID int64
	Provider       string
	SourceID       string
	SourceURL      string
	OriginalURL    string
	PreviewURL     string
	Title          string
	Author         string
}

type MaterializedAsset struct {
	MediaAssetID int64
	AssetID      string
	URL          string
	Width        int32
	Height       int32
	MIMEType     string
	Extension    string
}

// AssetMaterializer is caller-owned so Library keeps its URL-backed domain
// model while Media owns outbound fetch, storage and managed Asset lifecycle.
type AssetMaterializer interface {
	MaterializeLibraryAsset(context.Context, MaterializeInput) (MaterializedAsset, error)
}
