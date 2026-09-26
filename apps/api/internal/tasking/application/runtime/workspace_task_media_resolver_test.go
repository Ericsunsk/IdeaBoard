package runtime

import (
	"context"
	"strings"
	"testing"

	"ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/media/workspacemedia"
	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/workflow/paramkeys"
)

type resolverAssetReader struct {
	workspaceID string
	asset       *workspacemedia.Asset
}

func (r *resolverAssetReader) GetAsset(_ context.Context, workspaceID, assetID string) (*workspacemedia.Asset, error) {
	if workspaceID != r.workspaceID || assetID != r.asset.ID {
		return nil, workspacemedia.ErrNotFound
	}
	return r.asset, nil
}

type resolverObjectReader struct {
	profile string
	key     string
	object  *ports.StoredUploadObject
}

func (r *resolverObjectReader) ReadObjectAtProfile(_ context.Context, profile, key string, _ int64) (*ports.StoredUploadObject, error) {
	r.profile, r.key = profile, key
	return r.object, nil
}

func TestWorkspaceTaskRuntimeMediaResolverUsesWorkspaceScopedPublicAsset(t *testing.T) {
	asset := &workspacemedia.Asset{ID: "asset_input123", StorageKey: "workspace/ws_test/input.png", MIMEType: "image/png"}
	objects := &resolverObjectReader{object: &ports.StoredUploadObject{ContentType: "image/png", Data: []byte("png-bytes")}}
	resolver := NewWorkspaceTaskRuntimeMediaResolver(&resolverAssetReader{workspaceID: "ws_test", asset: asset}, objects)
	params := &tasks.WorkflowTaskParams{
		Media: &tasks.MediaInputs{Inputs: []tasks.MediaRef{{PublicAssetID: asset.ID, Role: tasks.MediaRoleInput}}},
		Extra: map[string]interface{}{paramkeys.WorkspaceID: "ws_test"},
	}
	resolved, err := resolver.ResolveRuntimeMedia(context.Background(), params)
	if err != nil {
		t.Fatalf("resolve runtime media: %v", err)
	}
	if len(resolved.Inputs) != 1 || !strings.HasPrefix(resolved.Inputs[0].URL, "data:image/png;base64,") {
		t.Fatalf("resolved=%+v, want image data URL", resolved)
	}
	if objects.profile != workspacemedia.DefaultStorageProfile || objects.key != asset.StorageKey {
		t.Fatalf("object lookup=(%q,%q), want workspace storage profile and key", objects.profile, objects.key)
	}
}

func TestWorkspaceTaskRuntimeMediaResolverRejectsNumericOnlyRef(t *testing.T) {
	resolver := NewWorkspaceTaskRuntimeMediaResolver(&resolverAssetReader{workspaceID: "ws_test", asset: &workspacemedia.Asset{ID: "asset_other"}}, &resolverObjectReader{})
	_, err := resolver.ResolveRuntimeMedia(context.Background(), &tasks.WorkflowTaskParams{
		Media: &tasks.MediaInputs{Inputs: []tasks.MediaRef{{AssetID: 7, Role: tasks.MediaRoleInput}}},
		Extra: map[string]interface{}{paramkeys.WorkspaceID: "ws_test"},
	})
	if err == nil || !strings.Contains(err.Error(), "missing public asset id") {
		t.Fatalf("error=%v, want numeric-only workspace media rejection", err)
	}
}
