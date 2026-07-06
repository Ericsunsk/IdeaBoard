package storage

import (
	"context"
	"errors"
	"fmt"
	"strconv"
)

var (
	ErrUploadAccessInvalidPath       = errors.New("upload access invalid path")
	ErrUploadAccessDenied            = errors.New("upload access denied")
	ErrUploadAccessOwnerMismatch     = errors.New("upload access owner mismatch")
	ErrUploadAccessDependencyMissing = errors.New("upload access dependency missing")
)

type UploadBoardAccessResolver interface {
	ResolveUploadBoardAccess(ctx context.Context, userID int64, boardID string) (ownerUserID int64, err error)
}

type UploadOwnerStorageUIDResolver interface {
	GetStorageUIDByUserID(ctx context.Context, userID int64) (string, error)
}

type ReadableUploadAccess interface {
	AuthorizeUploadPathRead(ctx context.Context, userID int64, rawPath string) (ReadableUploadAccessResult, error)
	AuthorizeUploadResourceRead(ctx context.Context, userID int64, resource UploadResource) (ReadableUploadAccessResult, error)
}

type ReadableUploadAccessResult struct {
	Resource UploadResource
}

type readableUploadAccess struct {
	boardAccess  UploadBoardAccessResolver
	ownerStorage UploadOwnerStorageUIDResolver
}

func NewReadableUploadAccess(
	boardAccess UploadBoardAccessResolver,
	ownerStorage UploadOwnerStorageUIDResolver,
) ReadableUploadAccess {
	return readableUploadAccess{
		boardAccess:  boardAccess,
		ownerStorage: ownerStorage,
	}
}

func (a readableUploadAccess) AuthorizeUploadPathRead(
	ctx context.Context,
	userID int64,
	rawPath string,
) (ReadableUploadAccessResult, error) {
	resource, err := ParseUploadResource(rawPath)
	if err != nil {
		return ReadableUploadAccessResult{}, fmt.Errorf("%w: %w", ErrUploadAccessInvalidPath, err)
	}
	return a.AuthorizeUploadResourceRead(ctx, userID, resource)
}

func (a readableUploadAccess) AuthorizeUploadResourceRead(
	ctx context.Context,
	userID int64,
	resource UploadResource,
) (ReadableUploadAccessResult, error) {
	if err := authorizeUploadResourceRead(ctx, userID, resource, a.boardAccess, a.ownerStorage); err != nil {
		return ReadableUploadAccessResult{}, err
	}
	return ReadableUploadAccessResult{Resource: resource}, nil
}

func authorizeUploadResourceRead(
	ctx context.Context,
	userID int64,
	resource UploadResource,
	boardAccess UploadBoardAccessResolver,
	ownerStorage UploadOwnerStorageUIDResolver,
) error {
	switch resource.Kind {
	case UploadResourceKindSiteAsset:
		return nil
	case UploadResourceKindCanvas:
		if userID == 0 {
			return ErrUploadAccessDenied
		}
		return authorizeCanvasUploadResourceRead(ctx, userID, resource, boardAccess, ownerStorage)
	case UploadResourceKindProfileAvatar:
		if userID == 0 {
			return ErrUploadAccessDenied
		}
		if resource.ProfileAvatar == nil {
			return ErrUploadAccessInvalidPath
		}
		if ownerStorage == nil {
			return ErrUploadAccessDependencyMissing
		}
		storageUID, err := ownerStorage.GetStorageUIDByUserID(ctx, userID)
		if err != nil {
			return err
		}
		if resource.ProfileAvatar.OwnerSegment != storageUID {
			return ErrUploadAccessOwnerMismatch
		}
		return nil
	case UploadResourceKindTaskOutput:
		if userID == 0 {
			return ErrUploadAccessDenied
		}
		return authorizeTaskOutputUploadResourceRead(userID, resource)
	default:
		return ErrUploadAccessInvalidPath
	}
}

func authorizeCanvasUploadResourceRead(
	ctx context.Context,
	userID int64,
	resource UploadResource,
	boardAccess UploadBoardAccessResolver,
	ownerStorage UploadOwnerStorageUIDResolver,
) error {
	if resource.Canvas == nil {
		return ErrUploadAccessInvalidPath
	}
	if boardAccess == nil || ownerStorage == nil {
		return ErrUploadAccessDependencyMissing
	}

	ownerUserID, err := boardAccess.ResolveUploadBoardAccess(ctx, userID, resource.Canvas.BoardSegment)
	if err != nil {
		return err
	}
	storageUID, err := ownerStorage.GetStorageUIDByUserID(ctx, ownerUserID)
	if err != nil {
		return err
	}
	ownerSegment, err := NormalizeCanvasStorageUserSegment(storageUID)
	if err != nil {
		return err
	}
	if ownerSegment != resource.Canvas.OwnerSegment {
		return ErrUploadAccessOwnerMismatch
	}
	return nil
}

func authorizeTaskOutputUploadResourceRead(userID int64, resource UploadResource) error {
	if resource.TaskOutput == nil {
		return ErrUploadAccessInvalidPath
	}
	resourceUserID, err := strconv.ParseInt(resource.TaskOutput.UserSegment, 10, 64)
	if err != nil || resourceUserID == 0 {
		return ErrUploadAccessInvalidPath
	}
	if resourceUserID != userID {
		return ErrUploadAccessDenied
	}
	return nil
}
