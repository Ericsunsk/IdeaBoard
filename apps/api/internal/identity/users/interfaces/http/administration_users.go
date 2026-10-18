package http

import (
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/platform/errorrender"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

func (h *AdminHandler) ListUsers(c *gin.Context) {
	var req ports.UserListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		writeIdentityRequestInvalid(c, errorrender.MsgRespondRuntimeSettingsWriteError)
		return
	}

	result, err := h.userService.ListUsers(c.Request.Context(), &req)
	if err != nil {
		if errorrender.IsValidationError(err) {
			writeIdentityRequestInvalid(c, identityValidationMessage(err, errorrender.MsgRespondRuntimeSettingsWriteError))
			return
		}
		response.InternalError(c, 500, errorrender.MsgListUsers, err)
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
		writeIdentityRequestInvalid(c, errorrender.MsgRespondRuntimeSettingsWriteError)
		return
	}

	outcome, err := commands.SetRole(c.Request.Context(), actor, publicID, req.Role)
	if err != nil {
		RespondIdentityAdminCommandError(c, err)
		return
	}
	response.SuccessDTO(c, gin.H{"updated": true, "auth_sync_pending": outcome.SyncPending})
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
		writeIdentityRequestInvalid(c, errorrender.MsgRespondRuntimeSettingsWriteError)
		return
	}
	if req.ExpiresInDays < 0 {
		writeIdentityRequestInvalid(c, errorrender.MsgBanUser)
		return
	}

	outcome, err := commands.Ban(c.Request.Context(), actor, publicID, ports.IdentityAdminBanInput(req))
	if err != nil {
		RespondIdentityAdminCommandError(c, err)
		return
	}
	response.SuccessDTO(c, gin.H{"banned": true, "auth_sync_pending": outcome.SyncPending})
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

	outcome, err := commands.Unban(c.Request.Context(), actor, publicID)
	if err != nil {
		RespondIdentityAdminCommandError(c, err)
		return
	}
	response.SuccessDTO(c, gin.H{"unbanned": true, "auth_sync_pending": outcome.SyncPending})
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

	if _, err := commands.RevokeSessions(c.Request.Context(), actor, publicID); err != nil {
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

	if _, err := commands.Delete(c.Request.Context(), actor, publicID); err != nil {
		RespondIdentityAdminCommandError(c, err)
		return
	}

	response.SuccessDTO(c, gin.H{"deleted": true})
}
