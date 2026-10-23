package sources

import (
	"context"
	"errors"
	"strings"

	"ideaboard/api/internal/library/domain"
	"ideaboard/api/internal/platform/contract"
)

var ErrSourceUnavailable = errors.New("library source lookup is unavailable")

// SourceStore contains only the reads needed to resolve a Library reference.
type SourceStore interface {
	GetAsset(context.Context, string, string) (*domain.V1Asset, error)
	ListMaterializations(context.Context, string) ([]*domain.V1Materialization, error)
}

// SourceResolver selects canonical managed links or the stored provider preview.
// It neither creates materializations nor verifies managed Media readiness.
type SourceResolver struct {
	Store SourceStore
}

// Resolve returns the owned reference and its managed Media ID, when available.
// Media rows carry their managed link directly (no second lookup); an empty
// Media ID selects the stored provider preview. Malformed managed links read
// as not found (never handed to the media store, which requires asset_ shape).
func (r *SourceResolver) Resolve(ctx context.Context, workspacePublicID, assetPublicID string) (*domain.V1Asset, string, error) {
	if r == nil || r.Store == nil {
		return nil, "", ErrSourceUnavailable
	}
	asset, err := r.Store.GetAsset(ctx, workspacePublicID, assetPublicID)
	if err != nil || asset == nil {
		return nil, "", err
	}
	if asset.Source == domain.AssetSourceMedia {
		mediaID := strings.TrimSpace(asset.MediaAssetID)
		if !contract.IsAssetID(mediaID) {
			return asset, "", domain.ErrV1AssetNotFound
		}
		return asset, mediaID, nil
	}
	mediaID, _, _ := r.FindCompletedMaterializationMedia(ctx, workspacePublicID, asset.PublicID)
	return asset, mediaID, nil
}

// FindCompletedMaterializationMedia also supplies the materializer replay seam.
func (r *SourceResolver) FindCompletedMaterializationMedia(ctx context.Context, workspacePublicID, assetPublicID string) (string, bool, error) {
	if r == nil || r.Store == nil {
		return "", false, nil
	}
	jobs, err := r.Store.ListMaterializations(ctx, strings.TrimSpace(workspacePublicID))
	if err != nil {
		return "", false, err
	}
	mediaID, found := FindCompletedMedia(jobs, assetPublicID)
	return mediaID, found, nil
}

// FindCompletedMedia scans one workspace job listing for the newest
// completed link for the library asset. Malformed media links read
// as a miss (never handed to the media store or the fetcher, both
// of which require asset_ shape).
func FindCompletedMedia(jobs []*domain.V1Materialization, assetPublicID string) (string, bool) {
	trimmed := strings.TrimSpace(assetPublicID)
	for index := len(jobs) - 1; index >= 0; index-- {
		job := jobs[index]
		if job == nil || job.AssetPublicID != trimmed {
			continue
		}
		if job.Status != domain.V1MaterializationCompleted {
			continue
		}
		if !contract.IsAssetID(strings.TrimSpace(job.MediaAssetID)) {
			continue
		}
		return strings.TrimSpace(job.MediaAssetID), true
	}
	return "", false
}
