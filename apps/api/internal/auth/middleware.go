package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"api/internal/platform/response"
	"api/internal/users"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AuthMiddleware 认证中间件：通过 Better Auth session 校验并映射本地业务用户。
func AuthMiddleware(authService *ServiceClient, userRepo *users.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		if authService == nil || userRepo == nil {
			response.InternalError(c, http.StatusInternalServerError, "认证服务未配置", fmt.Errorf("auth middleware dependencies missing"))
			c.Abort()
			return
		}

		cookieHeader := strings.TrimSpace(c.GetHeader("Cookie"))
		if cookieHeader == "" {
			response.Error(c, http.StatusUnauthorized, "未登录")
			c.Abort()
			return
		}

		identity, err := authService.ValidateSession(c.Request.Context(), cookieHeader)
		if err != nil {
			switch {
			case errors.Is(err, ErrSessionUnauthorized):
				response.Error(c, http.StatusUnauthorized, "会话已失效，请重新登录")
			default:
				response.InternalError(c, http.StatusBadGateway, "认证服务暂不可用", err)
			}
			c.Abort()
			return
		}

		user, err := resolveLocalUserByAuthIdentity(userRepo, identity)
		if err != nil {
			response.InternalError(c, http.StatusUnauthorized, "用户不存在或已停用", err)
			c.Abort()
			return
		}
		if user.Status != 1 {
			response.Error(c, http.StatusUnauthorized, "用户不存在或已停用")
			c.Abort()
			return
		}

		c.Set("user_id", user.ID)
		c.Set("username", user.Username)
		c.Set("role", user.Role)
		c.Set("auth_user_id", identity.AuthUserID)
		c.Next()
	}
}

func resolveLocalUserByAuthIdentity(userRepo *users.UserRepository, identity *SessionIdentity) (*users.User, error) {
	if userRepo == nil || identity == nil {
		return nil, fmt.Errorf("invalid auth identity mapping input")
	}
	authUserID := strings.TrimSpace(identity.AuthUserID)
	if authUserID == "" {
		return nil, fmt.Errorf("auth user id is empty")
	}

	user, err := userRepo.GetByAuthUserID(authUserID)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	preferredUsername := users.NormalizeAuthUsername(identity.Username, identity.Email, authUserID)
	if preferredUsername != "" {
		existing, lookupErr := userRepo.GetByUsername(preferredUsername)
		if lookupErr == nil {
			existingAuthUserID := strings.TrimSpace(derefString(existing.AuthUserID))
			if strings.EqualFold(existingAuthUserID, authUserID) {
				return existing, nil
			}
		} else if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return nil, lookupErr
		}
	}

	for attempt := 0; attempt < 20; attempt++ {
		candidateUsername := users.BuildAuthUsernameCandidate(preferredUsername, authUserID, attempt)
		localUser := &users.User{
			Username:   candidateUsername,
			AuthUserID: ptrString(authUserID),
			Status:     1,
			Role:       "user",
		}
		createErr := userRepo.Create(localUser)
		if createErr == nil {
			return localUser, nil
		}
		if !users.IsDuplicateConstraintError(createErr) {
			return nil, createErr
		}
	}

	user, err = userRepo.GetByAuthUserID(authUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve user profile for auth user %s: %w", authUserID, err)
	}
	return user, nil
}

func ptrString(value string) *string {
	v := strings.TrimSpace(value)
	if v == "" {
		return nil
	}
	return &v
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// AdminMiddleware 管理员中间件 - 通过角色判断是否为管理员
func AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists || role != "admin" {
			response.Error(c, http.StatusForbidden, "需要管理员权限")
			c.Abort()
			return
		}
		c.Next()
	}
}
