package taskmedia

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/media/ingestion"
	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/platform/database"
	tasks "ideaboard/api/internal/tasking/domain"
	portmedia "ideaboard/api/internal/tasking/ports/media"
)

type StorageRuntime interface {
	ReadUploadResourceByURL(ctx context.Context, rawURL string, maxBytes int64) (*ports.StoredUploadObject, error)
	Presign(ctx context.Context, req ports.PresignRequest) (ports.PresignedURL, error)
	StoreRemoteRefAtProfile(ctx context.Context, profileID, remoteURL string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error)
	StoreBytesRefAtProfile(ctx context.Context, profileID, filename string, data []byte, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error)
	PrepareUploadRef(ctx context.Context, filename string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error)
	DeleteObjectByStorageProfile(ctx context.Context, storageProfileID string, objectKey string) error
}

type RuntimeTaskMediaStore struct {
	runtime  StorageRuntime
	registry ports.MediaIngestionRegistry
}

var _ portmedia.ResultMaterializer = (*RuntimeTaskMediaStore)(nil)
var _ portmedia.UploadReader = (*RuntimeTaskMediaStore)(nil)
var _ portmedia.RuntimeMediaResolver = (*RuntimeTaskMediaStore)(nil)

func NewRuntimeTaskMediaStore(runtime StorageRuntime, registry ports.MediaIngestionRegistry) *RuntimeTaskMediaStore {
	return &RuntimeTaskMediaStore{runtime: runtime, registry: registry}
}

func (s *RuntimeTaskMediaStore) ReadUploadObject(ctx context.Context, ref string, maxBytes int64) (*portmedia.StoredObject, error) {
	if s == nil || s.runtime == nil {
		return nil, fmt.Errorf("存储服务未初始化，无法读取上传文件: %q", ref)
	}
	obj, err := s.runtime.ReadUploadResourceByURL(ctx, ref, maxBytes)
	if err != nil {
		return nil, err
	}
	return taskMediaStoredObject(obj), nil
}

func (s *RuntimeTaskMediaStore) ResolveRuntimeMedia(ctx context.Context, params *tasks.WorkflowTaskParams) (*tasks.RuntimeMediaInputs, error) {
	if params == nil {
		return nil, fmt.Errorf("任务媒体参数为空")
	}
	if s == nil || s.runtime == nil {
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
		presigned, err := s.runtime.Presign(ctx, ports.PresignRequest{
			StorageProfileID:         ref.StorageProfileID,
			ObjectKey:                ref.ObjectKey,
			TTL:                      ports.DefaultPresignTTL,
			ExpectedMIMEType:         ref.MIMEType,
			ExpectedChecksumSHA256:   ref.ChecksumSHA256,
			ExpectedSizeBytes:        ref.SizeBytes,
			ExpectedObjectGeneration: ref.ObjectGeneration,
			Caller:                   "task-provider",
		})
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

func (s *RuntimeTaskMediaStore) MaterializeRemoteResults(ctx context.Context, userID int64, taskID string, sourceURLs []string, opts portmedia.ResultMaterializeOptions) (portmedia.MaterializedResults, error) {
	if len(sourceURLs) == 0 {
		return portmedia.MaterializedResults{}, nil
	}
	if s == nil || s.runtime == nil || s.registry == nil {
		return portmedia.MaterializedResults{}, fmt.Errorf("存储服务未初始化")
	}

	items := make([]portmedia.MaterializedResult, 0, len(sourceURLs))
	for index, sourceURL := range sourceURLs {
		handle, err := s.beginTaskResult(ctx, userID, taskID, index, sourceURL)
		if err != nil {
			return portmedia.MaterializedResults{}, err
		}
		ref, err := s.runtime.StoreRemoteRefAtProfile(ctx, handle.StorageProfile, sourceURL, ports.StorageStoreOptions{ObjectKey: handle.StorageKey, AllowHosts: opts.AllowHosts})
		if err != nil {
			s.failTaskResult(ctx, handle, err)
			return portmedia.MaterializedResults{}, fmt.Errorf("持久化任务结果图片失败: index=%d: %w", index, err)
		}
		if ref == nil {
			err = fmt.Errorf("存储引用为空")
			s.failTaskResult(ctx, handle, err)
			return portmedia.MaterializedResults{}, fmt.Errorf("持久化任务结果图片失败: index=%d: 存储引用为空", index)
		}
		completion := ref.IngestionCompletion(sourceURL)
		if reused, ok, err := s.reuseReadyAsset(ctx, handle, completion); err != nil {
			s.failTaskResult(ctx, handle, err)
			return portmedia.MaterializedResults{}, fmt.Errorf("查找重复任务结果媒体失败: index=%d: %w", index, err)
		} else if ok {
			items = append(items, portmedia.MaterializedResult{ImageURL: reused.Url, Output: taskOutputFromMediaAsset(taskID, reused, sourceURL, index)})
			continue
		}
		if err := s.registry.Complete(ctx, handle, completion); err != nil {
			if database.IsDuplicateConstraintError(err) {
				if reused, ok, lookupErr := s.reuseReadyAsset(ctx, handle, completion); lookupErr == nil && ok {
					items = append(items, portmedia.MaterializedResult{ImageURL: reused.Url, Output: taskOutputFromMediaAsset(taskID, reused, sourceURL, index)})
					continue
				}
			}
			s.failTaskResult(ctx, handle, err)
			return portmedia.MaterializedResults{}, fmt.Errorf("完成任务结果媒体摄取失败: index=%d: %w", index, err)
		}
		output := taskOutputFromStoredUploadRef(taskID, ref, sourceURL, index)
		output.MediaAssetID = &handle.AssetID
		output.AssetID = strings.TrimSpace(handle.PublicAssetID)
		items = append(items, portmedia.MaterializedResult{
			ImageURL: ref.URL,
			Output:   output,
		})
	}
	return portmedia.MaterializedResults{Items: items}, nil
}

func (s *RuntimeTaskMediaStore) MaterializeBytesResult(ctx context.Context, userID int64, taskID string, filename string, sourceURL string, data []byte, opts portmedia.ResultMaterializeOptions) (portmedia.MaterializedResults, error) {
	if s == nil || s.runtime == nil || s.registry == nil {
		return portmedia.MaterializedResults{}, fmt.Errorf("存储服务未初始化")
	}
	handle, err := s.beginTaskResult(ctx, userID, taskID, 0, sourceURL)
	if err != nil {
		return portmedia.MaterializedResults{}, err
	}

	ref, err := s.runtime.StoreBytesRefAtProfile(ctx, handle.StorageProfile, filename, data, ports.StorageStoreOptions{ObjectKey: handle.StorageKey, AllowHosts: opts.AllowHosts})
	if err != nil {
		s.failTaskResult(ctx, handle, err)
		return portmedia.MaterializedResults{}, fmt.Errorf("持久化任务结果图片失败: %w", err)
	}
	if ref == nil {
		err = fmt.Errorf("存储引用为空")
		s.failTaskResult(ctx, handle, err)
		return portmedia.MaterializedResults{}, fmt.Errorf("持久化任务结果图片失败: 存储引用为空")
	}
	completion := ref.IngestionCompletion(sourceURL)
	if reused, ok, err := s.reuseReadyAsset(ctx, handle, completion); err != nil {
		s.failTaskResult(ctx, handle, err)
		return portmedia.MaterializedResults{}, fmt.Errorf("查找重复任务结果媒体失败: %w", err)
	} else if ok {
		return portmedia.MaterializedResults{Items: []portmedia.MaterializedResult{{ImageURL: reused.Url, Output: taskOutputFromMediaAsset(taskID, reused, sourceURL, 0)}}}, nil
	}
	if err := s.registry.Complete(ctx, handle, completion); err != nil {
		if database.IsDuplicateConstraintError(err) {
			if reused, ok, lookupErr := s.reuseReadyAsset(ctx, handle, completion); lookupErr == nil && ok {
				return portmedia.MaterializedResults{Items: []portmedia.MaterializedResult{{ImageURL: reused.Url, Output: taskOutputFromMediaAsset(taskID, reused, sourceURL, 0)}}}, nil
			}
		}
		s.failTaskResult(ctx, handle, err)
		return portmedia.MaterializedResults{}, fmt.Errorf("完成任务结果媒体摄取失败: %w", err)
	}
	output := taskOutputFromStoredUploadRef(taskID, ref, sourceURL, 0)
	output.MediaAssetID = &handle.AssetID
	output.AssetID = strings.TrimSpace(handle.PublicAssetID)
	return portmedia.MaterializedResults{
		Items: []portmedia.MaterializedResult{{
			ImageURL: ref.URL,
			Output:   output,
		}},
	}, nil
}

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
	if err := s.runtime.DeleteObjectByStorageProfile(cleanupCtx, handle.StorageProfile, handle.StorageKey); err != nil {
		slog.Warn("discard duplicate task result object failed", "asset_id", handle.AssetID, "storage_profile", handle.StorageProfile, "storage_key", handle.StorageKey, "err", err)
	}
	if err := s.registry.Fail(cleanupCtx, handle, fmt.Errorf("任务结果复用已有媒体 Asset %d", existing.ID)); err != nil {
		slog.Error("mark duplicate task result ingestion failed", "asset_id", handle.AssetID, "err", err)
	}
	return existing, true, nil
}

func (s *RuntimeTaskMediaStore) beginTaskResult(ctx context.Context, userID int64, taskID string, resultIndex int, sourceURL string) (ingestion.Handle, error) {
	dirPrefix, err := domain.TaskOutputUploadDirPrefix(userID, taskID)
	if err != nil {
		return ingestion.Handle{}, err
	}
	objectKey, err := domain.StableUploadObjectKey(dirPrefix, fmt.Sprintf("result-%d", resultIndex))
	if err != nil {
		return ingestion.Handle{}, err
	}
	planned, err := s.runtime.PrepareUploadRef(ctx, "result", ports.StorageStoreOptions{ObjectKey: objectKey})
	if err != nil {
		return ingestion.Handle{}, err
	}
	handle, err := s.registry.Begin(ctx, ingestion.Request{
		OwnerUserID:    userID,
		StorageProfile: planned.StorageProfile,
		StorageKey:     planned.StorageKey,
		URL:            planned.URL,
		SourceURL:      sourceURL,
		Origin:         ingestion.TaskResultFacts{TaskID: taskID, ResultIndex: resultIndex},
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
	if err := s.runtime.DeleteObjectByStorageProfile(cleanupCtx, handle.StorageProfile, handle.StorageKey); err != nil {
		slog.Warn("cleanup failed task result object failed", "asset_id", handle.AssetID, "storage_profile", handle.StorageProfile, "storage_key", handle.StorageKey, "err", err)
	}
	if err := s.registry.Fail(cleanupCtx, handle, cause); err != nil {
		slog.Error("mark failed task result ingestion failed", "asset_id", handle.AssetID, "err", err)
	}
}

func taskOutputFromStoredUploadRef(taskID string, ref *ports.StoredUploadRef, sourceURL string, resultIndex int) tasks.TaskOutput {
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
	return tasks.TaskOutput{
		TaskID:         taskID,
		Role:           "image",
		URL:            asset.Url,
		StorageKey:     asset.StorageKey,
		MIMEType:       asset.MimeType,
		Width:          int(asset.Width),
		Height:         int(asset.Height),
		SizeBytes:      asset.SizeBytes,
		ChecksumSHA256: asset.ChecksumSha256,
		SourceURL:      sourceURL,
		StorageProfile: asset.StorageProfileID,
		ResultIndex:    resultIndex,
		MediaAssetID:   &assetID,
		AssetID:        asset.AssetID,
	}
}

func taskMediaStoredObject(obj *ports.StoredUploadObject) *portmedia.StoredObject {
	if obj == nil {
		return nil
	}
	return &portmedia.StoredObject{
		ObjectKey:   obj.ObjectKey,
		Filename:    obj.Filename,
		ContentType: obj.ContentType,
		Data:        obj.Data,
		MIMEType:    obj.ContentType,
		Ext:         strings.TrimPrefix(strings.ToLower(filepath.Ext(obj.Filename)), "."),
	}
}
