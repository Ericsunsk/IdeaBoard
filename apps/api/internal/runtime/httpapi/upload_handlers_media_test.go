package httpapi

import (
	"testing"

	"ideaboard/api/internal/media/storage/ports"
)

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
