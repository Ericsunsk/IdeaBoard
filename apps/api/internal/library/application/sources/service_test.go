package sources

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/library/domain"
)

type sourceReadStore struct {
	asset     *domain.V1Asset
	jobs      []*domain.V1Materialization
	getErr    error
	listErr   error
	listCalls int
}

func (s *sourceReadStore) GetAsset(_ context.Context, workspaceID, assetID string) (*domain.V1Asset, error) {
	if s.asset == nil || s.asset.WorkspacePublicID != workspaceID || s.asset.PublicID != assetID {
		return nil, domain.ErrV1AssetNotFound
	}
	return s.asset, s.getErr
}

func (s *sourceReadStore) ListMaterializations(context.Context, string) ([]*domain.V1Materialization, error) {
	s.listCalls++
	return s.jobs, s.listErr
}

func TestSourceResolverOwnedReadOnlySelection(t *testing.T) {
	lookupErr := errors.New("lookup failed")
	for _, tt := range []struct {
		name          string
		asset         *domain.V1Asset
		jobs          []*domain.V1Materialization
		listErr       error
		wantMedia     string
		wantErr       error
		wantListCalls int
	}{
		{name: "provider preview", asset: &domain.V1Asset{Source: domain.AssetSourceExternal, Provider: domain.ProviderUnsplash}, wantListCalls: 1},
		{name: "completed link", asset: &domain.V1Asset{Source: domain.AssetSourceExternal, Provider: domain.ProviderPexels}, jobs: []*domain.V1Materialization{
			{AssetPublicID: "lasset_sources", Status: domain.V1MaterializationCompleted, MediaAssetID: "asset_completed00000001"},
			{AssetPublicID: "lasset_sources", Status: domain.V1MaterializationRunning},
		}, wantMedia: "asset_completed00000001", wantListCalls: 1},
		{name: "invalid completed link", asset: &domain.V1Asset{Source: domain.AssetSourceExternal, Provider: domain.ProviderUnsplash}, jobs: []*domain.V1Materialization{
			{AssetPublicID: "lasset_sources", Status: domain.V1MaterializationCompleted, MediaAssetID: "invalid"},
		}, wantListCalls: 1},
		{name: "listing error leaves provider preview", asset: &domain.V1Asset{Source: domain.AssetSourceExternal, Provider: domain.ProviderUnsplash}, listErr: lookupErr, wantListCalls: 1},
		{name: "direct managed link", asset: &domain.V1Asset{Source: domain.AssetSourceMedia, MediaAssetID: "asset_direct00000001"}, wantMedia: "asset_direct00000001"},
		{name: "malformed direct link", asset: &domain.V1Asset{Source: domain.AssetSourceMedia, MediaAssetID: "invalid"}, wantErr: domain.ErrV1AssetNotFound},
		{name: "missing direct link", asset: &domain.V1Asset{Source: domain.AssetSourceMedia}, wantErr: domain.ErrV1AssetNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			asset := tt.asset
			asset.WorkspacePublicID = "ws_sources"
			asset.PublicID = "lasset_sources"
			asset.PreviewURL = "https://images.unsplash.com/preview"
			asset.OriginalURL = "https://images.unsplash.com/original"
			store := &sourceReadStore{asset: asset, jobs: tt.jobs, listErr: tt.listErr}
			resolver := &SourceResolver{Store: store}
			stored, mediaID, err := resolver.Resolve(context.Background(), "ws_sources", "lasset_sources")
			if stored != asset || mediaID != tt.wantMedia || !errors.Is(err, tt.wantErr) {
				t.Fatalf("Resolve = %v, %q, %v; want original reference, %q, %v", stored, mediaID, err, tt.wantMedia, tt.wantErr)
			}
			if store.listCalls != tt.wantListCalls {
				t.Fatalf("materialization reads = %d, want %d", store.listCalls, tt.wantListCalls)
			}
		})
	}
}

func TestSourceResolverMissingReferencesAndReplayErrors(t *testing.T) {
	ctx := context.Background()
	lookupErr := errors.New("lookup failed")
	store := &sourceReadStore{asset: &domain.V1Asset{WorkspacePublicID: "ws_sources", PublicID: "lasset_sources"}, getErr: lookupErr}
	resolver := &SourceResolver{Store: store}
	for _, workspace := range []string{"ws_sources", "ws_other"} {
		asset, mediaID, err := resolver.Resolve(ctx, workspace, "lasset_sources")
		if asset != nil || mediaID != "" || err == nil || store.listCalls != 0 {
			t.Fatal("unknown or failed reference lookup crossed source selection")
		}
	}
	store.listErr = lookupErr
	if _, found, err := resolver.FindCompletedMaterializationMedia(ctx, "ws_sources", "lasset_sources"); found || !errors.Is(err, lookupErr) {
		t.Fatal("replay must retain lookup errors")
	}
	for _, missing := range []*SourceResolver{nil, {}} {
		if asset, mediaID, err := missing.Resolve(ctx, "ws_sources", "lasset_sources"); asset != nil || mediaID != "" || !errors.Is(err, ErrSourceUnavailable) {
			t.Fatal("unwired resolver did not fail closed")
		}
	}
}

// TestFindCompletedMediaPinsNewest proves the replay scan converges
// on the newest completed link and never hands malformed links out.
func TestFindCompletedMediaPinsNewest(t *testing.T) {
	jobs := []*domain.V1Materialization{
		{AssetPublicID: "lasset_x", Status: domain.V1MaterializationCompleted, MediaAssetID: "asset_old00000000001"},
		{AssetPublicID: "lasset_x", Status: domain.V1MaterializationFailed},
		{AssetPublicID: "lasset_x", Status: domain.V1MaterializationCompleted, MediaAssetID: "not-an-asset"},
		{AssetPublicID: "lasset_x", Status: domain.V1MaterializationCompleted, MediaAssetID: "asset_new00000000001"},
		{AssetPublicID: "lasset_y", Status: domain.V1MaterializationCompleted, MediaAssetID: "asset_other000000001"},
	}
	mediaID, found := FindCompletedMedia(jobs, "lasset_x")
	if !found || mediaID != "asset_new00000000001" {
		t.Fatalf("completed = %q %v, want newest asset_ link", mediaID, found)
	}
	if _, found := FindCompletedMedia(jobs, "lasset_missing"); found {
		t.Fatal("unknown asset replayed, want miss")
	}
	malformed := []*domain.V1Materialization{
		{AssetPublicID: "lasset_x", Status: domain.V1MaterializationCompleted, MediaAssetID: "broken"},
	}
	if _, found := FindCompletedMedia(malformed, "lasset_x"); found {
		t.Fatal("malformed link replayed, want miss")
	}
}
