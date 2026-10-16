package domain

// Workspace-scoped library for the Public API v1. This file owns the V1
// value types, public-id helpers, and lifecycle words. Persistence lives
// behind WorkspaceLibraryStore in infrastructure/postgres and the
// test-only infrastructure/memory store.
//
// Invariants owned here:
//   - External ids mint lfolder_/lasset_/lmat_ via contract.MintPublicID,
//     matching the workspace library tables' public_id CHECKs
//     (lfolder_% / lasset_% / lmat_%); integer database ids are
//     never exposed. v1 lookups reject integer ids as malformed
//     (404, never a distinct status).
//   - Assets never store bytes. External rows are provider references
//     (provider URL + source metadata); media rows are managed saves
//     (Media Asset identity, no provider/URL keys, dimensions projected
//     from Media at read time, task provenance as lineage). Asset
//     resolution matches already-saved workspace assets by
//     (provider, source_id) or exact known URL; unknown references report
//     unresolved per item instead of fetching (provider lookup only, NOT
//     a general proxy; live provider fetch needs Control provider config).
//   - Save is idempotent per source: external re-saving on
//     (workspace, provider, source_id) and media re-saving on
//     (workspace, media_asset_id) return the existing asset
//     (created=false) without touching its folder or metadata.
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

// V1Folder is one workspace-scoped collection. The public read shape
// represents unfiled assets with FolderPublicID "" and hides system folders.
type V1Folder struct {
	WorkspacePublicID string
	PublicID          string
	Name              string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// V1Asset is one saved library reference. Source discriminates the
// shape: external rows carry provider/source identity plus real URLs;
// media rows carry a Media Asset identity and no provider/URL keys.
// Media display dimensions are projected from the managed Media row
// (Media owns the fact); Width/Height are paired (both nil = unknown,
// else both positive). Lineage is task provenance only (never a dedup
// key); the server-side operator identity is never serialized.
type V1Asset struct {
	WorkspacePublicID string
	PublicID          string
	FolderPublicID    string
	Source            AssetSource
	Provider          string
	SourceID          string
	SourceURL         string
	PreviewURL        string
	OriginalURL       string
	MediaAssetID      string
	Title             string
	Author            string
	Width             *int32
	Height            *int32
	Lineage           *TaskLineage
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// V1MaterializationStatus is the durable-job lifecycle. Records only
// move forward; completed/failed are terminal and immutable. The
// underlying wire and database words are unchanged (the public API
// exposes pending/running/completed/failed; the table stores
// processing for running per chk_library_materializations_status);
// the type only documents the allowed transitions at compile time
// (see CanTransition). SQL literals in
// internal/db/queries/workspace_library.sql stay byte-identical.
type V1MaterializationStatus string

const (
	V1MaterializationPending   V1MaterializationStatus = "pending"
	V1MaterializationRunning   V1MaterializationStatus = "running"
	V1MaterializationCompleted V1MaterializationStatus = "completed"
	V1MaterializationFailed    V1MaterializationStatus = "failed"
)

// allowedV1MaterializationTransitions mirrors the durable guards in
// internal/db/queries/workspace_library.sql: StartLibraryMaterialization
// fences pending -> processing (public running); Complete/Fail flip a
// live (pending/processing) job to completed/failed, so direct
// pending -> completed|failed is allowed. Terminal states
// (completed/failed) have no outgoing transitions; guarded misses read
// back the stored snapshot so idempotent replays converge instead of
// resurrecting jobs. Failed jobs stay retryable only by claiming a new
// job (ClaimMaterialization returns the existing pending/running/
// completed record, never flips a failed row), so failed has no
// outgoing transition here.
var allowedV1MaterializationTransitions = map[V1MaterializationStatus]map[V1MaterializationStatus]struct{}{
	V1MaterializationPending: {
		V1MaterializationRunning:   {},
		V1MaterializationCompleted: {},
		V1MaterializationFailed:    {},
	},
	V1MaterializationRunning: {
		V1MaterializationCompleted: {},
		V1MaterializationFailed:    {},
	},
}

// CanTransition reports whether a from -> to flip is durably allowed.
// It is documentation, not a gate: every transition it reports true
// for was already possible through the SQL guards, and no previously
// possible transition reports false.
func CanTransition(from, to V1MaterializationStatus) bool {
	nexts, ok := allowedV1MaterializationTransitions[from]
	if !ok {
		return false
	}
	_, ok = nexts[to]
	return ok
}

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

// V1SaveAssetInput carries one external save: optional folder binding
// plus the provider reference payload (validated including the
// provider-allowlisted https URL gate). OperatorUserID is the
// server-side operator identity (persisted, never serialized).
type V1SaveAssetInput struct {
	FolderPublicID string
	Asset          AssetInput
	OperatorUserID int64
}

// V1SaveManagedAssetInput carries one managed save into the single
// managed-save use case (manual and task-auto entries converge here):
// the Media Asset identity, display metadata, task provenance, and the
// server-side operator. FolderPublicID "" targets Quick Saves.
type V1SaveManagedAssetInput struct {
	FolderPublicID string
	MediaAssetID   string
	Title          string
	Author         string
	Lineage        *TaskLineage
	OperatorUserID int64
}

// V1ResolveItem is one typed batch-resolve entry: provider plus either
// a source_id or an exact known provider URL. Anything else resolves
// to unresolved (never fetched).
type V1ResolveItem struct {
	Provider  string
	SourceID  string
	SourceURL string
}

// V1ResolveStatus is one batch-resolve outcome. The wire words are
// unchanged ("resolved", "unresolved"); the type only documents the
// closed vocabulary at compile time. Resolve outcomes are lookup
// results, not a lifecycle: there is no transition table.
type V1ResolveStatus string

const (
	V1ResolveResolved   V1ResolveStatus = "resolved"
	V1ResolveUnresolved V1ResolveStatus = "unresolved"
)

// V1ResolveResult is one resolved/unresolved outcome. Asset is set only
// for resolved items.
type V1ResolveResult struct {
	Provider string
	SourceID string
	Status   V1ResolveStatus
	Asset    *V1Asset
}
