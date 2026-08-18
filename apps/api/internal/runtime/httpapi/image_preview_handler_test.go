package httpapi

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/media/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestImagePreviewHandlerReturnsDerivedImageWithoutServingOriginal(t *testing.T) {
	data := buildPreviewPNG(t, 200, 100)
	reader := &fakeUploadResourceReader{object: &storage.StoredUploadObject{
		Filename:    "source.png",
		ContentType: "image/png",
		Data:        data,
	}}
	previewStore := &fakeImagePreviewStore{}
	router := newImagePreviewTestRouterWithStore(reader, previewStore)

	managedURL := "https://api.ideaboard.art/uploads/canvas/OWNR1A/board-1/content/source.png"
	req := httptest.NewRequest(http.MethodGet, RouteUserImagePreview+"?url="+url.QueryEscape(managedURL)+"&max_side=64", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("X-IdeaBoard-Image-Preview"); got != "generated" {
		t.Fatalf("preview header = %q, want generated", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("content type = %q, want image/jpeg", got)
	}
	decoded, _, err := image.Decode(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if decoded.Bounds().Dx() != 64 || decoded.Bounds().Dy() != 32 {
		t.Fatalf("preview size = %v, want 64x32", decoded.Bounds())
	}
	if reader.calls != 1 {
		t.Fatalf("storage reader calls = %d, want 1", reader.calls)
	}
	if previewStore.storedMaxSide != 64 || previewStore.storedContentType != "image/jpeg" || len(previewStore.storedData) == 0 {
		t.Fatalf("persisted preview = max_side:%d content_type:%q bytes:%d", previewStore.storedMaxSide, previewStore.storedContentType, len(previewStore.storedData))
	}
}

func TestImagePreviewHandlerUsesPersistedDerivativeWithoutReadingOriginal(t *testing.T) {
	cached := buildPreviewPNG(t, 64, 32)
	reader := &fakeUploadResourceReader{}
	previewStore := &fakeImagePreviewStore{
		object: &storage.StoredUploadObject{ContentType: "image/png", Data: cached},
		found:  true,
	}
	router := newImagePreviewTestRouterWithStore(reader, previewStore)
	managedURL := "/uploads/canvas/OWNR1A/board-1/content/source.png"
	req := httptest.NewRequest(http.MethodGet, RouteUserImagePreview+"?url="+url.QueryEscape(managedURL)+"&max_side=64", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || rec.Header().Get("X-IdeaBoard-Image-Preview") != "cache" {
		t.Fatalf("status/preview = %d/%q, want 200/cache", rec.Code, rec.Header().Get("X-IdeaBoard-Image-Preview"))
	}
	if reader.calls != 0 {
		t.Fatalf("original storage reader calls = %d, want 0", reader.calls)
	}
}

func TestImagePreviewHandlerKeepsTransparency(t *testing.T) {
	data := buildPreviewPNG(t, 200, 100)
	reader := &fakeUploadResourceReader{object: &storage.StoredUploadObject{
		Filename:    "transparent.png",
		ContentType: "image/png",
		Data:        makeTransparentPNG(t, data),
	}}
	router := newImagePreviewTestRouter(reader)
	managedURL := "https://api.ideaboard.art/uploads/canvas/OWNR1A/board-1/content/transparent.png"
	req := httptest.NewRequest(http.MethodGet, RouteUserImagePreview+"?url="+url.QueryEscape(managedURL)+"&max_side=64", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("status/content type = %d/%q, want 200/image/png", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func TestImagePreviewHandlerRejectsUnmanagedURL(t *testing.T) {
	router := newImagePreviewTestRouter(&fakeUploadResourceReader{})
	req := httptest.NewRequest(http.MethodGet, RouteUserImagePreview+"?url="+url.QueryEscape("https://example.com/image.png"), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func newImagePreviewTestRouter(reader UploadResourceReader) *gin.Engine {
	return newImagePreviewTestRouterWithStore(reader, nil)
}

func newImagePreviewTestRouterWithStore(reader UploadResourceReader, previewStore ImagePreviewStore) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		identity, err := principal.New(42, uuid.New(), "auth-42", "Test User", principal.RoleUser.String())
		if err != nil || principal.Set(c, identity) != nil {
			panic("failed to set test principal")
		}
		c.Next()
	})
	router.GET(RouteUserImagePreview, imagePreviewHandler(UploadDownloadDeps{
		Reader:       reader,
		PreviewStore: previewStore,
		ReadAccess:   allowUploadReadAccess{},
	}))
	return router
}

type fakeImagePreviewStore struct {
	object            *storage.StoredUploadObject
	found             bool
	storedMaxSide     int
	storedContentType string
	storedData        []byte
}

func (f *fakeImagePreviewStore) ReadImagePreview(context.Context, string, int, int64) (*storage.StoredUploadObject, bool, error) {
	return f.object, f.found, nil
}

func (f *fakeImagePreviewStore) StoreImagePreview(_ context.Context, _ string, maxSide int, data []byte, contentType string) error {
	f.storedMaxSide = maxSide
	f.storedContentType = contentType
	f.storedData = append([]byte(nil), data...)
	return nil
}

func buildPreviewPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 200, A: 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func makeTransparentPNG(t *testing.T, source []byte) []byte {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	transparent := image.NewNRGBA(img.Bounds())
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			transparent.Set(x, y, color.NRGBA{R: 255, G: 0, B: 0, A: uint8((x + y) % 255)})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, transparent); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}
