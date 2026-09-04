package authorization

import (
	"context"
	"fmt"
	"strconv"

	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/ports"
)

type ReadableUploadAccess struct {
	boardAccess  ports.UploadBoardAccessResolver
	ownerStorage ports.UploadOwnerStorageUIDResolver
}

var _ ports.ReadableUploadAccess = (*ReadableUploadAccess)(nil)

func NewReadableUploadAccess(
	boardAccess ports.UploadBoardAccessResolver,
	ownerStorage ports.UploadOwnerStorageUIDResolver,
) *ReadableUploadAccess {
	return &ReadableUploadAccess{
		boardAccess:  boardAccess,
		ownerStorage: ownerStorage,
	}
}

func (a *ReadableUploadAccess) AuthorizeUploadPathRead(
	ctx context.Context,
	userID int64,
	rawPath string,
) (ports.ReadableUploadAccessResult, error) {
	resource, err := domain.ParseUploadResource(rawPath)
	if err != nil {
		return ports.ReadableUploadAccessResult{}, fmt.Errorf("%w: %w", domain.ErrUploadAccessInvalidPath, err)
	}
	return a.AuthorizeUploadResourceRead(ctx, userID, resource)
}

func (a *ReadableUploadAccess) AuthorizeUploadResourceRead(
	ctx context.Context,
	userID int64,
	resource domain.UploadResource,
) (ports.ReadableUploadAccessResult, error) {
	if err := authorizeUploadResourceRead(ctx, userID, resource, a.boardAccess, a.ownerStorage); err != nil {
		return ports.ReadableUploadAccessResult{}, err
	}
	return ports.ReadableUploadAccessResult{Resource: resource}, nil
}

func authorizeUploadResourceRead(
	ctx context.Context,
	userID int64,
	resource domain.UploadResource,
	boardAccess ports.UploadBoardAccessResolver,
	ownerStorage ports.UploadOwnerStorageUIDResolver,
) error {
	switch resource.Kind {
	case domain.UploadResourceKindSiteAsset:
		return nil
	case domain.UploadResourceKindCanvas:
		if userID == 0 {
			return domain.ErrUploadAccessDenied
		}
		return authorizeCanvasUploadResourceRead(ctx, userID, resource, boardAccess, ownerStorage)
	case domain.UploadResourceKindProfileAvatar:
		if userID == 0 {
			return domain.ErrUploadAccessDenied
		}
		if resource.ProfileAvatar == nil {
			return domain.ErrUploadAccessInvalidPath
		}
		if ownerStorage == nil {
			return domain.ErrUploadAccessDependencyMissing
		}
		storageUID, err := ownerStorage.GetStorageUIDByUserID(ctx, userID)
		if err != nil {
			return err
		}
		if resource.ProfileAvatar.OwnerSegment != storageUID {
			return domain.ErrUploadAccessOwnerMismatch
		}
		return nil
	case domain.UploadResourceKindTaskOutput:
		if userID == 0 {
			return domain.ErrUploadAccessDenied
		}
		return authorizeTaskOutputUploadResourceRead(userID, resource)
	default:
		return domain.ErrUploadAccessInvalidPath
	}
}

func authorizeCanvasUploadResourceRead(
	ctx context.Context,
	userID int64,
	resource domain.UploadResource,
	boardAccess ports.UploadBoardAccessResolver,
	ownerStorage ports.UploadOwnerStorageUIDResolver,
) error {
	if resource.Canvas == nil {
		return domain.ErrUploadAccessInvalidPath
	}
	if boardAccess == nil || ownerStorage == nil {
		return domain.ErrUploadAccessDependencyMissing
	}

	ownerUserID, err := boardAccess.ResolveUploadBoardAccess(ctx, userID, resource.Canvas.BoardSegment)
	if err != nil {
		return err
	}
	storageUID, err := ownerStorage.GetStorageUIDByUserID(ctx, ownerUserID)
	if err != nil {
		return err
	}
	ownerSegment, err := domain.NormalizeCanvasStorageUserSegment(storageUID)
	if err != nil {
		return err
	}
	if ownerSegment != resource.Canvas.OwnerSegment {
		return domain.ErrUploadAccessOwnerMismatch
	}
	return nil
}

func authorizeTaskOutputUploadResourceRead(userID int64, resource domain.UploadResource) error {
	if resource.TaskOutput == nil {
		return domain.ErrUploadAccessInvalidPath
	}
	resourceUserID, err := strconv.ParseInt(resource.TaskOutput.UserSegment, 10, 64)
	if err != nil || resourceUserID == 0 {
		return domain.ErrUploadAccessInvalidPath
	}
	if resourceUserID != userID {
		return domain.ErrUploadAccessDenied
	}
	return nil
}
