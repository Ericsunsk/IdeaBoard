package runtime

// Provider result materialization owns ingestion; rendering verifies assets before settlement.
// The worker owns task state, output references, and billing in its settlement transaction.

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"log/slog"
	"strings"
	"sync"
	"time"

	"ideaboard/api/internal/media/egress"
	"ideaboard/api/internal/media/fetch"
	"ideaboard/api/internal/media/mediarefs"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/safehttp"
	tasks "ideaboard/api/internal/tasking/domain"
	taskmedia "ideaboard/api/internal/tasking/ports/media"
	"ideaboard/api/internal/tasking/workspacetasks"
)

// WorkspaceTaskRenderer verifies ingested asset refs and compensates execution-owned
// assets only after a confirmed terminal failure. Unknown outcomes keep assets live.
type WorkspaceTaskRenderer interface {
	RenderOutputs(ctx context.Context, task *workspacetasks.WorkspaceTask, outputs []workspacetasks.OutputRef) ([]workspacetasks.OutputRef, error)
	CompensateRenderedOutputs(ctx context.Context, task *workspacetasks.WorkspaceTask, outputs []workspacetasks.OutputRef) error
}

// WorkspaceResultBytesFetcher downloads the bytes behind one remote output
// ref. Implementations own allowlisting, timeouts, and size caps; tests
// inject canned fixture bytes. The materializer requires a fetcher at construction.
type WorkspaceResultBytesFetcher interface {
	FetchResultBytes(ctx context.Context, task *workspacetasks.WorkspaceTask, ref workspacetasks.OutputRef) ([]byte, error)
}

// WorkspaceResultBytesFetchFunc adapts a function to WorkspaceResultBytesFetcher.
type WorkspaceResultBytesFetchFunc func(ctx context.Context, task *workspacetasks.WorkspaceTask, ref workspacetasks.OutputRef) ([]byte, error)

// FetchResultBytes implements WorkspaceResultBytesFetcher.
func (f WorkspaceResultBytesFetchFunc) FetchResultBytes(ctx context.Context, task *workspacetasks.WorkspaceTask, ref workspacetasks.OutputRef) ([]byte, error) {
	return f(ctx, task, ref)
}

// WorkspaceTaskRendererDeps wires asset verification and reference-safe cleanup.
type WorkspaceTaskRendererDeps struct {
	Media  workspacemedia.WorkspaceMediaStore
	Assets workspacetasks.AssetReader
}

type workspaceTaskRenderer struct {
	media  workspacemedia.WorkspaceMediaStore
	assets workspacetasks.AssetReader
}

var _ WorkspaceTaskRenderer = (*workspaceTaskRenderer)(nil)

func NewWorkspaceTaskRenderer(deps WorkspaceTaskRendererDeps) (WorkspaceTaskRenderer, error) {
	if deps.Media == nil {
		return nil, errors.New("workspace task renderer: media store is required")
	}
	if deps.Assets == nil {
		return nil, errors.New("workspace task renderer: asset reader is required")
	}
	return &workspaceTaskRenderer{media: deps.Media, assets: deps.Assets}, nil
}

// compensateIngestedResultAssets is the single cleanup entry for partial
// ingestion failures and confirmed terminal failures. It uses a bounded
// detached context and refuses durable references; cleanup errors keep assets
// live and never change settlement.
func compensateIngestedResultAssets(ctx context.Context, media workspacemedia.WorkspaceMediaStore, task *workspacetasks.WorkspaceTask, assetIDs []string) {
	if media == nil || task == nil || len(assetIDs) == 0 {
		return
	}
	compensationCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	for _, assetID := range assetIDs {
		assetID = strings.TrimSpace(assetID)
		if assetID == "" {
			continue
		}
		if _, err := media.ArchiveUnreferencedAsset(compensationCtx, task.WorkspaceID, assetID); err != nil {
			slog.Warn("workspace task result compensation failed; asset stays live",
				"task_id", task.ID, "asset_id", assetID, "err", err)
		}
	}
}

// RenderOutputs verifies settlement-ready assets without downloading or ingesting.
func (r *workspaceTaskRenderer) RenderOutputs(ctx context.Context, task *workspacetasks.WorkspaceTask, outputs []workspacetasks.OutputRef) ([]workspacetasks.OutputRef, error) {
	if r == nil || r.media == nil || r.assets == nil {
		return nil, errors.New(errorrender.MsgRenderOutputs)
	}
	if task == nil || strings.TrimSpace(task.ID) == "" || strings.TrimSpace(task.WorkspaceID) == "" {
		return nil, errors.New(errorrender.MsgRenderOutputs2)
	}
	if len(outputs) > workspacetasks.MaxOutputsPerTask {
		return nil, fmt.Errorf(errorrender.MsgRenderOutputs3, workspacetasks.MaxOutputsPerTask)
	}
	rendered := make([]workspacetasks.OutputRef, 0, len(outputs))
	for _, ref := range outputs {
		if err := workspacetasks.ValidateOutputRef(ref); err != nil {
			return nil, fmt.Errorf(errorrender.MsgRenderOutputs4, err)
		}
		if ref.Kind != workspacetasks.OutputKindAsset {
			return nil, errors.New("workspace task renderer requires ingested asset outputs")
		}
		verified, err := r.passthroughAsset(ctx, task, ref)
		if err != nil {
			return nil, err
		}
		rendered = append(rendered, verified)
	}
	return rendered, nil
}

// CompensateRenderedOutputs uses attempt-local ingestion receipts, never the
// presence of an asset-kind ref, as proof that this execution owns cleanup.
func (r *workspaceTaskRenderer) CompensateRenderedOutputs(ctx context.Context, task *workspacetasks.WorkspaceTask, outputs []workspacetasks.OutputRef) error {
	if r == nil || r.media == nil {
		return errors.New(errorrender.MsgRenderOutputs)
	}
	if task == nil || strings.TrimSpace(task.ID) == "" || strings.TrimSpace(task.WorkspaceID) == "" {
		return errors.New(errorrender.MsgRenderOutputs2)
	}
	if task.Status != workspacetasks.StatusFailed {
		return errors.New("result compensation requires a committed failed task")
	}
	scope, _ := ctx.Value(resultIngestionScopeKey{}).(*resultIngestionScope)
	if !scope.matches(task) {
		return errors.New("result compensation requires the matching execution scope")
	}
	assetIDs := make([]string, 0, len(outputs))
	for _, ref := range outputs {
		if ref.Kind == workspacetasks.OutputKindAsset && scope.owns(ref.AssetID) {
			assetIDs = append(assetIDs, ref.AssetID)
		}
	}
	compensateIngestedResultAssets(ctx, r.media, task, assetIDs)
	return nil
}

// passthroughAsset verifies an asset-kind ref resolves in-workspace
// (ready, unarchived) and normalizes it for settlement.
func (r *workspaceTaskRenderer) passthroughAsset(ctx context.Context, task *workspacetasks.WorkspaceTask, ref workspacetasks.OutputRef) (workspacetasks.OutputRef, error) {
	if _, err := r.assets.GetAsset(ctx, task.WorkspaceID, ref.AssetID); err != nil {
		switch {
		case errors.Is(err, workspacetasks.ErrMediaArchived):
			return workspacetasks.OutputRef{}, fmt.Errorf(errorrender.MsgPassthroughAsset, err)
		default:
			return workspacetasks.OutputRef{}, fmt.Errorf(errorrender.MsgPassthroughAsset2, workspacetasks.ErrMediaNotFound)
		}
	}
	verified := workspacetasks.OutputRef{
		Kind: workspacetasks.OutputKindAsset, AssetID: strings.TrimSpace(ref.AssetID),
		Role: strings.TrimSpace(ref.Role), ResultIndex: ref.ResultIndex,
		Width: ref.Width, Height: ref.Height, MIMEType: ref.MIMEType,
	}
	if err := workspacetasks.ValidateOutputRef(verified); err != nil {
		return workspacetasks.OutputRef{}, fmt.Errorf(errorrender.MsgPassthroughAsset3, err)
	}
	return verified, nil
}

// persistWorkspaceTaskResultAsset applies Task provenance to managed image ingestion.
func persistWorkspaceTaskResultAsset(ctx context.Context, media workspacemedia.WorkspaceMediaStore, objects workspacemedia.MediaObjects, task *workspacetasks.WorkspaceTask, data []byte, index int) (*workspacemedia.Asset, error) {
	if media == nil || objects == nil {
		return nil, errors.New(errorrender.MsgIngestRemote)
	}
	if task == nil || strings.TrimSpace(task.ID) == "" || strings.TrimSpace(task.WorkspaceID) == "" {
		return nil, errors.New(errorrender.MsgRenderOutputs2)
	}
	scope, _ := ctx.Value(resultIngestionScopeKey{}).(*resultIngestionScope)
	if !scope.matches(task) {
		return nil, errors.New("result ingestion requires the matching execution scope")
	}
	asset, err := workspacemedia.IngestImage(ctx, media, objects, workspacemedia.IngestImageParams{
		WorkspaceID: task.WorkspaceID, OwnerUserID: task.OwnerUserID,
		FilenameStem: fmt.Sprintf("task-result-%d", index), Data: data,
	})
	if err != nil {
		return nil, fmt.Errorf(errorrender.MsgPersistWorkspaceTaskResultAsset, err)
	}
	scope.record(asset.ID)
	slog.Info("workspace task result ingested as media asset",
		"task_id", task.ID, "asset_id", asset.ID, "mime", asset.MIMEType,
		"width", asset.Width, "height", asset.Height, "bytes", asset.SizeBytes)
	return asset, nil
}

// resultIngestionScope snapshots one claimed execution's immutable identity and
// records only assets it ingests. Fallback sends reuse this scope.
type resultIngestionScopeKey struct{}

type resultIngestionScope struct {
	workspaceID    string
	taskID         string
	ownerUserID    int64
	executionToken string
	mu             sync.Mutex
	assets         map[string]struct{}
}

// WithResultIngestionScope starts the ownership scope after a successful claim.
func WithResultIngestionScope(ctx context.Context, claimed *workspacetasks.WorkspaceTask) context.Context {
	scope := &resultIngestionScope{assets: make(map[string]struct{})}
	if claimed != nil {
		scope.workspaceID = strings.TrimSpace(claimed.WorkspaceID)
		scope.taskID = strings.TrimSpace(claimed.ID)
		scope.ownerUserID = claimed.OwnerUserID
		scope.executionToken = strings.TrimSpace(claimed.ExecutionToken)
	}
	return context.WithValue(ctx, resultIngestionScopeKey{}, scope)
}

func (s *resultIngestionScope) matches(task *workspacetasks.WorkspaceTask) bool {
	return s != nil && task != nil && s.workspaceID != "" && s.taskID != "" && s.ownerUserID > 0 && s.executionToken != "" &&
		s.workspaceID == strings.TrimSpace(task.WorkspaceID) && s.taskID == strings.TrimSpace(task.ID) &&
		s.ownerUserID == task.OwnerUserID && s.executionToken == strings.TrimSpace(task.ExecutionToken)
}

func (s *resultIngestionScope) record(assetID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.assets[strings.TrimSpace(assetID)] = struct{}{}
}

func (s *resultIngestionScope) owns(assetID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, owned := s.assets[strings.TrimSpace(assetID)]
	return owned
}

// WorkspaceProviderResultRole is the output role for live-ingested provider
// results (matches the fake-gateway fixtures the renderer tests use).
const WorkspaceProviderResultRole = "result"

// WorkspaceProviderResultMaterializerDeps wires the materializer. Media,
// Objects, and Fetcher are required (fail fast).
type WorkspaceProviderResultMaterializerDeps struct {
	Media workspacemedia.WorkspaceMediaStore
	// Objects carries result bytes (PUT) for the session-flow ingest.
	Objects workspacemedia.MediaObjects
	// Fetcher downloads https: provider results through the allowlisted
	// downloader. Inline data: results never touch it.
	Fetcher WorkspaceResultBytesFetcher
}

// workspaceProviderResultMaterializer is the production
// taskmedia.ResultMaterializer for workspace tasks.
type workspaceProviderResultMaterializer struct {
	media   workspacemedia.WorkspaceMediaStore
	objects workspacemedia.MediaObjects
	fetcher WorkspaceResultBytesFetcher
}

var _ taskmedia.ResultMaterializer = (*workspaceProviderResultMaterializer)(nil)

// NewWorkspaceProviderResultMaterializer builds the materializer. Nil
// media/objects/fetcher fail fast: a materializer that cannot ingest must
// never be wired into the executor.
func NewWorkspaceProviderResultMaterializer(deps WorkspaceProviderResultMaterializerDeps) (taskmedia.ResultMaterializer, error) {
	if deps.Media == nil {
		return nil, errors.New("workspace provider result materializer: media store is required")
	}
	if deps.Objects == nil {
		return nil, errors.New("workspace provider result materializer: object storage is required")
	}
	if deps.Fetcher == nil {
		return nil, errors.New("workspace provider result materializer: result fetcher is required")
	}
	return &workspaceProviderResultMaterializer{
		media: deps.Media, objects: deps.Objects,
		fetcher: deps.Fetcher,
	}, nil
}

// MaterializeRemoteResults ingests every provider image URL into a
// workspace media asset and returns asset-kind outputs (one per source,
// dense result indexes). Any failure is terminal for the attempt (the
// worker converges it deterministically, no retry-budget consumption).
//
// A later-item failure archives the assets this call already ingested
// (partial-success orphan compensation) before returning the error, so a
// failed attempt never leaves ingested-but-unreferenced assets behind.
func (m *workspaceProviderResultMaterializer) MaterializeRemoteResults(ctx context.Context, userID int64, taskID string, sourceURLs []string, opts taskmedia.ResultMaterializeOptions) (taskmedia.MaterializedResults, error) {
	if m == nil || m.media == nil || m.objects == nil || m.fetcher == nil {
		return taskmedia.MaterializedResults{}, errors.New("workspace provider result materializer is not configured")
	}
	scope, _ := ctx.Value(resultIngestionScopeKey{}).(*resultIngestionScope)
	if scope == nil {
		return taskmedia.MaterializedResults{}, errors.New("workspace provider result materializer: execution scope is missing")
	}
	task := &workspacetasks.WorkspaceTask{
		ID: strings.TrimSpace(taskID), WorkspaceID: scope.workspaceID,
		OwnerUserID: userID, ExecutionToken: scope.executionToken,
	}
	if !scope.matches(task) {
		return taskmedia.MaterializedResults{}, errors.New("workspace provider result materializer: execution identity mismatch")
	}
	if len(sourceURLs) == 0 {
		return taskmedia.MaterializedResults{}, errors.New("workspace provider result materializer: no provider results")
	}
	items := make([]taskmedia.MaterializedResult, 0, len(sourceURLs))
	ingested := make([]string, 0, len(sourceURLs))
	for index, raw := range sourceURLs {
		data, err := m.sourceBytes(ctx, task, raw, opts.AllowHosts)
		if err != nil {
			compensateIngestedResultAssets(ctx, m.media, task, ingested)
			return taskmedia.MaterializedResults{}, err
		}
		asset, err := persistWorkspaceTaskResultAsset(ctx, m.media, m.objects, task, data, index)
		if err != nil {
			compensateIngestedResultAssets(ctx, m.media, task, ingested)
			return taskmedia.MaterializedResults{}, err
		}
		// Asset-kind output: the finalize validation accepts it without
		// a URL round-trip, and the bridge maps it to a workspace asset
		// ref (ImageURL carries the asset id for the dropped result
		// envelope only; nothing persists it).
		ingested = append(ingested, asset.ID)
		items = append(items, taskmedia.MaterializedResult{
			ImageURL: asset.ID,
			Output: tasks.TaskOutput{
				TaskID: task.ID, AssetID: asset.ID,
				Role: WorkspaceProviderResultRole, ResultIndex: index,
			},
		})
	}
	return taskmedia.MaterializedResults{Items: items}, nil
}

// sourceBytes resolves one provider result URL to bytes. Inline data:
// URLs decode without network (the common generation path); anything else
// downloads through the allowlisted fetcher. Payloads never enter errors.
func (m *workspaceProviderResultMaterializer) sourceBytes(ctx context.Context, task *workspacetasks.WorkspaceTask, raw string, allowHosts []string) ([]byte, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, errors.New("workspace provider result is empty")
	}
	if data, ok, err := decodeInlineProviderResult(trimmed); ok || err != nil {
		if err != nil {
			return nil, err
		}
		return data, nil
	}
	data, err := m.fetcher.FetchResultBytes(WithWorkspaceResultAllowHosts(ctx, allowHosts), task, workspacetasks.OutputRef{
		Kind: workspacetasks.OutputKindRemote, URL: trimmed,
	})
	if err != nil {
		return nil, fmt.Errorf("workspace provider result download failed: %w", err)
	}
	return data, nil
}

// decodeInlineProviderResult decodes an inline data: image URL. ok=false
// means "not an inline URL, download it"; err non-nil means "inline but
// undecodable" (fail closed, payload never echoed). Decode logic is owned
// by mediarefs.ParseDataImageURL (the single data-URL seam shared with
// the storage upload path); this stays a thin shape adapter so provider
// results never fork data-URL parsing.
func decodeInlineProviderResult(raw string) (data []byte, ok bool, err error) {
	// Prefix-check without lowercasing the whole payload (provider
	// results can be megabytes of base64).
	if len(raw) < len("data:image/") || !strings.EqualFold(raw[:len("data:image/")], "data:image/") {
		return nil, false, nil
	}
	parsed, perr := mediarefs.ParseDataImageURL(raw, int(workspacemedia.MaxUploadSizeBytes))
	if perr != nil {
		return nil, true, fmt.Errorf("workspace provider inline result: %w", perr)
	}
	return parsed.Data, true, nil
}

// --- (from media_seams.go) ---

// WorkspaceTaskResultBytesFetcher is the production remote-result seam. The
// downloader owns SSRF protection, outbound timeout and response size caps;
// this adapter only turns its temporary validated image file into bytes.
type WorkspaceTaskResultBytesFetcher struct{}

var _ WorkspaceResultBytesFetcher = WorkspaceTaskResultBytesFetcher{}

type workspaceResultAllowHostsKey struct{}

func WithWorkspaceResultAllowHosts(ctx context.Context, hosts []string) context.Context {
	return context.WithValue(ctx, workspaceResultAllowHostsKey{}, append([]string(nil), hosts...))
}

func WorkspaceResultAllowHostsFromCtx(ctx context.Context) []string {
	if ctx == nil {
		return nil
	}
	hosts, _ := ctx.Value(workspaceResultAllowHostsKey{}).([]string)
	return append([]string(nil), hosts...)
}

func (WorkspaceTaskResultBytesFetcher) FetchResultBytes(ctx context.Context, _ *workspacetasks.WorkspaceTask, ref workspacetasks.OutputRef) ([]byte, error) {
	allowHosts := WorkspaceResultAllowHostsFromCtx(ctx)
	if len(allowHosts) == 0 {
		allowHosts = safehttp.AllowHosts(ctx)
	}
	// Closed-world gate BEFORE any network: the transport SSRF guard
	// below still applies, but on its own it permits any public host.
	// Provider result egress stays pinned to the executor-supplied
	// allowlist (exact host, or exact host:port); anything else fails
	// closed here and is never fetched. Gate owned by media/egress,
	// bytes owned by media/fetch.
	rawURL := strings.TrimSpace(ref.URL)
	if err := egress.CheckHostAllowlist(rawURL, allowHosts); err != nil {
		return nil, err
	}
	return fetch.FetchBytes(ctx, rawURL, allowHosts, workspacemedia.MaxUploadSizeBytes)
}
