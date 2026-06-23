package httpapi

import (
	"context"
	"testing"

	"ideaboard/api/internal/platform/settings"
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
