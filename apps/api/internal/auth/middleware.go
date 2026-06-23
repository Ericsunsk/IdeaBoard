package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/users"

	"github.com/gin-gonic/gin"
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
			if abortIfRequestCanceled(c, err) {
				return
			}
			switch {
			case errors.Is(err, ErrSessionUnauthorized):
				response.Error(c, http.StatusUnauthorized, "会话已失效，请重新登录")
			default:
				response.InternalError(c, http.StatusBadGateway, "认证服务暂不可用", err)
			}
			c.Abort()
			return
		}

		user, err := resolveLocalUserByAuthIdentity(c.Request.Context(), userRepo, identity)
		if err != nil {
			if abortIfRequestCanceled(c, err) {
				return
			}
			if database.IsNotFound(err) {
				response.Error(c, http.StatusUnauthorized, "用户不存在或已停用")
			} else {
				response.InternalError(c, http.StatusServiceUnavailable, "用户服务暂不可用", err)
			}
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

func abortIfRequestCanceled(c *gin.Context, err error) bool {
	if !isRequestCanceled(c.Request.Context(), err) {
		return false
	}
	c.Abort()
	return true
}

func isRequestCanceled(ctx context.Context, err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled)
}

func resolveLocalUserByAuthIdentity(ctx context.Context, userRepo *users.UserRepository, identity *SessionIdentity) (*users.User, error) {
	if userRepo == nil || identity == nil {
		return nil, fmt.Errorf("invalid auth identity mapping input")
	}
	authUserID := strings.TrimSpace(identity.AuthUserID)
	if authUserID == "" {
		return nil, fmt.Errorf("auth user id is empty")
	}

	user, err := userRepo.GetByAuthUserID(ctx, authUserID)
	if err != nil {
		return nil, err
	}
	return user, nil
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
