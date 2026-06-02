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

func AuthorizeUploadResourceRead(
	ctx context.Context,
	userID int64,
	resource UploadResource,
	boardAccess UploadBoardAccessResolver,
	ownerStorage UploadOwnerStorageUIDResolver,
) error {
	if userID == 0 {
		return ErrUploadAccessDenied
	}
	switch resource.Kind {
	case UploadResourceKindWorkspace:
		return authorizeWorkspaceUploadResourceRead(ctx, userID, resource, boardAccess, ownerStorage)
	case UploadResourceKindProfileAvatar:
		if resource.ProfileAvatar == nil {
			return ErrUploadAccessInvalidPath
		}
		return nil
	case UploadResourceKindTaskOutput:
		return authorizeTaskOutputUploadResourceRead(userID, resource)
	default:
		return ErrUploadAccessInvalidPath
	}
}

func authorizeWorkspaceUploadResourceRead(
	ctx context.Context,
	userID int64,
	resource UploadResource,
	boardAccess UploadBoardAccessResolver,
	ownerStorage UploadOwnerStorageUIDResolver,
) error {
	if resource.Workspace == nil {
		return ErrUploadAccessInvalidPath
	}
	if boardAccess == nil || ownerStorage == nil {
		return ErrUploadAccessDependencyMissing
	}

	ownerUserID, err := boardAccess.ResolveUploadBoardAccess(ctx, userID, resource.Workspace.BoardSegment)
	if err != nil {
		return err
	}
	storageUID, err := ownerStorage.GetStorageUIDByUserID(ctx, ownerUserID)
	if err != nil {
		return err
	}
	ownerSegment, err := NormalizeWorkspaceStorageUserSegment(storageUID)
	if err != nil {
		return err
	}
	if ownerSegment != resource.Workspace.OwnerSegment {
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

func AuthorizeUploadPathRead(
	ctx context.Context,
	userID int64,
	rawPath string,
	boardAccess UploadBoardAccessResolver,
	ownerStorage UploadOwnerStorageUIDResolver,
) (UploadResource, error) {
	resource, err := ParseUploadResource(rawPath)
	if err != nil {
		return UploadResource{}, fmt.Errorf("%w: %v", ErrUploadAccessInvalidPath, err)
	}
	if err := AuthorizeUploadResourceRead(ctx, userID, resource, boardAccess, ownerStorage); err != nil {
		return UploadResource{}, err
	}
	return resource, nil
}
