package preparation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ideaboard/api/internal/media/assets"
	"ideaboard/api/internal/media/ingestion"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/platform/database"
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
	FindReadyOwnedMediaAssetIDByChecksum(context.Context, int64, string, string) (int64, bool, error)
	Begin(context.Context, ingestion.Request) (ingestion.Handle, error)
	Complete(context.Context, ingestion.Handle, ingestion.Completion) error
	Fail(context.Context, ingestion.Handle, error) error
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
		// AssetID-first submissions already reference a durable, authorized asset.
		// Do not rematerialize it or attempt URL-based reuse.
		if media.AssetID > 0 && strings.TrimSpace(media.Source) == "" {
			continue
		}
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
		var handle ingestion.Handle
		if !found {
			handle, err = s.beginPreparedMedia(ctx, userID, clientActionID, *media)
			if err != nil {
				return tasks.GenerationRequestSnapshot{}, err
			}
			item = assets.PreparedGenerationMedia{AssetID: handle.AssetID, StorageProfile: handle.StorageProfile, StorageKey: handle.StorageKey}
		} else {
			handle = preparedMediaHandle(userID, clientActionID, *media, item)
		}
		stored, err := s.storage.StoreMediaRefAtProfile(ctx, item.StorageProfile, media.Source, storage.StorageStoreOptions{ObjectKey: item.StorageKey, AllowHosts: outbound.AllowHosts(ctx)})
		if err != nil {
			_ = s.registry.Fail(context.WithoutCancel(ctx), handle, err)
			return tasks.GenerationRequestSnapshot{}, fmt.Errorf("prepare media index=%d: %w", media.InputIndex, err)
		}
		completion := stored.IngestionCompletion(media.Source)
		// Signed delivery URLs rotate, so URL-based reuse can miss identical bytes.
		// Check before completion and again after a unique conflict to close the concurrent-writer race.
		if strings.TrimSpace(completion.ChecksumSHA256) != "" {
			if existingAssetID, found, err := s.registry.FindReadyOwnedMediaAssetIDByChecksum(ctx, userID, item.StorageProfile, completion.ChecksumSHA256); err != nil {
				cleanupErr := s.discardPreparedMedia(ctx, handle, err)
				return tasks.GenerationRequestSnapshot{}, errors.Join(fmt.Errorf("find prepared media checksum duplicate: %w", err), cleanupErr)
			} else if found {
				if cleanupErr := s.discardPreparedMedia(ctx, handle, fmt.Errorf("prepared media reused existing asset %d", existingAssetID)); cleanupErr != nil {
					return tasks.GenerationRequestSnapshot{}, fmt.Errorf("discard checksum-equivalent prepared media: %w", cleanupErr)
				}
				media.AssetID, media.Source = existingAssetID, ""
				continue
			}
		}
		if err := s.registry.Complete(ctx, handle, completion); err != nil {
			if database.IsDuplicateConstraintError(err) && strings.TrimSpace(completion.ChecksumSHA256) != "" {
				existingAssetID, found, lookupErr := s.registry.FindReadyOwnedMediaAssetIDByChecksum(ctx, userID, item.StorageProfile, completion.ChecksumSHA256)
				if lookupErr == nil && found {
					if cleanupErr := s.discardPreparedMedia(ctx, handle, err); cleanupErr != nil {
						return tasks.GenerationRequestSnapshot{}, errors.Join(err, fmt.Errorf("discard checksum-conflicting prepared media: %w", cleanupErr))
					}
					media.AssetID, media.Source = existingAssetID, ""
					continue
				}
				if lookupErr != nil {
					err = fmt.Errorf("%w; find checksum conflict winner: %v", err, lookupErr)
				}
			}
			return tasks.GenerationRequestSnapshot{}, errors.Join(err, s.discardPreparedMedia(ctx, handle, err))
		}
		media.AssetID, media.Source = item.AssetID, ""
	}
	return prepared, nil
}

func (s *Service) discardPreparedMedia(ctx context.Context, handle ingestion.Handle, cause error) error {
	cleanupCtx := context.WithoutCancel(ctx)
	deleteErr := s.storage.DeleteObjectByStorageProfile(cleanupCtx, handle.StorageProfile, handle.StorageKey)
	failErr := s.registry.Fail(cleanupCtx, handle, cause)
	return errors.Join(deleteErr, failErr)
}

func (s *Service) beginPreparedMedia(ctx context.Context, userID int64, clientActionID string, media tasks.GenerationMediaIntent) (ingestion.Handle, error) {
	dirPrefix, err := storage.TaskOutputUploadDirPrefix(userID, "prepare-"+clientActionID)
	if err != nil {
		return ingestion.Handle{}, err
	}
	filename := fmt.Sprintf("%s-%d", strings.TrimSpace(media.Role), media.InputIndex)
	objectKey, err := storage.StableUploadObjectKey(dirPrefix, filename)
	if err != nil {
		return ingestion.Handle{}, err
	}
	planned, err := s.storage.PrepareUploadRef(ctx, filename, storage.StorageStoreOptions{ObjectKey: objectKey})
	if err != nil {
		return ingestion.Handle{}, err
	}
	handle, err := s.registry.Begin(ctx, ingestion.Request{
		OwnerUserID: userID, StorageProfile: planned.StorageProfile, StorageKey: planned.StorageKey,
		URL: planned.URL, SourceURL: media.Source,
		Origin: ingestion.GenerationPreparationFacts{RefID: clientActionID, Role: media.Role, InputIndex: media.InputIndex},
	})
	if err != nil {
		return ingestion.Handle{}, err
	}
	return handle, nil
}

func preparedMediaHandle(userID int64, clientActionID string, media tasks.GenerationMediaIntent, item assets.PreparedGenerationMedia) ingestion.Handle {
	return ingestion.Handle{
		AssetID: item.AssetID, OwnerUserID: userID, StorageProfile: item.StorageProfile, StorageKey: item.StorageKey,
		Origin: ingestion.GenerationPreparationFacts{RefID: clientActionID, Role: media.Role, InputIndex: media.InputIndex},
	}
}
