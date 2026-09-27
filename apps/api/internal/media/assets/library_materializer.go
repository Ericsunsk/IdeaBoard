// Package assets owns the media-side library materialization fetcher.
//
// The legacy user-scoped ingestion path (MediaAssetRegistry Begin/Complete
// over the dropped media_assets owner model) is a fail-closed stub since
// #817 Phase 3b-08. The live persistence owner for workspace-scoped media
// rows is workspacemedia.Service (Phase 4-03 of #809), so this fetcher
// persists through the #820 session flow (CreateSession -> PUT bytes ->
// CompleteSession) — the same ingest the tasking renderer and the
// workspace provider-result materializer share — instead of the dead
// IngestionRegistry stubs. Provenance for the fetch is the durable
// library materialization job record itself (library_asset_id ->
// media asset_ link); media_assets.metadata carries no origin facts
// (CompleteSession persists '{}'), so origin details (provider,
// source_id) travel in structured logs only, never in errors or URLs.
package assets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"strings"

	"ideaboard/api/internal/library/domain"
	"ideaboard/api/internal/media/derivatives/ports"
	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/media/storage/infrastructure/remote"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/contract"
)

// WorkspaceAssetMaterializer is the media-owned fetch seam for turning an
// owned workspace library reference into a managed media asset. It
// defines the WorkspaceAssetMaterializer seam
// (MaterializeWorkspaceAsset with the same signature), so composition can
// assign a *LibraryMaterializer to that slot without touching the
// contract file; the interface lives here to avoid importing the HTTP
// layer into Media. The returned id always carries the asset_ shape.
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
// are required (fail fast); Renderer is optional with deferred-tier
// semantics (nil skips generation, tiers stay pending); Replay is
// optional (nil skips the replay check).
type LibraryMaterializerDeps struct {
	Media    workspacemedia.SessionStore
	Objects  workspacemedia.DerivativeObjects
	Fetcher  LibrarySourceFetcher
	Renderer ports.Renderer
	Replay   MaterializationReplayStore
}

// LibraryMaterializer is the internal (non-HTTP, non-billing,
// non-event) materialization fetcher. It owns remote fetch, storage,
// and media registration; callers own the job record
// (Create -> Running -> Complete/Failed through the library store) and
// map fetcher errors to failed records with their own fixed message.
// No quote, hold, capture, or release crosses this path (permanent free
// tier); no events are emitted; no error_code is persisted.
type LibraryMaterializer struct {
	media    workspacemedia.SessionStore
	objects  workspacemedia.DerivativeObjects
	fetcher  LibrarySourceFetcher
	renderer ports.Renderer
	replay   MaterializationReplayStore
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
		renderer: deps.Renderer, replay: deps.Replay,
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
	// is unchanged (exact hosts, DNS-filtered downloader).
	allowHosts := domain.ProviderFetchHosts(strings.ToLower(strings.TrimSpace(asset.Provider)))
	if err := checkLibraryEgressAllowlist(downloadURL, allowHosts); err != nil {
		return "", err
	}
	data, err := m.fetcher.FetchSourceBytes(ctx, downloadURL, allowHosts)
	if err != nil {
		return "", fmt.Errorf("library materializer: download failed: %w", err)
	}
	managed, err := m.ingestMaterializedBytes(ctx, workspaceID, ownerUserID, asset, data)
	if err != nil {
		return "", err
	}
	generateLibraryDerivativesBestEffort(ctx, m.media, m.objects, m.renderer, workspaceID, managed)
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

// checkLibraryEgressAllowlist enforces the closed-world egress gate
// BEFORE any network: the URL must be https without userinfo and its
// host must match the code-owned provider hosts exactly (bare host
// matches any port, host:port matches exactly). Transport still
// re-enforces the same hosts through the shared downloader's SSRF guard.
func checkLibraryEgressAllowlist(rawURL string, allowHosts []string) error {
	trimmed := strings.TrimSpace(rawURL)
	parsed, err := url.Parse(trimmed)
	if err != nil || strings.TrimSpace(parsed.Hostname()) == "" {
		return errors.New("library materializer: provider URL is invalid")
	}
	if strings.ToLower(parsed.Scheme) != "https" {
		return fmt.Errorf("library materializer: provider URL scheme %q is not allowed", strings.ToLower(parsed.Scheme))
	}
	if parsed.User != nil {
		return errors.New("library materializer: provider URL is invalid")
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	port := strings.TrimSpace(parsed.Port())
	if port == "" {
		port = "443"
	}
	matched, configured := matchLibraryEgressHost(host, port, allowHosts)
	if !configured {
		return errors.New("library materializer: download allowlist is not configured")
	}
	if !matched {
		return fmt.Errorf("library materializer: provider host %q is not allowlisted", host)
	}
	return nil
}

func matchLibraryEgressHost(host, port string, allowHosts []string) (matched, configured bool) {
	for _, raw := range allowHosts {
		entryHost, entryPort := normalizeLibraryEgressEntry(raw)
		if entryHost == "" {
			continue
		}
		configured = true
		if entryHost != host {
			continue
		}
		if entryPort == "" || entryPort == port {
			return true, true
		}
	}
	return false, configured
}

// normalizeLibraryEgressEntry splits one policy entry into its
// normalized (host, port) pair, mirroring the outbound policy and
// safehttp entry shapes. Unusable entries read as ("", "") and are
// skipped.
func normalizeLibraryEgressEntry(raw string) (string, string) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "", ""
	}
	if parsed, err := url.Parse(value); err == nil && strings.TrimSpace(parsed.Hostname()) != "" {
		host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
		if host == "" {
			return "", ""
		}
		port := strings.TrimSpace(parsed.Port())
		if port == "" {
			switch strings.ToLower(parsed.Scheme) {
			case "http":
				port = "80"
			case "https":
				port = "443"
			}
		}
		return host, port
	}
	if strings.Contains(value, "/") {
		return "", ""
	}
	if entryHost, entryPort, err := net.SplitHostPort(value); err == nil {
		entryHost = strings.ToLower(strings.TrimSuffix(strings.Trim(strings.TrimSpace(entryHost), "[]"), "."))
		if entryHost == "" {
			return "", ""
		}
		return entryHost, strings.TrimSpace(entryPort)
	}
	host := strings.ToLower(strings.TrimSuffix(strings.Trim(strings.TrimSpace(value), "[]"), "."))
	if host == "" {
		return "", ""
	}
	return host, ""
}

// ingestMaterializedBytes validates the downloaded bytes and persists
// them as a new workspace-scoped media asset via the #820 session flow.
// The asset row commits before any later job-record update (byte PUT
// cannot join a PG tx); a later caller-side failure orphans the row
// harmlessly (listable, metadata-readable, same posture as unused
// uploads). Payloads never enter errors.
func (m *LibraryMaterializer) ingestMaterializedBytes(ctx context.Context, workspaceID string, ownerUserID int64, asset *domain.V1Asset, data []byte) (*workspacemedia.Asset, error) {
	if len(data) == 0 {
		return nil, errors.New("library materializer: provider result is empty")
	}
	if int64(len(data)) > workspacemedia.MaxUploadSizeBytes {
		return nil, fmt.Errorf("library materializer: provider result exceeds size limit")
	}
	facts, err := imageformat.InspectDetectedBytes(data)
	if err != nil {
		return nil, fmt.Errorf("library materializer: provider result is not a supported image: %w", err)
	}
	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])
	filename := fmt.Sprintf("library-materialized%s", facts.Extension)
	session, err := m.media.CreateSession(ctx, workspacemedia.CreateSessionParams{
		WorkspaceID: workspaceID, OwnerUserID: ownerUserID, Filename: filename,
		MIMEType: facts.MIMEType, SizeBytes: int64(len(data)),
		ChecksumSHA256: checksum, MediaKind: workspacemedia.MediaKindImage,
	})
	if err != nil {
		return nil, fmt.Errorf("library materializer: ingest failed: %w", err)
	}
	if err := m.objects.PutObjectAtProfile(ctx, workspacemedia.DefaultStorageProfile, session.StorageKey, data, facts.MIMEType); err != nil {
		_, _ = m.media.FailSession(context.WithoutCancel(ctx), workspaceID, session.ID, "library materialization byte ingest failed")
		return nil, fmt.Errorf("library materializer: byte store failed: %w", err)
	}
	_, managed, err := m.media.CompleteSession(ctx, workspaceID, session.ID, workspacemedia.CompleteFacts{
		ChecksumSHA256: checksum, SizeBytes: int64(len(data)),
		MIMEType: facts.MIMEType, MediaKind: workspacemedia.MediaKindImage,
		Width: int32(facts.Width), Height: int32(facts.Height),
	}, workspacemedia.CreateAssetParams{})
	if err != nil {
		return nil, fmt.Errorf("library materializer: ingest failed: %w", err)
	}
	return managed, nil
}

// generateLibraryDerivativesBestEffort runs sequential display-tier
// generation after a successful ingest. Every failure mode (nil seams,
// missing bytes, render errors) is absorbed: the asset is already ready
// and unmaterialized tiers stay pending (409 at the derivative-bytes
// endpoint). A nil renderer defers generation with a logged reason —
// derivatives stay lazy by design; no derivative engine lives here.
func generateLibraryDerivativesBestEffort(ctx context.Context, media workspacemedia.SessionStore, objects workspacemedia.DerivativeObjects, renderer ports.Renderer, workspaceID string, asset *workspacemedia.Asset) {
	assetID := ""
	if asset != nil {
		assetID = asset.ID
	}
	if media == nil || objects == nil || renderer == nil {
		slog.Info("library materialization derivatives deferred: renderer not configured; tiers stay pending",
			"workspace_id", workspaceID, "asset_id", assetID)
		return
	}
	store, ok := media.(workspacemedia.AssetStore)
	if !ok {
		slog.Info("library materialization derivatives deferred: media store cannot record tiers; tiers stay pending",
			"workspace_id", workspaceID, "asset_id", assetID)
		return
	}
	if _, err := workspacemedia.EnsureDerivatives(ctx, store, objects, renderer, workspaceID, asset); err != nil {
		slog.Warn("library materialization derivatives best-effort failed; tiers stay pending",
			"workspace_id", workspaceID, "asset_id", assetID, "err", err)
	}
}

// RemoteLibrarySourceFetcher is the production LibrarySourceFetcher. The
// shared remote downloader owns the SSRF guard, timeout, and size cap;
// this adapter only turns its temporary validated image file into bytes.
type RemoteLibrarySourceFetcher struct{}

// Compile-time convergence with the fetch seam.
var _ LibrarySourceFetcher = RemoteLibrarySourceFetcher{}

func (RemoteLibrarySourceFetcher) FetchSourceBytes(ctx context.Context, rawURL string, allowHosts []string) ([]byte, error) {
	file, err := remote.DownloadImageWithLimit(ctx, strings.TrimSpace(rawURL), allowHosts, workspacemedia.MaxUploadSizeBytes)
	if err != nil {
		return nil, err
	}
	defer file.Cleanup()
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewind remote image: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(file, workspacemedia.MaxUploadSizeBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read remote image: %w", err)
	}
	if int64(len(data)) > workspacemedia.MaxUploadSizeBytes {
		return nil, fmt.Errorf("remote image exceeds size limit")
	}
	return data, nil
}
