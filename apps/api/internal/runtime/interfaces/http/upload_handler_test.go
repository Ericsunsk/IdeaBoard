package httpapi

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

var (
	errDedupeRetireBoom     = errors.New("retire boom")
	errDedupeProbeTransient = errors.New("probe transient")
)

func dbsqlcMediaAssetStub() dbsqlc.MediaAsset {
	return dbsqlc.MediaAsset{
		ID:               7,
		AssetID:          "asset_dedupe_stub",
		OwnerUserID:      42,
		StorageProfileID: "builtin-local",
		StorageKey:       "uploads/canvas/owner/board/content/image.png",
		Status:           "ready",
	}
}

type fakeMediaUploadSettingsProvider struct {
	setting settings.MediaUploadSetting
}

func (f *fakeMediaUploadSettingsProvider) GetMediaUpload(context.Context) (settings.MediaUploadSetting, error) {
	return f.setting, nil
}

func TestAcquireUploadSlotUsesRuntimeMaxConcurrent(t *testing.T) {
	oldLimiter := uploadLimiter
	uploadLimiter = &uploadConcurrencyLimiter{}
	t.Cleanup(func() {
		uploadLimiter = oldLimiter
	})

	provider := &fakeMediaUploadSettingsProvider{
		setting: settings.MediaUploadSetting{UploadMaxConcurrent: 1},
	}

	firstRelease, ok := acquireUploadSlot(context.Background(), provider)
	if !ok {
		t.Fatal("first upload slot was rejected")
	}
	if release, ok := acquireUploadSlot(context.Background(), provider); ok {
		release()
		t.Fatal("second upload slot was accepted with limit 1")
	}

	provider.setting.UploadMaxConcurrent = 2
	secondRelease, ok := acquireUploadSlot(context.Background(), provider)
	if !ok {
		t.Fatal("second upload slot was rejected after increasing limit to 2")
	}

	provider.setting.UploadMaxConcurrent = 1
	if release, ok := acquireUploadSlot(context.Background(), provider); ok {
		release()
		t.Fatal("third upload slot was accepted after decreasing limit to 1")
	}

	firstRelease()
	if release, ok := acquireUploadSlot(context.Background(), provider); ok {
		release()
		t.Fatal("new upload slot was accepted while one upload still matches limit 1")
	}

	secondRelease()
	if release, ok := acquireUploadSlot(context.Background(), provider); !ok {
		t.Fatal("upload slot was rejected after all prior uploads finished")
	} else {
		release()
	}
}

func TestRequireAuthenticatedUserIDUsesTypedPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("valid", func(t *testing.T) {
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		testsupport.SetPrincipal(t, context, 42, principal.RoleUser)
		userID, ok := requireAuthenticatedUserID(context)
		if !ok || userID != 42 {
			t.Fatalf("userID = %d ok = %v", userID, ok)
		}
	})

	for _, tt := range []struct {
		name  string
		setup func(*gin.Context)
	}{
		{name: "missing"},
		{name: "invalid Principal", setup: func(c *gin.Context) { _ = principal.Set(c, principal.Principal{}) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			if tt.setup != nil {
				tt.setup(context)
			}
			if userID, ok := requireAuthenticatedUserID(context); ok || userID != 0 {
				t.Fatalf("userID = %d ok = %v", userID, ok)
			}
			if recorder.Code != http.StatusUnauthorized || !context.IsAborted() || strings.Count(recorder.Body.String(), `"code"`) != 1 {
				t.Fatalf("status = %d aborted = %v body = %s", recorder.Code, context.IsAborted(), recorder.Body.String())
			}
		})
	}
}

func TestValidateUploadFileHeaderCanonicalizesMismatchedKnownImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "remote.jpg")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	if _, err := part.Write(imageBytes.Bytes()); err != nil {
		t.Fatalf("write avif header: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	file, err := context.FormFile("file")
	if err != nil {
		t.Fatalf("read form file: %v", err)
	}

	ext, ok := validateUploadFileHeader(context, file, 1024*1024, map[string]bool{".jpg": true, ".png": true})
	if !ok || ext != ".png" {
		t.Fatalf("ext = %q, ok = %v, status = %d, body = %s", ext, ok, recorder.Code, recorder.Body.String())
	}
}

func TestValidateUploadFileHeaderRendersFixedSafeMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "truncated.png")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	// Valid PNG magic with a truncated body: format detection succeeds but
	// dimension decoding fails with a wrapped detail error. The wire must
	// carry only the fixed safe copy, never the wrapped detail.
	if _, err := part.Write([]byte("\x89PNG\r\n\x1a\ntruncated-body")); err != nil {
		t.Fatalf("write png header: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	file, err := context.FormFile("file")
	if err != nil {
		t.Fatalf("read form file: %v", err)
	}

	if _, ok := validateUploadFileHeader(context, file, 1024*1024, map[string]bool{".png": true}); ok {
		t.Fatal("truncated image was accepted")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "文件内容不合法") {
		t.Fatalf("body = %s, want fixed safe message", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "读取图片尺寸失败") {
		t.Fatalf("body leaked wrapped decode detail: %s", recorder.Body.String())
	}
}

func TestUploadImageResponseUsesStoredCanonicalMetadata(t *testing.T) {
	stored := &ports.StoredUploadRef{
		URL: "https://cdn.example.com/uploads/site/assets/logo.png", MIMEType: "image/png",
		Width: 320, Height: 180,
	}
	payload := newUploadImageResponseFromStored("url", stored)
	if payload.URL != stored.URL || payload.Mime != stored.MIMEType || payload.Ext != "png" || payload.Width != 320 || payload.Height != 180 {
		t.Fatalf("payload = %+v, want stored canonical metadata", payload)
	}
}

func TestUploadImageResponseUsesAvatarFieldForStoredReference(t *testing.T) {
	stored := &ports.StoredUploadRef{URL: "/uploads/profile/OWNER/avatar/avatar.webp", MIMEType: "image/webp"}
	payload := newUploadImageResponseFromStored("avatar", stored)
	if payload.Avatar != profileAvatarDeliveryURL || payload.URL != "" || payload.Ext != "webp" {
		t.Fatalf("payload = %+v, want avatar response", payload)
	}
}

type fakeDedupeProber struct {
	object *ports.StoredUploadObject
	err    error
}

func (f *fakeDedupeProber) ReadObjectAtProfile(context.Context, string, string, int64) (*ports.StoredUploadObject, error) {
	return f.object, f.err
}

type fakeDedupeRetirer struct {
	err   error
	calls [][2]int64
}

func (f *fakeDedupeRetirer) RetireMissingOwnedMediaAsset(_ context.Context, ownerUserID, assetID int64) error {
	f.calls = append(f.calls, [2]int64{ownerUserID, assetID})
	return f.err
}

func TestConfirmDedupedBoardAssetGatesMissingObjects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	newContext := func() (*gin.Context, *httptest.ResponseRecorder) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/uploads", nil)
		return c, recorder
	}
	existing := dbsqlcMediaAssetStub()

	t.Run("live object is served", func(t *testing.T) {
		c, _ := newContext()
		retirer := &fakeDedupeRetirer{}
		got, found, ok := confirmDedupedBoardAsset(c,
			&fakeDedupeProber{object: &ports.StoredUploadObject{Data: []byte{1}}},
			retirer, 42, existing)
		if !ok || !found || got.ID != existing.ID {
			t.Fatalf("got id=%d found=%v ok=%v, want hit", got.ID, found, ok)
		}
		if len(retirer.calls) != 0 {
			t.Fatal("retirer must not run for live objects")
		}
	})

	t.Run("missing object retires and misses", func(t *testing.T) {
		c, _ := newContext()
		retirer := &fakeDedupeRetirer{}
		_, found, ok := confirmDedupedBoardAsset(c,
			&fakeDedupeProber{err: domain.ErrStoredObjectNotFound},
			retirer, 42, existing)
		if !ok || found {
			t.Fatalf("found=%v ok=%v, want miss with ok", found, ok)
		}
		if len(retirer.calls) != 1 || retirer.calls[0] != [2]int64{42, existing.ID} {
			t.Fatalf("retirer calls = %v", retirer.calls)
		}
	})

	t.Run("empty object counts as missing", func(t *testing.T) {
		c, _ := newContext()
		retirer := &fakeDedupeRetirer{}
		_, found, ok := confirmDedupedBoardAsset(c,
			&fakeDedupeProber{object: &ports.StoredUploadObject{}},
			retirer, 42, existing)
		if !ok || found || len(retirer.calls) != 1 {
			t.Fatalf("found=%v ok=%v calls=%v, want retired miss", found, ok, retirer.calls)
		}
	})

	t.Run("retire failure is a 500", func(t *testing.T) {
		c, recorder := newContext()
		retirer := &fakeDedupeRetirer{err: errDedupeRetireBoom}
		_, _, ok := confirmDedupedBoardAsset(c,
			&fakeDedupeProber{err: domain.ErrStoredObjectNotFound},
			retirer, 42, existing)
		if ok {
			t.Fatal("ok should be false when retirement fails")
		}
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", recorder.Code)
		}
	})

	t.Run("transient probe errors fail open", func(t *testing.T) {
		c, _ := newContext()
		retirer := &fakeDedupeRetirer{}
		got, found, ok := confirmDedupedBoardAsset(c,
			&fakeDedupeProber{err: errDedupeProbeTransient},
			retirer, 42, existing)
		if !ok || !found || got.ID != existing.ID {
			t.Fatalf("got id=%d found=%v ok=%v, want fail-open hit", got.ID, found, ok)
		}
		if len(retirer.calls) != 0 {
			t.Fatal("retirer must not run on transient errors")
		}
	})
}
