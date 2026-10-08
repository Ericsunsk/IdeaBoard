package tests

import (
	"context"
	"errors"
	"testing"
)

func TestReadableUploadAccessProfileAvatarRejectsOwnerMismatch(t *testing.T) {
	access := NewReadableUploadAccess(fakeReadableUploadOwnerStorage{uids: map[int64]string{2: "MEMB1A"}})

	_, err := access.AuthorizeUploadPathRead(context.Background(), 2, "/uploads/profile/OWNR1A/avatar/avatar.png")
	if !errors.Is(err, ErrUploadAccessOwnerMismatch) {
		t.Fatalf("error = %v, want ErrUploadAccessOwnerMismatch", err)
	}
}

func TestReadableUploadAccessTaskOutputAllowsOwnerOnly(t *testing.T) {
	access := NewReadableUploadAccess(nil)

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
