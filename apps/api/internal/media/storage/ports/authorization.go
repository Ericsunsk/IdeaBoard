package ports

import (
	"context"

	"ideaboard/api/internal/media/storage/domain"
)

type UploadBoardAccessResolver interface {
	ResolveUploadBoardAccess(ctx context.Context, userID int64, boardID string) (ownerUserID int64, err error)
}

type UploadOwnerStorageUIDResolver interface {
	GetStorageUIDByUserID(ctx context.Context, userID int64) (string, error)
}

type ReadableUploadAccess interface {
	AuthorizeUploadPathRead(ctx context.Context, userID int64, rawPath string) (ReadableUploadAccessResult, error)
	AuthorizeUploadResourceRead(ctx context.Context, userID int64, resource domain.UploadResource) (ReadableUploadAccessResult, error)
}

type ReadableUploadAccessResult struct {
	Resource domain.UploadResource
}
