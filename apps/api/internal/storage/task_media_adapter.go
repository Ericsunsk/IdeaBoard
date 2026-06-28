package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"ideaboard/api/internal/taskmedia"
)

type TaskMediaAdapter struct {
	service      *StorageService
	boardAccess  UploadBoardAccessResolver
	ownerStorage UploadOwnerStorageUIDResolver
}

func NewTaskMediaAdapter(
	service *StorageService,
	boardAccess UploadBoardAccessResolver,
	ownerStorage UploadOwnerStorageUIDResolver,
) *TaskMediaAdapter {
	return &TaskMediaAdapter{
		service:      service,
		boardAccess:  boardAccess,
		ownerStorage: ownerStorage,
	}
}

func (a *TaskMediaAdapter) AuthorizeWorkspaceUpload(ctx context.Context, userID int64, rawPath string) error {
	if a == nil {
		return fmt.Errorf("task media storage adapter is not configured")
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

func (a *TaskMediaAdapter) ReadUploadObject(ctx context.Context, ref string, maxBytes int64) (*taskmedia.StoredObject, error) {
	if a == nil || a.service == nil {
		return nil, fmt.Errorf("存储服务未初始化，无法读取上传文件: %q", ref)
	}
	obj, err := a.service.ReadUploadResourceByURL(ctx, ref, maxBytes)
	if err != nil {
		return nil, err
	}
	return taskMediaStoredObject(obj), nil
}

func (a *TaskMediaAdapter) UsesObjectStorage(ctx context.Context) bool {
	return a != nil && a.service != nil && a.service.UsesObjectStorage(ctx)
}

func (a *TaskMediaAdapter) StoreRemoteResult(ctx context.Context, userID int64, taskID string, remoteURL string, opts taskmedia.StoreOptions) (*taskmedia.StoredRef, error) {
	if a == nil || a.service == nil {
		return nil, fmt.Errorf("存储服务未初始化")
	}
	storeOpts := StorageStoreOptions{AllowHosts: opts.AllowHosts}
	if dirPrefix, err := TaskOutputUploadDirPrefix(userID, taskID); err == nil {
		storeOpts.DirPrefix = dirPrefix
	}
	ref, err := a.service.StoreRemoteRef(ctx, remoteURL, storeOpts)
	if err != nil {
		return nil, err
	}
	return taskMediaStoredRef(ref), nil
}

func (a *TaskMediaAdapter) StoreBytesResult(ctx context.Context, userID int64, taskID string, filename string, data []byte, opts taskmedia.StoreOptions) (*taskmedia.StoredRef, error) {
	if a == nil || a.service == nil {
		return nil, fmt.Errorf("存储服务未初始化")
	}
	storeOpts := StorageStoreOptions{AllowHosts: opts.AllowHosts}
	if dirPrefix, err := TaskOutputUploadDirPrefix(userID, taskID); err == nil {
		storeOpts.DirPrefix = dirPrefix
	}
	ref, err := a.service.StoreBytesRef(ctx, filename, data, storeOpts)
	if err != nil {
		return nil, err
	}
	return taskMediaStoredRef(ref), nil
}

func (a *TaskMediaAdapter) DeleteObjectByKey(ctx context.Context, objectKey string) error {
	if a == nil || a.service == nil {
		return fmt.Errorf("存储服务未初始化")
	}
	return a.service.DeleteObjectByKey(ctx, objectKey)
}

func (a *TaskMediaAdapter) DeleteTaskOutputsByUserTask(ctx context.Context, userID int64, taskID string) error {
	if a == nil || a.service == nil {
		return fmt.Errorf("存储服务未初始化")
	}
	return a.service.DeleteTaskOutputsByUserTask(ctx, userID, taskID)
}

func taskMediaStoredObject(obj *StoredUploadObject) *taskmedia.StoredObject {
	if obj == nil {
		return nil
	}
	return &taskmedia.StoredObject{
		ObjectKey:   obj.ObjectKey,
		Filename:    obj.Filename,
		ContentType: obj.ContentType,
		Data:        obj.Data,
		MIMEType:    obj.ContentType,
		Ext:         strings.TrimPrefix(strings.ToLower(filepath.Ext(obj.Filename)), "."),
	}
}

func taskMediaStoredRef(ref *StoredUploadRef) *taskmedia.StoredRef {
	if ref == nil {
		return nil
	}
	return &taskmedia.StoredRef{
		URL:            ref.URL,
		StorageKey:     ref.StorageKey,
		MIMEType:       ref.MIMEType,
		Width:          ref.Width,
		Height:         ref.Height,
		SizeBytes:      ref.SizeBytes,
		StorageProfile: ref.StorageProfile,
	}
}
