package providerinfra

// 透传层只测组装：exact-key 渲染与 presign 装配经 WorkerTransfer seam 断言；
// workermedia 签名/校验原语由 platform/workermedia 自测，此处不重复。

import (
	"context"
	"strings"
	"testing"
	"time"

	storageports "ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/media/workspacemedia"
)

type fakeWorkerAssetStore struct {
	assets map[string]*workspacemedia.Asset
	err    error
}

func (f *fakeWorkerAssetStore) GetAsset(_ context.Context, workspaceID, assetID string) (*workspacemedia.Asset, error) {
	if f.err != nil {
		return nil, f.err
	}
	asset, ok := f.assets[strings.TrimSpace(workspaceID)+"/"+strings.TrimSpace(assetID)]
	if !ok {
		return nil, errFakeWorkerAssetMissing
	}
	return asset, nil
}

var errFakeWorkerAssetMissing = errFakeWorkerAsset("asset not found")

type errFakeWorkerAsset string

func (e errFakeWorkerAsset) Error() string { return string(e) }

type fakeWorkerObjects struct {
	data map[string][]byte
	ct   map[string]string
}

func (f *fakeWorkerObjects) ReadObjectAtProfile(_ context.Context, _ string, key string, _ int64) (*storageports.StoredUploadObject, error) {
	raw, ok := f.data[key]
	if !ok {
		return nil, errFakeWorkerAsset("object not found")
	}
	return &storageports.StoredUploadObject{ObjectKey: key, ContentType: f.ct[key], Data: raw}, nil
}

func (f *fakeWorkerObjects) PutObjectAtProfile(_ context.Context, _ string, key string, data []byte, contentType string) error {
	if f.data == nil {
		f.data = map[string][]byte{}
	}
	if f.ct == nil {
		f.ct = map[string]string{}
	}
	f.data[key] = append([]byte(nil), data...)
	f.ct[key] = contentType
	return nil
}

func testWorkerTransfer(objects *fakeWorkerObjects) *workerTransfer {
	assets := &fakeWorkerAssetStore{assets: map[string]*workspacemedia.Asset{
		"ws_test/asset_input1": {ID: "asset_input1", WorkspaceID: "ws_test", StorageKey: "uploads/ws/test/asset_input1/input.png", MIMEType: "image/png"},
		"ws_test/asset_mask1":  {ID: "asset_mask1", WorkspaceID: "ws_test", StorageKey: "uploads/ws/test/asset_mask1/mask.png", MIMEType: "image/png"},
	}}
	if objects == nil {
		objects = &fakeWorkerObjects{}
	}
	return &workerTransfer{assets: assets, objects: objects, origin: "https://api.example.test", secret: "test-secret"}
}

func TestMintWorkerOutputKeyAlignsWithMediaScheme(t *testing.T) {
	got := MintWorkerOutputKey("ws_abc", "task-1")
	if got != "uploads/ws/abc/task-1/result-0.png" {
		t.Fatalf("key = %q, want uploads/ws/abc/task-1/result-0.png", got)
	}
	got = MintWorkerOutputKey("", "")
	if !strings.HasPrefix(got, "uploads/ws/") || !strings.HasSuffix(got, "/result-0.png") {
		t.Fatalf("fallback key = %q, want uploads/ws/.../result-0.png", got)
	}
	got = MintWorkerOutputKey("ws_../evil", "task-1")
	for _, segment := range strings.Split(got, "/") {
		if segment == ".." {
			t.Fatalf("traversal key = %q, want contained key", got)
		}
	}
	if !strings.HasPrefix(got, "uploads/ws/") {
		t.Fatalf("traversal key = %q, want slot root", got)
	}
}

func TestPresignOutputPUTBindsKeyAndContentType(t *testing.T) {
	transfer := testWorkerTransfer(nil)
	key := MintWorkerOutputKey("ws_abc", "task-1")
	putURL, expiresAt, err := transfer.PresignOutputPUT(key, WorkerSlotContentType())
	if err != nil {
		t.Fatalf("PresignOutputPUT() error = %v", err)
	}
	if WorkerSlotContentType() != "image/png" {
		t.Fatalf("slot content-type = %q, want image/png", WorkerSlotContentType())
	}
	if ttl := time.Until(expiresAt); ttl <= 0 || ttl > 6*time.Minute {
		t.Fatalf("slot ttl = %s, want short positive validity", ttl)
	}
	for _, want := range []string{"/api/v1/worker/media/uploads/ws/abc/task-1/result-0.png", "ct=image%2Fpng", "exp=", "sig="} {
		if !strings.Contains(putURL, want) {
			t.Fatalf("put url = %q, want %q", putURL, want)
		}
	}
	recovered, err := transfer.ImageKeyFromURL(putURL)
	if err != nil || recovered != key {
		t.Fatalf("ImageKeyFromURL = %q, %v; want %q", recovered, err, key)
	}
	// Query-less landed URLs resolve too (the Worker echoes upload_url
	// without its query).
	bare := putURL
	if idx := strings.Index(bare, "?"); idx >= 0 {
		bare = bare[:idx]
	}
	if recovered, err := transfer.ImageKeyFromURL(bare); err != nil || recovered != key {
		t.Fatalf("ImageKeyFromURL(bare) = %q, %v; want %q", recovered, err, key)
	}
}

func TestVerifyLandedKeyEnforcesExactSlot(t *testing.T) {
	transfer := testWorkerTransfer(nil)
	key := MintWorkerOutputKey("ws_abc", "task-1")
	putURL, _, err := transfer.PresignOutputPUT(key, WorkerSlotContentType())
	if err != nil {
		t.Fatalf("PresignOutputPUT() error = %v", err)
	}
	if _, err := transfer.VerifyLandedKey(putURL, key); err != nil {
		t.Fatalf("VerifyLandedKey(match) error = %v", err)
	}
	other := MintWorkerOutputKey("ws_abc", "task-2")
	if _, err := transfer.VerifyLandedKey(putURL, other); err == nil {
		t.Fatal("VerifyLandedKey(other slot) accepted, want fail-closed")
	}
	if _, err := transfer.VerifyLandedKey(putURL, ""); err == nil {
		t.Fatal("VerifyLandedKey(empty expected) accepted, want fail-closed")
	}
	if _, err := transfer.VerifyLandedKey("https://api.example.test/other/x.png", key); err == nil {
		t.Fatal("VerifyLandedKey(off-route) accepted, want fail-closed")
	}
}

func TestPresignInputGETResolvesAssetKey(t *testing.T) {
	transfer := testWorkerTransfer(nil)
	getURL, key, err := transfer.PresignInputGET(context.Background(), "ws_test", "asset_input1")
	if err != nil {
		t.Fatalf("PresignInputGET() error = %v", err)
	}
	if key != "uploads/ws/test/asset_input1/input.png" {
		t.Fatalf("key = %q, want asset storage key", key)
	}
	for _, want := range []string{"/api/v1/worker/media/uploads/ws/test/asset_input1/input.png", "ct=image%2Fpng"} {
		if !strings.Contains(getURL, want) {
			t.Fatalf("get url = %q, want %q", getURL, want)
		}
	}
	if _, _, err := transfer.PresignInputGET(context.Background(), "ws_test", "missing"); err == nil {
		t.Fatal("missing asset accepted, want error")
	}
}

func TestReadLandedObjectFailsClosed(t *testing.T) {
	objects := &fakeWorkerObjects{data: map[string][]byte{"uploads/ws/a/task/result-0.png": {1, 2, 3}}, ct: map[string]string{"uploads/ws/a/task/result-0.png": "image/png"}}
	transfer := testWorkerTransfer(objects)
	data, ct, err := transfer.ReadLandedObject(context.Background(), "uploads/ws/a/task/result-0.png")
	if err != nil || len(data) != 3 || ct != "image/png" {
		t.Fatalf("read = %v, %q, %v", len(data), ct, err)
	}
	if _, _, err := transfer.ReadLandedObject(context.Background(), "uploads/ws/a/task/missing.png"); err == nil {
		t.Fatal("missing object accepted, want error")
	}
	if _, _, err := transfer.ReadLandedObject(context.Background(), "../evil.png"); err == nil {
		t.Fatal("traversal key accepted, want error")
	}
}
