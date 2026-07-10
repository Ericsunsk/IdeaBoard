package users

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ideaboard/api/internal/platform/validation"
)

type recordingUserUploadCleaner struct {
	calls []userUploadCleanupCall
	err   error
}

type userUploadCleanupCall struct {
	userID     int64
	storageUID string
}

type recordingUserLifecycle struct {
	progress       DeletionProgress
	statusCalls    []int16
	advanceCalls   [][2]DeletionPhase
	tombstoneCalls int
}

func (l *recordingUserLifecycle) SetUserStatus(_ context.Context, _ int64, status int16) error {
	l.statusCalls = append(l.statusCalls, status)
	return nil
}

func (l *recordingUserLifecycle) BeginUserDeletion(_ context.Context, userID int64) (*DeletionProgress, error) {
	l.progress.UserID = userID
	progress := l.progress
	return &progress, nil
}

func (l *recordingUserLifecycle) AdvanceUserDeletion(_ context.Context, _ int64, from, to DeletionPhase) error {
	l.advanceCalls = append(l.advanceCalls, [2]DeletionPhase{from, to})
	l.progress.Phase = to
	return nil
}

func (l *recordingUserLifecycle) TombstoneUser(_ context.Context, _ int64) error {
	l.tombstoneCalls++
	l.progress.Phase = DeletionCompleted
	return nil
}

type recordingAuthUserDeleter struct {
	authUserIDs []string
	err         error
}

func (d *recordingAuthUserDeleter) DeleteUser(_ context.Context, authUserID string) error {
	d.authUserIDs = append(d.authUserIDs, authUserID)
	return d.err
}

func (c *recordingUserUploadCleaner) DeleteUserUploads(_ context.Context, userID int64, storageUID string) error {
	c.calls = append(c.calls, userUploadCleanupCall{userID: userID, storageUID: storageUID})
	return c.err
}

func TestListUsersRejectsOverlongKeyword(t *testing.T) {
	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)
	service := NewUserService(repo, nil)

	_, err := service.ListUsers(context.Background(), &UserListRequest{
		Keyword:  strings.Repeat("a", maxUserKeywordLength+1),
		Page:     1,
		PageSize: 10,
	})
	if err == nil {
		t.Fatal("expected keyword length validation error")
	}
}

func TestListUsersRejectsInvalidFilters(t *testing.T) {
	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)
	service := NewUserService(repo, nil)

	tests := []struct {
		name    string
		request UserListRequest
		want    string
	}{
		{
			name: "invalid role",
			request: UserListRequest{
				Role:     "owner",
				Page:     1,
				PageSize: 10,
			},
			want: "role 仅支持 admin 或 user",
		},
		{
			name: "invalid status",
			request: UserListRequest{
				Status:   "2",
				Page:     1,
				PageSize: 10,
			},
			want: "status 仅支持 0 或 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.ListUsers(context.Background(), &tt.request)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !validation.Is(err) {
				t.Fatalf("error type = %T, want user list validation error", err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want contains %q", err.Error(), tt.want)
			}
		})
	}
}

func TestDeleteUserRunsDurablePhasesInOrder(t *testing.T) {
	ctx := context.Background()
	cleaner := &recordingUserUploadCleaner{}
	lifecycle := &recordingUserLifecycle{progress: DeletionProgress{
		UserID: 42, AuthUserID: "auth-42", StorageUID: "CLN123", Phase: DeletionPendingAuth,
	}}
	authDeleter := &recordingAuthUserDeleter{}
	service := NewUserService(nil, lifecycle, cleaner)

	if err := service.DeleteUser(ctx, 42, authDeleter); err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}
	if len(authDeleter.authUserIDs) != 1 || authDeleter.authUserIDs[0] != "auth-42" {
		t.Fatalf("auth delete calls=%v", authDeleter.authUserIDs)
	}
	if len(cleaner.calls) != 1 || cleaner.calls[0].userID != 42 || cleaner.calls[0].storageUID != "CLN123" {
		t.Fatalf("cleaner calls=%+v, want one call for user uploads", cleaner.calls)
	}
	if len(lifecycle.advanceCalls) != 2 || lifecycle.tombstoneCalls != 1 {
		t.Fatalf("advance calls=%v tombstone calls=%d", lifecycle.advanceCalls, lifecycle.tombstoneCalls)
	}
}

func TestDeleteUserRetryResumesAtPendingStorage(t *testing.T) {
	ctx := context.Background()
	lifecycle := &recordingUserLifecycle{progress: DeletionProgress{
		UserID: 42, AuthUserID: "auth-42", StorageUID: "FAIL42", Phase: DeletionPendingStorage,
	}}
	cleaner := &recordingUserUploadCleaner{err: errors.New("storage down")}
	authDeleter := &recordingAuthUserDeleter{}
	service := NewUserService(nil, lifecycle, cleaner)
	if err := service.DeleteUser(ctx, 42, authDeleter); err == nil {
		t.Fatal("expected DeleteUser to fail when upload cleanup fails")
	}
	if len(authDeleter.authUserIDs) != 0 || lifecycle.progress.Phase != DeletionPendingStorage {
		t.Fatalf("retry should skip Auth and remain pending storage: auth=%v phase=%s", authDeleter.authUserIDs, lifecycle.progress.Phase)
	}
}
