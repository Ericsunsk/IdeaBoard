package users

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ideaboard/api/internal/platform/database"
)

type recordingUserUploadCleaner struct {
	calls []userUploadCleanupCall
	err   error
}

type userUploadCleanupCall struct {
	userID     int64
	storageUID string
}

func (c *recordingUserUploadCleaner) DeleteUserUploads(_ context.Context, userID int64, storageUID string) error {
	c.calls = append(c.calls, userUploadCleanupCall{userID: userID, storageUID: storageUID})
	return c.err
}

func TestListUsersRejectsOverlongKeyword(t *testing.T) {
	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)
	service := NewUserService(repo)

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
	service := NewUserService(repo)

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
			if !isUserListValidationError(err) {
				t.Fatalf("error type = %T, want user list validation error", err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want contains %q", err.Error(), tt.want)
			}
		})
	}
}

func TestDeleteUserCleansUploadsBeforeDeletingLocalRecords(t *testing.T) {
	ctx := context.Background()
	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)
	cleaner := &recordingUserUploadCleaner{}
	service := NewUserService(repo, cleaner)

	user := &User{
		Username:   "cleanup_user",
		StorageUID: "CLN123",
		Role:       "user",
		Status:     1,
	}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO tasks (task_id, user_id, prompt, user_prompt, params, status, error_msg, result) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		"delete-user-cleanup-task", user.ID, "", "", "", "pending", "", "",
	); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	if err := service.DeleteUser(ctx, user.ID); err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}

	if len(cleaner.calls) != 1 || cleaner.calls[0].userID != user.ID || cleaner.calls[0].storageUID != user.StorageUID {
		t.Fatalf("cleaner calls=%+v, want one call for user uploads", cleaner.calls)
	}
	if _, err := repo.GetByID(ctx, user.ID); !database.IsNotFound(err) {
		t.Fatalf("user lookup err = %v, want record not found", err)
	}
	var taskCount int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE user_id = $1`, user.ID).Scan(&taskCount); err != nil {
		t.Fatalf("count tasks failed: %v", err)
	}
	if taskCount != 0 {
		t.Fatalf("task count = %d, want 0", taskCount)
	}
}

func TestDeleteUserStopsWhenUploadCleanupFails(t *testing.T) {
	ctx := context.Background()
	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)
	service := NewUserService(repo, &recordingUserUploadCleaner{err: errors.New("storage down")})

	user := &User{
		Username:   "cleanup_failure_user",
		StorageUID: "FAIL42",
		Role:       "user",
		Status:     1,
	}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	if err := service.DeleteUser(ctx, user.ID); err == nil {
		t.Fatal("expected DeleteUser to fail when upload cleanup fails")
	}
	if _, err := repo.GetByID(ctx, user.ID); err != nil {
		t.Fatalf("user should remain after cleanup failure, lookup err = %v", err)
	}
}
