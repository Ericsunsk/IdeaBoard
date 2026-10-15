package application

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

type managedSourceReader struct {
	mediaID string
	err     error
	calls   int
}

func (r *managedSourceReader) FindManagedMedia(_ context.Context, workspaceID, assetID string) (string, error) {
	r.calls++
	if workspaceID != "ws_sources" || assetID != "lasset_sources" {
		return "", errors.New("lookup lost owned identity")
	}
	return r.mediaID, r.err
}

func TestSourceResolverOwnedReadOnlySelection(t *testing.T) {
	lookupErr := errors.New("lookup failed")
	for _, tt := range []struct {
		name          string
		provider      string
		jobs          []*domain.V1Materialization
		listErr       error
		managed       *managedSourceReader
		wantMedia     string
		wantErr       error
		wantListCalls int
	}{
		{name: "provider preview", provider: domain.ProviderUnsplash, wantListCalls: 1},
		{name: "completed link", provider: domain.ProviderPexels, jobs: []*domain.V1Materialization{
			{AssetPublicID: "lasset_sources", Status: domain.V1MaterializationCompleted, MediaAssetID: "asset_completed00000001"},
			{AssetPublicID: "lasset_sources", Status: domain.V1MaterializationRunning},
		}, wantMedia: "asset_completed00000001", wantListCalls: 1},
		{name: "invalid completed link", provider: domain.ProviderUnsplash, jobs: []*domain.V1Materialization{
			{AssetPublicID: "lasset_sources", Status: domain.V1MaterializationCompleted, MediaAssetID: "invalid"},
		}, wantListCalls: 1},
		{name: "listing error leaves provider preview", provider: domain.ProviderUnsplash, listErr: lookupErr, wantListCalls: 1},
		{name: "direct managed link", provider: domain.ProviderWorkspaceMedia, managed: &managedSourceReader{mediaID: "asset_direct00000001"}, wantMedia: "asset_direct00000001"},
		{name: "missing direct reader", provider: domain.ProviderWorkspaceMedia, wantErr: ErrSourceUnavailable},
		{name: "direct lookup error", provider: domain.ProviderWorkspaceMedia, managed: &managedSourceReader{err: lookupErr}, wantErr: lookupErr},
		{name: "missing direct link", provider: domain.ProviderWorkspaceMedia, managed: &managedSourceReader{}, wantErr: domain.ErrV1AssetNotFound},
		{name: "malformed direct link", provider: domain.ProviderWorkspaceMedia, managed: &managedSourceReader{mediaID: "invalid"}, wantErr: domain.ErrV1AssetNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			asset := &domain.V1Asset{WorkspacePublicID: "ws_sources", PublicID: "lasset_sources", Provider: tt.provider, PreviewURL: "https://images.unsplash.com/preview", OriginalURL: "https://images.unsplash.com/original"}
			store := &sourceReadStore{asset: asset, jobs: tt.jobs, listErr: tt.listErr}
			resolver := &SourceResolver{Store: store}
			if tt.managed != nil {
				resolver.ManagedMedia = tt.managed
			}
			stored, mediaID, err := resolver.Resolve(context.Background(), "ws_sources", "lasset_sources")
			if stored != asset || mediaID != tt.wantMedia || !errors.Is(err, tt.wantErr) {
				t.Fatalf("Resolve = %v, %q, %v; want original reference, %q, %v", stored, mediaID, err, tt.wantMedia, tt.wantErr)
			}
			if store.listCalls != tt.wantListCalls {
				t.Fatalf("materialization reads = %d, want %d", store.listCalls, tt.wantListCalls)
			}
			if tt.managed != nil && tt.managed.calls != 1 {
				t.Fatalf("direct link reads = %d, want 1", tt.managed.calls)
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
