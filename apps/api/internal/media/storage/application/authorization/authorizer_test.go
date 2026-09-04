package authorization

import (
	"context"
	"testing"

	"ideaboard/api/internal/media/storage/domain"
)

type stubBoardAccessResolver struct {
	ownerUserID int64
	err         error
}

func (r stubBoardAccessResolver) ResolveUploadBoardAccess(_ context.Context, _ int64, _ string) (int64, error) {
	return r.ownerUserID, r.err
}

type stubOwnerStorageUIDResolver struct {
	storageUID string
	err        error
}

func (r stubOwnerStorageUIDResolver) GetStorageUIDByUserID(_ context.Context, _ int64) (string, error) {
	return r.storageUID, r.err
}

func TestReadableUploadAccessSiteAsset(t *testing.T) {
	access := NewReadableUploadAccess(stubBoardAccessResolver{}, stubOwnerStorageUIDResolver{})
	res, err := access.AuthorizeUploadPathRead(context.Background(), 10, "uploads/site/assets/logo.png")
	if err != nil {
		t.Fatalf("expected site asset to be readable, got: %v", err)
	}
	if res.Resource.Kind != domain.UploadResourceKindSiteAsset {
		t.Fatalf("kind = %v, want site_asset", res.Resource.Kind)
	}
}

func TestTaskMediaAuthorizer(t *testing.T) {
	boardResolver := stubBoardAccessResolver{ownerUserID: 10}
	uidResolver := stubOwnerStorageUIDResolver{storageUID: "OWNR1A"}
	authorizer := NewTaskMediaAuthorizer(boardResolver, uidResolver)

	// Canvas path matching owner OWNR1A
	err := authorizer.AuthorizeCanvasUpload(context.Background(), 10, "uploads/canvas/OWNR1A/board-1/content/img.png")
	if err != nil {
		t.Fatalf("expected authorized, got: %v", err)
	}

	// Non-canvas path rejected
	err = authorizer.AuthorizeCanvasUpload(context.Background(), 10, "uploads/site/assets/logo.png")
	if err == nil {
		t.Fatal("expected non-canvas upload to be rejected")
	}
}
