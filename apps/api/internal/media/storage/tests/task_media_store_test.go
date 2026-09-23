package tests

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"time"

	"github.com/jackc/pgx/v5/pgconn"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/media/ingestion"
	"ideaboard/api/internal/platform/outbound"
	tasks "ideaboard/api/internal/tasking/domain"
	taskmedia "ideaboard/api/internal/tasking/ports/media"
)

type recordingTaskResultRegistry struct {
	nextID      int64
	started     []ingestion.Request
	completed   []ingestion.Handle
	completions []ingestion.Completion
	failed      []ingestion.Handle
	existing    *dbsqlc.MediaAsset
	completeErr error
}

func (r *recordingTaskResultRegistry) Begin(_ context.Context, request ingestion.Request) (ingestion.Handle, error) {
	r.started = append(r.started, request)
	if r.nextID == 0 {
		r.nextID = 100
	}
	id := r.nextID
	r.nextID++
	return ingestion.Handle{
		AssetID: id, PublicAssetID: fmt.Sprintf("asset_test_%d", id), OwnerUserID: request.OwnerUserID, StorageProfile: request.StorageProfile,
		StorageKey: request.StorageKey, Origin: request.Origin,
	}, nil
}

func (r *recordingTaskResultRegistry) Complete(_ context.Context, handle ingestion.Handle, completion ingestion.Completion) error {
	if r.completeErr != nil {
		return r.completeErr
	}
	r.completed = append(r.completed, handle)
	r.completions = append(r.completions, completion)
	return nil
}

func (r *recordingTaskResultRegistry) Fail(_ context.Context, handle ingestion.Handle, _ error) error {
	r.failed = append(r.failed, handle)
	return nil
}

func (r *recordingTaskResultRegistry) FindReadyOwnedMediaAssetByChecksum(_ context.Context, _ int64, _ string, _ string) (dbsqlc.MediaAsset, bool, error) {
	if r.existing == nil {
		return dbsqlc.MediaAsset{}, false, nil
	}
	return *r.existing, true, nil
}

func s3PresignFakeServer(t *testing.T, imageBytes []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.URL.Query()["location"]; ok {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><LocationConstraint>us-east-1</LocationConstraint>`))
			return
		}
		if r.Method == http.MethodHead {
			w.Header().Set("X-Amz-Meta-Object-Generation", "1")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(imageBytes)))
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("X-Amz-Meta-Object-Generation", "1")
		_, _ = w.Write(imageBytes)
	}))
}

func saveS3PresignProfileForTest(t *testing.T, service *StorageService, ctx context.Context, id, endpoint string) {
	t.Helper()
	if _, err := service.SaveProfile(ctx, &StorageProfile{
		ID: id, Name: id, Provider: StorageProviderS3Compatible, Enabled: true,
		Config: map[string]interface{}{
			"access_key_id": "ak", storageSecretAccessKeyName: "sk", "bucket_name": "bucket",
			"endpoint": endpoint, "region_id": "us-east-1", "access_domain": "https://cdn.example.com",
			"force_path_style": true,
		},
	}); err != nil {
		t.Fatalf("save %s profile: %v", id, err)
	}
}

func TestRuntimeTaskMediaStoreResolvesAssetThroughOwningProfileAfterCurrentSwitch(t *testing.T) {
	restore := outbound.ConfigurePolicyProvider(testStorageOutboundPolicyProvider{hosts: []string{"127.0.0.1"}})
	t.Cleanup(restore)
	service, _ := newStorageServiceTest(t)
	ctx := t.Context()
	imageBytes := tinyPNGBytes(t)
	assetServer := s3PresignFakeServer(t, imageBytes)
	t.Cleanup(assetServer.Close)
	currentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(currentServer.Close)
	saveS3PresignProfileForTest(t, service, ctx, "asset-profile", assetServer.URL)
	saveS3PresignProfileForTest(t, service, ctx, "current-profile", currentServer.URL)
	if _, err := service.ApplyProfile(ctx, "current-profile"); err != nil {
		t.Fatal(err)
	}

	checksumBytes := sha256.Sum256(imageBytes)
	checksum := hex.EncodeToString(checksumBytes[:])
	manifest, err := tasks.NewTaskInputManifest([]tasks.TaskInputManifestRef{{
		AssetID: "asset_91", DerivativeID: "derivative_91", DerivativeTier: tasks.TierProcessingInput,
		StorageProfileID: "asset-profile", ObjectKey: "uploads/tasks/42/task/input.png", ObjectGeneration: 1,
		ChecksumSHA256: checksum, MIMEType: "image/png", SizeBytes: int64(len(imageBytes)),
	}}, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	store := NewRuntimeTaskMediaStore(service, &recordingTaskResultRegistry{})
	params := &tasks.WorkflowTaskParams{
		Media: &tasks.MediaInputs{Inputs: []tasks.MediaRef{{
			AssetID: 91, Role: tasks.MediaRoleInput,
		}}},
		InputManifest: manifest,
	}
	resolved, err := store.ResolveRuntimeMedia(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resolved.Inputs[0].URL, "uploads/tasks/42/task/input.png") {
		t.Fatalf("resolved media = %#v, want manifest object key", resolved)
	}
	if strings.Contains(resolved.Inputs[0].URL, currentServer.URL[strings.Index(currentServer.URL, "127.0.0.1"):]) {
		t.Fatalf("resolver followed current profile: resolved=%q", resolved.Inputs[0].URL)
	}
}

func TestRuntimeTaskMediaStoreRejectsLocalAssetWithoutProviderURL(t *testing.T) {
	service, _ := newStorageServiceTest(t)
	store := NewRuntimeTaskMediaStore(service, &recordingTaskResultRegistry{})
	manifest, err := tasks.NewTaskInputManifest([]tasks.TaskInputManifestRef{{
		AssetID: "asset_92", DerivativeID: "derivative_92", DerivativeTier: tasks.TierProcessingInput,
		StorageProfileID: builtinLocalProfileID(), ObjectKey: "uploads/canvas/owner/board/content/input.png",
		ObjectGeneration: 1, ChecksumSHA256: strings.Repeat("b", 64), MIMEType: "image/png", SizeBytes: 1,
	}}, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	params := &tasks.WorkflowTaskParams{Media: &tasks.MediaInputs{Inputs: []tasks.MediaRef{{
		AssetID: 92, Role: tasks.MediaRoleInput,
	}}}, InputManifest: manifest}
	if _, err := store.ResolveRuntimeMedia(t.Context(), params); err == nil {
		t.Fatal("expected local asset without a provider URL to be rejected")
	}
}

func TestRuntimeTaskMediaStoreSurfacesMissingProfileWithoutFallback(t *testing.T) {
	service, _ := newStorageServiceTest(t)
	store := NewRuntimeTaskMediaStore(service, &recordingTaskResultRegistry{})
	manifest, err := tasks.NewTaskInputManifest([]tasks.TaskInputManifestRef{{
		AssetID: "asset_93", DerivativeID: "derivative_93", DerivativeTier: tasks.TierProcessingInput,
		StorageProfileID: "profile-missing", ObjectKey: "uploads/canvas/owner/board/content/image.png",
		ObjectGeneration: 1, ChecksumSHA256: strings.Repeat("c", 64), MIMEType: "image/png", SizeBytes: 1,
	}}, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	params := &tasks.WorkflowTaskParams{Media: &tasks.MediaInputs{Inputs: []tasks.MediaRef{{
		AssetID: 93, Role: tasks.MediaRoleInput,
	}}}, InputManifest: manifest}
	if _, err := store.ResolveRuntimeMedia(t.Context(), params); err == nil {
		t.Fatal("expected missing manifest profile to surface without fallback")
	}
}

func TestRuntimeTaskMediaStoreRejectsExtraManifestRefs(t *testing.T) {
	service, _ := newStorageServiceTest(t)
	store := NewRuntimeTaskMediaStore(service, &recordingTaskResultRegistry{})
	manifest, err := tasks.NewTaskInputManifest([]tasks.TaskInputManifestRef{
		{
			AssetID: "asset_extra", DerivativeID: "derivative_extra", DerivativeTier: tasks.TierProcessingInput,
			StorageProfileID: "profile_primary", ObjectKey: "uploads/processing/extra.webp", ObjectGeneration: 1,
			ChecksumSHA256: strings.Repeat("a", 64), MIMEType: "image/webp", SizeBytes: 1,
		},
	}, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	params := &tasks.WorkflowTaskParams{Media: &tasks.MediaInputs{}, InputManifest: manifest}
	if _, err := store.ResolveRuntimeMedia(t.Context(), params); err == nil || !strings.Contains(err.Error(), "manifest 与 media params 不匹配") {
		t.Fatalf("expected extra manifest refs to be rejected, got %v", err)
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
	registry := &recordingTaskResultRegistry{}
	adapter := NewRuntimeTaskMediaStore(service, registry)
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
		if output.MediaAssetID == nil || *output.MediaAssetID != int64(100+i) {
			t.Fatalf("output[%d] asset = %#v", i, output.MediaAssetID)
		}
		if output.AssetID != fmt.Sprintf("asset_test_%d", 100+i) {
			t.Fatalf("output[%d] public asset id = %q", i, output.AssetID)
		}
		assertLocalUploadBytes(t, output.StorageKey, imageBytes)
	}
	if len(registry.completed) != 2 || registry.completed[0].AssetID != 100 || registry.completed[1].AssetID != 101 {
		t.Fatalf("completed ingestions = %#v", registry.completed)
	}
	for index, request := range registry.started {
		facts, ok := request.Origin.(ingestion.TaskResultFacts)
		if !ok || facts.TaskID != "task-abc" || facts.ResultIndex != index {
			t.Fatalf("started request[%d] = %#v", index, request)
		}
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
	registry := &recordingTaskResultRegistry{}
	adapter := NewRuntimeTaskMediaStore(service, registry)

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
	} else if len(entries) != 1 {
		t.Fatalf("expected first tracked result to remain retryable, found %d entries", len(entries))
	}
	if len(registry.failed) != 1 || registry.failed[0].AssetID != 101 {
		t.Fatalf("failed ingestions = %#v, want [101]", registry.failed)
	}
	assertDirEmpty(t, tmpDir)
}

func TestRuntimeTaskMediaStoreMaterializeBytesResultStoresOutput(t *testing.T) {
	withTempWorkingDir(t)

	imageBytes := tinyPNGBytes(t)
	service, _ := newStorageServiceTest(t)
	adapter := NewRuntimeTaskMediaStore(service, &recordingTaskResultRegistry{})

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
	if output.AssetID != "asset_test_100" {
		t.Fatalf("output public asset id = %q", output.AssetID)
	}
	assertLocalUploadBytes(t, output.StorageKey, imageBytes)
}

func TestRuntimeTaskMediaStoreMaterializeBytesResultReusesChecksumAsset(t *testing.T) {
	withTempWorkingDir(t)
	imageBytes := tinyPNGBytes(t)
	digest := sha256.Sum256(imageBytes)
	service, _ := newStorageServiceTest(t)
	canonical := &dbsqlc.MediaAsset{
		ID: 77, OwnerUserID: 42, StorageProfileID: builtinLocalProfileID(),
		StorageKey: "uploads/tasks/42/existing/result.png", Url: "https://cdn.example/existing.png",
		MimeType: "image/png", Width: 1, Height: 1, SizeBytes: int64(len(imageBytes)),
		ChecksumSha256: hex.EncodeToString(digest[:]), Status: "ready",
	}
	registry := &recordingTaskResultRegistry{existing: canonical}
	adapter := NewRuntimeTaskMediaStore(service, registry)

	materialized, err := adapter.MaterializeBytesResult(context.Background(), 42, "task-dedup", "result.png", "upstream://image", imageBytes, taskmedia.ResultMaterializeOptions{})
	if err != nil {
		t.Fatalf("MaterializeBytesResult returned error: %v", err)
	}
	if got := materialized.Items[0].ImageURL; got != canonical.Url {
		t.Fatalf("materialized URL = %q, want canonical %q", got, canonical.Url)
	}
	if materialized.Items[0].Output.MediaAssetID == nil || *materialized.Items[0].Output.MediaAssetID != canonical.ID {
		t.Fatalf("materialized asset = %#v, want %d", materialized.Items[0].Output.MediaAssetID, canonical.ID)
	}
	if len(registry.completed) != 0 {
		t.Fatalf("deduplicated result should not complete new asset: %#v", registry.completed)
	}
	if len(registry.failed) != 1 {
		t.Fatalf("deduplicated uploading asset should be failed/cleaned: %#v", registry.failed)
	}
}

func TestRuntimeTaskMediaStoreMaterializeBytesResultConvergesAfterChecksumConflict(t *testing.T) {
	withTempWorkingDir(t)
	imageBytes := tinyPNGBytes(t)
	digest := sha256.Sum256(imageBytes)
	canonical := &dbsqlc.MediaAsset{ID: 88, OwnerUserID: 42, StorageProfileID: builtinLocalProfileID(), StorageKey: "uploads/tasks/42/existing/result.png", Url: "https://cdn.example/existing.png", MimeType: "image/png", Width: 1, Height: 1, SizeBytes: int64(len(imageBytes)), ChecksumSha256: hex.EncodeToString(digest[:]), Status: "ready"}
	registry := &recordingTaskResultRegistry{existing: canonical, completeErr: &pgconn.PgError{Code: "23505", ConstraintName: "uq_media_assets_owned_checksum"}}
	service, _ := newStorageServiceTest(t)
	adapter := NewRuntimeTaskMediaStore(service, registry)

	materialized, err := adapter.MaterializeBytesResult(context.Background(), 42, "task-dedup-race", "result.png", "upstream://image", imageBytes, taskmedia.ResultMaterializeOptions{})
	if err != nil {
		t.Fatalf("MaterializeBytesResult returned error: %v", err)
	}
	if materialized.Items[0].ImageURL != canonical.Url || len(registry.failed) != 1 {
		t.Fatalf("result did not converge on canonical asset: materialized=%#v failed=%#v", materialized, registry.failed)
	}
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
