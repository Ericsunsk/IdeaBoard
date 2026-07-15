package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/identity/principal"
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
		{name: "legacy key", setup: func(c *gin.Context) { c.Set("user_id", int64(42)) }},
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
