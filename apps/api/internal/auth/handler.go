package auth

import (
	"api/internal/platform/response"
	"api/internal/users"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	userService *users.UserService
}

func NewAuthHandler(userService *users.UserService) *AuthHandler {
	return &AuthHandler{
		userService: userService,
	}
}

// GetCurrentUser 获取当前用户信息
func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	userID, _ := c.Get("user_id")
	user, err := h.userService.GetByID(c.Request.Context(), userID.(int64))
	if err != nil {
		response.Error(c, 404, "用户不存在")
		return
	}

	response.Success(c, gin.H{
		"user": user,
	})
}
