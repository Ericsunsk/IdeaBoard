package users

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"unicode/utf8"
)

const maxUserKeywordLength = 64

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
	if utf8.RuneCountInString(req.Keyword) > maxUserKeywordLength {
		return nil, errors.New("keyword 长度不能超过 64 个字符")
	}
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 10
	}

	users, total, err := s.repo.List(ctx, req.Keyword, req.Page, req.PageSize)
	if err != nil {
		return nil, err
	}

	return &UserListResponse{
		Total: total,
		List:  users,
	}, nil
}

// UpdateStatus 更新用户状态
func (s *UserService) UpdateStatus(ctx context.Context, userID int64, status int16) error {
	if status != 0 && status != 1 {
		return errors.New("状态值无效，仅支持0或1")
	}

	return s.repo.UpdateStatus(ctx, userID, status)
}

func (s *UserService) UpdateRole(ctx context.Context, userID int64, role string) error {
	normalizedRole := strings.ToLower(strings.TrimSpace(role))
	if normalizedRole != "admin" && normalizedRole != "user" {
		return errors.New("角色值无效，仅支持 admin 或 user")
	}

	return s.repo.UpdateRoleByID(ctx, userID, normalizedRole)
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

	return s.repo.Delete(ctx, userID)
}
