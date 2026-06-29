package storage

import (
	"context"
	"fmt"

	"ideaboard/api/internal/tasking/taskmedia"
)

type TaskMediaAuthorizer struct {
	boardAccess  UploadBoardAccessResolver
	ownerStorage UploadOwnerStorageUIDResolver
}

var _ taskmedia.UploadAuthorizer = (*TaskMediaAuthorizer)(nil)

func NewTaskMediaAuthorizer(
	boardAccess UploadBoardAccessResolver,
	ownerStorage UploadOwnerStorageUIDResolver,
) *TaskMediaAuthorizer {
	return &TaskMediaAuthorizer{
		boardAccess:  boardAccess,
		ownerStorage: ownerStorage,
	}
}

func (a *TaskMediaAuthorizer) AuthorizeWorkspaceUpload(ctx context.Context, userID int64, rawPath string) error {
	if a == nil {
		return fmt.Errorf("task media authorizer is not configured")
	}
	resource, err := ParseUploadResource(rawPath)
	if err != nil {
		return err
	}
	if resource.Kind != UploadResourceKindWorkspace {
		return fmt.Errorf("仅支持工作区上传资源")
	}
	return AuthorizeUploadResourceRead(ctx, userID, resource, a.boardAccess, a.ownerStorage)
}
