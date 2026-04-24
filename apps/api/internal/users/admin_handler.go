package users

import (
	"fmt"
	"strconv"

	"api/internal/platform/redis"
	"api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type AdminHandler struct {
	userService *UserService
	rdb         *redis.Client
}

func NewAdminHandler(userService *UserService, rdb *redis.Client) *AdminHandler {
	return &AdminHandler{
		userService: userService,
		rdb:         rdb,
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
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	user, err := h.userService.CreateUser(&req)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}

	response.Success(c, gin.H{
		"user": user,
	})
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
	if *req.Status == 0 {
		h.revokeUserSessionToken(c, targetUser.ID)
	}

	response.Success(c, nil)
}

// UpdateUserPassword 管理员修改用户密码
func (h *AdminHandler) UpdateUserPassword(c *gin.Context) {
	var req UpdateUserPasswordRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	targetUser, ok := h.getTargetUserByParam(c)
	if !ok {
		return
	}

	if targetUser.Role == "admin" {
		response.Error(c, 400, "不能修改管理员密码")
		return
	}

	if err := h.userService.AdminUpdatePassword(targetUser.ID, req.NewPassword); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	h.revokeUserSessionToken(c, targetUser.ID)

	response.Success(c, nil)
}

// DeleteUser 删除用户
func (h *AdminHandler) DeleteUser(c *gin.Context) {
	targetUser, ok := h.getTargetUserByParam(c)
	if !ok {
		return
	}

	if targetUser.Role == "admin" {
		response.Error(c, 400, "不能删除管理员")
		return
	}

	if err := h.userService.DeleteUser(targetUser.ID); err != nil {
		response.InternalError(c, 500, "删除用户失败", err)
		return
	}
	h.revokeUserSessionToken(c, targetUser.ID)

	response.Success(c, nil)
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

func userSessionTokenKey(userID uint64) string {
	return fmt.Sprintf("token:%d", userID)
}

func (h *AdminHandler) revokeUserSessionToken(c *gin.Context, userID uint64) {
	if h.rdb == nil {
		return
	}
	_ = h.rdb.Del(c.Request.Context(), userSessionTokenKey(userID))
}
