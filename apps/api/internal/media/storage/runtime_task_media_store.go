package storage

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/media/ingestion"
	"ideaboard/api/internal/platform/database"
	tasks "ideaboard/api/internal/tasking/domain"
	taskmedia "ideaboard/api/internal/tasking/ports/media"
)

type RuntimeTaskMediaStore struct {
	service  *StorageService
	registry MediaIngestionRegistry
}

type MediaIngestionRegistry interface {
	Begin(context.Context, ingestion.Request) (ingestion.Handle, error)
	Complete(context.Context, ingestion.Handle, ingestion.Completion) error
	Fail(context.Context, ingestion.Handle, error) error
	FindReadyOwnedMediaAssetByChecksum(context.Context, int64, string, string) (dbsqlc.MediaAsset, bool, error)
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
	if params.Media == nil {
		return nil, nil
	}
	refs, err := validateRuntimeMediaManifest(params)
	if err != nil {
		return nil, err
	}
	urls, err := s.presignRuntimeMedia(ctx, refs)
	if err != nil {
		return nil, err
	}
	return assembleRuntimeMedia(params.Media, urls)
}

func validateRuntimeMediaManifest(params *tasks.WorkflowTaskParams) ([]tasks.TaskInputManifestRef, error) {
	manifest := params.InputManifest
	if manifest == nil {
		return nil, fmt.Errorf("任务媒体 manifest 缺失")
	}
	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("任务媒体 manifest 无效: %w", err)
	}
	refs := manifest.ResolveInputs()
	expectedRefs := len(params.Media.Inputs) + len(params.Media.References)
	if params.Media.Original != nil {
		expectedRefs++
	}
	if params.Media.Mask != nil {
		expectedRefs++
	}
	if len(refs) != expectedRefs {
		return nil, fmt.Errorf("任务媒体 manifest 与 media params 不匹配")
	}
	return refs, nil
}

func (s *RuntimeTaskMediaStore) presignRuntimeMedia(ctx context.Context, refs []tasks.TaskInputManifestRef) ([]string, error) {
	urls := make([]string, len(refs))
	for index, ref := range refs {
		presigned, err := s.service.Presign(ctx, PresignRequest{StorageProfileID: ref.StorageProfileID, ObjectKey: ref.ObjectKey, TTL: DefaultPresignTTL, ExpectedMIMEType: ref.MIMEType, ExpectedChecksumSHA256: ref.ChecksumSHA256, ExpectedSizeBytes: ref.SizeBytes, ExpectedObjectGeneration: ref.ObjectGeneration, Caller: "task-provider"})
		if err != nil {
			return nil, fmt.Errorf("presign task media %d: %w", index, err)
		}
		if presigned.ObjectGeneration != ref.ObjectGeneration || presigned.StorageProfileID != ref.StorageProfileID {
			return nil, fmt.Errorf("任务媒体 object generation/profile mismatch")
		}
		urls[index] = presigned.URL
	}
	return urls, nil
}

func assembleRuntimeMedia(media *tasks.MediaInputs, urls []string) (*tasks.RuntimeMediaInputs, error) {
	position := 0
	next := func() (string, error) {
		if position >= len(urls) {
			return "", fmt.Errorf("任务媒体 manifest 与 media params 不匹配")
		}
		url := urls[position]
		position++
		return url, nil
	}
	resolved := &tasks.RuntimeMediaInputs{UseMask: media.UseMask}
	for index, ref := range media.Inputs {
		url, err := next()
		if err != nil {
			return nil, fmt.Errorf("解析 input 媒体失败: index=%d: %w", index, err)
		}
		resolved.Inputs = append(resolved.Inputs, tasks.RuntimeMediaRef{URL: url, Role: ref.Role, Weight: ref.Weight})
	}
	for index, ref := range media.References {
		url, err := next()
		if err != nil {
			return nil, fmt.Errorf("解析 reference 媒体失败: index=%d: %w", index, err)
		}
		resolved.References = append(resolved.References, tasks.RuntimeMediaRef{URL: url, Role: ref.Role, Weight: ref.Weight})
	}
	if ref := media.Original; ref != nil {
		url, err := next()
		if err != nil {
			return nil, fmt.Errorf("解析 original 媒体失败: %w", err)
		}
		resolved.Original = &tasks.RuntimeMediaRef{URL: url, Role: ref.Role, Weight: ref.Weight}
	}
	if ref := media.Mask; ref != nil {
		url, err := next()
		if err != nil {
			return nil, fmt.Errorf("解析 mask 媒体失败: %w", err)
		}
		resolved.Mask = &tasks.RuntimeMediaRef{URL: url, Role: ref.Role, Weight: ref.Weight}
	}
	if position != len(urls) {
		return nil, fmt.Errorf("任务媒体 manifest 与 media params 不匹配")
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
		completion := ref.IngestionCompletion(sourceURL)
		if reused, ok, err := s.reuseReadyAsset(ctx, handle, completion); err != nil {
			s.failTaskResult(ctx, handle, err)
			return taskmedia.MaterializedResults{}, fmt.Errorf("查找重复任务结果媒体失败: index=%d: %w", index, err)
		} else if ok {
			items = append(items, taskmedia.MaterializedResult{ImageURL: reused.Url, Output: taskOutputFromMediaAsset(taskID, reused, sourceURL, index)})
			continue
		}
		if err := s.registry.Complete(ctx, handle, completion); err != nil {
			if database.IsDuplicateConstraintError(err) {
				if reused, ok, lookupErr := s.reuseReadyAsset(ctx, handle, completion); lookupErr == nil && ok {
					items = append(items, taskmedia.MaterializedResult{ImageURL: reused.Url, Output: taskOutputFromMediaAsset(taskID, reused, sourceURL, index)})
					continue
				}
			}
			s.failTaskResult(ctx, handle, err)
			return taskmedia.MaterializedResults{}, fmt.Errorf("完成任务结果媒体摄取失败: index=%d: %w", index, err)
		}
		output := taskOutputFromStoredUploadRef(taskID, ref, sourceURL, index)
		output.MediaAssetID = &handle.AssetID
		output.AssetID = strings.TrimSpace(handle.PublicAssetID)
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
	completion := ref.IngestionCompletion(sourceURL)
	if reused, ok, err := s.reuseReadyAsset(ctx, handle, completion); err != nil {
		s.failTaskResult(ctx, handle, err)
		return taskmedia.MaterializedResults{}, fmt.Errorf("查找重复任务结果媒体失败: %w", err)
	} else if ok {
		return taskmedia.MaterializedResults{Items: []taskmedia.MaterializedResult{{ImageURL: reused.Url, Output: taskOutputFromMediaAsset(taskID, reused, sourceURL, 0)}}}, nil
	}
	if err := s.registry.Complete(ctx, handle, completion); err != nil {
		if database.IsDuplicateConstraintError(err) {
			if reused, ok, lookupErr := s.reuseReadyAsset(ctx, handle, completion); lookupErr == nil && ok {
				return taskmedia.MaterializedResults{Items: []taskmedia.MaterializedResult{{ImageURL: reused.Url, Output: taskOutputFromMediaAsset(taskID, reused, sourceURL, 0)}}}, nil
			}
		}
		s.failTaskResult(ctx, handle, err)
		return taskmedia.MaterializedResults{}, fmt.Errorf("完成任务结果媒体摄取失败: %w", err)
	}
	output := taskOutputFromStoredUploadRef(taskID, ref, sourceURL, 0)
	output.MediaAssetID = &handle.AssetID
	output.AssetID = strings.TrimSpace(handle.PublicAssetID)
	return taskmedia.MaterializedResults{
		Items: []taskmedia.MaterializedResult{{
			ImageURL: ref.URL,
			Output:   output,
		}},
	}, nil
}

// reuseReadyAsset converges concurrent task-result materialization on the
// owner's canonical ready asset. The newly downloaded object is discarded and
// its uploading asset is failed after the winner has been identified.
func (s *RuntimeTaskMediaStore) reuseReadyAsset(ctx context.Context, handle ingestion.Handle, completion ingestion.Completion) (dbsqlc.MediaAsset, bool, error) {
	if strings.TrimSpace(completion.ChecksumSHA256) == "" {
		return dbsqlc.MediaAsset{}, false, nil
	}
	existing, found, err := s.registry.FindReadyOwnedMediaAssetByChecksum(ctx, handle.OwnerUserID, handle.StorageProfile, completion.ChecksumSHA256)
	if err != nil || !found {
		return dbsqlc.MediaAsset{}, found, err
	}
	if handle.ExistingReady || existing.ID == handle.AssetID {
		return existing, true, nil
	}
	cleanupCtx := context.WithoutCancel(ctx)
	if err := s.service.DeleteObjectByStorageProfile(cleanupCtx, handle.StorageProfile, handle.StorageKey); err != nil {
		slog.Warn("discard duplicate task result object failed", "asset_id", handle.AssetID, "storage_profile", handle.StorageProfile, "storage_key", handle.StorageKey, "err", err)
	}
	if err := s.registry.Fail(cleanupCtx, handle, fmt.Errorf("任务结果复用已有媒体 Asset %d", existing.ID)); err != nil {
		slog.Error("mark duplicate task result ingestion failed", "asset_id", handle.AssetID, "err", err)
	}
	return existing, true, nil
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
	if err := s.service.DeleteObjectByStorageProfile(cleanupCtx, handle.StorageProfile, handle.StorageKey); err != nil {
		slog.Warn("cleanup failed task result object failed", "asset_id", handle.AssetID, "storage_profile", handle.StorageProfile, "storage_key", handle.StorageKey, "err", err)
	}
	if err := s.registry.Fail(cleanupCtx, handle, cause); err != nil {
		slog.Error("mark failed task result ingestion failed", "asset_id", handle.AssetID, "err", err)
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
		ChecksumSHA256: ref.ChecksumSHA256,
		Animated:       ref.Animated,
		SourceURL:      sourceURL,
		StorageProfile: ref.StorageProfile,
		ResultIndex:    resultIndex,
	}
}

func taskOutputFromMediaAsset(taskID string, asset dbsqlc.MediaAsset, sourceURL string, resultIndex int) tasks.TaskOutput {
	assetID := asset.ID
	return tasks.TaskOutput{TaskID: taskID, Role: "image", URL: asset.Url, StorageKey: asset.StorageKey,
		MIMEType: asset.MimeType, Width: int(asset.Width), Height: int(asset.Height), SizeBytes: asset.SizeBytes,
		ChecksumSHA256: asset.ChecksumSha256, SourceURL: sourceURL, StorageProfile: asset.StorageProfileID, ResultIndex: resultIndex,
		MediaAssetID: &assetID, AssetID: asset.AssetID}
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
