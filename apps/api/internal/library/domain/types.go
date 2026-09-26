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

	"ideaboard/api/internal/platform/pagination"
)

const (
	ProviderUnsplash       = "unsplash"
	ProviderPexels         = "pexels"
	ProviderWorkspaceMedia = "workspace_media"
	AllAssetsFolder        = "all"
	QuickSavesFolder       = "quick-saves"
	// DefaultPageSize tracks the shared pagination standard.
	DefaultPageSize = pagination.DefaultLimit
	// MaxPageSize tracks the shared pagination standard.
	MaxPageSize = pagination.MaxLimit
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
