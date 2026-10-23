package v1

import (
	"context"
	"net/http"
	"strings"
	"testing"

	librarysources "ideaboard/api/internal/library/application/sources"
	librarydomain "ideaboard/api/internal/library/domain"
	librarymemory "ideaboard/api/internal/library/infrastructure/memory"
	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	"ideaboard/api/internal/media/derivatives/infrastructure/preview"
	"ideaboard/api/internal/media/workspacemedia"
)

func TestV1LibraryDisplayAuthorizationAndNoMaterialization(t *testing.T) {
	f := newV1MediaFixture(t)
	lib := librarymemory.NewInMemoryWorkspaceLibraryStoreForTests()
	id, err := librarydomain.NewV1AssetPublicID()
	if err != nil {
		t.Fatal(err)
	}
	asset, _, err := lib.SaveAsset(context.Background(), f.ws.PublicID, librarydomain.V1SaveAssetInput{Asset: librarydomain.AssetInput{Provider: "unsplash", SourceID: "image", SourceURL: "https://www.unsplash.com/image", PreviewURL: "https://www.unsplash.com/image", OriginalURL: "https://images.unsplash.com/original", Width: 1200, Height: 800}}, id)
	if err != nil {
		t.Fatal(err)
	}
	cfg := v1TestImgproxyConfig(t)
	renderer := imgproxy.NewRenderer(cfg, nil)
	previews, err := preview.NewService(cfg, t.TempDir(), renderer)
	if err != nil {
		t.Fatal(err)
	}
	sources := &librarysources.SourceResolver{Store: lib}
	engine := newV1MediaTestEngine(f.store, f.dir, ContractV1Deps{Media: f.media, MediaObjects: f.objects, MediaRenderer: renderer, MediaPreviews: previews, Library: ContractV1LibraryDeps{Store: lib, Sources: sources}})
	path := libraryDisplay(f.ws.PublicID, asset).Variants[0].URL
	for _, test := range []struct {
		user int64
		path string
		want int
	}{
		{0, path, http.StatusUnauthorized},
		{f.other, path, http.StatusNotFound},
		{f.owner, strings.Replace(path, f.ws.PublicID, f.ws2.PublicID, 1), http.StatusNotFound},
		{f.member, path + "x", http.StatusNotFound},
		{f.member, strings.Replace(path, "/thumb?", "/processing-input?", 1), http.StatusBadRequest},
		{f.member, path, http.StatusConflict},
	} {
		status, headers, _ := doV1RoundTrip(t, engine, v1Call{method: "GET", path: test.path, userID: test.user}, "library-display-")
		if status != test.want || (status != http.StatusUnauthorized && headers.Get("Cache-Control") != "no-store") {
			t.Fatalf("display status=%d, want=%d", status, test.want)
		}
	}
	status, _, payload := doV1(t, engine, v1Call{method: "GET", path: "/api/v1/workspaces/" + f.ws.PublicID + "/library/assets", userID: f.member})
	if status != 200 {
		t.Fatal("list failed")
	}
	items := v1Data(t, payload).([]any)
	if items[0].(map[string]any)["display"] == nil {
		t.Fatal("list lacks inline display")
	}
	jobs, err := lib.ListMaterializations(context.Background(), f.ws.PublicID)
	if err != nil || len(jobs) != 0 {
		t.Fatal("browsing created materialization state")
	}
	if err := lib.DeleteAsset(context.Background(), f.ws.PublicID, id); err != nil {
		t.Fatal(err)
	}
	status, _, _ = doV1RoundTrip(t, engine, v1Call{method: "GET", path: path, userID: f.member}, "library-display-deleted-")
	if status != 404 {
		t.Fatal("deleted reference remained readable")
	}
}

func TestV1LibraryDisplayUsesCompletedManagedLink(t *testing.T) {
	f := newV1MediaFixture(t)
	lib := librarymemory.NewInMemoryWorkspaceLibraryStoreForTests()
	id, err := librarydomain.NewV1AssetPublicID()
	if err != nil {
		t.Fatal(err)
	}
	asset, _, err := lib.SaveAsset(context.Background(), f.ws.PublicID, librarydomain.V1SaveAssetInput{Asset: librarydomain.AssetInput{Provider: "unsplash", SourceID: "image", SourceURL: "https://unsplash.com/image", PreviewURL: "https://images.unsplash.com/preview", OriginalURL: "https://images.unsplash.com/original"}}, id)
	if err != nil {
		t.Fatal(err)
	}
	managed, err := workspacemedia.IngestImage(context.Background(), f.media, f.objects, workspacemedia.IngestImageParams{WorkspaceID: f.ws.PublicID, OwnerUserID: f.owner, FilenameStem: "display", Data: readThroughPNGBytes(t)})
	if err != nil {
		t.Fatal(err)
	}
	jobID, err := librarydomain.NewV1MaterializationPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := lib.ClaimMaterialization(context.Background(), f.ws.PublicID, id, jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.SetMaterializationResult(context.Background(), f.ws.PublicID, jobID, librarydomain.V1MaterializationCompleted, managed.ID, "", ""); err != nil {
		t.Fatal(err)
	}
	// #936: the completed materialization link publishes the managed asset
	// to workspace members; seed the same existence fact on the media seam
	// (twin of the library_assets row, which lives in the library store).
	f.media.SeedLibraryLinkForTests(f.ws.PublicID, managed.ID)
	cfg := v1TestImgproxyConfig(t)
	renderer := imgproxy.NewRenderer(cfg, nil)
	previews, err := preview.NewService(cfg, t.TempDir(), renderer)
	if err != nil {
		t.Fatal(err)
	}
	sources := &librarysources.SourceResolver{Store: lib}
	engine := newV1MediaTestEngine(f.store, f.dir, ContractV1Deps{Media: f.media, MediaObjects: f.objects, MediaRenderer: renderer, MediaPreviews: previews, Library: ContractV1LibraryDeps{Store: lib, Sources: sources}})
	status, headers, body := doV1RoundTrip(t, engine, v1Call{method: "GET", path: libraryDisplay(f.ws.PublicID, asset).Variants[0].URL, userID: f.member}, "library-display-linked-")
	if status != 200 || headers.Get("Cache-Control") != "private, max-age=300" || !strings.Contains(body, managed.StorageKey) {
		t.Fatalf("managed display failed status=%d", status)
	}
	jobs, err := lib.ListMaterializations(context.Background(), f.ws.PublicID)
	if err != nil || len(jobs) != 1 {
		t.Fatal("display changed materialization state")
	}
}

func TestV1LibraryDisplayDirectManagedLinks(t *testing.T) {
	f := newV1MediaFixture(t)
	lib := librarymemory.NewInMemoryWorkspaceLibraryStoreForTests()
	id, err := librarydomain.NewV1AssetPublicID()
	if err != nil {
		t.Fatal(err)
	}
	managed, err := workspacemedia.IngestImage(context.Background(), f.media, f.objects, workspacemedia.IngestImageParams{WorkspaceID: f.ws.PublicID, OwnerUserID: f.owner, FilenameStem: "direct", Data: readThroughPNGBytes(t)})
	if err != nil {
		t.Fatal(err)
	}
	lib.RegisterManagedMediaForTests(f.ws.PublicID, managed.ID, nil, nil, false)
	lib.RegisterManagedMediaForTests(f.ws.PublicID, "asset_dangling000000001", nil, nil, false)
	asset, _, err := lib.SaveManagedMediaAsset(context.Background(), f.ws.PublicID, librarydomain.V1SaveManagedAssetInput{MediaAssetID: managed.ID, OperatorUserID: f.owner}, id)
	if err != nil {
		t.Fatal(err)
	}
	// #936: the managed save publishes the asset to workspace members; seed
	// the existence fact on the media seam (twin of the library_assets row).
	f.media.SeedLibraryLinkForTests(f.ws.PublicID, managed.ID)
	danglingID, err := librarydomain.NewV1AssetPublicID()
	if err != nil {
		t.Fatal(err)
	}
	dangling, _, err := lib.SaveManagedMediaAsset(context.Background(), f.ws.PublicID, librarydomain.V1SaveManagedAssetInput{MediaAssetID: "asset_dangling000000001", OperatorUserID: f.owner}, danglingID)
	if err != nil {
		t.Fatal(err)
	}
	cfg := v1TestImgproxyConfig(t)
	renderer := imgproxy.NewRenderer(cfg, nil)
	previews, err := preview.NewService(cfg, t.TempDir(), renderer)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name   string
		target *librarydomain.V1Asset
		status int
	}{
		{"ready canonical asset", asset, http.StatusOK},
		{"dangling canonical asset", dangling, http.StatusNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sources := &librarysources.SourceResolver{Store: lib}
			engine := newV1MediaTestEngine(f.store, f.dir, ContractV1Deps{Media: f.media, MediaObjects: f.objects, MediaRenderer: renderer, MediaPreviews: previews, Library: ContractV1LibraryDeps{Store: lib, Sources: sources}})
			status, headers, _ := doV1RoundTrip(t, engine, v1Call{method: "GET", path: libraryDisplay(f.ws.PublicID, tt.target).Variants[0].URL, userID: f.member}, "library-direct-")
			if status != tt.status {
				t.Fatalf("status = %d, want %d", status, tt.status)
			}
			if status != http.StatusOK && headers.Get("Cache-Control") != "no-store" {
				t.Fatal("failed managed link was cacheable")
			}
		})
	}
	if jobs, err := lib.ListMaterializations(context.Background(), f.ws.PublicID); err != nil || len(jobs) != 0 {
		t.Fatal("managed display initiated materialization")
	}
}
