package handler

import (
	"fmt"
	"backend/internal/model"
	"backend/internal/service"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type AdminHandler struct {
	userService *service.UserService
}

func NewAdminHandler(userService *service.UserService) *AdminHandler {
	return &AdminHandler{
		userService: userService,
	}
}

// ListUsers 获取用户列表
func (h *AdminHandler) ListUsers(c *gin.Context) {
	var req model.UserListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}

	result, err := h.userService.ListUsers(&req)
	if err != nil {
		response.Error(c, 500, "获取用户列表失败: "+err.Error())
		return
	}

	response.Success(c, result)
}

// CreateUser 创建用户
func (h *AdminHandler) CreateUser(c *gin.Context) {
	var req model.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
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
	userID := c.Param("id")

	var req struct {
		Status int8 `json:"status" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}

	var userIDUint uint64
	if _, err := fmt.Sscanf(userID, "%d", &userIDUint); err != nil {
		response.Error(c, 400, "无效的用户ID")
		return
	}

	if err := h.userService.UpdateStatus(userIDUint, req.Status); err != nil {
		response.Error(c, 400, err.Error())
		return
	}

	response.Success(c, nil)
}

// UpdateUserPassword 管理员修改用户密码
func (h *AdminHandler) UpdateUserPassword(c *gin.Context) {
	userID := c.Param("id")
	var req model.UpdateUserPasswordRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}

	var userIDUint uint64
	if _, err := fmt.Sscanf(userID, "%d", &userIDUint); err != nil {
		response.Error(c, 400, "无效的用户ID")
		return
	}

	// 不能修改admin的密码
	if userIDUint == 1 {
		response.Error(c, 400, "不能修改管理员密码")
		return
	}

	if err := h.userService.AdminUpdatePassword(userIDUint, req.NewPassword); err != nil {
		response.Error(c, 400, err.Error())
		return
	}

	response.Success(c, nil)
}

// DeleteUser 删除用户
func (h *AdminHandler) DeleteUser(c *gin.Context) {
	userID := c.Param("id")

	var userIDUint uint64
	if _, err := fmt.Sscanf(userID, "%d", &userIDUint); err != nil {
		response.Error(c, 400, "无效的用户ID")
		return
	}

	// 不能删除admin
	if userIDUint == 1 {
		response.Error(c, 400, "不能删除管理员")
		return
	}

	if err := h.userService.DeleteUser(userIDUint); err != nil {
		response.Error(c, 400, err.Error())
		return
	}

	response.Success(c, nil)
}
