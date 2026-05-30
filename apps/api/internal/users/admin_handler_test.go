package users

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"api/internal/autherrors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type fakeAdminAuthManager struct {
	deleteErr         error
	deletedAuthUserID string
}

func (f *fakeAdminAuthManager) DeleteUser(_ context.Context, authUserID string) error {
	f.deletedAuthUserID = authUserID
	return f.deleteErr
}

func (f *fakeAdminAuthManager) SetUserRole(_ context.Context, _ string, _ string) error {
	return nil
}

func TestAdminDeleteUserContinuesLocalCleanupWhenAuthUserAlreadyMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newUserRepoTestDB(t)
	repo := NewUserRepository(db)
	authUserID := "missing-auth-user"
	targetUser := &User{
		Username:   "stale_user",
		AuthUserID: &authUserID,
		Role:       "user",
		Status:     1,
	}
	if err := repo.Create(context.Background(), targetUser); err != nil {
		t.Fatalf("create target user failed: %v", err)
	}
	if err := db.Exec(`INSERT INTO tasks (user_id) VALUES (?)`, targetUser.ID).Error; err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	authManager := &fakeAdminAuthManager{deleteErr: autherrors.ErrAuthUserNotFound}
	handler := NewAdminHandler(NewUserService(repo), nil, authManager)
	router := gin.New()
	router.DELETE("/admin/users/:id", func(c *gin.Context) {
		c.Set("user_id", uint64(999))
		handler.DeleteUser(c)
	})

	req := httptest.NewRequest(http.MethodDelete, "/admin/users/"+strconv.FormatUint(targetUser.ID, 10), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if authManager.deletedAuthUserID != authUserID {
		t.Fatalf("deleted auth user id = %q, want %q", authManager.deletedAuthUserID, authUserID)
	}

	if _, err := repo.GetByID(context.Background(), targetUser.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("target user lookup err = %v, want record not found", err)
	}
	var taskCount int64
	if err := db.Raw(`SELECT COUNT(*) FROM tasks WHERE user_id = ?`, targetUser.ID).Scan(&taskCount).Error; err != nil {
		t.Fatalf("count tasks failed: %v", err)
	}
	if taskCount != 0 {
		t.Fatalf("task count = %d, want 0", taskCount)
	}
}
