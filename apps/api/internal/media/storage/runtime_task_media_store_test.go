package storage

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ideaboard/api/internal/tasking/taskmedia"
	"ideaboard/api/internal/tasking/tasks"
)

type fixedAssetLocator map[int64]StoredUploadLocation

func (l fixedAssetLocator) LocateStoredUpload(_ context.Context, lookup StoredUploadLookup) (StoredUploadLocation, bool, error) {
	location, ok := l[lookup.AssetID]
	return location, ok, nil
}

func TestRuntimeTaskMediaStoreResolvesAssetThroughOwningProfileAfterCurrentSwitch(t *testing.T) {
	service, _ := newStorageServiceTest(t)
	ctx := t.Context()
	profile := func(id, domain string) *StorageProfile {
		return &StorageProfile{
			ID: id, Name: id, Provider: StorageProviderS3Compatible, Enabled: true,
			Config: map[string]interface{}{
				"access_key_id": "ak", storageSecretAccessKeyName: "sk", "bucket_name": "bucket",
				"endpoint": "s3.example.com", "region_id": "us-east-1", "access_domain": domain,
			},
		}
	}
	if _, err := service.SaveProfile(ctx, profile("asset-profile", "https://asset-cdn.example.com")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveProfile(ctx, profile("current-profile", "https://current-cdn.example.com")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApplyProfile(ctx, "current-profile"); err != nil {
		t.Fatal(err)
	}
	service.assetLocator = fixedAssetLocator{91: {
		StorageProfileID: "asset-profile",
		StorageKey:       "uploads/tasks/42/task/input.png",
	}}

	store := NewRuntimeTaskMediaStore(service)
	params := &tasks.WorkflowTaskParams{
		Media: &tasks.MediaInputs{Inputs: []tasks.MediaRef{{
			AssetID: 91, Role: tasks.MediaRoleInput,
		}}},
	}
	resolved, err := store.ResolveRuntimeMedia(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://asset-cdn.example.com/uploads/tasks/42/task/input.png"
	if resolved.Inputs[0].URL != want {
		t.Fatalf("resolved media = %#v, want %q", resolved, want)
	}
	if strings.Contains(resolved.Inputs[0].URL, "current-cdn") {
		t.Fatalf("resolver followed current profile: resolved=%q", resolved.Inputs[0].URL)
	}
}

func TestRuntimeTaskMediaStoreRejectsLocalAssetWithoutProviderURL(t *testing.T) {
	service, _ := newStorageServiceTest(t)
	service.assetLocator = fixedAssetLocator{92: {
		StorageProfileID: builtinLocalProfileID(),
		StorageKey:       "uploads/canvas/owner/board/content/input.png",
	}}
	store := NewRuntimeTaskMediaStore(service)
	params := &tasks.WorkflowTaskParams{Media: &tasks.MediaInputs{Inputs: []tasks.MediaRef{{
		AssetID: 92, Role: tasks.MediaRoleInput,
	}}}}
	if _, err := store.ResolveRuntimeMedia(t.Context(), params); err == nil {
		t.Fatal("expected local asset without a provider URL to be rejected")
	}
}

func TestRuntimeTaskMediaStoreMaterializeRemoteResultsStoresOutputs(t *testing.T) {
	withTempWorkingDir(t)
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)

	imageBytes := tinyPNGBytes(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(imageBytes)
	}))
	defer server.Close()

	service, _ := newStorageServiceTest(t)
	adapter := NewRuntimeTaskMediaStore(service)
	sourceURLs := []string{server.URL + "/one.png", server.URL + "/two.png"}

	materialized, err := adapter.MaterializeRemoteResults(context.Background(), 42, "task-abc", sourceURLs, taskmedia.ResultMaterializeOptions{
		AllowHosts: []string{server.URL},
	})
	if err != nil {
		t.Fatalf("MaterializeRemoteResults returned error: %v", err)
	}
	if len(materialized.Items) != 2 {
		t.Fatalf("materialized count = %d, want 2", len(materialized.Items))
	}
	materializedURLs := materialized.ImageURLs()
	if sourceURLs[0] == materializedURLs[0] {
		t.Fatalf("sourceURLs was not materialized: %q", materializedURLs[0])
	}
	for i, item := range materialized.Items {
		output := item.Output
		if output.TaskID != "task-abc" || output.Role != "image" || output.SourceURL != sourceURLs[i] || output.ResultIndex != i {
			t.Fatalf("output[%d] metadata = %#v", i, output)
		}
		if item.ImageURL != output.URL || output.URL != materializedURLs[i] || !strings.HasPrefix(output.StorageKey, "uploads/tasks/42/task-abc/") {
			t.Fatalf("output[%d] storage fields = %#v", i, output)
		}
		if output.MIMEType != "image/png" || output.Width != 1 || output.Height != 1 || output.SizeBytes != int64(len(imageBytes)) || output.StorageProfile != builtinLocalProfileID() {
			t.Fatalf("output[%d] media fields = %#v", i, output)
		}
		assertLocalUploadBytes(t, output.StorageKey, imageBytes)
	}
	assertDirEmpty(t, tmpDir)
}

func TestRuntimeTaskMediaStoreMaterializeRemoteResultsCleansStoredKeysOnFailure(t *testing.T) {
	withTempWorkingDir(t)
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)

	imageBytes := tinyPNGBytes(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad.png" {
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("nope"))
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(imageBytes)
	}))
	defer server.Close()

	service, _ := newStorageServiceTest(t)
	adapter := NewRuntimeTaskMediaStore(service)

	_, err := adapter.MaterializeRemoteResults(context.Background(), 42, "task-rollback", []string{
		server.URL + "/ok.png",
		server.URL + "/bad.png",
	}, taskmedia.ResultMaterializeOptions{AllowHosts: []string{server.URL}})
	if err == nil {
		t.Fatal("expected MaterializeRemoteResults to fail")
	}
	taskDir := filepath.FromSlash("uploads/tasks/42/task-rollback")
	if entries, readErr := os.ReadDir(taskDir); readErr != nil && !os.IsNotExist(readErr) {
		t.Fatalf("read task output dir failed: %v", readErr)
	} else if len(entries) != 0 {
		t.Fatalf("expected materialized task output files to be cleaned up, found %d entries", len(entries))
	}
	assertDirEmpty(t, tmpDir)
}

func TestRuntimeTaskMediaStoreMaterializeBytesResultStoresOutput(t *testing.T) {
	withTempWorkingDir(t)

	imageBytes := tinyPNGBytes(t)
	service, _ := newStorageServiceTest(t)
	adapter := NewRuntimeTaskMediaStore(service)

	materialized, err := adapter.MaterializeBytesResult(context.Background(), 42, "task-bytes", "result.png", "upstream://image", imageBytes, taskmedia.ResultMaterializeOptions{})
	if err != nil {
		t.Fatalf("MaterializeBytesResult returned error: %v", err)
	}
	if len(materialized.Items) != 1 {
		t.Fatalf("materialized count = %d, want 1", len(materialized.Items))
	}
	output := materialized.Items[0].Output
	if output.TaskID != "task-bytes" || output.Role != "image" || output.URL != materialized.Items[0].ImageURL || output.SourceURL != "upstream://image" || output.ResultIndex != 0 {
		t.Fatalf("output metadata = %#v", output)
	}
	if !strings.HasPrefix(output.StorageKey, "uploads/tasks/42/task-bytes/") || output.MIMEType != "image/png" || output.Width != 1 || output.Height != 1 || output.SizeBytes != int64(len(imageBytes)) {
		t.Fatalf("output storage fields = %#v", output)
	}
	assertLocalUploadBytes(t, output.StorageKey, imageBytes)
}

func assertLocalUploadBytes(t *testing.T, objectKey string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(filepath.FromSlash(objectKey))
	if err != nil {
		t.Fatalf("read stored object %q failed: %v", objectKey, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("stored object %q bytes mismatch", objectKey)
	}
}
