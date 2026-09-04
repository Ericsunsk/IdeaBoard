package preparation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ideaboard/api/internal/media/assets"
	"ideaboard/api/internal/media/ingestion"
	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/outbound"
	tasks "ideaboard/api/internal/tasking/domain"
)

type Service struct {
	storage  mediaStore
	registry assets.PreparationRegistry
}

type mediaStore interface {
	PrepareUploadRef(context.Context, string, ports.StorageStoreOptions) (*ports.StoredUploadRef, error)
	StoreMediaRefAtProfile(context.Context, string, string, ports.StorageStoreOptions) (*ports.StoredUploadRef, error)
	DeleteObjectByStorageProfile(context.Context, string, string) error
}

func NewService(storageService mediaStore, registry assets.PreparationRegistry) *Service {
	return &Service{storage: storageService, registry: registry}
}

func (s *Service) PrepareGenerationMedia(ctx context.Context, userID int64, clientActionID string, snapshot *tasks.GenerationRequestSnapshot) (tasks.GenerationRequestSnapshot, error) {
	if err := validatePreparationRequest(s, snapshot); err != nil {
		return tasks.GenerationRequestSnapshot{}, err
	}
	prepared := cloneGenerationSnapshot(snapshot)
	for index := range prepared.Media {
		media := &prepared.Media[index]
		if err := s.prepareGenerationMediaIntent(ctx, userID, clientActionID, media); err != nil {
			return tasks.GenerationRequestSnapshot{}, err
		}
	}
	return prepared, nil
}

func validatePreparationRequest(s *Service, snapshot *tasks.GenerationRequestSnapshot) error {
	if snapshot == nil || len(snapshot.Media) == 0 {
		return fmt.Errorf("generation media snapshot is empty")
	}
	if s == nil || s.storage == nil || s.registry == nil {
		return fmt.Errorf("generation media preparer is unavailable")
	}
	return nil
}

func cloneGenerationSnapshot(snapshot *tasks.GenerationRequestSnapshot) tasks.GenerationRequestSnapshot {
	prepared := *snapshot
	prepared.Media = append([]tasks.GenerationMediaIntent(nil), snapshot.Media...)
	return prepared
}

func (s *Service) prepareGenerationMediaIntent(ctx context.Context, userID int64, clientActionID string, media *tasks.GenerationMediaIntent) error {
	// AssetID-first submissions already reference a durable, authorized asset.
	// Do not rematerialize it or attempt URL-based reuse.
	if media.AssetID > 0 && strings.TrimSpace(media.Source) == "" {
		return nil
	}
	if handled, err := s.reusePreparedGenerationMedia(ctx, userID, clientActionID, media); err != nil || handled {
		return err
	}
	return s.materializePreparedGenerationMedia(ctx, userID, clientActionID, media)
}

func (s *Service) reusePreparedGenerationMedia(ctx context.Context, userID int64, clientActionID string, media *tasks.GenerationMediaIntent) (bool, error) {
	existing, found, err := s.registry.FindPreparedGenerationMedia(ctx, userID, clientActionID, media.Role, media.InputIndex)
	if err != nil {
		return false, err
	}
	if found {
		media.AssetID, media.Source = existing.AssetID, ""
		return true, nil
	}
	reusableAssetID, found, err := s.registry.FindReusableGenerationMedia(ctx, userID, media.Source)
	if err != nil {
		return false, err
	}
	if found {
		media.AssetID, media.Source = reusableAssetID, ""
	}
	return found, nil
}

func (s *Service) materializePreparedGenerationMedia(ctx context.Context, userID int64, clientActionID string, media *tasks.GenerationMediaIntent) error {
	item, handle, err := s.resolvePreparedMediaHandle(ctx, userID, clientActionID, *media)
	if err != nil {
		return err
	}
	stored, err := s.storage.StoreMediaRefAtProfile(ctx, item.StorageProfile, media.Source, ports.StorageStoreOptions{ObjectKey: item.StorageKey, AllowHosts: outbound.AllowHosts(ctx)})
	if err != nil {
		_ = s.registry.Fail(context.WithoutCancel(ctx), handle, err)
		return fmt.Errorf("prepare media index=%d: %w", media.InputIndex, err)
	}
	completion := stored.IngestionCompletion(media.Source)
	if handled, err := s.reuseChecksumEquivalentMedia(ctx, userID, item, handle, completion, media); err != nil || handled {
		return err
	}
	reused, err := s.completePreparedMedia(ctx, userID, item, handle, completion, media)
	if err != nil {
		return err
	}
	if reused {
		return nil
	}
	media.AssetID, media.Source = item.AssetID, ""
	return nil
}

func (s *Service) resolvePreparedMediaHandle(ctx context.Context, userID int64, clientActionID string, media tasks.GenerationMediaIntent) (assets.PreparedGenerationMedia, ingestion.Handle, error) {
	item, found, err := s.registry.FindIngestingPreparedGenerationMedia(ctx, userID, clientActionID, media.Role, media.InputIndex)
	if err != nil {
		return assets.PreparedGenerationMedia{}, ingestion.Handle{}, err
	}
	if found {
		return item, preparedMediaHandle(userID, clientActionID, media, item), nil
	}
	handle, err := s.beginPreparedMedia(ctx, userID, clientActionID, media)
	if err != nil {
		return assets.PreparedGenerationMedia{}, ingestion.Handle{}, err
	}
	return assets.PreparedGenerationMedia{AssetID: handle.AssetID, StorageProfile: handle.StorageProfile, StorageKey: handle.StorageKey}, handle, nil
}

func (s *Service) reuseChecksumEquivalentMedia(ctx context.Context, userID int64, item assets.PreparedGenerationMedia, handle ingestion.Handle, completion ingestion.Completion, media *tasks.GenerationMediaIntent) (bool, error) {
	if strings.TrimSpace(completion.ChecksumSHA256) == "" {
		return false, nil
	}
	existingAssetID, found, err := s.registry.FindReadyOwnedMediaAssetIDByChecksum(ctx, userID, item.StorageProfile, completion.ChecksumSHA256)
	if err != nil {
		cleanupErr := s.discardPreparedMedia(ctx, handle, err)
		return false, errors.Join(fmt.Errorf("find prepared media checksum duplicate: %w", err), cleanupErr)
	}
	if !found {
		return false, nil
	}
	if cleanupErr := s.discardPreparedMedia(ctx, handle, fmt.Errorf("prepared media reused existing asset %d", existingAssetID)); cleanupErr != nil {
		return false, fmt.Errorf("discard checksum-equivalent prepared media: %w", cleanupErr)
	}
	media.AssetID, media.Source = existingAssetID, ""
	return true, nil
}

func (s *Service) completePreparedMedia(ctx context.Context, userID int64, item assets.PreparedGenerationMedia, handle ingestion.Handle, completion ingestion.Completion, media *tasks.GenerationMediaIntent) (bool, error) {
	if err := s.registry.Complete(ctx, handle, completion); err != nil {
		if database.IsDuplicateConstraintError(err) && strings.TrimSpace(completion.ChecksumSHA256) != "" {
			existingAssetID, found, lookupErr := s.registry.FindReadyOwnedMediaAssetIDByChecksum(ctx, userID, item.StorageProfile, completion.ChecksumSHA256)
			if lookupErr == nil && found {
				if cleanupErr := s.discardPreparedMedia(ctx, handle, err); cleanupErr != nil {
					return false, errors.Join(err, fmt.Errorf("discard checksum-conflicting prepared media: %w", cleanupErr))
				}
				media.AssetID, media.Source = existingAssetID, ""
				return true, nil
			}
			if lookupErr != nil {
				err = fmt.Errorf("%w; find checksum conflict winner: %v", err, lookupErr)
			}
		}
		return false, errors.Join(err, s.discardPreparedMedia(ctx, handle, err))
	}
	return false, nil
}

func (s *Service) discardPreparedMedia(ctx context.Context, handle ingestion.Handle, cause error) error {
	cleanupCtx := context.WithoutCancel(ctx)
	deleteErr := s.storage.DeleteObjectByStorageProfile(cleanupCtx, handle.StorageProfile, handle.StorageKey)
	failErr := s.registry.Fail(cleanupCtx, handle, cause)
	return errors.Join(deleteErr, failErr)
}

func (s *Service) beginPreparedMedia(ctx context.Context, userID int64, clientActionID string, media tasks.GenerationMediaIntent) (ingestion.Handle, error) {
	dirPrefix, err := domain.TaskOutputUploadDirPrefix(userID, "prepare-"+clientActionID)
	if err != nil {
		return ingestion.Handle{}, err
	}
	filename := fmt.Sprintf("%s-%d", strings.TrimSpace(media.Role), media.InputIndex)
	objectKey, err := domain.StableUploadObjectKey(dirPrefix, filename)
	if err != nil {
		return ingestion.Handle{}, err
	}
	planned, err := s.storage.PrepareUploadRef(ctx, filename, ports.StorageStoreOptions{ObjectKey: objectKey})
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
