package runtime

// Task result rendering completes media ingestion before terminal settlement.
// The worker owns task state, output references, and billing in its settlement transaction.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"ideaboard/api/internal/media/workspacemedia"
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
	asset, err := persistWorkspaceTaskResultAsset(ctx, r.media, r.objects, task, data, index, strings.TrimSpace(ref.ChecksumSHA256))
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

// persistWorkspaceTaskResultAsset applies Task provenance to managed image ingestion.
func persistWorkspaceTaskResultAsset(ctx context.Context, media workspacemedia.WorkspaceMediaStore, objects workspacemedia.MediaObjects, task *workspacetasks.WorkspaceTask, data []byte, index int, expectChecksum string) (*workspacemedia.Asset, error) {
	if media == nil || objects == nil {
		return nil, errors.New(msgIngestRemote)
	}
	if task == nil || strings.TrimSpace(task.ID) == "" || strings.TrimSpace(task.WorkspaceID) == "" {
		return nil, errors.New(msgRenderOutputs2)
	}
	asset, err := workspacemedia.IngestImage(ctx, media, objects, workspacemedia.IngestImageParams{
		WorkspaceID: task.WorkspaceID, OwnerUserID: task.OwnerUserID,
		FilenameStem: fmt.Sprintf("task-result-%d", index), Data: data,
		ExpectedChecksumSHA256: expectChecksum,
	})
	if err != nil {
		return nil, fmt.Errorf(msgPersistWorkspaceTaskResultAsset, err)
	}
	slog.Info("workspace task result ingested as media asset",
		"task_id", task.ID, "asset_id", asset.ID, "mime", asset.MIMEType,
		"width", asset.Width, "height", asset.Height, "bytes", asset.SizeBytes)
	return asset, nil
}
