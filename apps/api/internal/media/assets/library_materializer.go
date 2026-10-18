// Package assets materializes provider-owned Library references as managed media assets.
package assets

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"ideaboard/api/internal/library/domain"
	"ideaboard/api/internal/media/egress"
	"ideaboard/api/internal/media/fetch"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/contract"
)

// WorkspaceAssetMaterializer is the media-owned fetch seam for turning an
// owned workspace library reference into a managed media asset.
// The returned id always carries the asset_ shape.
type WorkspaceAssetMaterializer interface {
	MaterializeWorkspaceAsset(ctx context.Context, workspacePublicID string, asset *domain.V1Asset) (string, error)
}

// LibrarySourceFetcher downloads one provider URL to bytes. The
// production implementation rides the codebase's shared outbound remote
// downloader (SSRF allowlist, timeout, size cap); tests inject stubs.
// No new HTTP client exists on this path.
type LibrarySourceFetcher interface {
	FetchSourceBytes(ctx context.Context, rawURL string, allowHosts []string) ([]byte, error)
}

// MaterializationReplayStore answers whether a library asset already
// materialized into a completed managed asset, so repeat calls converge
// on the same asset_ id without refetching. Production can back it by
// scanning the concrete library store's ListMaterializations for a
// completed job on the same asset (no new SQL, no migration); tests
// inject fakes. Nil disables the check (every call fetches).
type MaterializationReplayStore interface {
	FindCompletedMaterializationMedia(ctx context.Context, workspacePublicID, assetPublicID string) (string, bool, error)
}

// LibraryMaterializerDeps wires the fetcher. Media, Objects, and Fetcher
// are required (fail fast); Replay is optional (nil skips the replay
// check).
type LibraryMaterializerDeps struct {
	Media   workspacemedia.SessionStore
	Objects workspacemedia.MediaObjects
	Fetcher LibrarySourceFetcher
	Replay  MaterializationReplayStore
}

// LibraryMaterializer is the internal (non-HTTP, non-billing,
// non-event) materialization fetcher. It owns remote fetch, storage,
// and media registration; callers own the job record
// (Create -> Running -> Complete/Failed through the library store) and
// map fetcher errors to failed records with their own fixed message.
// No quote, hold, capture, or release crosses this path (permanent free
// tier); no events are emitted; no error_code is persisted.
type LibraryMaterializer struct {
	media   workspacemedia.SessionStore
	objects workspacemedia.MediaObjects
	fetcher LibrarySourceFetcher
	replay  MaterializationReplayStore
}

var _ WorkspaceAssetMaterializer = (*LibraryMaterializer)(nil)

// NewLibraryMaterializer builds the fetcher. Nil media/objects/fetcher
// fail fast: a materializer that cannot fetch or ingest must never run.
func NewLibraryMaterializer(deps LibraryMaterializerDeps) (*LibraryMaterializer, error) {
	if deps.Media == nil {
		return nil, errors.New("library materializer: media store is required")
	}
	if deps.Objects == nil {
		return nil, errors.New("library materializer: object storage is required")
	}
	if deps.Fetcher == nil {
		return nil, errors.New("library materializer: source fetcher is required")
	}
	return &LibraryMaterializer{
		media: deps.Media, objects: deps.Objects, fetcher: deps.Fetcher,
		replay: deps.Replay,
	}, nil
}

// libraryMaterializerOwnerKey carries the triggering workspace member id
// for the session-flow ingest (CreateSession requires a positive owner).
// Only the composing caller stamps it; the fetcher only reads it.
type libraryMaterializerOwnerKey struct{}

// WithLibraryMaterializationOwner stamps the triggering user id for one
// materialization. Composition must set it from the request principal
// before invoking the fetcher.
func WithLibraryMaterializationOwner(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, libraryMaterializerOwnerKey{}, userID)
}

func libraryMaterializationOwnerFromCtx(ctx context.Context) int64 {
	if ctx == nil {
		return 0
	}
	userID, _ := ctx.Value(libraryMaterializerOwnerKey{}).(int64)
	return userID
}

// MaterializeWorkspaceAsset fetches the owned library reference and
// ingests it as a managed workspace media asset, returning its asset_
// id. The asset record itself (provider URLs + source metadata) is the
// only URL source: browser-supplied URLs never reach this method.
// Provider-denied references, missing egress policy, and download or
// ingest failures all fail closed with plain errors; the caller folds
// them into the failed job record.
func (m *LibraryMaterializer) MaterializeWorkspaceAsset(ctx context.Context, workspacePublicID string, asset *domain.V1Asset) (string, error) {
	if m == nil || m.media == nil || m.objects == nil || m.fetcher == nil {
		return "", errors.New("library materializer is not configured")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	workspaceID := strings.TrimSpace(workspacePublicID)
	if workspaceID == "" {
		return "", errors.New("library materializer: workspace identity required")
	}
	if asset == nil || strings.TrimSpace(asset.PublicID) == "" {
		return "", errors.New("library materializer: library asset identity required")
	}
	if replayed, ok, err := m.replayCompleted(ctx, workspaceID, strings.TrimSpace(asset.PublicID)); err != nil || ok {
		return replayed, err
	}
	ownerUserID := libraryMaterializationOwnerFromCtx(ctx)
	if ownerUserID <= 0 {
		return "", errors.New("library materializer: owner identity is required")
	}
	downloadURL, err := pickLibraryDownloadURL(asset)
	if err != nil {
		return "", err
	}
	// Code-owned provider hosts (not the operator outbound policy): the
	// fetch URL is already pinned to the closed provider world above, so
	// the same list doubles as the transport allowlist. SSRF protection
	// is unchanged (exact hosts, DNS-filtered downloader). Gate owned by
	// media/egress (https-only for library fetches).
	allowHosts := domain.ProviderFetchHosts(strings.ToLower(strings.TrimSpace(asset.Provider)))
	if err := egress.CheckHTTPSHostAllowlist(downloadURL, allowHosts); err != nil {
		return "", err
	}
	data, err := m.fetcher.FetchSourceBytes(ctx, downloadURL, allowHosts)
	if err != nil {
		return "", fmt.Errorf("library materializer: download failed: %w", err)
	}
	managed, err := workspacemedia.IngestImage(ctx, m.media, m.objects, workspacemedia.IngestImageParams{
		WorkspaceID: workspaceID, OwnerUserID: ownerUserID,
		FilenameStem: "library-materialized", Data: data,
	})
	if err != nil {
		return "", fmt.Errorf("library materializer: ingest failed: %w", err)
	}
	slog.Info("library asset materialized as media asset",
		"workspace_id", workspaceID, "library_asset", strings.TrimSpace(asset.PublicID),
		"provider", strings.TrimSpace(asset.Provider), "source_id", strings.TrimSpace(asset.SourceID),
		"asset_id", managed.ID, "mime", managed.MIMEType,
		"width", managed.Width, "height", managed.Height, "bytes", managed.SizeBytes)
	return managed.ID, nil
}

// replayCompleted returns the previously completed managed asset id for
// the library asset without fetching. A malformed replay hit fails
// closed (never handed to the job record, which requires asset_ shape).
func (m *LibraryMaterializer) replayCompleted(ctx context.Context, workspaceID, assetPublicID string) (string, bool, error) {
	if m.replay == nil {
		return "", false, nil
	}
	mediaAssetID, found, err := m.replay.FindCompletedMaterializationMedia(ctx, workspaceID, assetPublicID)
	if err != nil || !found {
		return "", false, err
	}
	trimmed := strings.TrimSpace(mediaAssetID)
	if !contract.IsAssetID(trimmed) {
		return "", false, fmt.Errorf("library materializer: replayed media asset id is malformed")
	}
	return trimmed, true, nil
}

// pickLibraryDownloadURL resolves the fetch URL from the owned record,
// preferring the original and failing closed when no recorded URL passes
// the provider-allowlisted https gate. Errors name the provider and
// field only, never the URL.
func pickLibraryDownloadURL(asset *domain.V1Asset) (string, error) {
	provider := strings.ToLower(strings.TrimSpace(asset.Provider))
	switch provider {
	case domain.ProviderUnsplash, domain.ProviderPexels:
	default:
		return "", fmt.Errorf("library materializer: provider %q cannot be fetched", provider)
	}
	for _, raw := range []string{asset.OriginalURL, asset.PreviewURL, asset.SourceURL} {
		if domain.IsAllowedProviderURL(provider, raw) {
			return strings.TrimSpace(raw), nil
		}
	}
	return "", fmt.Errorf("library materializer: provider %q reference has no fetchable URL", provider)
}

// RemoteLibrarySourceFetcher is the production LibrarySourceFetcher. The
// shared fetch seam (media/fetch, bounded by MaxUploadSizeBytes) owns the
// download-to-bytes logic; this adapter only satisfies the fetcher shape.
type RemoteLibrarySourceFetcher struct{}

// Compile-time convergence with the fetch seam.
var _ LibrarySourceFetcher = RemoteLibrarySourceFetcher{}

func (RemoteLibrarySourceFetcher) FetchSourceBytes(ctx context.Context, rawURL string, allowHosts []string) ([]byte, error) {
	return fetch.FetchBytes(ctx, strings.TrimSpace(rawURL), allowHosts, workspacemedia.MaxUploadSizeBytes)
}
