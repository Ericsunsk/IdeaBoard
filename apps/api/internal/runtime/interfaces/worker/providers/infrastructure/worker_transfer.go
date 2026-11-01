package providerinfra

import (
	"context"
	"fmt"
	"strings"
	"time"

	storageports "ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/media/workspacemedia"
	platformworkermedia "ideaboard/api/internal/platform/workermedia"
)

// WorkerTransfer is the single presigned-media seam for Worker execution:
// exact keys, bound content-types, short TTLs, plus landed-object reads.
// Production mints HMAC URLs served by the API media route; tests inject a
// fake over httptest.
type WorkerTransfer interface {
	// PresignInputGET mints a presigned GET for one canonical input asset.
	PresignInputGET(ctx context.Context, workspaceID, publicAssetID string) (url, key string, err error)
	// MintOutputKey renders the exact storage key for the single result.
	MintOutputKey(workspaceID, taskID string) string
	// PresignOutputPUT mints the presigned PUT for the exact output key.
	PresignOutputPUT(key, contentType string) (url string, expiresAt time.Time, err error)
	// ImageKeyFromURL recovers the exact key from a landed image URL.
	ImageKeyFromURL(rawURL string) (string, error)
	// VerifyLandedKey recovers the exact key from a landed image URL and
	// fails closed unless it equals the minted slot key: parse and
	// exact-equality in one call, so callers cannot use an unverified key.
	VerifyLandedKey(rawURL, expectedKey string) (string, error)
	// ReadLandedObject reads back bytes the Worker landed at key.
	ReadLandedObject(ctx context.Context, key string) (data []byte, contentType string, err error)
}

// WorkerAssetStore resolves canonical asset storage facts for presigning.
type WorkerAssetStore interface {
	GetAsset(ctx context.Context, workspaceID, assetID string) (*workspacemedia.Asset, error)
}

// WorkerObjectStore reads back landed Worker bytes by exact key.
type WorkerObjectStore interface {
	ReadObjectAtProfile(ctx context.Context, profileID, objectKey string, maxBytes int64) (*storageports.StoredUploadObject, error)
}

// workerTransfer is the production WorkerTransfer: HMAC URLs served by the
// API media route, keys resolved from the workspace media store, bytes read
// back through the object store (local or S3 behind MediaObjects).
type workerTransfer struct {
	assets  WorkerAssetStore
	objects WorkerObjectStore
	origin  string
	secret  string
}

var _ WorkerTransfer = (*workerTransfer)(nil)

// NewWorkerTransfer builds the production presigned-media seam. Empty origin
// or secret fails closed at first use (executor surfaces it before any send).
func NewWorkerTransfer(assets WorkerAssetStore, objects WorkerObjectStore, origin, secret string) WorkerTransfer {
	return &workerTransfer{
		assets: assets, objects: objects,
		origin: strings.TrimRight(strings.TrimSpace(origin), "/"),
		secret: secret,
	}
}

// WorkerSlotContentType returns the bound output slot content-type.
func WorkerSlotContentType() string { return platformworkermedia.SlotContentType }

// WorkerSlotTTL returns the short presigned-URL validity.
func WorkerSlotTTL() time.Duration { return platformworkermedia.SlotTTL }

// MintWorkerOutputKey renders the exact storage key for the single result.
func MintWorkerOutputKey(workspaceID, taskID string) string {
	return platformworkermedia.MintOutputKey(workspaceID, taskID)
}

func (t *workerTransfer) MintOutputKey(workspaceID, taskID string) string {
	return platformworkermedia.MintOutputKey(workspaceID, taskID)
}

func (t *workerTransfer) baseURL() (string, error) {
	if t == nil || strings.TrimSpace(t.origin) == "" {
		return "", fmt.Errorf("worker media origin is not configured")
	}
	if strings.TrimSpace(t.secret) == "" {
		return "", fmt.Errorf("worker media secret is not configured (INTERNAL_SECRET)")
	}
	return t.origin, nil
}

// PresignInputGET mints a presigned GET for one canonical input asset: the
// exact storage key, bound to the asset MIME, short TTL.
func (t *workerTransfer) PresignInputGET(ctx context.Context, workspaceID, publicAssetID string) (string, string, error) {
	if t == nil || t.assets == nil {
		return "", "", fmt.Errorf("worker asset store is not configured")
	}
	asset, err := t.assets.GetAsset(ctx, strings.TrimSpace(workspaceID), strings.TrimSpace(publicAssetID))
	if err != nil {
		return "", "", fmt.Errorf("resolve worker input asset: %w", err)
	}
	if asset == nil || strings.TrimSpace(asset.StorageKey) == "" {
		return "", "", fmt.Errorf("worker input asset has no storage object")
	}
	mimeType := strings.ToLower(strings.TrimSpace(asset.MIMEType))
	if !strings.HasPrefix(mimeType, "image/") {
		return "", "", fmt.Errorf("worker input asset is not an image")
	}
	base, err := t.baseURL()
	if err != nil {
		return "", "", err
	}
	expiresAt := time.Now().UTC().Add(platformworkermedia.SlotTTL)
	signed := platformworkermedia.SignURL(base, t.secret, "GET", asset.StorageKey, mimeType, expiresAt)
	return signed, asset.StorageKey, nil
}

// PresignOutputPUT mints the presigned PUT for the exact output key, bound
// to the slot content-type with short expiry.
func (t *workerTransfer) PresignOutputPUT(key, contentType string) (string, time.Time, error) {
	if t == nil {
		return "", time.Time{}, fmt.Errorf("worker transfer is not configured")
	}
	base, err := t.baseURL()
	if err != nil {
		return "", time.Time{}, err
	}
	key = strings.TrimSpace(key)
	if !platformworkermedia.ValidSlotKey(key) {
		return "", time.Time{}, fmt.Errorf("worker media key is invalid")
	}
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		return "", time.Time{}, fmt.Errorf("worker media slot requires a content type")
	}
	expiresAt := time.Now().UTC().Add(platformworkermedia.SlotTTL)
	return platformworkermedia.SignURL(base, t.secret, "PUT", key, contentType, expiresAt), expiresAt, nil
}

// ImageKeyFromURL recovers the exact key from a landed image URL.
func (t *workerTransfer) ImageKeyFromURL(rawURL string) (string, error) {
	if t == nil {
		return "", fmt.Errorf("worker transfer is not configured")
	}
	return platformworkermedia.ParseKey(rawURL)
}

// VerifyLandedKey parses a landed image URL and enforces exact equality
// with the minted slot key in one call. The Worker echoes upload_url
// without its query, so no signature rides along to verify: safety comes
// from this equality check (plus the PUT-time HMAC verified by the media
// route and the slot-root check on the read path).
func VerifyLandedKey(rawURL, expectedKey string) (string, error) {
	key, err := platformworkermedia.ParseKey(rawURL)
	if err != nil {
		return "", fmt.Errorf("worker image URL does not match the presigned output slot: %v", err)
	}
	expectedKey = strings.TrimSpace(expectedKey)
	if expectedKey == "" || strings.TrimSpace(key) != expectedKey {
		return "", fmt.Errorf("worker image URL does not match the presigned output slot")
	}
	return key, nil
}

// VerifyLandedKey implements WorkerTransfer: parse plus exact-equality
// against the minted slot key in one call.
func (t *workerTransfer) VerifyLandedKey(rawURL, expectedKey string) (string, error) {
	if t == nil {
		return "", fmt.Errorf("worker transfer is not configured")
	}
	return VerifyLandedKey(rawURL, expectedKey)
}

// ReadLandedObject reads back bytes the Worker landed at the exact key.
func (t *workerTransfer) ReadLandedObject(ctx context.Context, key string) ([]byte, string, error) {
	if t == nil || t.objects == nil {
		return nil, "", fmt.Errorf("worker object store is not configured")
	}
	key = strings.TrimSpace(key)
	if !platformworkermedia.ValidSlotKey(key) {
		return nil, "", fmt.Errorf("worker media key is invalid")
	}
	object, err := t.objects.ReadObjectAtProfile(ctx, workspacemedia.DefaultStorageProfile, key, workspacemedia.MaxUploadSizeBytes)
	if err != nil {
		return nil, "", fmt.Errorf("read worker landed object: %w", err)
	}
	if object == nil || len(object.Data) == 0 {
		return nil, "", fmt.Errorf("worker landed object is empty")
	}
	contentType := strings.ToLower(strings.TrimSpace(object.ContentType))
	if contentType == "" {
		contentType = platformworkermedia.SlotContentType
	}
	return object.Data, contentType, nil
}
