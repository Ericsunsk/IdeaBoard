package handler

import (
	"context"
	"fmt"
	"time"

	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/service"
	"backend/pkg/redis"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type AuthHandler struct {
	userService *service.UserService
	rdb         *redis.Client
}

func NewAuthHandler(userService *service.UserService, rdb *redis.Client) *AuthHandler {
	return &AuthHandler{
		userService: userService,
		rdb:         rdb,
	}
}

// Register 注册
func (h *AuthHandler) Register(c *gin.Context) {
	var req model.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}

	user, err := h.userService.Register(&req)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}

	response.Success(c, gin.H{
		"user": user,
	})
}

// Login 登录
func (h *AuthHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}

	user, err := h.userService.Login(&req)
	if err != nil {
		response.Error(c, 401, err.Error())
		return
	}

	// 生成JWT token
	token, err := h.generateToken(user)
	if err != nil {
		response.Error(c, 500, "生成令牌失败")
		return
	}

	// 存储token到Redis
	tokenKey := "token:" + fmt.Sprintf("%d", user.ID)
	if err := h.rdb.SetWithExpiry(context.Background(), tokenKey, token, 24*time.Hour); err != nil {
		response.Error(c, 500, "存储令牌失败")
		return
	}

	response.Success(c, gin.H{
		"token": token,
		"user":  user,
	})
}

// Logout 登出
func (h *AuthHandler) Logout(c *gin.Context) {
	userID, _ := c.Get("user_id")
	tokenKey := "token:" + fmt.Sprintf("%d", userID.(uint64))
	h.rdb.Del(context.Background(), tokenKey)

	response.Success(c, nil)
}

// GetCurrentUser 获取当前用户信息
func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	userID, _ := c.Get("user_id")
	user, err := h.userService.GetByID(userID.(uint64))
	if err != nil {
		response.Error(c, 404, "用户不存在")
		return
	}

	response.Success(c, gin.H{
		"user": user,
	})
}

// UpdatePassword 修改密码
func (h *AuthHandler) UpdatePassword(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var req model.UpdatePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}

	if err := h.userService.UpdatePassword(userID.(uint64), req.OldPassword, req.NewPassword); err != nil {
		response.Error(c, 400, err.Error())
		return
	}

	response.Success(c, nil)
}

// generateToken 生成JWT token
func (h *AuthHandler) generateToken(user *model.User) (string, error) {
	claims := &middleware.Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(middleware.JWTSecret)
}
