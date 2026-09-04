package ports

import "context"

type Verifier interface {
	Verify(ctx context.Context, storageProfileID, storageKey, checksum, mimeType string, sizeBytes int64) error
}
