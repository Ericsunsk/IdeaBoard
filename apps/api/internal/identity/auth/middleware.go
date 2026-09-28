package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/identity/trust"
	"ideaboard/api/internal/identity/users/domain"
	"ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type localUserReader interface {
	GetByAuthUserID(context.Context, string) (*domain.User, error)
}

type IdentityProvisioner interface {
	ProvisionIdentity(context.Context, ports.VerifiedIdentity) (*ports.ProvisionResult, error)
}

// AuthMiddleware 认证中间件：通过 Better Auth session 校验并映射本地业务用户。
func AuthMiddleware(authService *ServiceClient, userRepo localUserReader, lifecycle IdentityProvisioner) gin.HandlerFunc {
	return func(c *gin.Context) {
		if authService == nil || userRepo == nil {
			response.InternalError(c, http.StatusInternalServerError, msgAuthMiddleware, fmt.Errorf("auth middleware dependencies missing"))
			c.Abort()
			return
		}

		cookieHeader := strings.TrimSpace(c.GetHeader("Cookie"))
		if cookieHeader == "" {
			response.Error(c, http.StatusUnauthorized, msgAuthMiddleware2)
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
				response.Error(c, http.StatusUnauthorized, msgAuthMiddleware3)
			default:
				response.InternalError(c, http.StatusBadGateway, msgAuthMiddleware4, err)
			}
			c.Abort()
			return
		}

		user, err := resolveLocalUserByAuthIdentity(c.Request.Context(), userRepo, lifecycle, identity)
		if err != nil {
			if abortIfRequestCanceled(c, err) {
				return
			}
			if database.IsNotFound(err) {
				response.Error(c, http.StatusUnauthorized, msgAuthMiddleware5)
			} else {
				response.InternalError(c, http.StatusServiceUnavailable, msgAuthMiddleware6, err)
			}
			c.Abort()
			return
		}
		if user.Status != 1 {
			response.Error(c, http.StatusUnauthorized, msgAuthMiddleware5)
			c.Abort()
			return
		}

		identityPrincipal, err := principal.New(user.ID, user.PublicID, identity.AuthUserID, user.DisplayName, user.Role)
		if err != nil || principal.Set(c, identityPrincipal) != nil {
			response.Error(c, http.StatusUnauthorized, msgAuthMiddleware5)
			c.Abort()
			return
		}

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

func resolveLocalUserByAuthIdentity(ctx context.Context, userRepo localUserReader, lifecycle IdentityProvisioner, identity *SessionIdentity) (*domain.User, error) {
	if userRepo == nil || identity == nil {
		return nil, fmt.Errorf("invalid auth identity mapping input")
	}
	authUserID := strings.TrimSpace(identity.AuthUserID)
	if authUserID == "" {
		return nil, fmt.Errorf("auth user id is empty")
	}

	user, err := userRepo.GetByAuthUserID(ctx, authUserID)
	if database.IsNotFound(err) && lifecycle != nil {
		provisioningIdentity, trustErr := trust.ProvisioningIdentityFromSession(*identity, "user")
		if trustErr != nil {
			return nil, fmt.Errorf("map authenticated identity for provisioning: %w", trustErr)
		}
		result, provisionErr := lifecycle.ProvisionIdentity(ctx, provisioningIdentity)
		if provisionErr != nil {
			return nil, provisionErr
		}
		return &result.User, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// AdminMiddleware 管理员中间件 - 通过角色判断是否为管理员
func AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		identityPrincipal, exists := principal.From(c)
		if !exists || !identityPrincipal.IsAdmin() {
			response.Error(c, http.StatusForbidden, msgAdminMiddleware)
			c.Abort()
			return
		}
		c.Next()
	}
}
