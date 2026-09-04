package tests

import (
	"context"
	"strings"
	"testing"

	"ideaboard/api/internal/platform/validation"
)

const maxUserKeywordLength = 64

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
		{DisplayName: "batch-page-first", Role: "user", Status: 1},
		{DisplayName: "batch-page-second", Role: "user", Status: 1},
		{DisplayName: "batch-page-third", Role: "user", Status: 1},
	}
	balances := make(map[int64]int64, len(seeded))
	for index, user := range seeded {
		if err := repo.Create(context.Background(), user); err != nil {
			t.Fatalf("create user %q failed: %v", user.DisplayName, err)
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
	if result.List[0].PublicID != seeded[2].PublicID || result.List[1].PublicID != seeded[1].PublicID {
		t.Fatalf("public ids = %v, want current page users in response order", []string{result.List[0].PublicID.String(), result.List[1].PublicID.String()})
	}
	for _, userID := range reader.calls[0] {
		if userID == seeded[0].ID {
			t.Fatalf("batch IDs = %v, must not include users outside page", reader.calls[0])
		}
	}
}

func TestDeleteUserDelegatesLifecyclePhases(t *testing.T) {
	ctx := context.Background()
	lifecycle := &recordingUserLifecycle{}
	authDeleter := &recordingAuthUserDeleter{}
	service := NewUserService(nil, lifecycle, nil)

	if err := service.DeleteUser(ctx, 42, authDeleter); err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}
	if len(lifecycle.deleteCalls) != 1 || lifecycle.deleteCalls[0] != 42 {
		t.Fatalf("lifecycle delete calls=%v, want one call for user 42", lifecycle.deleteCalls)
	}
	if len(authDeleter.authUserIDs) != 0 {
		t.Fatalf("UserService must not execute Auth deletion, got calls=%v", authDeleter.authUserIDs)
	}
}
