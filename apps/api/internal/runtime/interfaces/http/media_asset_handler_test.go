package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/identity/principal"
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
	repaired       []int64
	repairErr      error
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

func (f *fakeMediaAssetRegistry) EnsureDerivative(_ context.Context, _ int64, _ int64, _ string) error {
	return nil
}

func (f *fakeMediaAssetRegistry) RequestDerivativeRepair(_ context.Context, _ int64, derivative dbsqlc.MediaDerivative) error {
	if f.repairErr != nil {
		return f.repairErr
	}
	f.repaired = append(f.repaired, derivative.ID)
	return nil
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

func readyDerivativeFixture() dbsqlc.MediaDerivative {
	return dbsqlc.MediaDerivative{
		ID:               7,
		DerivativeID:     "derivative_7",
		MediaAssetID:     3,
		OwnerUserID:      42,
		Tier:             "preview",
		Format:           "webp",
		Status:           "ready",
		StorageProfileID: "builtin-local",
		StorageKey:       "uploads/derivative-preview.webp",
		MimeType:         "image/webp",
		ObjectGeneration: 1,
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

func TestDerivativeContentServesReadyFile(t *testing.T) {
	registry := &fakeMediaAssetRegistry{asset: readyAssetFixture(), derivatives: []dbsqlc.MediaDerivative{readyDerivativeFixture()}}
	reader := &fakeMediaObjectReader{objects: map[string]*storageports.StoredUploadObject{
		"uploads/derivative-preview.webp": {ObjectKey: "uploads/derivative-preview.webp", ContentType: "image/webp", Data: []byte("derivative-bytes")},
	}}

	rec := serveDerivativeContent(t, newMediaAssetHandler(registry, reader), "asset_3", "preview")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/webp" {
		t.Fatalf("content type = %q, want image/webp", got)
	}
	if string(rec.Body.Bytes()) != "derivative-bytes" {
		t.Fatalf("body = %q, want derivative bytes", rec.Body.String())
	}
	if len(registry.repaired) != 0 {
		t.Fatalf("repair requests = %v, want none for a healthy derivative", registry.repaired)
	}
}

func TestDerivativeContentFallsBackToOriginalAndRepairsOnMissingFile(t *testing.T) {
	registry := &fakeMediaAssetRegistry{asset: readyAssetFixture(), derivatives: []dbsqlc.MediaDerivative{readyDerivativeFixture()}}
	reader := &fakeMediaObjectReader{objects: map[string]*storageports.StoredUploadObject{
		"uploads/original.jpg": {ObjectKey: "uploads/original.jpg", ContentType: "image/jpeg", Data: []byte("original-bytes")},
	}}

	rec := serveDerivativeContent(t, newMediaAssetHandler(registry, reader), "asset_3", "preview")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("content type = %q, want image/jpeg original fallback", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("cache control = %q, want private, no-store for original fallback", got)
	}
	if string(rec.Body.Bytes()) != "original-bytes" {
		t.Fatalf("body = %q, want original bytes", rec.Body.String())
	}
	if len(registry.repaired) != 1 || registry.repaired[0] != 7 {
		t.Fatalf("repair requests = %v, want [7]", registry.repaired)
	}
}

func TestDerivativeContentReturnsNotFoundWhenBothDerivativeAndOriginalAreMissing(t *testing.T) {
	registry := &fakeMediaAssetRegistry{asset: readyAssetFixture(), derivatives: []dbsqlc.MediaDerivative{readyDerivativeFixture()}}
	reader := &fakeMediaObjectReader{objects: map[string]*storageports.StoredUploadObject{}}

	rec := serveDerivativeContent(t, newMediaAssetHandler(registry, reader), "asset_3", "preview")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	if len(registry.repaired) != 1 {
		t.Fatalf("repair requests = %v, want one self-heal request", registry.repaired)
	}
}

func TestDerivativeContentReturnsAcceptedWhileRegenerating(t *testing.T) {
	pending := readyDerivativeFixture()
	pending.Status = "pending"
	registry := &fakeMediaAssetRegistry{asset: readyAssetFixture(), derivatives: []dbsqlc.MediaDerivative{pending}}
	reader := &fakeMediaObjectReader{objects: map[string]*storageports.StoredUploadObject{}}

	rec := serveDerivativeContent(t, newMediaAssetHandler(registry, reader), "asset_3", "preview")

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if len(registry.repaired) != 0 {
		t.Fatalf("repair requests = %v, want none while regenerating", registry.repaired)
	}
}

func TestDerivativeContentReturnsNotFoundForUnknownAsset(t *testing.T) {
	registry := &fakeMediaAssetRegistry{assetErr: sql.ErrNoRows}
	reader := &fakeMediaObjectReader{objects: map[string]*storageports.StoredUploadObject{}}

	rec := serveDerivativeContent(t, newMediaAssetHandler(registry, reader), "asset_missing", "preview")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}
