package ports

import (
	"context"
	"fmt"
	"mime/multipart"
	"time"

	"ideaboard/api/internal/media/ingestion"
	"ideaboard/api/internal/media/storage/domain"
)

// --- Storage Store DTOs & Options ---

type StorageStoreOptions struct {
	DirPrefix  string
	AllowHosts []string
	ObjectKey  string
}

type StoredUploadRef struct {
	URL            string
	StorageKey     string
	StorageProfile string
	MIMEType       string
	Width          int
	Height         int
	SizeBytes      int64
	ChecksumSHA256 string
	Animated       bool
}

func (r StoredUploadRef) IngestionCompletion(sourceURL string) ingestion.Completion {
	return ingestion.Completion{
		StorageKey:     r.StorageKey,
		URL:            r.URL,
		SourceURL:      sourceURL,
		MIMEType:       r.MIMEType,
		Width:          r.Width,
		Height:         r.Height,
		SizeBytes:      r.SizeBytes,
		ChecksumSHA256: r.ChecksumSHA256,
		Animated:       r.Animated,
	}
}

type StoredUploadObject struct {
	ObjectKey   string
	Filename    string
	ContentType string
	Data        []byte
}

// --- Driver Interface ---

type StorageDriver interface {
	Provider() domain.StorageProvider
	UsesObjectStorage() bool
	IsProfileReady(profile domain.StorageProfile) bool
	TestProfile(ctx context.Context, profile *domain.StorageProfile) error
	GetBucketInfo(ctx context.Context, profile *domain.StorageProfile) (map[string]interface{}, error)
	UploadRef(ctx context.Context, profile *domain.StorageProfile, file *multipart.FileHeader, opts StorageStoreOptions) (*StoredUploadRef, error)
	Delete(ctx context.Context, profile *domain.StorageProfile, objectKey string) error
	DeleteByPrefix(ctx context.Context, profile *domain.StorageProfile, objectKeyPrefix string) error
	ListObjectKeysByPrefix(ctx context.Context, profile *domain.StorageProfile, objectKeyPrefix string) ([]string, error)
	PutObject(ctx context.Context, profile *domain.StorageProfile, objectKey string, data []byte, contentType string) error
	StoreBytesRef(ctx context.Context, profile *domain.StorageProfile, filename string, data []byte, opts StorageStoreOptions) (*StoredUploadRef, error)
	StoreRemoteRef(ctx context.Context, profile *domain.StorageProfile, remoteURL string, opts StorageStoreOptions) (*StoredUploadRef, error)
	Read(ctx context.Context, profile *domain.StorageProfile, objectKey string, maxBytes int64) (*StoredUploadObject, error)
	SignedURL(ctx context.Context, profile *domain.StorageProfile, objectKey string, expiry time.Duration) (string, error)
	ProviderURL(profile *domain.StorageProfile, objectKey string) (string, error)
	VerifyObjectGeneration(ctx context.Context, profile *domain.StorageProfile, objectKey string, expected int64) error
}

// --- Presigning ---

type PresignRequest struct {
	StorageProfileID         string
	ObjectKey                string
	TTL                      time.Duration
	ExpectedMIMEType         string
	ExpectedChecksumSHA256   string
	ExpectedSizeBytes        int64
	ExpectedObjectGeneration int64
	Caller                   string
}

type PresignedURL struct {
	URL              string
	ExpiresAt        time.Time
	ObjectGeneration int64
	StorageProfileID string
	PresignID        string
}

const (
	DefaultPresignTTL = 5 * time.Minute
	MaxPresignTTL     = 5 * time.Minute
)

func ClampTTL(requested time.Duration) (time.Duration, error) {
	if requested <= 0 {
		return DefaultPresignTTL, nil
	}
	if requested > MaxPresignTTL {
		return MaxPresignTTL, fmt.Errorf("%w (requested %s, capped to %s)", domain.ErrPresignTTLTooLong, requested, MaxPresignTTL)
	}
	return requested, nil
}

type ObjectStore interface {
	Presign(ctx context.Context, req PresignRequest) (PresignedURL, error)
}

// --- Profile Repository ---

type ProfileRepository interface {
	ListProfiles(ctx context.Context, includeDeleted bool) ([]domain.StorageProfile, error)
	ListProfilesWithSecrets(ctx context.Context, includeDeleted bool) ([]domain.StorageProfile, error)
	SaveProfile(ctx context.Context, profile domain.StorageProfile) error
	SaveAndApplyProfile(ctx context.Context, profile domain.StorageProfile) error
	DeleteProfile(ctx context.Context, id string) error
	ApplyProfile(ctx context.Context, id string) error
}
