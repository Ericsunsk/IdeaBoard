package authorization

import (
	"context"
	"testing"

	"ideaboard/api/internal/media/storage/domain"
)

type stubOwnerStorageUIDResolver struct {
	storageUID string
	err        error
}

func (r stubOwnerStorageUIDResolver) GetStorageUIDByUserID(_ context.Context, _ int64) (string, error) {
	return r.storageUID, r.err
}

func TestReadableUploadAccessSiteAsset(t *testing.T) {
	access := NewReadableUploadAccess(stubOwnerStorageUIDResolver{})
	res, err := access.AuthorizeUploadPathRead(context.Background(), 10, "uploads/site/assets/logo.png")
	if err != nil {
		t.Fatalf("expected site asset to be readable, got: %v", err)
	}
	if res.Resource.Kind != domain.UploadResourceKindSiteAsset {
		t.Fatalf("kind = %v, want site_asset", res.Resource.Kind)
	}
}
