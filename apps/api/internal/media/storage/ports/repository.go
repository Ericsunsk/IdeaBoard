package ports

import (
	"context"

	"ideaboard/api/internal/media/storage/domain"
)

type StorageProfileRepository interface {
	ListProfiles(ctx context.Context, includeDeleted bool) ([]domain.StorageProfile, error)
	ListProfilesWithSecrets(ctx context.Context, includeDeleted bool) ([]domain.StorageProfile, error)
	SaveProfile(ctx context.Context, profile domain.StorageProfile) error
	SaveAndApplyProfile(ctx context.Context, profile domain.StorageProfile) error
	DeleteProfile(ctx context.Context, id string) error
	ApplyProfile(ctx context.Context, id string) error
}
