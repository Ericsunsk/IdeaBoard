package application

import (
	"context"

	"ideaboard/api/internal/library/domain"
	"ideaboard/api/internal/library/ports"
)

// MaterializationStore extends the seam with the concrete job-listing
// operation. Only the postgres Service implements it; the seam itself
// stays frozen (handlers expose status-addressable GETs).
type MaterializationStore interface {
	ports.WorkspaceLibraryStore
	ListMaterializations(ctx context.Context, workspacePublicID string) ([]*domain.V1Materialization, error)
}

// MaterializationService orchestrates durable materialization jobs
// (pending -> running -> completed|failed; terminal states immutable).
// It advances no state machine of its own: settlement converges inside
// infrastructure/postgres SQL guards so replays stay idempotent.
type MaterializationService struct {
	store MaterializationStore
}

// NewMaterializationService builds a MaterializationService over store.
func NewMaterializationService(store MaterializationStore) *MaterializationService {
	return &MaterializationService{store: store}
}

func (s *MaterializationService) CreateMaterialization(ctx context.Context, workspacePublicID, assetPublicID, publicID string) (*domain.V1Materialization, error) {
	return s.store.CreateMaterialization(ctx, workspacePublicID, assetPublicID, publicID)
}

func (s *MaterializationService) GetMaterialization(ctx context.Context, workspacePublicID, materializationPublicID string) (*domain.V1Materialization, error) {
	return s.store.GetMaterialization(ctx, workspacePublicID, materializationPublicID)
}

func (s *MaterializationService) ListMaterializations(ctx context.Context, workspacePublicID string) ([]*domain.V1Materialization, error) {
	return s.store.ListMaterializations(ctx, workspacePublicID)
}

func (s *MaterializationService) SetMaterializationResult(ctx context.Context, workspacePublicID, materializationPublicID string, status domain.V1MaterializationStatus, mediaAssetID, errorCode, errorMessage string) (*domain.V1Materialization, error) {
	return s.store.SetMaterializationResult(ctx, workspacePublicID, materializationPublicID, status, mediaAssetID, errorCode, errorMessage)
}
