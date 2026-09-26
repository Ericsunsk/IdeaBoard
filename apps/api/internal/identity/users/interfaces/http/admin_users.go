package http

import (
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/validation"

	"github.com/gin-gonic/gin"
)

func (h *AdminHandler) ListUsers(c *gin.Context) {
	var req ports.UserListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		writeIdentityRequestInvalid(c, "参数错误")
		return
	}

	result, err := h.userService.ListUsers(c.Request.Context(), &req)
	if err != nil {
		if validation.Is(err) {
			writeIdentityRequestInvalid(c, identityValidationMessage(err, "参数错误"))
			return
		}
		response.InternalError(c, 500, "获取用户列表失败", err)
		return
	}

	response.SuccessDTO(c, result)
}

func (h *AdminHandler) UpdateUserRole(c *gin.Context) {
	actor, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	commands, ok := h.requireIdentityAdminCommands(c)
	if !ok {
		return
	}
	publicID, ok := getTargetUserPublicIDParam(c)
	if !ok {
		return
	}

	var req struct {
		Role string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeIdentityRequestInvalid(c, "参数错误")
		return
	}

	if err := commands.SetRole(c.Request.Context(), actor, publicID, req.Role); err != nil {
		RespondIdentityAdminCommandError(c, err)
		return
	}

	response.SuccessDTO(c, gin.H{"updated": true})
}

func (h *AdminHandler) BanUser(c *gin.Context) {
	actor, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	commands, ok := h.requireIdentityAdminCommands(c)
	if !ok {
		return
	}
	publicID, ok := getTargetUserPublicIDParam(c)
	if !ok {
		return
	}

	var req ports.AdminBanUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeIdentityRequestInvalid(c, "参数错误")
		return
	}
	if req.ExpiresInDays < 0 {
		writeIdentityRequestInvalid(c, "expires_in_days 不能小于 0")
		return
	}

	if err := commands.Ban(c.Request.Context(), actor, publicID, ports.IdentityAdminBanInput{
		Reason:        strings.TrimSpace(req.Reason),
		ExpiresInDays: req.ExpiresInDays,
	}); err != nil {
		RespondIdentityAdminCommandError(c, err)
		return
	}

	response.SuccessDTO(c, gin.H{"banned": true})
}

func (h *AdminHandler) UnbanUser(c *gin.Context) {
	actor, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	commands, ok := h.requireIdentityAdminCommands(c)
	if !ok {
		return
	}
	publicID, ok := getTargetUserPublicIDParam(c)
	if !ok {
		return
	}

	if err := commands.Unban(c.Request.Context(), actor, publicID); err != nil {
		RespondIdentityAdminCommandError(c, err)
		return
	}

	response.SuccessDTO(c, gin.H{"unbanned": true})
}

func (h *AdminHandler) RevokeUserSessions(c *gin.Context) {
	actor, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	commands, ok := h.requireIdentityAdminCommands(c)
	if !ok {
		return
	}
	publicID, ok := getTargetUserPublicIDParam(c)
	if !ok {
		return
	}

	if err := commands.RevokeSessions(c.Request.Context(), actor, publicID); err != nil {
		RespondIdentityAdminCommandError(c, err)
		return
	}

	response.SuccessDTO(c, gin.H{"revoked": true})
}

func (h *AdminHandler) DeleteUser(c *gin.Context) {
	actor, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	commands, ok := h.requireIdentityAdminCommands(c)
	if !ok {
		return
	}
	publicID, ok := getTargetUserPublicIDParam(c)
	if !ok {
		return
	}

	if err := commands.Delete(c.Request.Context(), actor, publicID); err != nil {
		RespondIdentityAdminCommandError(c, err)
		return
	}

	response.SuccessDTO(c, gin.H{"deleted": true})
}
