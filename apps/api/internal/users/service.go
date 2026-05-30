package users

import (
	"context"
	"errors"
	"log"
	"strings"
	"unicode/utf8"
)

const maxUserKeywordLength = 64

type UserService struct {
	repo             *UserRepository
	workspaceCleaner UserWorkspaceCleaner
}

type UserWorkspaceCleaner interface {
	DeleteWorkspaceByStorageUID(ctx context.Context, storageUID string) error
}

func NewUserService(repo *UserRepository, workspaceCleaner ...UserWorkspaceCleaner) *UserService {
	var cleaner UserWorkspaceCleaner
	if len(workspaceCleaner) > 0 {
		cleaner = workspaceCleaner[0]
	}
	return &UserService{
		repo:             repo,
		workspaceCleaner: cleaner,
	}
}

// GetByID 根据ID获取用户
func (s *UserService) GetByID(ctx context.Context, id uint64) (*User, error) {
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
func (s *UserService) UpdateStatus(ctx context.Context, userID uint64, status int8) error {
	if status != 0 && status != 1 {
		return errors.New("状态值无效，仅支持0或1")
	}

	return s.repo.UpdateStatus(ctx, userID, status)
}

func (s *UserService) UpdateRole(ctx context.Context, userID uint64, role string) error {
	normalizedRole := strings.ToLower(strings.TrimSpace(role))
	if normalizedRole != "admin" && normalizedRole != "user" {
		return errors.New("角色值无效，仅支持 admin 或 user")
	}

	return s.repo.UpdateRoleByID(ctx, userID, normalizedRole)
}

// DeleteUser 删除用户
func (s *UserService) DeleteUser(ctx context.Context, userID uint64) error {
	targetUser, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return errors.New("用户不存在")
	}

	if s.workspaceCleaner != nil {
		if err := s.workspaceCleaner.DeleteWorkspaceByStorageUID(ctx, targetUser.StorageUID); err != nil {
			log.Printf("[users] delete workspace uploads failed for user_id=%d uid=%s: %v", targetUser.ID, targetUser.StorageUID, err)
			return errors.New("删除失败：用户工作区文件清理失败，请查看服务日志")
		}
	}

	return s.repo.Delete(ctx, userID)
}
