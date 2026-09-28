package authorization

import (
	"context"
	"fmt"

	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/ports"
	taskmedia "ideaboard/api/internal/tasking/ports/media"
)

type TaskMediaAuthorizer struct {
	readAccess ports.ReadableUploadAccess
}

var _ taskmedia.UploadAuthorizer = (*TaskMediaAuthorizer)(nil)

func NewTaskMediaAuthorizer(
	boardAccess ports.UploadBoardAccessResolver,
	ownerStorage ports.UploadOwnerStorageUIDResolver,
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
	resource, err := domain.ParseUploadResource(rawPath)
	if err != nil {
		return err
	}
	if resource.Kind != domain.UploadResourceKindCanvas {
		return fmt.Errorf(msgAuthorizeCanvasUpload)
	}
	_, err = a.readAccess.AuthorizeUploadResourceRead(ctx, userID, resource)
	return err
}
