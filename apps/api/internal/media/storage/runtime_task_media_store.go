package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"ideaboard/api/internal/media/ingestion"
	"ideaboard/api/internal/tasking/taskmedia"
	"ideaboard/api/internal/tasking/tasks"
)

type RuntimeTaskMediaStore struct {
	service  *StorageService
	registry MediaIngestionRegistry
}

type MediaIngestionRegistry interface {
	Begin(context.Context, ingestion.Request) (ingestion.Handle, error)
	Complete(context.Context, ingestion.Handle, ingestion.Completion) error
	Fail(context.Context, ingestion.Handle, error) error
}

var _ taskmedia.ResultMaterializer = (*RuntimeTaskMediaStore)(nil)
var _ taskmedia.UploadReader = (*RuntimeTaskMediaStore)(nil)
var _ taskmedia.RuntimeMediaResolver = (*RuntimeTaskMediaStore)(nil)

func NewRuntimeTaskMediaStore(service *StorageService, registry MediaIngestionRegistry) *RuntimeTaskMediaStore {
	return &RuntimeTaskMediaStore{service: service, registry: registry}
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

func (s *RuntimeTaskMediaStore) ResolveRuntimeMedia(ctx context.Context, params *tasks.WorkflowTaskParams) (*tasks.RuntimeMediaInputs, error) {
	if params == nil {
		return nil, fmt.Errorf("任务媒体参数为空")
	}
	if s == nil || s.service == nil {
		return nil, fmt.Errorf("存储服务未初始化")
	}
	urls := make(map[int64]string)
	resolve := func(assetID int64) (string, error) {
		if assetID <= 0 {
			return "", fmt.Errorf("媒体 AssetID 缺失")
		}
		if existing := urls[assetID]; existing != "" {
			return existing, nil
		}
		providerURL, err := s.service.ResolveProviderAssetURL(ctx, assetID)
		if err != nil {
			return "", err
		}
		urls[assetID] = providerURL
		return providerURL, nil
	}

	if params.Media == nil {
		return nil, nil
	}
	resolved := &tasks.RuntimeMediaInputs{UseMask: params.Media.UseMask}
	for index, ref := range params.Media.Inputs {
		url, err := resolve(ref.AssetID)
		if err != nil {
			return nil, fmt.Errorf("解析 input 媒体失败: index=%d: %w", index, err)
		}
		resolved.Inputs = append(resolved.Inputs, tasks.RuntimeMediaRef{URL: url, Role: ref.Role, Weight: ref.Weight})
	}
	for index, ref := range params.Media.References {
		url, err := resolve(ref.AssetID)
		if err != nil {
			return nil, fmt.Errorf("解析 reference 媒体失败: index=%d: %w", index, err)
		}
		resolved.References = append(resolved.References, tasks.RuntimeMediaRef{URL: url, Role: ref.Role, Weight: ref.Weight})
	}
	if ref := params.Media.Original; ref != nil {
		url, err := resolve(ref.AssetID)
		if err != nil {
			return nil, fmt.Errorf("解析 original 媒体失败: %w", err)
		}
		resolved.Original = &tasks.RuntimeMediaRef{URL: url, Role: ref.Role, Weight: ref.Weight}
	}
	if ref := params.Media.Mask; ref != nil {
		url, err := resolve(ref.AssetID)
		if err != nil {
			return nil, fmt.Errorf("解析 mask 媒体失败: %w", err)
		}
		resolved.Mask = &tasks.RuntimeMediaRef{URL: url, Role: ref.Role, Weight: ref.Weight}
	}
	return resolved, nil
}

func (s *RuntimeTaskMediaStore) MaterializeRemoteResults(ctx context.Context, userID int64, taskID string, sourceURLs []string, opts taskmedia.ResultMaterializeOptions) (taskmedia.MaterializedResults, error) {
	if len(sourceURLs) == 0 {
		return taskmedia.MaterializedResults{}, nil
	}
	if s == nil || s.service == nil || s.registry == nil {
		return taskmedia.MaterializedResults{}, fmt.Errorf("存储服务未初始化")
	}

	items := make([]taskmedia.MaterializedResult, 0, len(sourceURLs))
	for index, sourceURL := range sourceURLs {
		handle, err := s.beginTaskResult(ctx, userID, taskID, index, sourceURL)
		if err != nil {
			return taskmedia.MaterializedResults{}, err
		}
		ref, err := s.service.storeRemoteRefAtProfile(ctx, handle.StorageProfile, sourceURL, StorageStoreOptions{ObjectKey: handle.StorageKey, AllowHosts: opts.AllowHosts})
		if err != nil {
			s.failTaskResult(ctx, handle, err)
			return taskmedia.MaterializedResults{}, fmt.Errorf("持久化任务结果图片失败: index=%d: %w", index, err)
		}
		if ref == nil {
			err = fmt.Errorf("存储引用为空")
			s.failTaskResult(ctx, handle, err)
			return taskmedia.MaterializedResults{}, fmt.Errorf("持久化任务结果图片失败: index=%d: 存储引用为空", index)
		}
		if err := s.registry.Complete(ctx, handle, ref.IngestionCompletion(sourceURL)); err != nil {
			s.failTaskResult(ctx, handle, err)
			return taskmedia.MaterializedResults{}, fmt.Errorf("完成任务结果媒体摄取失败: index=%d: %w", index, err)
		}
		output := taskOutputFromStoredUploadRef(taskID, ref, sourceURL, index)
		output.MediaAssetID = &handle.AssetID
		items = append(items, taskmedia.MaterializedResult{
			ImageURL: ref.URL,
			Output:   output,
		})
	}
	return taskmedia.MaterializedResults{Items: items}, nil
}

func (s *RuntimeTaskMediaStore) MaterializeBytesResult(ctx context.Context, userID int64, taskID string, filename string, sourceURL string, data []byte, opts taskmedia.ResultMaterializeOptions) (taskmedia.MaterializedResults, error) {
	if s == nil || s.service == nil || s.registry == nil {
		return taskmedia.MaterializedResults{}, fmt.Errorf("存储服务未初始化")
	}
	handle, err := s.beginTaskResult(ctx, userID, taskID, 0, sourceURL)
	if err != nil {
		return taskmedia.MaterializedResults{}, err
	}

	ref, err := s.service.storeBytesRefAtProfile(ctx, handle.StorageProfile, filename, data, StorageStoreOptions{ObjectKey: handle.StorageKey, AllowHosts: opts.AllowHosts})
	if err != nil {
		s.failTaskResult(ctx, handle, err)
		return taskmedia.MaterializedResults{}, fmt.Errorf("持久化任务结果图片失败: %w", err)
	}
	if ref == nil {
		err = fmt.Errorf("存储引用为空")
		s.failTaskResult(ctx, handle, err)
		return taskmedia.MaterializedResults{}, fmt.Errorf("持久化任务结果图片失败: 存储引用为空")
	}
	if err := s.registry.Complete(ctx, handle, ref.IngestionCompletion(sourceURL)); err != nil {
		s.failTaskResult(ctx, handle, err)
		return taskmedia.MaterializedResults{}, fmt.Errorf("完成任务结果媒体摄取失败: %w", err)
	}
	output := taskOutputFromStoredUploadRef(taskID, ref, sourceURL, 0)
	output.MediaAssetID = &handle.AssetID
	return taskmedia.MaterializedResults{
		Items: []taskmedia.MaterializedResult{{
			ImageURL: ref.URL,
			Output:   output,
		}},
	}, nil
}

func (s *RuntimeTaskMediaStore) beginTaskResult(ctx context.Context, userID int64, taskID string, resultIndex int, sourceURL string) (ingestion.Handle, error) {
	dirPrefix, err := TaskOutputUploadDirPrefix(userID, taskID)
	if err != nil {
		return ingestion.Handle{}, err
	}
	objectKey, err := StableUploadObjectKey(dirPrefix, fmt.Sprintf("result-%d", resultIndex))
	if err != nil {
		return ingestion.Handle{}, err
	}
	planned, err := s.service.PrepareUploadRef(ctx, "result", StorageStoreOptions{ObjectKey: objectKey})
	if err != nil {
		return ingestion.Handle{}, err
	}
	handle, err := s.registry.Begin(ctx, ingestion.Request{
		OwnerUserID: userID, StorageProfile: planned.StorageProfile, StorageKey: planned.StorageKey,
		URL: planned.URL, SourceURL: sourceURL,
		Origin: ingestion.TaskResultFacts{TaskID: taskID, ResultIndex: resultIndex},
	})
	if err != nil {
		return ingestion.Handle{}, fmt.Errorf("登记任务结果媒体失败: index=%d: %w", resultIndex, err)
	}
	return handle, nil
}

func (s *RuntimeTaskMediaStore) failTaskResult(ctx context.Context, handle ingestion.Handle, cause error) {
	if handle.ExistingReady {
		return
	}
	cleanupCtx := context.WithoutCancel(ctx)
	_ = s.service.DeleteObjectByStorageProfile(cleanupCtx, handle.StorageProfile, handle.StorageKey)
	_ = s.registry.Fail(cleanupCtx, handle, cause)
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
		ChecksumSHA256: ref.ChecksumSHA256,
		Animated:       ref.Animated,
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
