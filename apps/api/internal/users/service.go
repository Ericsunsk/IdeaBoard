package users

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"unicode/utf8"

	"ideaboard/api/internal/platform/database"
)

const maxUserKeywordLength = 64

type userListValidationError string

func (e userListValidationError) Error() string {
	return string(e)
}

func newUserListValidationError(message string) error {
	return userListValidationError(message)
}

func isUserListValidationError(err error) bool {
	var target userListValidationError
	return errors.As(err, &target)
}

type UserService struct {
	repo          *UserRepository
	uploadCleaner UserUploadCleaner
}

type UserUploadCleaner interface {
	DeleteUserUploads(ctx context.Context, userID int64, storageUID string) error
}

func NewUserService(repo *UserRepository, uploadCleaner ...UserUploadCleaner) *UserService {
	var cleaner UserUploadCleaner
	if len(uploadCleaner) > 0 {
		cleaner = uploadCleaner[0]
	}
	return &UserService{
		repo:          repo,
		uploadCleaner: cleaner,
	}
}

// GetByID 根据ID获取用户
func (s *UserService) GetByID(ctx context.Context, id int64) (*User, error) {
	return s.repo.GetByID(ctx, id)
}

// ListUsers 获取用户列表
func (s *UserService) ListUsers(ctx context.Context, req *UserListRequest) (*UserListResponse, error) {
	req.Keyword = strings.TrimSpace(req.Keyword)
	req.Role = normalizeUserListRole(req.Role)
	if utf8.RuneCountInString(req.Keyword) > maxUserKeywordLength {
		return nil, newUserListValidationError("keyword 长度不能超过 64 个字符")
	}
	if req.Role != "" && req.Role != "admin" && req.Role != "user" {
		return nil, newUserListValidationError("role 仅支持 admin 或 user")
	}
	status, hasStatus, err := normalizeUserListStatus(req.Status)
	if err != nil {
		return nil, err
	}
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 10
	}

	users, counts, err := s.repo.List(ctx, req.Keyword, req.Role, status, hasStatus, req.Page, req.PageSize)
	if err != nil {
		return nil, err
	}

	return &UserListResponse{
		UserListCounts: counts,
		List:           users,
	}, nil
}

func normalizeUserListRole(role string) string {
	return strings.ToLower(strings.TrimSpace(role))
}

func normalizeUserListStatus(raw string) (int16, bool, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, false, nil
	}
	status, err := strconv.Atoi(value)
	if err != nil || (status != 0 && status != 1) {
		return 0, false, newUserListValidationError("status 仅支持 0 或 1")
	}
	return int16(status), true, nil
}

// UpdateStatus 更新用户状态
func (s *UserService) UpdateStatus(ctx context.Context, userID int64, status int16) error {
	if status != 0 && status != 1 {
		return errors.New("状态值无效，仅支持0或1")
	}

	if err := s.repo.UpdateStatus(ctx, userID, status); err != nil {
		if database.IsNotFound(err) {
			return errors.New("用户不存在")
		}
		return err
	}
	return nil
}

func (s *UserService) UpdateRole(ctx context.Context, userID int64, role string) error {
	normalizedRole := strings.ToLower(strings.TrimSpace(role))
	if normalizedRole != "admin" && normalizedRole != "user" {
		return errors.New("角色值无效，仅支持 admin 或 user")
	}

	if err := s.repo.UpdateRoleByID(ctx, userID, normalizedRole); err != nil {
		if database.IsNotFound(err) {
			return errors.New("用户不存在")
		}
		return err
	}
	return nil
}

// DeleteUser 删除用户
func (s *UserService) DeleteUser(ctx context.Context, userID int64) error {
	targetUser, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return errors.New("用户不存在")
	}

	if s.uploadCleaner != nil {
		if err := s.uploadCleaner.DeleteUserUploads(ctx, targetUser.ID, targetUser.StorageUID); err != nil {
			slog.Warn("delete user uploads failed", "user_id", targetUser.ID, "storage_uid", targetUser.StorageUID, "err", err)
			return errors.New("删除失败：用户文件清理失败，请查看服务日志")
		}
	}

	if err := s.repo.Delete(ctx, userID); err != nil {
		if database.IsNotFound(err) {
			return errors.New("用户不存在")
		}
		return err
	}
	return nil
}
