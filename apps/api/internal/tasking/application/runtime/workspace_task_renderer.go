package runtime

// Workspace task result renderer (#827, Phase 4b-03 of #809).
//
// Task completes -> rendered output becomes a workspace-scoped media asset
// row; task_outputs refs persist through the atomic Complete path (#826:
// state flip + outputs + Capture in one tx). The renderer runs BETWEEN
// provider Generate and settler Complete: it transforms provider OutputRefs
// into asset-kind refs and NEVER settles itself (no Complete/AddOutputs
// calls, no billing, no queue). Persistence still flows through
// WorkspaceTaskWorkerSettler.Complete, so the single-tx boundary is intact.
//
// Input contract:
//   - asset-kind refs pass through after an in-workspace readiness check
//     via the TaskAssetAdapter (unknown/archived/cross-workspace ids fail
//     deterministically, same anti-enumeration posture as input refs).
//   - remote-kind refs (URL + checksum) are ingested: fetch bytes ->
//     validate kind/MIME/dims via imageformat -> CreateSession -> PUT bytes
//     to the reserved slot via the existing storage seam -> CompleteSession
//     (fact echo) -> asset-kind ref. Every failure is fail-loud and
//     deterministic (worker failTerminal, no retry-budget consumption).
//
// Byte seams (no new queues, no new infra):
//   - Media: workspacemedia.WorkspaceMediaStore (postgres Service in
//     production, in-memory sibling in tests). Assets are created ONLY
//     through the #820 session flow (CreateSession/CompleteSession), never
//     by direct row inserts.
//   - Objects: workspacemedia.DerivativeObjects (Read + PUT). *storage.Service
//     satisfies it in production; tests inject map-backed fakes.
//   - Fetcher: WorkspaceResultBytesFetcher downloads remote output bytes.
//     Production wires the allowlisted downloader
//     (workspaceTaskResultBytesFetcher: SSRF protection, timeout and size
//     caps); a nil fetcher fails every remote ingest closed
//     ("result ingestion unavailable"). Gateway credentials themselves
//     arrive only via the operator-managed AI gateway config seam and are
//     never added here; without them the worker still fails
//     deterministically before any download.
//   - Derivatives: derivativeports.Renderer, run SEQUENTIALLY via #820
//     EnsureDerivatives BEST-EFFORT (failures warn-logged, tiers stay
//     pending, the task still completes). Nil skips generation with a
//     logged reason, mirroring the upload completion path
//     (contract_v1_media.go generateMediaDerivativesBestEffort).
//
// Transactional note: the asset row commits BEFORE the settlement tx (byte
// PUT cannot join a PG tx). If the later Complete fails, the asset row
// orphans harmlessly (listable, metadata-readable, same as an unused
// upload); no task_outputs row ever points at a missing asset because refs
// are built only from completed assets.
//
// Live dispatch note: v1 submit mints the durable execution plan
// (canonical params via CompileWorkspaceParams plus the submit-time
// resolved model), the gateway bridge consumes it through CallGatewayAPI,
// and live provider bytes land here through the workspace provider-result
// materializer (executor finalize seam) as asset-kind refs. The success
// path below stays proven with the fake/stub gateway seam for remote
// refs: request shaping + result mapping + this renderer.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"ideaboard/api/internal/media/derivatives/ports"
	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/tasking/workspacetasks"
)

// WorkspaceTaskRenderer materializes provider output refs into asset-kind
// refs before terminal settlement. The worker calls it between Generate
// and Complete; nil (unwired) keeps the previous passthrough behavior.
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
// required (fail fast); Objects, Fetcher, and Derivatives are optional
// with fail-loud/deferred semantics documented above.
type WorkspaceTaskRendererDeps struct {
	Media  workspacemedia.WorkspaceMediaStore
	Assets workspacetasks.AssetReader
	// Objects carries result bytes (PUT) and serves derivative reads.
	// Nil fails every remote ingest closed at render time.
	Objects workspacemedia.DerivativeObjects
	// Fetcher downloads remote output bytes. Nil fails every remote
	// ingest closed at render time.
	Fetcher WorkspaceResultBytesFetcher
	// Derivatives renders display tiers best-effort. Nil defers
	// generation (tiers stay pending) with a logged reason.
	Derivatives ports.Renderer
}

// workspaceTaskRenderer is the production WorkspaceTaskRenderer.
type workspaceTaskRenderer struct {
	media       workspacemedia.WorkspaceMediaStore
	assets      workspacetasks.AssetReader
	objects     workspacemedia.DerivativeObjects
	fetcher     WorkspaceResultBytesFetcher
	derivatives ports.Renderer
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
		objects: deps.Objects, fetcher: deps.Fetcher, derivatives: deps.Derivatives,
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
		return nil, errors.New(msgRenderOutputs)
	}
	if task == nil || strings.TrimSpace(task.ID) == "" || strings.TrimSpace(task.WorkspaceID) == "" {
		return nil, errors.New(msgRenderOutputs2)
	}
	if len(outputs) > workspacetasks.MaxOutputsPerTask {
		return nil, fmt.Errorf(msgRenderOutputs3, workspacetasks.MaxOutputsPerTask)
	}
	rendered := make([]workspacetasks.OutputRef, 0, len(outputs))
	for index, ref := range outputs {
		if err := workspacetasks.ValidateOutputRef(ref); err != nil {
			return nil, fmt.Errorf(msgRenderOutputs4, err)
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
			return nil, fmt.Errorf(msgRenderOutputs5, ref.Kind)
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
			return workspacetasks.OutputRef{}, fmt.Errorf(msgPassthroughAsset, err)
		default:
			return workspacetasks.OutputRef{}, fmt.Errorf(msgPassthroughAsset2, workspacetasks.ErrMediaNotFound)
		}
	}
	verified := workspacetasks.OutputRef{
		Kind: workspacetasks.OutputKindAsset, AssetID: strings.TrimSpace(ref.AssetID),
		Role: strings.TrimSpace(ref.Role), ResultIndex: ref.ResultIndex,
	}
	if err := workspacetasks.ValidateOutputRef(verified); err != nil {
		return workspacetasks.OutputRef{}, fmt.Errorf(msgPassthroughAsset3, err)
	}
	return verified, nil
}

// ingestRemote fetches one remote output's bytes and persists them as a
// new workspace-scoped media asset via the shared session-flow ingest,
// returning the settlement-ready asset-kind ref.
func (r *workspaceTaskRenderer) ingestRemote(ctx context.Context, task *workspacetasks.WorkspaceTask, ref workspacetasks.OutputRef, index int) (workspacetasks.OutputRef, error) {
	if r.fetcher == nil {
		return workspacetasks.OutputRef{}, errors.New(msgIngestRemote)
	}
	if r.objects == nil {
		return workspacetasks.OutputRef{}, errors.New(msgIngestRemote2)
	}
	data, err := r.fetcher.FetchResultBytes(ctx, task, ref)
	if err != nil {
		return workspacetasks.OutputRef{}, fmt.Errorf(msgIngestRemote3, err)
	}
	asset, err := persistWorkspaceTaskResultAsset(ctx, r.media, r.objects, r.derivatives, task, data, index, strings.TrimSpace(ref.ChecksumSHA256))
	if err != nil {
		return workspacetasks.OutputRef{}, err
	}
	rendered := workspacetasks.OutputRef{
		Kind: workspacetasks.OutputKindAsset, AssetID: asset.ID,
		Role: strings.TrimSpace(ref.Role), ResultIndex: ref.ResultIndex,
	}
	if err := workspacetasks.ValidateOutputRef(rendered); err != nil {
		return workspacetasks.OutputRef{}, fmt.Errorf(msgPassthroughAsset3, err)
	}
	return rendered, nil
}

// persistWorkspaceTaskResultAsset ingests provider result bytes into a
// new workspace-scoped media asset via the #820 session flow and triggers
// best-effort derivative tiers. It is shared by the renderer (remote refs
// with a declared checksum) and the workspace provider-result materializer
// (live gateway outputs without one), so both ingest paths produce
// identical asset facts. expectChecksum empty skips the comparison.
//
// Transactional note (unchanged): the asset row commits BEFORE the
// settlement tx (byte PUT cannot join a PG tx). If the later Complete
// fails, the asset row orphans harmlessly (listable, metadata-readable,
// same as an unused upload); no task_outputs row ever points at a missing
// asset because refs are built only from completed assets.
func persistWorkspaceTaskResultAsset(ctx context.Context, media workspacemedia.WorkspaceMediaStore, objects workspacemedia.DerivativeObjects, derivatives ports.Renderer, task *workspacetasks.WorkspaceTask, data []byte, index int, expectChecksum string) (*workspacemedia.Asset, error) {
	if media == nil || objects == nil {
		return nil, errors.New(msgIngestRemote)
	}
	if task == nil || strings.TrimSpace(task.ID) == "" || strings.TrimSpace(task.WorkspaceID) == "" {
		return nil, errors.New(msgRenderOutputs2)
	}
	if len(data) == 0 {
		return nil, errors.New(msgPersistWorkspaceTaskResultAsset)
	}
	if int64(len(data)) > workspacemedia.MaxUploadSizeBytes {
		return nil, fmt.Errorf(msgPersistWorkspaceTaskResultAsset2, len(data))
	}
	facts, err := imageformat.InspectDetectedBytes(data)
	if err != nil {
		return nil, fmt.Errorf(msgPersistWorkspaceTaskResultAsset3, err)
	}
	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])
	if strings.TrimSpace(expectChecksum) != "" && !strings.EqualFold(checksum, strings.TrimSpace(expectChecksum)) {
		return nil, errors.New(msgPersistWorkspaceTaskResultAsset4)
	}
	workspaceID := strings.TrimSpace(task.WorkspaceID)
	filename := fmt.Sprintf("task-result-%d%s", index, facts.Extension)
	session, err := media.CreateSession(ctx, workspacemedia.CreateSessionParams{
		WorkspaceID: workspaceID, OwnerUserID: task.OwnerUserID, Filename: filename,
		MIMEType: facts.MIMEType, SizeBytes: int64(len(data)),
		ChecksumSHA256: checksum, MediaKind: workspacemedia.MediaKindImage,
	})
	if err != nil {
		return nil, fmt.Errorf(msgPersistWorkspaceTaskResultAsset5, err)
	}
	if err := objects.PutObjectAtProfile(ctx, workspacemedia.DefaultStorageProfile, session.StorageKey, data, facts.MIMEType); err != nil {
		_, _ = media.FailSession(ctx, workspaceID, session.ID, "result byte ingest failed")
		return nil, fmt.Errorf(msgPersistWorkspaceTaskResultAsset6, err)
	}
	_, asset, err := media.CompleteSession(ctx, workspaceID, session.ID, workspacemedia.CompleteFacts{
		ChecksumSHA256: checksum, SizeBytes: int64(len(data)),
		MIMEType: facts.MIMEType, MediaKind: workspacemedia.MediaKindImage,
		Width: int32(facts.Width), Height: int32(facts.Height),
	}, workspacemedia.CreateAssetParams{})
	if err != nil {
		return nil, fmt.Errorf(msgPersistWorkspaceTaskResultAsset5, err)
	}
	generateWorkspaceTaskDerivativesBestEffort(ctx, media, objects, derivatives, workspaceID, asset)
	slog.Info("workspace task result ingested as media asset",
		"task_id", task.ID, "asset_id", asset.ID, "mime", facts.MIMEType,
		"width", facts.Width, "height", facts.Height, "bytes", len(data))
	return asset, nil
}

// generateWorkspaceTaskDerivativesBestEffort runs sequential #820
// derivative generation after a successful ingest. Every failure mode (nil
// seams, missing bytes, render errors) is absorbed: the asset is already
// ready and unmaterialized tiers stay pending (409 at the derivative-bytes
// endpoint). A nil renderer defers generation with a logged reason.
func generateWorkspaceTaskDerivativesBestEffort(ctx context.Context, media workspacemedia.WorkspaceMediaStore, objects workspacemedia.DerivativeObjects, derivatives ports.Renderer, workspaceID string, asset *workspacemedia.Asset) {
	if media == nil || objects == nil || derivatives == nil {
		slog.Info("workspace task derivatives deferred: renderer not configured; tiers stay pending",
			"workspace_id", workspaceID, "asset_id", assetIDForLog(asset))
		return
	}
	if _, err := workspacemedia.EnsureDerivatives(ctx, media, objects, derivatives, workspaceID, asset); err != nil {
		slog.Warn("workspace task derivatives best-effort failed; tiers stay pending",
			"workspace_id", workspaceID, "asset_id", assetIDForLog(asset), "err", err)
	}
}

func assetIDForLog(asset *workspacemedia.Asset) string {
	if asset == nil {
		return ""
	}
	return asset.ID
}
