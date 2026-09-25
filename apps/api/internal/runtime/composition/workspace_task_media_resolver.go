package composition

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/media/workspacemedia"
	tasks "ideaboard/api/internal/tasking/domain"
	taskmedia "ideaboard/api/internal/tasking/ports/media"
	"ideaboard/api/internal/workflow/paramkeys"
)

type workspaceMediaAssetReader interface {
	GetAsset(context.Context, string, string) (*workspacemedia.Asset, error)
}

type workspaceTaskObjectReader interface {
	ReadObjectAtProfile(context.Context, string, string, int64) (*ports.StoredUploadObject, error)
}

// workspaceTaskRuntimeMediaResolver resolves canonical workspace asset refs
// into provider-safe in-memory URLs. The workspace id is carried only in the
// worker bridge's typed Extra field and is never trusted from user params.
type workspaceTaskRuntimeMediaResolver struct {
	media   workspaceMediaAssetReader
	objects workspaceTaskObjectReader
}

var _ taskmedia.RuntimeMediaResolver = (*workspaceTaskRuntimeMediaResolver)(nil)
var _ taskmedia.UploadReader = (*workspaceTaskRuntimeMediaResolver)(nil)

func newWorkspaceTaskRuntimeMediaResolver(media workspaceMediaAssetReader, objects workspaceTaskObjectReader) *workspaceTaskRuntimeMediaResolver {
	return &workspaceTaskRuntimeMediaResolver{media: media, objects: objects}
}

func (r *workspaceTaskRuntimeMediaResolver) ReadUploadObject(context.Context, string, int64) (*taskmedia.StoredObject, error) {
	return nil, fmt.Errorf("workspace task media requires canonical public asset ids")
}

func (r *workspaceTaskRuntimeMediaResolver) ResolveRuntimeMedia(ctx context.Context, params *tasks.WorkflowTaskParams) (*tasks.RuntimeMediaInputs, error) {
	if params == nil || params.Media == nil {
		return nil, nil
	}
	if r == nil || r.media == nil || r.objects == nil {
		return nil, fmt.Errorf("workspace media resolver is unavailable")
	}
	workspaceID, _ := params.Extra[paramkeys.WorkspaceID].(string)
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace media scope is missing")
	}
	resolved := &tasks.RuntimeMediaInputs{}
	resolve := func(ref tasks.MediaRef) (tasks.RuntimeMediaRef, error) {
		publicID := strings.TrimSpace(ref.PublicAssetID)
		if publicID == "" {
			return tasks.RuntimeMediaRef{}, fmt.Errorf("workspace media input missing public asset id")
		}
		asset, err := r.media.GetAsset(ctx, workspaceID, publicID)
		if err != nil {
			return tasks.RuntimeMediaRef{}, fmt.Errorf("resolve workspace media asset %s: %w", publicID, err)
		}
		if asset == nil || strings.TrimSpace(asset.StorageKey) == "" {
			return tasks.RuntimeMediaRef{}, fmt.Errorf("workspace media asset %s has no storage object", publicID)
		}
		object, err := r.objects.ReadObjectAtProfile(ctx, workspacemedia.DefaultStorageProfile, asset.StorageKey, workspacemedia.MaxUploadSizeBytes)
		if err != nil {
			return tasks.RuntimeMediaRef{}, fmt.Errorf("read workspace media asset %s: %w", publicID, err)
		}
		if object == nil || len(object.Data) == 0 {
			return tasks.RuntimeMediaRef{}, fmt.Errorf("workspace media asset %s is empty", publicID)
		}
		mimeType := strings.ToLower(strings.TrimSpace(asset.MIMEType))
		if mimeType == "" {
			mimeType = strings.ToLower(strings.TrimSpace(object.ContentType))
		}
		if !strings.HasPrefix(mimeType, "image/") {
			return tasks.RuntimeMediaRef{}, fmt.Errorf("workspace media asset %s is not an image", publicID)
		}
		return tasks.RuntimeMediaRef{URL: "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(object.Data), Role: ref.Role, Weight: ref.Weight}, nil
	}
	for _, ref := range params.Media.Inputs {
		value, err := resolve(ref)
		if err != nil {
			return nil, err
		}
		resolved.Inputs = append(resolved.Inputs, value)
	}
	for _, ref := range params.Media.References {
		value, err := resolve(ref)
		if err != nil {
			return nil, err
		}
		resolved.References = append(resolved.References, value)
	}
	if params.Media.Original != nil {
		value, err := resolve(*params.Media.Original)
		if err != nil {
			return nil, err
		}
		resolved.Original = &value
	}
	if params.Media.Mask != nil {
		value, err := resolve(*params.Media.Mask)
		if err != nil {
			return nil, err
		}
		resolved.Mask = &value
	}
	if params.Media.UseMask != nil {
		useMask := *params.Media.UseMask
		resolved.UseMask = &useMask
	}
	return resolved, nil
}
