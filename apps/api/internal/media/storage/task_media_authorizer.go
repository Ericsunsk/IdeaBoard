package storage

import (
	"context"
	"fmt"

	"ideaboard/api/internal/tasking/taskmedia"
)

type TaskMediaAuthorizer struct {
	readAccess ReadableUploadAccess
}

var _ taskmedia.UploadAuthorizer = (*TaskMediaAuthorizer)(nil)

func NewTaskMediaAuthorizer(
	boardAccess UploadBoardAccessResolver,
	ownerStorage UploadOwnerStorageUIDResolver,
) *TaskMediaAuthorizer {
	return &TaskMediaAuthorizer{
		readAccess: NewReadableUploadAccess(boardAccess, ownerStorage),
	}
}

func (a *TaskMediaAuthorizer) AuthorizeCanvasUpload(ctx context.Context, userID int64, rawPath string) error {
	if a == nil {
		return fmt.Errorf("task media authorizer is not configured")
	}
	if a.readAccess == nil {
		return fmt.Errorf("task media readable upload access is not configured")
	}
	resource, err := ParseUploadResource(rawPath)
	if err != nil {
		return err
	}
	if resource.Kind != UploadResourceKindCanvas {
		return fmt.Errorf("仅支持画布上传资源")
	}
	_, err = a.readAccess.AuthorizeUploadResourceRead(ctx, userID, resource)
	return err
}
