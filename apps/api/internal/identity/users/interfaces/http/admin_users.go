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
		writeIdentityRequestInvalid(c, msgRespondRuntimeSettingsWriteError)
		return
	}

	result, err := h.userService.ListUsers(c.Request.Context(), &req)
	if err != nil {
		if validation.Is(err) {
			writeIdentityRequestInvalid(c, identityValidationMessage(err, msgRespondRuntimeSettingsWriteError))
			return
		}
		response.InternalError(c, 500, msgListUsers, err)
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
		writeIdentityRequestInvalid(c, msgRespondRuntimeSettingsWriteError)
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
		writeIdentityRequestInvalid(c, msgRespondRuntimeSettingsWriteError)
		return
	}
	if req.ExpiresInDays < 0 {
		writeIdentityRequestInvalid(c, msgBanUser)
		return
	}

	outcome, err := commands.Ban(c.Request.Context(), actor, publicID, ports.IdentityAdminBanInput{
		Reason:        strings.TrimSpace(req.Reason),
		ExpiresInDays: req.ExpiresInDays,
	})
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
