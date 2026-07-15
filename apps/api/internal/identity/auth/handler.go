package auth

import (
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/identity/users"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	userService *users.UserService
}

type currentUserResponse struct {
	User *users.User `json:"user"`
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

	response.SuccessDTO(c, currentUserResponse{User: user})
}
