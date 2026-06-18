package auth

import (
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/users"

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
	userID := c.GetInt64("user_id")
	if userID == 0 {
		response.Error(c, 401, "未登录")
		return
	}

	user, err := h.userService.GetByID(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, 404, "用户不存在")
		return
	}

	response.SuccessDTO(c, currentUserResponse{User: user})
}
