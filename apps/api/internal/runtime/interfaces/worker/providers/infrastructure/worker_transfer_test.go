package providerinfra

// Output-slot tests cross the same acceptance seam as the executor; the
// store is an in-memory adapter, while signing and image validation are real.

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
	"time"

	storageports "ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/media/workspacemedia"
	platformworkermedia "ideaboard/api/internal/platform/workermedia"
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
	data     map[string][]byte
	ct       map[string]string
	reads    int
	maxBytes int64
}

func (f *fakeWorkerObjects) ReadObjectAtProfile(_ context.Context, _ string, key string, maxBytes int64) (*storageports.StoredUploadObject, error) {
	f.reads++
	f.maxBytes = maxBytes
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

func TestPrepareOutputBindsFixedImagePolicy(t *testing.T) {
	transfer := testWorkerTransfer(nil)
	slot, err := transfer.PrepareOutput("ws_abc", "task-1")
	if err != nil {
		t.Fatal(err)
	}
	verified, err := platformworkermedia.VerifyURL("test-secret", slot.UploadURL(), "PUT")
	if err != nil {
		t.Fatal(err)
	}
	if verified.Key != "uploads/ws/abc/task-1/result-0.png" || verified.ContentType != platformworkermedia.SlotImagePolicy {
		t.Fatalf("unexpected slot binding: %+v", verified)
	}
	if ttl := time.Until(verified.ExpiresAt); ttl <= 0 || ttl > platformworkermedia.SlotTTL {
		t.Fatalf("slot ttl = %s", ttl)
	}
	if _, err := platformworkermedia.VerifyURL("test-secret", slot.UploadURL(), "GET"); err == nil {
		t.Fatal("PUT slot accepted as GET")
	}
}

func slotImage(t *testing.T, mime string) []byte {
	t.Helper()
	var b bytes.Buffer
	switch mime {
	case "image/png":
		if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
			t.Fatal(err)
		}
	case "image/jpeg":
		if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 1, 1)), nil); err != nil {
			t.Fatal(err)
		}
	case "image/webp":
		raw, err := base64.StdEncoding.DecodeString("UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA==")
		if err != nil {
			t.Fatal(err)
		}
		return raw
	default:
		t.Fatalf("unsupported fixture MIME %s", mime)
	}
	return b.Bytes()
}

func TestWorkerOutputSlotAcceptsActualImageFormats(t *testing.T) {
	for _, mime := range []string{"image/png", "image/jpeg", "image/webp"} {
		t.Run(mime, func(t *testing.T) {
			objects := &fakeWorkerObjects{data: map[string][]byte{}, ct: map[string]string{}}
			slot, err := testWorkerTransfer(objects).PrepareOutput("ws_test", "format")
			if err != nil {
				t.Fatal(err)
			}
			data := slotImage(t, mime)
			verified, err := platformworkermedia.VerifyURL("test-secret", slot.UploadURL(), "PUT")
			if err != nil {
				t.Fatal(err)
			}
			objects.data[verified.Key], objects.ct[verified.Key] = data, mime
			// Both the full signed URL and the query-stripped Worker echo bind to this slot.
			for _, echoed := range []string{slot.UploadURL(), strings.Split(slot.UploadURL(), "?")[0]} {
				got, err := slot.Accept(context.Background(), echoed, mime)
				want := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
				if err != nil || got != want {
					t.Fatalf("accept: %q, %v", got, err)
				}
			}
			if objects.maxBytes != workspacemedia.MaxUploadSizeBytes {
				t.Fatalf("unbounded read: %d", objects.maxBytes)
			}
		})
	}
}

func TestWorkerOutputSlotRejectsOtherSlotsBeforeReading(t *testing.T) {
	objects := &fakeWorkerObjects{}
	transfer := testWorkerTransfer(objects)
	slot, err := transfer.PrepareOutput("ws_test", "first")
	if err != nil {
		t.Fatal(err)
	}
	other, err := transfer.PrepareOutput("ws_test", "second")
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{other.UploadURL(), "https://api.example.test/other/x.png", ""} {
		if _, err := slot.Accept(context.Background(), raw, "image/png"); err == nil {
			t.Fatal("unbound image URL accepted")
		}
	}
	if objects.reads != 0 {
		t.Fatal("read object before exact slot verification")
	}
}

func TestWorkerOutputSlotRejectsUntrustedImages(t *testing.T) {
	for _, mime := range []string{"image/png", "image/jpeg", "image/webp"} {
		data := slotImage(t, mime)
		truncated := data[:len(data)-3]
		if mime == "image/png" {
			truncated = data[:33]
		}
		if _, _, err := image.DecodeConfig(bytes.NewReader(truncated)); err != nil {
			t.Fatalf("incomplete config fixture: %v", err)
		}
		for name, tc := range map[string]struct {
			data             []byte
			stored, reported string
		}{
			"missing":           {nil, mime, mime},
			"empty":             {[]byte{}, mime, mime},
			"stored-mismatch":   {data, "image/gif", mime},
			"reported-mismatch": {data, mime, "image/gif"},
			"body-truncated":    {truncated, mime, mime},
			"disguised":         {[]byte("<svg/>"), mime, mime},
		} {
			t.Run(mime+"/"+name, func(t *testing.T) {
				objects := &fakeWorkerObjects{data: map[string][]byte{}, ct: map[string]string{}}
				slot, err := testWorkerTransfer(objects).PrepareOutput("ws_test", "invalid")
				if err != nil {
					t.Fatal(err)
				}
				verified, err := platformworkermedia.VerifyURL("test-secret", slot.UploadURL(), "PUT")
				if err != nil {
					t.Fatal(err)
				}
				if tc.data != nil {
					objects.data[verified.Key] = tc.data
				}
				objects.ct[verified.Key] = tc.stored
				if _, err := slot.Accept(context.Background(), slot.UploadURL(), tc.reported); err == nil {
					t.Fatal("untrusted image accepted")
				}
			})
		}
	}
}

func TestWorkerOutputSlotEnforcesSizeEvenForInvalidStoreAdapter(t *testing.T) {
	objects := &fakeWorkerObjects{data: map[string][]byte{}, ct: map[string]string{}}
	slot, err := testWorkerTransfer(objects).PrepareOutput("ws_test", "oversized")
	if err != nil {
		t.Fatal(err)
	}
	verified, err := platformworkermedia.VerifyURL("test-secret", slot.UploadURL(), "PUT")
	if err != nil {
		t.Fatal(err)
	}
	objects.data[verified.Key] = make([]byte, workspacemedia.MaxUploadSizeBytes+1)
	objects.ct[verified.Key] = "image/png"
	if _, err := slot.Accept(context.Background(), slot.UploadURL(), "image/png"); err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("oversized read: %v", err)
	}
}

func TestWorkerOutputSlotFailsClosedWithoutConfiguration(t *testing.T) {
	for _, transfer := range []*workerTransfer{nil, {objects: &fakeWorkerObjects{}, secret: "test"}, {objects: &fakeWorkerObjects{}, origin: "https://api.test"}, {origin: "https://api.test", secret: "test"}} {
		if _, err := transfer.PrepareOutput("ws_test", "task"); err == nil {
			t.Fatal("unconfigured output slot prepared")
		}
	}
	for _, slot := range []*WorkerOutputSlot{nil, {}} {
		if _, err := slot.Accept(context.Background(), "", "image/png"); err == nil {
			t.Fatal("unconfigured slot accepted image")
		}
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
