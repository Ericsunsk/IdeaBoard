package preparation

import (
	"context"
	"fmt"

	"ideaboard/api/internal/media/assets"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/tasking/tasks"
)

type Service struct {
	storage  *storage.StorageService
	registry *assets.MediaAssetRegistry
}

func NewService(storageService *storage.StorageService, registry *assets.MediaAssetRegistry) *Service {
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
	created := make([]assets.PreparedGenerationMedia, 0, len(prepared.Media))
	cleanup := func() {
		for _, media := range created {
			_ = s.storage.DeleteObjectByStorageProfile(context.WithoutCancel(ctx), media.StorageProfile, media.StorageKey)
		}
	}
	for index := range prepared.Media {
		media := &prepared.Media[index]
		if existing, found, err := s.registry.FindPreparedGenerationMedia(ctx, userID, clientActionID, media.Role, media.InputIndex); err != nil {
			cleanup()
			return tasks.GenerationRequestSnapshot{}, err
		} else if found {
			media.AssetID, media.Source = existing.AssetID, ""
			continue
		}
		dirPrefix, err := storage.TaskOutputUploadDirPrefix(userID, "prepare-"+clientActionID)
		if err != nil {
			cleanup()
			return tasks.GenerationRequestSnapshot{}, err
		}
		stored, err := s.storage.StoreMediaRef(ctx, media.Source, storage.StorageStoreOptions{DirPrefix: dirPrefix, AllowHosts: outbound.AllowHosts(ctx)})
		if err != nil {
			cleanup()
			return tasks.GenerationRequestSnapshot{}, fmt.Errorf("prepare media index=%d: %w", media.InputIndex, err)
		}
		item := assets.PreparedGenerationMedia{OwnerUserID: userID, StorageProfile: stored.StorageProfile, StorageKey: stored.StorageKey, URL: stored.URL, SourceURL: media.Source, MIMEType: stored.MIMEType, Width: stored.Width, Height: stored.Height, SizeBytes: stored.SizeBytes, InputIndex: media.InputIndex, Role: media.Role, RefID: clientActionID}
		assetID, err := s.registry.RegisterPreparedGenerationMedia(ctx, item)
		if err != nil {
			_ = s.storage.DeleteObjectByStorageProfile(context.WithoutCancel(ctx), item.StorageProfile, item.StorageKey)
			cleanup()
			return tasks.GenerationRequestSnapshot{}, err
		}
		item.AssetID = assetID
		created = append(created, item)
		media.AssetID, media.Source = assetID, ""
	}
	return prepared, nil
}
