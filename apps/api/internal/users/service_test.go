package users

import (
	"context"
	"strings"
	"testing"
)

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
