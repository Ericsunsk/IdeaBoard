package tests

import (
	"context"
	"errors"
	"testing"
)

func TestReadableUploadAccessSiteAssetAllowsAnonymousRawPath(t *testing.T) {
	access := NewReadableUploadAccess(nil, nil)

	result, err := access.AuthorizeUploadPathRead(context.Background(), 0, "/uploads/site/assets/logo.png")
	if err != nil {
		t.Fatalf("AuthorizeUploadPathRead returned error: %v", err)
	}
	if result.Resource.Kind != UploadResourceKindSiteAsset {
		t.Fatalf("kind = %q, want %q", result.Resource.Kind, UploadResourceKindSiteAsset)
	}
	if result.Resource.ObjectKey != "uploads/site/assets/logo.png" {
		t.Fatalf("object key = %q, want uploads/site/assets/logo.png", result.Resource.ObjectKey)
	}
}

func TestReadableUploadAccessCanvasAllowsOwnerAndMember(t *testing.T) {
	access := NewReadableUploadAccess(
		fakeReadableUploadBoardAccess{ownerUserID: 1, members: map[int64]bool{2: true}},
		fakeReadableUploadOwnerStorage{uids: map[int64]string{1: "OWNR1A", 2: "MEMB1A"}},
	)

	ownerResult, err := access.AuthorizeUploadPathRead(context.Background(), 1, "/uploads/canvas/OWNR1A/board-1/content/demo.png")
	if err != nil {
		t.Fatalf("owner AuthorizeUploadPathRead returned error: %v", err)
	}
	if ownerResult.Resource.Canvas == nil || ownerResult.Resource.Canvas.BoardSegment != "board-1" {
		t.Fatalf("owner canvas resource = %#v, want board-1", ownerResult.Resource.Canvas)
	}

	resource, err := ParseUploadResource("/uploads/canvas/OWNR1A/board-1/content/demo.png")
	if err != nil {
		t.Fatalf("ParseUploadResource returned error: %v", err)
	}
	memberResult, err := access.AuthorizeUploadResourceRead(context.Background(), 2, resource)
	if err != nil {
		t.Fatalf("member AuthorizeUploadResourceRead returned error: %v", err)
	}
	if memberResult.Resource.ObjectKey != resource.ObjectKey {
		t.Fatalf("member object key = %q, want %q", memberResult.Resource.ObjectKey, resource.ObjectKey)
	}
}

func TestReadableUploadAccessProfileAvatarRejectsOwnerMismatch(t *testing.T) {
	access := NewReadableUploadAccess(nil, fakeReadableUploadOwnerStorage{uids: map[int64]string{2: "MEMB1A"}})

	_, err := access.AuthorizeUploadPathRead(context.Background(), 2, "/uploads/profile/OWNR1A/avatar/avatar.png")
	if !errors.Is(err, ErrUploadAccessOwnerMismatch) {
		t.Fatalf("error = %v, want ErrUploadAccessOwnerMismatch", err)
	}
}

func TestReadableUploadAccessTaskOutputAllowsOwnerOnly(t *testing.T) {
	access := NewReadableUploadAccess(nil, nil)

	result, err := access.AuthorizeUploadPathRead(context.Background(), 42, "/uploads/tasks/42/task-demo/output.png")
	if err != nil {
		t.Fatalf("owner AuthorizeUploadPathRead returned error: %v", err)
	}
	if result.Resource.TaskOutput == nil || result.Resource.TaskOutput.UserSegment != "42" {
		t.Fatalf("task output resource = %#v, want user segment 42", result.Resource.TaskOutput)
	}

	_, err = access.AuthorizeUploadPathRead(context.Background(), 43, "/uploads/tasks/42/task-demo/output.png")
	if !errors.Is(err, ErrUploadAccessDenied) {
		t.Fatalf("error = %v, want ErrUploadAccessDenied", err)
	}
}

type fakeReadableUploadBoardAccess struct {
	ownerUserID int64
	members     map[int64]bool
}

func (r fakeReadableUploadBoardAccess) ResolveUploadBoardAccess(_ context.Context, userID int64, boardID string) (int64, error) {
	if boardID != "board-1" {
		return 0, ErrUploadAccessDenied
	}
	if userID == r.ownerUserID || r.members[userID] {
		return r.ownerUserID, nil
	}
	return 0, ErrUploadAccessDenied
}

type fakeReadableUploadOwnerStorage struct {
	uids map[int64]string
}

func (r fakeReadableUploadOwnerStorage) GetStorageUIDByUserID(_ context.Context, userID int64) (string, error) {
	uid := r.uids[userID]
	if uid == "" {
		return "", ErrUploadAccessDenied
	}
	return uid, nil
}
