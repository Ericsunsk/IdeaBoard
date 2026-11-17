package workertransfer

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/media/imageformat"
	storageports "ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/media/workspacemedia"
	platformworkermedia "ideaboard/api/internal/platform/workermedia"
)

// WorkerTransfer owns the presigned-media handoff. Output slots retain their
// exact key and image policy; callers receive only a verified image result.
type WorkerTransfer interface {
	// PresignInputGET mints a presigned GET for one canonical input asset.
	PresignInputGET(ctx context.Context, workspaceID, publicAssetID string) (url, key string, err error)
	PrepareOutput(workspaceID, taskID string, resultIndex int) (*WorkerOutputSlot, error)
}

// WorkerOutputSlot binds one upload to its trusted result. Its key, policy and
// object reader are private so acceptance cannot skip exact-key verification.
type WorkerOutputSlot struct {
	uploadURL string
	key       string
	objects   WorkerObjectStore
}

func (s *WorkerOutputSlot) UploadURL() string {
	if s == nil {
		return ""
	}
	return s.uploadURL
}

// Accept verifies the echoed URL before any bounded object read, then checks
// complete image bytes and agreement between actual, stored and reported MIME.
// The PUT route independently verifies the upload's HMAC and image integrity.
func (s *WorkerOutputSlot) Accept(ctx context.Context, imageURL, contentType string) (string, error) {
	if s == nil || s.objects == nil || !platformworkermedia.ValidSlotKey(s.key) {
		return "", fmt.Errorf("worker output slot is not configured")
	}
	key, err := platformworkermedia.ParseKey(imageURL)
	if err != nil || key != s.key {
		return "", fmt.Errorf("worker image URL does not match the presigned output slot")
	}
	object, err := s.objects.ReadObjectAtProfile(ctx, workspacemedia.DefaultStorageProfile, s.key, workspacemedia.MaxUploadSizeBytes)
	if err != nil {
		return "", fmt.Errorf("read worker landed object: %w", err)
	}
	if object == nil || len(object.Data) == 0 || int64(len(object.Data)) > workspacemedia.MaxUploadSizeBytes {
		return "", fmt.Errorf("worker landed object is empty or exceeds size limit")
	}
	facts, err := imageformat.InspectCompleteDetectedBytes(object.Data)
	if err != nil || !platformworkermedia.AllowedOutputContentType(facts.MIMEType) ||
		strings.ToLower(strings.TrimSpace(object.ContentType)) != facts.MIMEType || contentType != facts.MIMEType {
		return "", fmt.Errorf("worker landed image content type or bytes mismatch")
	}
	return "data:" + facts.MIMEType + ";base64," + base64.StdEncoding.EncodeToString(object.Data), nil
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

func (t *workerTransfer) baseURL() (string, error) {
	if t == nil || strings.TrimSpace(t.origin) == "" {
		return "", fmt.Errorf("worker media origin is not configured")
	}
	if strings.TrimSpace(t.secret) == "" {
		return "", fmt.Errorf("worker media secret is not configured (WORKER_INTERNAL_SECRET)")
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

// PrepareOutput creates one indexed fixed-policy slot and keeps its storage key
// inside the acceptance handle. An unconfigured object reader fails before send.
func (t *workerTransfer) PrepareOutput(workspaceID, taskID string, resultIndex int) (*WorkerOutputSlot, error) {
	if resultIndex < 0 {
		return nil, fmt.Errorf("worker output index must not be negative")
	}
	if t == nil || t.objects == nil {
		return nil, fmt.Errorf("worker object store is not configured")
	}
	base, err := t.baseURL()
	if err != nil {
		return nil, err
	}
	key := platformworkermedia.MintOutputKey(workspaceID, taskID, resultIndex)
	if !platformworkermedia.ValidSlotKey(key) {
		return nil, fmt.Errorf("worker media key is invalid")
	}
	expiresAt := time.Now().UTC().Add(platformworkermedia.SlotTTL)
	return &WorkerOutputSlot{
		uploadURL: platformworkermedia.SignURL(base, t.secret, "PUT", key, platformworkermedia.SlotImagePolicy, expiresAt),
		key:       key,
		objects:   t.objects,
	}, nil
}
