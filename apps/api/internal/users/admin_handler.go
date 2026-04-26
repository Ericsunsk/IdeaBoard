package users

import (
	"strconv"

	"api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type AdminHandler struct {
	userService *UserService
}

func NewAdminHandler(userService *UserService) *AdminHandler {
	return &AdminHandler{
		userService: userService,
	}
}

// ListUsers 获取用户列表
func (h *AdminHandler) ListUsers(c *gin.Context) {
	var req UserListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	result, err := h.userService.ListUsers(&req)
	if err != nil {
		response.InternalError(c, 500, "获取用户列表失败", err)
		return
	}

	response.Success(c, result)
}

// CreateUser 创建用户
func (h *AdminHandler) CreateUser(c *gin.Context) {
	response.Error(c, 400, "请使用注册页面创建账号，管理员后台已停用手工建号")
}

// UpdateUserStatus 更新用户状态
func (h *AdminHandler) UpdateUserStatus(c *gin.Context) {
	var req struct {
		Status *int8 `json:"status" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	targetUser, ok := h.getTargetUserByParam(c)
	if !ok {
		return
	}

	if targetUser.Role == "admin" {
		response.Error(c, 400, "不能修改管理员状态")
		return
	}

	if err := h.userService.UpdateStatus(targetUser.ID, *req.Status); err != nil {
		response.Error(c, 400, err.Error())
		return
	}

	response.Success(c, nil)
}

// UpdateUserPassword 管理员修改用户密码
func (h *AdminHandler) UpdateUserPassword(c *gin.Context) {
	response.Error(c, 400, "Better Auth 接管后不支持后台改密，请在 Better Auth Studio 或前台找回流程处理")
}

// DeleteUser 删除用户
func (h *AdminHandler) DeleteUser(c *gin.Context) {
	response.Error(c, 400, "Better Auth 接管后已停用后台删除，请先封禁，再在 Better Auth Studio 执行物理删号")
}

func (h *AdminHandler) getTargetUserByParam(c *gin.Context) (*User, bool) {
	userID := c.Param("id")
	userIDUint, err := strconv.ParseUint(userID, 10, 64)
	if err != nil {
		response.Error(c, 400, "无效的用户ID")
		return nil, false
	}

	user, err := h.userService.GetByID(userIDUint)
	if err != nil {
		response.Error(c, 400, "用户不存在")
		return nil, false
	}

	return user, true
}
