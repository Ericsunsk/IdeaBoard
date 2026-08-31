package auth

import (
	"net/url"
	"strings"
	"time"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/identity/users"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	userService *users.UserService
}

type currentUserResponse struct {
	User currentUserPublicInfo `json:"user"`
}

type currentUserPublicInfo struct {
	PublicID         string    `json:"public_id"`
	DisplayName      string    `json:"display_name"`
	StorageNamespace string    `json:"storage_namespace"`
	Avatar           string    `json:"avatar,omitempty"`
	Role             string    `json:"role"`
	Status           int16     `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func NewAuthHandler(userService *users.UserService) *AuthHandler {
	return &AuthHandler{
		userService: userService,
	}
}

// GetCurrentUser 获取当前用户信息
func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return
	}

	user, err := h.userService.GetByID(c.Request.Context(), identityPrincipal.UserID())
	if err != nil {
		response.Error(c, 404, "用户不存在")
		return
	}

	response.SuccessDTO(c, currentUserResponse{User: currentUserPublicInfo{
		PublicID: user.PublicID.String(), DisplayName: user.DisplayName, StorageNamespace: user.StorageUID,
		Avatar: canonicalProfileAvatarURL(user.Avatar), Role: user.Role, Status: user.Status, CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt,
	}})
}

func canonicalProfileAvatarURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || !strings.HasPrefix("/"+strings.TrimPrefix(parsed.Path, "/"), "/uploads/") {
		return trimmed
	}
	if parsed.IsAbs() && !strings.EqualFold(parsed.Host, "ideaboard.art") {
		return trimmed
	}
	return "/api/user/profile/avatar/content"
}
