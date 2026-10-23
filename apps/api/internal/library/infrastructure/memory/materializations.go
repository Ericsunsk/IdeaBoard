package memory

import (
	"context"
	"strings"

	"ideaboard/api/internal/library/domain"
)

// ClaimMaterialization atomically converges internal triggers in the test
// store, matching the postgres advisory-lock implementation.
func (s *InMemoryWorkspaceLibraryStore) ClaimMaterialization(ctx context.Context, workspacePublicID, assetPublicID, publicID string) (*domain.V1Materialization, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if !domain.IsV1MaterializationID(publicID) {
		return nil, false, domain.ErrV1MaterializationNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.assets[workspacePublicID][assetPublicID] == nil {
		return nil, false, domain.ErrV1AssetNotFound
	}
	for _, id := range s.materializationOrder[workspacePublicID] {
		record := s.materializations[workspacePublicID][id]
		if record == nil || record.AssetPublicID != assetPublicID {
			continue
		}
		if record.Status == domain.V1MaterializationPending || record.Status == domain.V1MaterializationRunning || record.Status == domain.V1MaterializationCompleted {
			return copyV1Materialization(record), false, nil
		}
	}
	now := s.clock()
	record := &domain.V1Materialization{WorkspacePublicID: workspacePublicID, PublicID: publicID, AssetPublicID: assetPublicID, Status: domain.V1MaterializationPending, CreatedAt: now, UpdatedAt: now}
	if s.materializations[workspacePublicID] == nil {
		s.materializations[workspacePublicID] = make(map[string]*domain.V1Materialization)
	}
	s.materializations[workspacePublicID][publicID] = record
	s.materializationOrder[workspacePublicID] = append(s.materializationOrder[workspacePublicID], publicID)
	return copyV1Materialization(record), true, nil
}

// ListMaterializations returns every workspace job in creation order
// (concrete-store parity for the read-through completed-job scan and
// trigger dedup; unknown workspaces list empty).
func (s *InMemoryWorkspaceLibraryStore) ListMaterializations(ctx context.Context, workspacePublicID string) ([]*domain.V1Materialization, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	order := s.materializationOrder[workspacePublicID]
	all := make([]*domain.V1Materialization, 0, len(order))
	for _, publicID := range order {
		if record := s.materializations[workspacePublicID][publicID]; record != nil {
			all = append(all, copyV1Materialization(record))
		}
	}
	return all, nil
}

// SetMaterializationResult advances pending|running jobs. Terminal
// records are immutable: repeat calls return the stored snapshot so
// idempotent replays converge instead of resurrecting jobs.
func (s *InMemoryWorkspaceLibraryStore) SetMaterializationResult(ctx context.Context, workspacePublicID, materializationPublicID string, status domain.V1MaterializationStatus, mediaAssetID, errorCode, errorMessage string) (*domain.V1Materialization, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record := s.materializations[workspacePublicID][materializationPublicID]
	if record == nil {
		return nil, domain.ErrV1MaterializationNotFound
	}
	if record.Status == domain.V1MaterializationCompleted || record.Status == domain.V1MaterializationFailed {
		return copyV1Materialization(record), nil
	}
	switch status {
	case domain.V1MaterializationRunning, domain.V1MaterializationCompleted, domain.V1MaterializationFailed:
		record.Status = status
	default:
		return nil, domain.ErrV1MaterializationNotFound
	}
	record.MediaAssetID = strings.TrimSpace(mediaAssetID)
	record.ErrorCode = strings.TrimSpace(errorCode)
	record.ErrorMessage = strings.TrimSpace(errorMessage)
	record.UpdatedAt = s.clock()
	return copyV1Materialization(record), nil
}
