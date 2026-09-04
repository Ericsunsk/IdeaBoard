package ports

import (
	"context"
	"fmt"
	"time"

	"ideaboard/api/internal/media/storage/domain"
)

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
