package users

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/validation"
)

const maxUserKeywordLength = 64

type UserService struct {
	repo                *UserRepository
	creditBalanceReader UserCreditBalanceReader
	uploadCleaner       UserUploadCleaner
	emailReader         UserEmailReader
	lifecycle           UserLifecycle
}

type UserLifecycle interface {
	SetUserStatus(context.Context, int64, int16) error
	DeleteUser(context.Context, int64, AuthUserDeleter, UserUploadCleaner) error
}

type UserCreditBalanceReader interface {
	GetAvailableBalances(context.Context, []int64) (map[int64]int64, error)
}

type UserEmailReader interface {
	GetUserEmails(context.Context, []string) (map[string]string, error)
}

func NewUserService(
	repo *UserRepository,
	lifecycle UserLifecycle,
	creditBalanceReader UserCreditBalanceReader,
	uploadCleaner ...UserUploadCleaner,
) *UserService {
	var cleaner UserUploadCleaner
	if len(uploadCleaner) > 0 {
		cleaner = uploadCleaner[0]
	}
	return newUserService(repo, lifecycle, creditBalanceReader, cleaner, nil)
}

func NewUserServiceWithEmailReader(
	repo *UserRepository,
	lifecycle UserLifecycle,
	creditBalanceReader UserCreditBalanceReader,
	uploadCleaner UserUploadCleaner,
	emailReader UserEmailReader,
) *UserService {
	return newUserService(repo, lifecycle, creditBalanceReader, uploadCleaner, emailReader)
}

func newUserService(
	repo *UserRepository,
	lifecycle UserLifecycle,
	creditBalanceReader UserCreditBalanceReader,
	uploadCleaner UserUploadCleaner,
	emailReader UserEmailReader,
) *UserService {
	return &UserService{
		repo:                repo,
		creditBalanceReader: creditBalanceReader,
		uploadCleaner:       uploadCleaner,
		emailReader:         emailReader,
		lifecycle:           lifecycle,
	}
}

// GetByID 根据ID获取用户
func (s *UserService) GetByID(ctx context.Context, id int64) (*User, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *UserService) GetByPublicID(ctx context.Context, publicID string) (*User, error) {
	return s.repo.GetByPublicID(ctx, publicID)
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

	list := make([]*AdminUserListItem, 0, len(users))
	if len(users) > 0 {
		if s.creditBalanceReader == nil {
			return nil, errors.New("用户Idea余额读取服务未配置")
		}
		userIDs := make([]int64, 0, len(users))
		for _, user := range users {
			userIDs = append(userIDs, user.ID)
		}
		balances, err := s.creditBalanceReader.GetAvailableBalances(ctx, userIDs)
		if err != nil {
			return nil, fmt.Errorf("读取用户Idea余额失败: %w", err)
		}
		emails := make(map[string]string)
		if s.emailReader != nil {
			authUserIDs := make([]string, 0, len(users))
			for _, user := range users {
				if user.AuthUserID != nil {
					authUserIDs = append(authUserIDs, *user.AuthUserID)
				}
			}
			emails, err = s.emailReader.GetUserEmails(ctx, authUserIDs)
			if err != nil {
				return nil, fmt.Errorf("读取用户邮箱失败: %w", err)
			}
		}
		for _, user := range users {
			availableCredits, ok := balances[user.ID]
			if !ok {
				return nil, fmt.Errorf("用户 %d 的Idea余额缺失", user.ID)
			}
			list = append(list, &AdminUserListItem{
				PublicID:         user.PublicID,
				Email:            userEmail(emails, user.AuthUserID),
				DisplayName:      user.DisplayName,
				Avatar:           user.Avatar,
				Role:             user.Role,
				Status:           user.Status,
				CreatedAt:        user.CreatedAt,
				UpdatedAt:        user.UpdatedAt,
				AvailableCredits: availableCredits,
			})
		}
	}

	return &UserListResponse{
		UserListCounts: counts,
		List:           list,
	}, nil
}

func userEmail(emails map[string]string, authUserID *string) string {
	if authUserID == nil {
		return ""
	}
	return emails[*authUserID]
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
	if s.lifecycle == nil {
		return errors.New("用户删除依赖未配置")
	}
	err := s.lifecycle.DeleteUser(ctx, userID, authDeleter, s.uploadCleaner)
	if err != nil {
		if database.IsNotFound(err) {
			return errors.New("用户不存在")
		}
		return err
	}
	return nil
}
