package media

import (
	"context"
	"testing"

	"ideaboard/api/internal/platform/config"
)

func TestSubmitMediaValidatorCanonicalizesAPIPublicUploadRef(t *testing.T) {
	validator := NewSubmitMediaValidator(config.PublicOriginsConfig{
		Web: "https://app.example.com",
		API: "https://api.example.com",
	}, nil, nil)

	got := validator.CanonicalizePublicUploadRef(context.Background(), "https://api.example.com/uploads/canvas/OWNR1A/board-1/content/a.png?token=ignored")
	want := "/uploads/canvas/OWNR1A/board-1/content/a.png"
	if got != want {
		t.Fatalf("canonical ref = %q, want %q", got, want)
	}
}
