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

type spyUserCreditBalanceReader struct {
	calls    [][]int64
	balances map[int64]int64
}

func (r *spyUserCreditBalanceReader) GetAvailableBalances(_ context.Context, userIDs []int64) (map[int64]int64, error) {
	r.calls = append(r.calls, append([]int64(nil), userIDs...))
	balances := make(map[int64]int64, len(userIDs))
	for _, userID := range userIDs {
		balances[userID] = r.balances[userID]
	}
	return balances, nil
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
	service := NewUserService(repo, nil, nil)

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
	service := NewUserService(repo, nil, nil)

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

func TestListUsersReadsOnlyCurrentPageBalancesInOneBatch(t *testing.T) {
	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)
	seeded := []*User{
		{Username: "batch-page-first", Role: "user", Status: 1},
		{Username: "batch-page-second", Role: "user", Status: 1},
		{Username: "batch-page-third", Role: "user", Status: 1},
	}
	balances := make(map[int64]int64, len(seeded))
	for index, user := range seeded {
		if err := repo.Create(context.Background(), user); err != nil {
			t.Fatalf("create user %q failed: %v", user.Username, err)
		}
		balances[user.ID] = int64(index + 1)
	}
	reader := &spyUserCreditBalanceReader{balances: balances}
	service := NewUserService(repo, nil, reader)

	result, err := service.ListUsers(context.Background(), &UserListRequest{Keyword: "batch-page", Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}
	if len(result.List) != 2 {
		t.Fatalf("page length = %d, want 2", len(result.List))
	}
	if len(reader.calls) != 1 {
		t.Fatalf("batch reader calls = %d, want exactly 1", len(reader.calls))
	}
	if len(reader.calls[0]) != len(result.List) {
		t.Fatalf("batch IDs = %v, want only current page IDs", reader.calls[0])
	}
	for index, item := range result.List {
		if reader.calls[0][index] != item.ID {
			t.Fatalf("batch IDs = %v, want current page IDs in response order", reader.calls[0])
		}
	}
	for _, userID := range reader.calls[0] {
		if userID == seeded[0].ID {
			t.Fatalf("batch IDs = %v, must not include users outside page", reader.calls[0])
		}
	}
}

func TestDeleteUserRunsDurablePhasesInOrder(t *testing.T) {
	ctx := context.Background()
	cleaner := &recordingUserUploadCleaner{}
	lifecycle := &recordingUserLifecycle{progress: DeletionProgress{
		UserID: 42, AuthUserID: "auth-42", StorageUID: "CLN123", Phase: DeletionPendingAuth,
	}}
	authDeleter := &recordingAuthUserDeleter{}
	service := NewUserService(nil, lifecycle, nil, cleaner)

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
	service := NewUserService(nil, lifecycle, nil, cleaner)
	if err := service.DeleteUser(ctx, 42, authDeleter); err == nil {
		t.Fatal("expected DeleteUser to fail when upload cleanup fails")
	}
	if len(authDeleter.authUserIDs) != 0 || lifecycle.progress.Phase != DeletionPendingStorage {
		t.Fatalf("retry should skip Auth and remain pending storage: auth=%v phase=%s", authDeleter.authUserIDs, lifecycle.progress.Phase)
	}
}
