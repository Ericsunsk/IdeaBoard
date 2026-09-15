package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	storagedomain "ideaboard/api/internal/media/storage/domain"
	storageports "ideaboard/api/internal/media/storage/ports"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type fakeMediaAssetRegistry struct {
	asset          dbsqlc.MediaAsset
	assetErr       error
	derivatives    []dbsqlc.MediaDerivative
	derivativesErr error
}

func (f *fakeMediaAssetRegistry) GetByID(_ context.Context, _ int64, _ int64) (dbsqlc.MediaAsset, error) {
	return dbsqlc.MediaAsset{}, sql.ErrNoRows
}

func (f *fakeMediaAssetRegistry) GetByAssetID(_ context.Context, _ int64, _ string) (dbsqlc.MediaAsset, error) {
	return f.asset, f.assetErr
}

func (f *fakeMediaAssetRegistry) ListDerivatives(_ context.Context, _ int64, _ int64) ([]dbsqlc.MediaDerivative, error) {
	return f.derivatives, f.derivativesErr
}

type fakeMediaObjectReader struct {
	objects map[string]*storageports.StoredUploadObject
	err     error
}

func (f *fakeMediaObjectReader) ReadObjectAtProfile(_ context.Context, _ string, objectKey string, _ int64) (*storageports.StoredUploadObject, error) {
	if f.err != nil {
		return nil, f.err
	}
	if obj, ok := f.objects[objectKey]; ok {
		return obj, nil
	}
	return nil, storagedomain.ErrStoredObjectNotFound
}

func serveDerivativeContent(t *testing.T, handler *mediaAssetHandler, assetID, tier string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		identity, err := principal.New(42, uuid.New(), "auth-42", "Test User", principal.RoleUser.String())
		if err != nil || principal.Set(c, identity) != nil {
			t.Fatal("failed to set test principal")
		}
		c.Next()
	})
	router.GET("/media-assets/:asset_id/derivatives/:tier/content", handler.Content)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/media-assets/"+assetID+"/derivatives/"+tier+"/content", nil)
	router.ServeHTTP(rec, req)
	return rec
}

func testImgproxyConfig() imgproxy.Config {
	return imgproxy.Config{
		BaseURL: "http://imgproxy.local",
		KeyHex:  strings.Repeat("ab", 32),
		SaltHex: strings.Repeat("cd", 32),
	}
}

func readyAssetFixture() dbsqlc.MediaAsset {
	return dbsqlc.MediaAsset{
		ID:               3,
		AssetID:          "asset_3",
		OwnerUserID:      42,
		StorageProfileID: "builtin-local",
		StorageKey:       "uploads/original.jpg",
		MimeType:         "image/jpeg",
		Status:           "ready",
	}
}

func TestDerivativeContentRedirectsReadyAssetToSignedTierURL(t *testing.T) {
	registry := &fakeMediaAssetRegistry{asset: readyAssetFixture()}
	reader := &fakeMediaObjectReader{}

	rec := serveDerivativeContent(t, newMediaAssetHandler(registry, reader, testImgproxyConfig()), "asset_3", "preview")

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusFound, rec.Body.String())
	}
	target := rec.Header().Get("Location")
	if !strings.HasPrefix(target, "http://imgproxy.local/") {
		t.Fatalf("location = %q, want an imgproxy tier URL", target)
	}
	// Geometry is carried by imgproxy processing options, not the path.
	if !strings.Contains(target, "rs:fit:640:640") {
		t.Fatalf("location = %q, want the preview tier geometry", target)
	}
	if !strings.Contains(target, "plain/local:///uploads/original.jpg") {
		t.Fatalf("location = %q, want the asset storage key as the source", target)
	}
}

// The tier is resolved from the asset row alone: no derivative row is needed
// for a ready asset, which is the point of the pull model.
func TestDerivativeContentRedirectsWithoutDerivativeRows(t *testing.T) {
	registry := &fakeMediaAssetRegistry{asset: readyAssetFixture(), derivatives: nil}
	reader := &fakeMediaObjectReader{}

	rec := serveDerivativeContent(t, newMediaAssetHandler(registry, reader, testImgproxyConfig()), "asset_3", "processing-input")

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusFound, rec.Body.String())
	}
}

func TestDerivativeContentReturnsAcceptedWhileAssetNotReady(t *testing.T) {
	pending := readyAssetFixture()
	pending.Status = "preparing"
	registry := &fakeMediaAssetRegistry{asset: pending}
	reader := &fakeMediaObjectReader{}

	rec := serveDerivativeContent(t, newMediaAssetHandler(registry, reader, testImgproxyConfig()), "asset_3", "preview")

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
}

func TestDerivativeContentRejectsUnknownTier(t *testing.T) {
	registry := &fakeMediaAssetRegistry{asset: readyAssetFixture()}
	reader := &fakeMediaObjectReader{}

	rec := serveDerivativeContent(t, newMediaAssetHandler(registry, reader, testImgproxyConfig()), "asset_3", "not-a-tier")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestDerivativeContentReturnsNotFoundForUnknownAsset(t *testing.T) {
	registry := &fakeMediaAssetRegistry{assetErr: sql.ErrNoRows}
	reader := &fakeMediaObjectReader{objects: map[string]*storageports.StoredUploadObject{}}

	rec := serveDerivativeContent(t, newMediaAssetHandler(registry, reader, testImgproxyConfig()), "asset_missing", "preview")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// A generation-matched stored derivative is served as bytes directly so
// same-origin fetch consumers (retouch export) never follow the
// cross-origin imgproxy redirect that browsers block without CORS headers.
func TestDerivativeContentServesStoredDerivativeDirectly(t *testing.T) {
	asset := readyAssetFixture()
	asset.ObjectGeneration = 1
	registry := &fakeMediaAssetRegistry{
		asset: asset,
		derivatives: []dbsqlc.MediaDerivative{{
			Tier: "processing-input", Status: "ready", Format: "jpeg",
			StorageProfileID: "builtin-local", StorageKey: "uploads/processing-input.jpg",
			MimeType: "image/jpeg", SourceObjectGeneration: 1,
		}},
	}
	reader := &fakeMediaObjectReader{objects: map[string]*storageports.StoredUploadObject{
		"uploads/processing-input.jpg": {ObjectKey: "uploads/processing-input.jpg", ContentType: "image/jpeg", Data: []byte("processing-bytes")},
	}}

	rec := serveDerivativeContent(t, newMediaAssetHandler(registry, reader, testImgproxyConfig()), "asset_3", "processing-input")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if rec.Header().Get("Location") != "" {
		t.Fatalf("location = %q, want no redirect for a stored derivative", rec.Header().Get("Location"))
	}
	if rec.Body.String() != "processing-bytes" {
		t.Fatalf("body = %q, want stored derivative bytes", rec.Body.String())
	}
}

// A stale row (generation mismatch) or a missing stored object falls back
// to the pull renderer instead of failing the tier.
func TestDerivativeContentFallsBackToRedirectWhenStoredDerivativeStale(t *testing.T) {
	asset := readyAssetFixture()
	asset.ObjectGeneration = 2
	registry := &fakeMediaAssetRegistry{
		asset: asset,
		derivatives: []dbsqlc.MediaDerivative{{
			Tier: "preview", Status: "ready", Format: "jpeg",
			StorageProfileID: "builtin-local", StorageKey: "uploads/preview.jpg",
			MimeType: "image/jpeg", SourceObjectGeneration: 1,
		}},
	}
	reader := &fakeMediaObjectReader{objects: map[string]*storageports.StoredUploadObject{
		"uploads/preview.jpg": {ObjectKey: "uploads/preview.jpg", ContentType: "image/jpeg", Data: []byte("stale-bytes")},
	}}

	rec := serveDerivativeContent(t, newMediaAssetHandler(registry, reader, testImgproxyConfig()), "asset_3", "preview")

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusFound, rec.Body.String())
	}
}

// Profiles without an imgproxy source must not serve original bytes as a tier.
func TestDerivativeContentRejectsNonLocalProfileWithoutMaterializedDerivative(t *testing.T) {
	asset := readyAssetFixture()
	asset.StorageProfileID = "s3-profile"
	registry := &fakeMediaAssetRegistry{asset: asset}
	reader := &fakeMediaObjectReader{objects: map[string]*storageports.StoredUploadObject{
		"uploads/original.jpg": {ObjectKey: "uploads/original.jpg", ContentType: "image/jpeg", Data: []byte("original-bytes")},
	}}

	rec := serveDerivativeContent(t, newMediaAssetHandler(registry, reader, testImgproxyConfig()), "asset_3", "preview")

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
}

func TestProcessingInputContentServesMaterializedNonLocalDerivative(t *testing.T) {
	asset := readyAssetFixture()
	asset.StorageProfileID = "s3-profile"
	asset.ObjectGeneration = 7
	registry := &fakeMediaAssetRegistry{
		asset: asset,
		derivatives: []dbsqlc.MediaDerivative{{
			Tier: "processing-input", Status: "ready", StorageProfileID: "s3-profile",
			StorageKey: "uploads/derivatives/processing-input.webp", MimeType: "image/webp",
			SourceObjectGeneration: 7,
		}},
	}
	reader := &fakeMediaObjectReader{objects: map[string]*storageports.StoredUploadObject{
		"uploads/derivatives/processing-input.webp": {ObjectKey: "uploads/derivatives/processing-input.webp", ContentType: "image/webp", Data: []byte("normalized-bytes")},
	}}

	rec := serveDerivativeContent(t, newMediaAssetHandler(registry, reader, testImgproxyConfig()), "asset_3", "processing-input")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/webp" {
		t.Fatalf("content type = %q, want image/webp", got)
	}
	if rec.Body.String() != "normalized-bytes" {
		t.Fatalf("body = %q, want normalized derivative bytes", rec.Body.String())
	}
}
