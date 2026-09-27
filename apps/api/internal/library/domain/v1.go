package domain

// Workspace-scoped library for the Public API v1 (issue #806).
//
// Additive only: the legacy user-scoped tables (library_folders,
// library_assets keyed by integer ids + owner_user_id) were removed
// in #809. This file owns the V1 value types, public-id
// helpers, and lifecycle words. Persistence lives behind ports
// (WorkspaceLibraryStore); the postgres sibling lives in
// infrastructure/postgres and the test-only in-memory sibling stays in
// the library root package.
//
// Invariants owned here:
//   - External ids mint lfolder_/lasset_/lmat_ via contract.MintPublicID,
//     matching the workspace library tables' public_id CHECKs
//     (lfolder_% / lasset_% / lmat_%, #809); integer database ids are
//     never exposed. v1 lookups reject legacy integer ids as malformed
//     (404, never a distinct status).
//   - Assets are saved external references ONLY: provider URL + source
//     metadata. No bytes are stored, fetched, or proxied here. Asset
//     resolution matches already-saved workspace assets by
//     (provider, source_id) or exact known URL; unknown references report
//     unresolved per item instead of fetching (provider lookup only, NOT
//     a general proxy; live provider fetch needs Control provider config
//     and is #808-owned).
//   - Save is idempotent on (workspace, provider, source_id): re-saving
//     the same reference returns the existing asset (created=false).
//   - Materializations are status-addressable records
//     (pending -> running -> completed|failed); terminal states are
//     immutable. The result links a media asset_... id produced by the
//     caller-owned materializer seam (bytes/storage stay in Media).
//   - No cross-workspace refs: every lookup is keyed by workspace first;
//     foreign-workspace ids miss and read as 404.

import (
	"time"

	"ideaboard/api/internal/platform/contract"
)

const (
	// V1FolderPrefix mints lfolder_ folder ids (contract kernel prefix,
	// matching the library_folders public_id CHECK). Exported so sibling
	// contexts (search vision refs) and handlers validate shape through
	// the kernel helper instead of copying the literal.
	V1FolderPrefix = contract.PrefixLibraryFolder
	// V1AssetPrefix mints lasset_ asset ids.
	V1AssetPrefix = contract.PrefixLibraryAsset
	// V1MaterializationPrefix mints lmat_ materialization ids.
	V1MaterializationPrefix = contract.PrefixLibraryMaterialization
	// V1MaxResolveItems bounds one typed batch resolve (fail-closed 400
	// above it; the endpoint is a lookup, not a proxy).
	V1MaxResolveItems = 20
)

// NewV1FolderPublicID mints an opaque lfolder_ public identifier.
func NewV1FolderPublicID() (string, error) {
	return contract.MintPublicID(V1FolderPrefix, 12)
}

// NewV1AssetPublicID mints an opaque lasset_ public identifier.
func NewV1AssetPublicID() (string, error) {
	return contract.MintPublicID(V1AssetPrefix, 12)
}

// NewV1MaterializationPublicID mints an opaque lmat_ public identifier.
func NewV1MaterializationPublicID() (string, error) {
	return contract.MintPublicID(V1MaterializationPrefix, 12)
}

// IsV1FolderID reports lfolder_ shape (opaque URL segment for {folder_id}).
func IsV1FolderID(id string) bool { return contract.IsValidPublicID(V1FolderPrefix, id) }

// IsV1AssetID reports lasset_ shape (opaque URL segment for {asset_id}).
func IsV1AssetID(id string) bool { return contract.IsValidPublicID(V1AssetPrefix, id) }

// IsV1MaterializationID reports lmat_ shape.
func IsV1MaterializationID(id string) bool {
	return contract.IsValidPublicID(V1MaterializationPrefix, id)
}

// V1Folder is one workspace-scoped collection. Assets without a folder
// are unfiled (FolderPublicID "") rather than forced into a system
// folder; the legacy quick-saves concept stays on the old surface.
type V1Folder struct {
	WorkspacePublicID string
	PublicID          string
	Name              string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// V1Asset is one saved library reference. External rows carry provider URLs;
// task output rows use ProviderWorkspaceMedia and point at media_assets through
// the database-managed media_asset_id column, without storing bytes in Library.
type V1Asset struct {
	WorkspacePublicID string
	PublicID          string
	FolderPublicID    string
	Provider          string
	SourceID          string
	SourceURL         string
	PreviewURL        string
	OriginalURL       string
	Title             string
	Author            string
	Width             int32
	Height            int32
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// V1MaterializationStatus is the durable-job lifecycle. Records only
// move forward; completed/failed are terminal and immutable.
type V1MaterializationStatus string

const (
	V1MaterializationPending   V1MaterializationStatus = "pending"
	V1MaterializationRunning   V1MaterializationStatus = "running"
	V1MaterializationCompleted V1MaterializationStatus = "completed"
	V1MaterializationFailed    V1MaterializationStatus = "failed"
)

// V1Materialization is one separate durable materialization job: an
// owned library reference turned into a managed Media Asset. Result
// links asset_... on success; failures carry a machine code + message
// for the status poller.
type V1Materialization struct {
	WorkspacePublicID string
	PublicID          string
	AssetPublicID     string
	Status            V1MaterializationStatus
	MediaAssetID      string
	ErrorCode         string
	ErrorMessage      string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// V1SaveAssetInput carries one save: optional folder binding plus the
// provider reference payload (validated like the legacy surface,
// including the provider-allowlisted https URL gate).
type V1SaveAssetInput struct {
	FolderPublicID string
	Asset          AssetInput
}

// V1ResolveItem is one typed batch-resolve entry: provider plus either
// a source_id or an exact known provider URL. Anything else resolves
// to unresolved (never fetched).
type V1ResolveItem struct {
	Provider  string
	SourceID  string
	SourceURL string
}

// V1ResolveResult is one resolved/unresolved outcome. Asset is set only
// for resolved items.
type V1ResolveResult struct {
	Provider string
	SourceID string
	Status   string
	Asset    *V1Asset
}
