package users

import (
	"errors"
		
	"golang.org/x/crypto/bcrypt"
)

type UserService struct {
	repo *UserRepository
}

func NewUserService(repo *UserRepository) *UserService {
	return &UserService{repo: repo}
}

// Register 用户注册
func (s *UserService) Register(req *RegisterRequest) (*User, error) {
	// 检查用户名是否已存在
	exists, err := s.repo.ExistsUsername(req.Username)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errors.New("用户名已存在")
	}

	// 加密密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// 创建用户
	user := &User{
		Username: req.Username,
		Password: string(hashedPassword),
		Status:   1,
	}

	if err := s.repo.Create(user); err != nil {
		return nil, err
	}

	return user, nil
}

// Login 用户登录
func (s *UserService) Login(req *LoginRequest) (*User, error) {
	user, err := s.repo.GetByUsername(req.Username)
	if err != nil {
		return nil, errors.New("用户名或密码错误")
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, errors.New("用户名或密码错误")
	}

	// 检查用户状态
	if user.Status == 0 {
		return nil, errors.New("账号已被封禁")
	}

	return user, nil
}

// GetByID 根据ID获取用户
func (s *UserService) GetByID(id uint64) (*User, error) {
	return s.repo.GetByID(id)
}

// UpdatePassword 修改密码
func (s *UserService) UpdatePassword(userID uint64, oldPassword, newPassword string) error {
	user, err := s.repo.GetByID(userID)
	if err != nil {
		return errors.New("用户不存在")
	}

	// 验证旧密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(oldPassword)); err != nil {
		return errors.New("原密码错误")
	}

	// 加密新密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	return s.repo.UpdatePassword(userID, string(hashedPassword))
}

// AdminUpdatePassword 管理员修改用户密码
func (s *UserService) AdminUpdatePassword(userID uint64, newPassword string) error {
	user, err := s.repo.GetByID(userID)
	if err != nil {
		return errors.New("用户不存在")
	}

	// 加密新密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	return s.repo.UpdatePassword(user.ID, string(hashedPassword))
}

// CreateUser 管理员创建用户
func (s *UserService) CreateUser(req *CreateUserRequest) (*User, error) {
	// 检查用户名是否已存在
	exists, err := s.repo.ExistsUsername(req.Username)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errors.New("用户名已存在")
	}

	// 加密密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// 创建用户
	user := &User{
		Username: req.Username,
		Password: string(hashedPassword),
		Status:   1,
	}

	if err := s.repo.Create(user); err != nil {
		return nil, err
	}

	return user, nil
}

// ListUsers 获取用户列表
func (s *UserService) ListUsers(req *UserListRequest) (*UserListResponse, error) {
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 10
	}

	users, total, err := s.repo.List(req.Keyword, req.Page, req.PageSize)
	if err != nil {
		return nil, err
	}

	return &UserListResponse{
		Total: total,
		List:  users,
	}, nil
}

// UpdateStatus 更新用户状态
func (s *UserService) UpdateStatus(userID uint64, status int8) error {
	return s.repo.UpdateStatus(userID, status)
}

// DeleteUser 删除用户
func (s *UserService) DeleteUser(userID uint64) error {
	return s.repo.Delete(userID)
}

// HashPassword 哈希密码（用于初始化）
func (s *UserService) HashPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedPassword), nil
}
