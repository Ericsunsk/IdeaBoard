package httpapi

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

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
