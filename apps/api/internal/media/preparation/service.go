package preparation

import (
	"context"
	"fmt"
	"strings"

	"ideaboard/api/internal/media/assets"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/tasking/tasks"
)

type Service struct {
	storage  mediaStore
	registry mediaRegistry
}

type mediaStore interface {
	PrepareUploadRef(context.Context, string, storage.StorageStoreOptions) (*storage.StoredUploadRef, error)
	StoreMediaRefAtProfile(context.Context, string, string, storage.StorageStoreOptions) (*storage.StoredUploadRef, error)
	DeleteObjectByStorageProfile(context.Context, string, string) error
}

type mediaRegistry interface {
	FindPreparedGenerationMedia(context.Context, int64, string, string, int) (assets.PreparedGenerationMedia, bool, error)
	FindIngestingPreparedGenerationMedia(context.Context, int64, string, string, int) (assets.PreparedGenerationMedia, bool, error)
	FindReusableGenerationMedia(context.Context, int64, string) (int64, bool, error)
	BeginPreparedGenerationMedia(context.Context, assets.PreparedGenerationMedia) (int64, error)
	CompletePreparedGenerationMedia(context.Context, assets.PreparedGenerationMedia, int64) error
	FailPreparedGenerationMedia(context.Context, int64, int64, error) error
}

func NewService(storageService mediaStore, registry mediaRegistry) *Service {
	return &Service{storage: storageService, registry: registry}
}

func (s *Service) PrepareGenerationMedia(ctx context.Context, userID int64, clientActionID string, snapshot *tasks.GenerationRequestSnapshot) (tasks.GenerationRequestSnapshot, error) {
	if snapshot == nil || len(snapshot.Media) == 0 {
		return tasks.GenerationRequestSnapshot{}, fmt.Errorf("generation media snapshot is empty")
	}
	if s == nil || s.storage == nil || s.registry == nil {
		return tasks.GenerationRequestSnapshot{}, fmt.Errorf("generation media preparer is unavailable")
	}
	prepared := *snapshot
	prepared.Media = append([]tasks.GenerationMediaIntent(nil), snapshot.Media...)
	for index := range prepared.Media {
		media := &prepared.Media[index]
		if existing, found, err := s.registry.FindPreparedGenerationMedia(ctx, userID, clientActionID, media.Role, media.InputIndex); err != nil {
			return tasks.GenerationRequestSnapshot{}, err
		} else if found {
			media.AssetID, media.Source = existing.AssetID, ""
			continue
		}
		reusableAssetID, found, err := s.registry.FindReusableGenerationMedia(ctx, userID, media.Source)
		if err != nil {
			return tasks.GenerationRequestSnapshot{}, err
		}
		if found {
			media.AssetID, media.Source = reusableAssetID, ""
			continue
		}
		item, found, err := s.registry.FindIngestingPreparedGenerationMedia(ctx, userID, clientActionID, media.Role, media.InputIndex)
		if err != nil {
			return tasks.GenerationRequestSnapshot{}, err
		}
		if !found {
			item, err = s.beginPreparedMedia(ctx, userID, clientActionID, *media)
			if err != nil {
				return tasks.GenerationRequestSnapshot{}, err
			}
		}
		stored, err := s.storage.StoreMediaRefAtProfile(ctx, item.StorageProfile, media.Source, storage.StorageStoreOptions{ObjectKey: item.StorageKey, AllowHosts: outbound.AllowHosts(ctx)})
		if err != nil {
			_ = s.registry.FailPreparedGenerationMedia(context.WithoutCancel(ctx), userID, item.AssetID, err)
			return tasks.GenerationRequestSnapshot{}, fmt.Errorf("prepare media index=%d: %w", media.InputIndex, err)
		}
		item.URL, item.SourceURL, item.MIMEType, item.Animated = stored.URL, media.Source, stored.MIMEType, stored.Animated
		item.Width, item.Height, item.SizeBytes, item.ChecksumSHA256 = stored.Width, stored.Height, stored.SizeBytes, stored.ChecksumSHA256
		if err := s.registry.CompletePreparedGenerationMedia(ctx, item, item.AssetID); err != nil {
			_ = s.storage.DeleteObjectByStorageProfile(context.WithoutCancel(ctx), item.StorageProfile, item.StorageKey)
			_ = s.registry.FailPreparedGenerationMedia(context.WithoutCancel(ctx), userID, item.AssetID, err)
			return tasks.GenerationRequestSnapshot{}, err
		}
		media.AssetID, media.Source = item.AssetID, ""
	}
	return prepared, nil
}

func (s *Service) beginPreparedMedia(ctx context.Context, userID int64, clientActionID string, media tasks.GenerationMediaIntent) (assets.PreparedGenerationMedia, error) {
	dirPrefix, err := storage.TaskOutputUploadDirPrefix(userID, "prepare-"+clientActionID)
	if err != nil {
		return assets.PreparedGenerationMedia{}, err
	}
	filename := fmt.Sprintf("%s-%d", strings.TrimSpace(media.Role), media.InputIndex)
	objectKey, err := storage.StableUploadObjectKey(dirPrefix, filename)
	if err != nil {
		return assets.PreparedGenerationMedia{}, err
	}
	planned, err := s.storage.PrepareUploadRef(ctx, filename, storage.StorageStoreOptions{ObjectKey: objectKey})
	if err != nil {
		return assets.PreparedGenerationMedia{}, err
	}
	item := assets.PreparedGenerationMedia{
		OwnerUserID: userID, StorageProfile: planned.StorageProfile, StorageKey: planned.StorageKey,
		URL: planned.URL, SourceURL: media.Source, InputIndex: media.InputIndex, Role: media.Role, RefID: clientActionID,
	}
	item.AssetID, err = s.registry.BeginPreparedGenerationMedia(ctx, item)
	if err != nil {
		return assets.PreparedGenerationMedia{}, err
	}
	return item, nil
}
