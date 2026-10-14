package workspacemedia

import (
	"context"

	storageports "ideaboard/api/internal/media/storage/ports"
)

// MediaObjects stores original bytes at reserved slots and reads them for
// upload verification and content delivery.
type MediaObjects interface {
	ReadObjectAtProfile(ctx context.Context, profileID, objectKey string, maxBytes int64) (*storageports.StoredUploadObject, error)
	PutObjectAtProfile(ctx context.Context, profileID, objectKey string, data []byte, contentType string) error
}
