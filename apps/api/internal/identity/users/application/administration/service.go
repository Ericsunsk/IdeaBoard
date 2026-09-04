package administration

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"ideaboard/api/internal/identity/authidentity"
	"ideaboard/api/internal/identity/users/domain"
	"ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/validation"
)

const maxUserKeywordLength = 64

type Service struct {
	repo                ports.UserRepository
	creditBalanceReader ports.UserCreditBalanceReader
	uploadCleaner       ports.UserUploadCleaner
	emailReader         ports.UserEmailReader
	lifecycle           ports.UserLifecycle
}

func NewService(
	repo ports.UserRepository,
	lifecycle ports.UserLifecycle,
	creditBalanceReader ports.UserCreditBalanceReader,
	uploadCleaner ports.UserUploadCleaner,
	emailReader ports.UserEmailReader,
) *Service {
	return &Service{
		repo:                repo,
		creditBalanceReader: creditBalanceReader,
		uploadCleaner:       uploadCleaner,
		emailReader:         emailReader,
		lifecycle:           lifecycle,
	}
}

// GetByID 根据ID获取用户
func (s *Service) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) GetByPublicID(ctx context.Context, publicID string) (*domain.User, error) {
	return s.repo.GetByPublicID(ctx, publicID)
}

// ListUsers 获取用户列表
func (s *Service) ListUsers(ctx context.Context, req *ports.UserListRequest) (*ports.UserListResponse, error) {
	status, hasStatus, err := normalizeUserListRequest(req)
	if err != nil {
		return nil, err
	}

	users, counts, err := s.repo.List(ctx, req.Keyword, req.Role, status, hasStatus, req.Page, req.PageSize)
	if err != nil {
		return nil, err
	}

	list, err := s.enrichUserList(ctx, users)
	if err != nil {
		return nil, err
	}

	return &ports.UserListResponse{
		UserListCounts: counts,
		List:           list,
	}, nil
}

func normalizeUserListRequest(req *ports.UserListRequest) (int16, bool, error) {
	req.Keyword = strings.TrimSpace(req.Keyword)
	req.Role = normalizeUserListRole(req.Role)
	if utf8.RuneCountInString(req.Keyword) > maxUserKeywordLength {
		return 0, false, validation.New("keyword 长度不能超过 64 个字符")
	}
	if req.Role != "" && req.Role != domain.RoleAdmin && req.Role != domain.RoleUser {
		return 0, false, validation.New("role 仅支持 admin 或 user")
	}
	status, hasStatus, err := normalizeUserListStatus(req.Status)
	if err != nil {
		return 0, false, err
	}
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 10
	}
	return status, hasStatus, nil
}

func (s *Service) enrichUserList(ctx context.Context, users []*domain.User) ([]*ports.AdminUserListItem, error) {
	list := make([]*ports.AdminUserListItem, 0, len(users))
	if len(users) == 0 {
		return list, nil
	}
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
	identities := make(map[string]authidentity.UserIdentity)
	if s.emailReader != nil {
		authUserIDs := make([]string, 0, len(users))
		for _, user := range users {
			if user.AuthUserID != nil {
				authUserIDs = append(authUserIDs, *user.AuthUserID)
			}
		}
		identities, err = s.emailReader.GetUserEmails(ctx, authUserIDs)
		if err != nil {
			return nil, fmt.Errorf("读取用户邮箱失败: %w", err)
		}
	}
	for _, user := range users {
		availableCredits, ok := balances[user.ID]
		if !ok {
			return nil, fmt.Errorf("用户 %d 的Idea余额缺失", user.ID)
		}
		identity := identities[authUserIDValue(user.AuthUserID)]
		list = append(list, &ports.AdminUserListItem{
			PublicID:         user.PublicID,
			Email:            identity.Email,
			Providers:        append([]string(nil), identity.Providers...),
			DisplayName:      user.DisplayName,
			Avatar:           user.Avatar,
			Role:             user.Role,
			Status:           user.Status,
			CreatedAt:        user.CreatedAt,
			UpdatedAt:        user.UpdatedAt,
			AvailableCredits: availableCredits,
			LastLoginIP:      user.LastLoginIP,
			LastLoginCountry: user.LastLoginCountry,
		})
	}
	return list, nil
}

func authUserIDValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
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
func (s *Service) UpdateStatus(ctx context.Context, userID int64, status int16) error {
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

func (s *Service) UpdateRole(ctx context.Context, userID int64, role string) error {
	normalizedRole := strings.ToLower(strings.TrimSpace(role))
	if normalizedRole != domain.RoleAdmin && normalizedRole != domain.RoleUser {
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
func (s *Service) DeleteUser(ctx context.Context, userID int64, authDeleter ports.AuthUserDeleter) error {
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

var _ ports.UserMutationQueries = (*Service)(nil)
