package users

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"ideaboard/api/internal/identity/autherrors"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/validation"
)

const maxUserKeywordLength = 64

type UserService struct {
	repo          *UserRepository
	uploadCleaner UserUploadCleaner
	lifecycle     UserLifecycle
}

type UserLifecycle interface {
	SetUserStatus(context.Context, int64, int16) error
	BeginUserDeletion(context.Context, int64) (*DeletionProgress, error)
	AdvanceUserDeletion(context.Context, int64, DeletionPhase, DeletionPhase) error
	TombstoneUser(context.Context, int64) error
}

type AuthUserDeleter interface {
	DeleteUser(context.Context, string) error
}

type UserUploadCleaner interface {
	DeleteUserUploads(ctx context.Context, userID int64, storageUID string) error
}

func NewUserService(repo *UserRepository, lifecycle UserLifecycle, uploadCleaner ...UserUploadCleaner) *UserService {
	var cleaner UserUploadCleaner
	if len(uploadCleaner) > 0 {
		cleaner = uploadCleaner[0]
	}
	return &UserService{
		repo:          repo,
		uploadCleaner: cleaner,
		lifecycle:     lifecycle,
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
		return nil, validation.New("keyword 长度不能超过 64 个字符")
	}
	if req.Role != "" && req.Role != "admin" && req.Role != "user" {
		return nil, validation.New("role 仅支持 admin 或 user")
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
		return 0, false, validation.New("status 仅支持 0 或 1")
	}
	return int16(status), true, nil
}

// UpdateStatus 更新用户状态
func (s *UserService) UpdateStatus(ctx context.Context, userID int64, status int16) error {
	if status != 0 && status != 1 {
		return errors.New("状态值无效，仅支持0或1")
	}

	if s.lifecycle == nil {
		return errors.New("用户生命周期服务未配置")
	}
	err := s.lifecycle.SetUserStatus(ctx, userID, status)
	if err != nil {
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
func (s *UserService) DeleteUser(ctx context.Context, userID int64, authDeleter AuthUserDeleter) error {
	if s.lifecycle == nil || authDeleter == nil || s.uploadCleaner == nil {
		return errors.New("用户删除依赖未配置")
	}
	progress, err := s.lifecycle.BeginUserDeletion(ctx, userID)
	if err != nil {
		if database.IsNotFound(err) {
			return errors.New("用户不存在")
		}
		return err
	}

	if progress.Phase == DeletionPendingAuth {
		if err := authDeleter.DeleteUser(ctx, progress.AuthUserID); err != nil && !errors.Is(err, autherrors.ErrAuthUserNotFound) {
			return fmt.Errorf("删除 Auth 用户失败: %w", err)
		}
		if err := s.lifecycle.AdvanceUserDeletion(ctx, userID, DeletionPendingAuth, DeletionPendingStorage); err != nil {
			return fmt.Errorf("记录 Auth 用户已删除失败: %w", err)
		}
		progress.Phase = DeletionPendingStorage
	}

	if progress.Phase == DeletionPendingStorage {
		if err := s.uploadCleaner.DeleteUserUploads(ctx, progress.UserID, progress.StorageUID); err != nil {
			return fmt.Errorf("删除用户文件失败: %w", err)
		}
		if err := s.lifecycle.AdvanceUserDeletion(ctx, userID, DeletionPendingStorage, DeletionPendingTombstone); err != nil {
			return fmt.Errorf("记录用户文件已清理失败: %w", err)
		}
		progress.Phase = DeletionPendingTombstone
	}

	if progress.Phase == DeletionPendingTombstone {
		if err := s.lifecycle.TombstoneUser(ctx, userID); err != nil {
			return fmt.Errorf("匿名化用户失败: %w", err)
		}
	}
	return nil
}
