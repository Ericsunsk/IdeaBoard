package runtime

// Workspace provider-result materializer: the production
// taskmedia.ResultMaterializer for the gateway executor finalize seam.
//
// The legacy executor materializes provider image URLs into legacy user
// storage, which workspace tasks must never use: workspace outputs belong
// to workspace-scoped media rows. This materializer ingests each provider
// image (inline data: URLs decoded without network, https: URLs through
// the allowlisted result fetcher) into a workspace media asset via the
// shared #820 session-flow ingest and returns asset-kind outputs. The
// executor finalize validation accepts asset-kind outputs (no URL
// round-trip); the worker bridge then maps them to workspace asset refs,
// the renderer re-verifies them in-workspace, and settlement persists
// them atomically.
//
// Scope: the legacy materialize signature carries no workspace identity,
// so the gateway bridge stamps it on ctx (WithWorkspaceTaskScope) before
// CallGatewayAPI; the executor passes ctx through untouched. A call
// without scope fails closed (tasks converge to failed, never strand).
//
// Credentials: this seam handles result BYTES only. Gateway credentials
// resolve per execution inside the gateway client (operator-managed AI
// gateway config) and never pass through here; without them the call
// fails before materialization runs.

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"ideaboard/api/internal/media/derivatives/ports"
	"ideaboard/api/internal/media/workspacemedia"
	tasks "ideaboard/api/internal/tasking/domain"
	taskmedia "ideaboard/api/internal/tasking/ports/media"
	"ideaboard/api/internal/tasking/workspacetasks"
)

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
// Objects, and Fetcher are required (fail fast); Derivatives is optional
// with deferred-tier semantics (nil skips generation, tiers stay pending).
type WorkspaceProviderResultMaterializerDeps struct {
	Media workspacemedia.WorkspaceMediaStore
	// Objects carries result bytes (PUT) for the session-flow ingest.
	Objects workspacemedia.DerivativeObjects
	// Fetcher downloads https: provider results through the allowlisted
	// downloader. Inline data: results never touch it.
	Fetcher WorkspaceResultBytesFetcher
	// Derivatives renders display tiers best-effort. Nil defers
	// generation (tiers stay pending) with a logged reason.
	Derivatives ports.Renderer
}

// workspaceProviderResultMaterializer is the production
// taskmedia.ResultMaterializer for workspace tasks.
type workspaceProviderResultMaterializer struct {
	media       workspacemedia.WorkspaceMediaStore
	objects     workspacemedia.DerivativeObjects
	fetcher     WorkspaceResultBytesFetcher
	derivatives ports.Renderer
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
		fetcher: deps.Fetcher, derivatives: deps.Derivatives,
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
		asset, err := persistWorkspaceTaskResultAsset(ctx, m.media, m.objects, m.derivatives, task, data, index, "")
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
	asset, err := persistWorkspaceTaskResultAsset(ctx, m.media, m.objects, m.derivatives, task, data, 0, "")
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
// undecodable" (fail closed, payload never echoed).
func decodeInlineProviderResult(raw string) (data []byte, ok bool, err error) {
	// Prefix-check without lowercasing the whole payload (provider
	// results can be megabytes of base64).
	if len(raw) < len("data:image/") || !strings.EqualFold(raw[:len("data:image/")], "data:image/") {
		return nil, false, nil
	}
	comma := strings.Index(raw, ",")
	if comma < 0 {
		return nil, true, errors.New("workspace provider inline result is malformed")
	}
	meta := strings.ToLower(raw[:comma])
	if !strings.HasSuffix(meta, ";base64") {
		return nil, true, errors.New("workspace provider inline result must be base64")
	}
	decoded, err := base64.StdEncoding.DecodeString(raw[comma+1:])
	if err != nil {
		return nil, true, fmt.Errorf("workspace provider inline result decode failed: %w", err)
	}
	if int64(len(decoded)) > workspacemedia.MaxUploadSizeBytes {
		return nil, true, fmt.Errorf("workspace provider inline result exceeds size limit")
	}
	return decoded, true, nil
}
