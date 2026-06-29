package taskmedia

import (
	"context"
	"testing"

	"ideaboard/api/internal/platform/settings"
)

type publicURLProviderFunc func(context.Context) (settings.PublicURLSetting, error)

func (f publicURLProviderFunc) GetPublicURL(ctx context.Context) (settings.PublicURLSetting, error) {
	return f(ctx)
}

func TestSubmitMediaValidatorCanonicalizesConfiguredPublicUploadRef(t *testing.T) {
	provider := publicURLProviderFunc(func(context.Context) (settings.PublicURLSetting, error) {
		return settings.PublicURLSetting{
			PublicBaseURL:      "https://app.example.com",
			PublicAssetBaseURL: "https://assets.example.com/uploads",
		}, nil
	})
	validator := NewSubmitMediaValidator(provider, nil)

	got := validator.CanonicalizePublicUploadRef(context.Background(), "https://assets.example.com/uploads/canvas/OWNR1A/board-1/content/a.png?token=ignored")
	want := "/uploads/canvas/OWNR1A/board-1/content/a.png"
	if got != want {
		t.Fatalf("canonical ref = %q, want %q", got, want)
	}
}
