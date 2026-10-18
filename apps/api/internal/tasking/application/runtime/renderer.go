package runtime

// Task result rendering completes media ingestion before terminal settlement.
// The worker owns task state, output references, and billing in its settlement transaction.

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"log/slog"
	"strings"

	"ideaboard/api/internal/media/egress"
	"ideaboard/api/internal/media/fetch"
	"ideaboard/api/internal/media/mediarefs"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/safehttp"
	tasks "ideaboard/api/internal/tasking/domain"
	taskmedia "ideaboard/api/internal/tasking/ports/media"
	"ideaboard/api/internal/tasking/workspacetasks"
)

// WorkspaceTaskRenderer materializes provider output refs into asset-kind
// refs before terminal settlement. The worker calls it between Generate
// and Complete.
type WorkspaceTaskRenderer interface {
	RenderOutputs(ctx context.Context, task *workspacetasks.WorkspaceTask, outputs []workspacetasks.OutputRef) ([]workspacetasks.OutputRef, error)
}

// WorkspaceResultBytesFetcher downloads the bytes behind one remote output
// ref. Implementations own allowlisting, timeouts, and size caps; tests
// inject canned fixture bytes. A nil fetcher fails every remote ref
// closed with an explicit message (fail-loud, never stranded).
type WorkspaceResultBytesFetcher interface {
	FetchResultBytes(ctx context.Context, task *workspacetasks.WorkspaceTask, ref workspacetasks.OutputRef) ([]byte, error)
}

// WorkspaceResultBytesFetchFunc adapts a function to WorkspaceResultBytesFetcher.
type WorkspaceResultBytesFetchFunc func(ctx context.Context, task *workspacetasks.WorkspaceTask, ref workspacetasks.OutputRef) ([]byte, error)

// FetchResultBytes implements WorkspaceResultBytesFetcher.
func (f WorkspaceResultBytesFetchFunc) FetchResultBytes(ctx context.Context, task *workspacetasks.WorkspaceTask, ref workspacetasks.OutputRef) ([]byte, error) {
	return f(ctx, task, ref)
}

// WorkspaceTaskRendererDeps wires the renderer. Media and Assets are
// required (fail fast); Objects and Fetcher are optional with
// fail-loud semantics documented above.
type WorkspaceTaskRendererDeps struct {
	Media  workspacemedia.WorkspaceMediaStore
	Assets workspacetasks.AssetReader
	// Objects carries result bytes (PUT) for the session-flow ingest.
	// Nil fails every remote ingest closed at render time.
	Objects workspacemedia.MediaObjects
	// Fetcher downloads remote output bytes. Nil fails every remote
	// ingest closed at render time.
	Fetcher WorkspaceResultBytesFetcher
}

// workspaceTaskRenderer is the production WorkspaceTaskRenderer.
type workspaceTaskRenderer struct {
	media   workspacemedia.WorkspaceMediaStore
	assets  workspacetasks.AssetReader
	objects workspacemedia.MediaObjects
	fetcher WorkspaceResultBytesFetcher
}

var _ WorkspaceTaskRenderer = (*workspaceTaskRenderer)(nil)

// NewWorkspaceTaskRenderer builds the result renderer. Nil media/assets
// fail fast: a renderer that cannot verify or persist must never claim to
// render.
func NewWorkspaceTaskRenderer(deps WorkspaceTaskRendererDeps) (WorkspaceTaskRenderer, error) {
	if deps.Media == nil {
		return nil, errors.New("workspace task renderer: media store is required")
	}
	if deps.Assets == nil {
		return nil, errors.New("workspace task renderer: asset reader is required")
	}
	return &workspaceTaskRenderer{
		media: deps.Media, assets: deps.Assets,
		objects: deps.Objects, fetcher: deps.Fetcher,
	}, nil
}

// RenderOutputs transforms provider output refs into settlement-ready
// asset-kind refs. Asset refs are verified in-workspace; remote refs are
// ingested into new workspace media assets (validated via imageformat).
// The returned refs satisfy ValidateOutputRef and resolve to existing
// asset rows, so the atomic Complete path persists them without further
// checks.
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
	for index, ref := range outputs {
		if err := workspacetasks.ValidateOutputRef(ref); err != nil {
			return nil, fmt.Errorf(errorrender.MsgRenderOutputs4, err)
		}
		switch ref.Kind {
		case workspacetasks.OutputKindAsset:
			verified, err := r.passthroughAsset(ctx, task, ref)
			if err != nil {
				return nil, err
			}
			rendered = append(rendered, verified)
		case workspacetasks.OutputKindRemote:
			ingested, err := r.ingestRemote(ctx, task, ref, index)
			if err != nil {
				return nil, err
			}
			rendered = append(rendered, ingested)
		default:
			return nil, fmt.Errorf(errorrender.MsgRenderOutputs5, ref.Kind)
		}
	}
	return rendered, nil
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
	}
	if err := workspacetasks.ValidateOutputRef(verified); err != nil {
		return workspacetasks.OutputRef{}, fmt.Errorf(errorrender.MsgPassthroughAsset3, err)
	}
	return verified, nil
}

// ingestRemote fetches one remote output's bytes and persists them as a
// new workspace-scoped media asset via the shared session-flow ingest,
// returning the settlement-ready asset-kind ref.
func (r *workspaceTaskRenderer) ingestRemote(ctx context.Context, task *workspacetasks.WorkspaceTask, ref workspacetasks.OutputRef, index int) (workspacetasks.OutputRef, error) {
	if r.fetcher == nil {
		return workspacetasks.OutputRef{}, errors.New(errorrender.MsgIngestRemote)
	}
	if r.objects == nil {
		return workspacetasks.OutputRef{}, errors.New(errorrender.MsgIngestRemote2)
	}
	data, err := r.fetcher.FetchResultBytes(ctx, task, ref)
	if err != nil {
		return workspacetasks.OutputRef{}, fmt.Errorf(errorrender.MsgIngestRemote3, err)
	}
	asset, err := persistWorkspaceTaskResultAsset(ctx, r.media, r.objects, task, data, index, strings.TrimSpace(ref.ChecksumSHA256))
	if err != nil {
		return workspacetasks.OutputRef{}, err
	}
	rendered := workspacetasks.OutputRef{
		Kind: workspacetasks.OutputKindAsset, AssetID: asset.ID,
		Role: strings.TrimSpace(ref.Role), ResultIndex: ref.ResultIndex,
	}
	if err := workspacetasks.ValidateOutputRef(rendered); err != nil {
		return workspacetasks.OutputRef{}, fmt.Errorf(errorrender.MsgPassthroughAsset3, err)
	}
	return rendered, nil
}

// persistWorkspaceTaskResultAsset applies Task provenance to managed image ingestion.
func persistWorkspaceTaskResultAsset(ctx context.Context, media workspacemedia.WorkspaceMediaStore, objects workspacemedia.MediaObjects, task *workspacetasks.WorkspaceTask, data []byte, index int, expectChecksum string) (*workspacemedia.Asset, error) {
	if media == nil || objects == nil {
		return nil, errors.New(errorrender.MsgIngestRemote)
	}
	if task == nil || strings.TrimSpace(task.ID) == "" || strings.TrimSpace(task.WorkspaceID) == "" {
		return nil, errors.New(errorrender.MsgRenderOutputs2)
	}
	asset, err := workspacemedia.IngestImage(ctx, media, objects, workspacemedia.IngestImageParams{
		WorkspaceID: task.WorkspaceID, OwnerUserID: task.OwnerUserID,
		FilenameStem: fmt.Sprintf("task-result-%d", index), Data: data,
		ExpectedChecksumSHA256: expectChecksum,
	})
	if err != nil {
		return nil, fmt.Errorf(errorrender.MsgPersistWorkspaceTaskResultAsset, err)
	}
	slog.Info("workspace task result ingested as media asset",
		"task_id", task.ID, "asset_id", asset.ID, "mime", asset.MIMEType,
		"width", asset.Width, "height", asset.Height, "bytes", asset.SizeBytes)
	return asset, nil
}

// --- (from provider_materializer.go) ---

// workspaceTaskScopeKey carries the workspace identity for one provider
// result materialization. Unexported: only the gateway bridge sets it and
// only this materializer reads it (same package, executor passes ctx
// through).
type workspaceTaskScopeKey struct{}

// WithWorkspaceTaskScope stamps the workspace identity for the materializer.
func WithWorkspaceTaskScope(ctx context.Context, workspaceID string) context.Context {
	return context.WithValue(ctx, workspaceTaskScopeKey{}, strings.TrimSpace(workspaceID))
}

// workspaceTaskScopeFromCtx reads the stamped workspace identity.
func workspaceTaskScopeFromCtx(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	workspaceID, _ := ctx.Value(workspaceTaskScopeKey{}).(string)
	return strings.TrimSpace(workspaceID)
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
func (m *workspaceProviderResultMaterializer) MaterializeRemoteResults(ctx context.Context, userID int64, taskID string, sourceURLs []string, opts taskmedia.ResultMaterializeOptions) (taskmedia.MaterializedResults, error) {
	if m == nil || m.media == nil || m.objects == nil || m.fetcher == nil {
		return taskmedia.MaterializedResults{}, errors.New("workspace provider result materializer is not configured")
	}
	workspaceID := workspaceTaskScopeFromCtx(ctx)
	if workspaceID == "" {
		return taskmedia.MaterializedResults{}, errors.New("workspace provider result materializer: workspace scope is missing")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return taskmedia.MaterializedResults{}, errors.New("workspace provider result materializer: task identity is required")
	}
	if len(sourceURLs) == 0 {
		return taskmedia.MaterializedResults{}, errors.New("workspace provider result materializer: no provider results")
	}
	task := &workspacetasks.WorkspaceTask{ID: taskID, WorkspaceID: workspaceID, OwnerUserID: userID}
	items := make([]taskmedia.MaterializedResult, 0, len(sourceURLs))
	for index, raw := range sourceURLs {
		data, err := m.sourceBytes(ctx, task, raw, opts.AllowHosts)
		if err != nil {
			return taskmedia.MaterializedResults{}, err
		}
		asset, err := persistWorkspaceTaskResultAsset(ctx, m.media, m.objects, task, data, index, "")
		if err != nil {
			return taskmedia.MaterializedResults{}, err
		}
		// Asset-kind output: the finalize validation accepts it without
		// a URL round-trip, and the bridge maps it to a workspace asset
		// ref (ImageURL carries the asset id for the dropped result
		// envelope only; nothing persists it).
		items = append(items, taskmedia.MaterializedResult{
			ImageURL: asset.ID,
			Output: tasks.TaskOutput{
				TaskID: taskID, AssetID: asset.ID,
				Role: WorkspaceProviderResultRole, ResultIndex: index,
			},
		})
	}
	return taskmedia.MaterializedResults{Items: items}, nil
}

// MaterializeBytesResult ingests caller-provided bytes directly (no
// source download). The executor never calls it today (all provider
// results arrive as URLs through MaterializeRemoteResults); it exists so
// the interface is fully workspace-scoped. The filename is informational
// only: slot naming stays deterministic inside the shared ingest.
func (m *workspaceProviderResultMaterializer) MaterializeBytesResult(ctx context.Context, userID int64, taskID, filename, sourceURL string, data []byte, _ taskmedia.ResultMaterializeOptions) (taskmedia.MaterializedResults, error) {
	if m == nil || m.media == nil || m.objects == nil {
		return taskmedia.MaterializedResults{}, errors.New("workspace provider result materializer is not configured")
	}
	workspaceID := workspaceTaskScopeFromCtx(ctx)
	if workspaceID == "" {
		return taskmedia.MaterializedResults{}, errors.New("workspace provider result materializer: workspace scope is missing")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return taskmedia.MaterializedResults{}, errors.New("workspace provider result materializer: task identity is required")
	}
	task := &workspacetasks.WorkspaceTask{ID: taskID, WorkspaceID: workspaceID, OwnerUserID: userID}
	asset, err := persistWorkspaceTaskResultAsset(ctx, m.media, m.objects, task, data, 0, "")
	if err != nil {
		return taskmedia.MaterializedResults{}, err
	}
	return taskmedia.MaterializedResults{Items: []taskmedia.MaterializedResult{{
		ImageURL: asset.ID,
		Output: tasks.TaskOutput{
			TaskID: taskID, AssetID: asset.ID,
			Role: WorkspaceProviderResultRole, ResultIndex: 0,
		},
	}}}, nil
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
