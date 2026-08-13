package storage

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"ideaboard/api/internal/tasking/taskmedia"
	"ideaboard/api/internal/tasking/tasks"
)

type RuntimeTaskMediaStore struct {
	service *StorageService
}

var _ taskmedia.ResultMaterializer = (*RuntimeTaskMediaStore)(nil)
var _ taskmedia.UploadReader = (*RuntimeTaskMediaStore)(nil)

func NewRuntimeTaskMediaStore(service *StorageService) *RuntimeTaskMediaStore {
	return &RuntimeTaskMediaStore{service: service}
}

func (s *RuntimeTaskMediaStore) ReadUploadObject(ctx context.Context, ref string, maxBytes int64) (*taskmedia.StoredObject, error) {
	if s == nil || s.service == nil {
		return nil, fmt.Errorf("存储服务未初始化，无法读取上传文件: %q", ref)
	}
	obj, err := s.service.ReadUploadResourceByURL(ctx, ref, maxBytes)
	if err != nil {
		return nil, err
	}
	return taskMediaStoredObject(obj), nil
}

func (s *RuntimeTaskMediaStore) MaterializeRemoteResults(ctx context.Context, userID int64, taskID string, sourceURLs []string, opts taskmedia.ResultMaterializeOptions) (taskmedia.MaterializedResults, error) {
	if len(sourceURLs) == 0 {
		return taskmedia.MaterializedResults{}, nil
	}
	if s == nil || s.service == nil {
		return taskmedia.MaterializedResults{}, fmt.Errorf("存储服务未初始化")
	}
	storeOpts, err := taskMediaResultStoreOptions(userID, taskID, opts.AllowHosts)
	if err != nil {
		return taskmedia.MaterializedResults{}, err
	}

	items := make([]taskmedia.MaterializedResult, 0, len(sourceURLs))
	storedKeys := make([]string, 0, len(sourceURLs))
	for index, sourceURL := range sourceURLs {
		ref, err := s.service.StoreRemoteRef(ctx, sourceURL, storeOpts)
		if err != nil {
			s.cleanupMaterializedResultKeys(ctx, taskID, storedKeys)
			return taskmedia.MaterializedResults{}, fmt.Errorf("持久化任务结果图片失败: index=%d: %w", index, err)
		}
		if ref == nil {
			s.cleanupMaterializedResultKeys(ctx, taskID, storedKeys)
			return taskmedia.MaterializedResults{}, fmt.Errorf("持久化任务结果图片失败: index=%d: 存储引用为空", index)
		}
		items = append(items, taskmedia.MaterializedResult{
			ImageURL: ref.URL,
			Output:   taskOutputFromStoredUploadRef(taskID, ref, sourceURL, len(items)),
		})
		if strings.TrimSpace(ref.StorageKey) != "" {
			storedKeys = append(storedKeys, ref.StorageKey)
		}
	}
	return taskmedia.MaterializedResults{Items: items}, nil
}

func (s *RuntimeTaskMediaStore) MaterializeBytesResult(ctx context.Context, userID int64, taskID string, filename string, sourceURL string, data []byte, opts taskmedia.ResultMaterializeOptions) (taskmedia.MaterializedResults, error) {
	if s == nil || s.service == nil {
		return taskmedia.MaterializedResults{}, fmt.Errorf("存储服务未初始化")
	}
	storeOpts, err := taskMediaResultStoreOptions(userID, taskID, opts.AllowHosts)
	if err != nil {
		return taskmedia.MaterializedResults{}, err
	}

	ref, err := s.service.StoreBytesRef(ctx, filename, data, storeOpts)
	if err != nil {
		return taskmedia.MaterializedResults{}, fmt.Errorf("持久化任务结果图片失败: %w", err)
	}
	if ref == nil {
		return taskmedia.MaterializedResults{}, fmt.Errorf("持久化任务结果图片失败: 存储引用为空")
	}
	return taskmedia.MaterializedResults{
		Items: []taskmedia.MaterializedResult{{
			ImageURL: ref.URL,
			Output:   taskOutputFromStoredUploadRef(taskID, ref, sourceURL, 0),
		}},
	}, nil
}

func taskMediaResultStoreOptions(userID int64, taskID string, allowHosts []string) (StorageStoreOptions, error) {
	dirPrefix, err := TaskOutputUploadDirPrefix(userID, taskID)
	if err != nil {
		return StorageStoreOptions{}, err
	}
	return StorageStoreOptions{
		DirPrefix:  dirPrefix,
		AllowHosts: allowHosts,
	}, nil
}

func (s *RuntimeTaskMediaStore) cleanupMaterializedResultKeys(ctx context.Context, taskID string, storageKeys []string) {
	for _, storageKey := range storageKeys {
		if strings.TrimSpace(storageKey) == "" {
			continue
		}
		if err := s.service.DeleteObjectByKey(ctx, storageKey); err != nil {
			slog.Warn("cleanup orphan materialized image failed", "task_id", taskID, "storage_key", storageKey, "err", err)
		}
	}
}

func taskOutputFromStoredUploadRef(taskID string, ref *StoredUploadRef, sourceURL string, resultIndex int) tasks.TaskOutput {
	if ref == nil {
		return tasks.TaskOutput{TaskID: taskID, Role: "image", SourceURL: sourceURL, ResultIndex: resultIndex}
	}
	return tasks.TaskOutput{
		TaskID:         taskID,
		Role:           "image",
		URL:            ref.URL,
		StorageKey:     ref.StorageKey,
		MIMEType:       ref.MIMEType,
		Width:          ref.Width,
		Height:         ref.Height,
		SizeBytes:      ref.SizeBytes,
		SourceURL:      sourceURL,
		StorageProfile: ref.StorageProfile,
		ResultIndex:    resultIndex,
	}
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
