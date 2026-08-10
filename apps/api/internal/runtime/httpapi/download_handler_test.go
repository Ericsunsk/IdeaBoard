package httpapi

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/media/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type fakeUploadResourceReader struct {
	object *storage.StoredUploadObject
	calls  int
}

func (f *fakeUploadResourceReader) ReadUploadResourceByURL(_ context.Context, _ string, _ int64) (*storage.StoredUploadObject, error) {
	f.calls++
	return f.object, nil
}

type allowUploadReadAccess struct{}

func (allowUploadReadAccess) AuthorizeUploadPathRead(_ context.Context, _ int64, _ string) (storage.ReadableUploadAccessResult, error) {
	return storage.ReadableUploadAccessResult{}, nil
}

func (allowUploadReadAccess) AuthorizeUploadResourceRead(_ context.Context, _ int64, resource storage.UploadResource) (storage.ReadableUploadAccessResult, error) {
	return storage.ReadableUploadAccessResult{Resource: resource}, nil
}

func TestDownloadImageHandlerReadsManagedUploadThroughStorage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pngData, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	reader := &fakeUploadResourceReader{object: &storage.StoredUploadObject{
		Filename:    "future.png",
		ContentType: "image/png",
		Data:        pngData,
	}}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		identity, err := principal.New(42, uuid.New(), "auth-42", "Test User", principal.RoleUser.String())
		if err != nil || principal.Set(c, identity) != nil {
			t.Fatal("failed to set test principal")
		}
		c.Next()
	})
	router.GET(RouteUserDownload, downloadImageHandler(UploadDownloadDeps{
		Reader:     reader,
		ReadAccess: allowUploadReadAccess{},
	}))

	managedURL := "https://api.ideaboard.art/uploads/canvas/OWNR1A/board-1/content/future.png"
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, RouteUserDownload+"?url="+url.QueryEscape(managedURL), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if reader.calls != 1 {
		t.Fatalf("storage reader calls = %d, want 1", reader.calls)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("content type = %q, want image/png", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("cache control = %q, want private, no-store", got)
	}
}
