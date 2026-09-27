package http

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/users/domain"
	"ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/platform/audit"
	"ideaboard/api/internal/platform/errorrender"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/platform/validation"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AdminUserService interface {
	ListUsers(ctx context.Context, req *ports.UserListRequest) (*ports.UserListResponse, error)
	GetByPublicID(ctx context.Context, publicID string) (*domain.User, error)
}

type AdminHandler struct {
	userService          AdminUserService
	runtimeSettings      *settings.RuntimeSettingsService
	identityCommands     ports.IdentityAdminCommands
	impersonator         ports.AdminUserImpersonator
	impersonationAuditor ImpersonationAuditor
}

// ImpersonationAuditor persists one admin impersonation attempt
// best-effort. *audit.Store satisfies it directly against the immutable
// control_audit_records store; a nil auditor disables durable writes
// while the structured slog record always emits.
type ImpersonationAuditor interface {
	AppendRecord(ctx context.Context, params audit.RecordParams) error
}

func NewAdminHandler(userService AdminUserService, runtimeSettings *settings.RuntimeSettingsService, identityCommands ports.IdentityAdminCommands, impersonator ...ports.AdminUserImpersonator) *AdminHandler {
	handler := &AdminHandler{
		userService:      userService,
		runtimeSettings:  runtimeSettings,
		identityCommands: identityCommands,
	}
	if len(impersonator) > 0 {
		handler.impersonator = impersonator[0]
	}
	return handler
}

// SetImpersonationAuditor attaches the durable impersonation-attempt sink.
// Nil clears it. Existing constructors are untouched; callers that never
// set an auditor keep the previous behavior plus the slog trail.
func (h *AdminHandler) SetImpersonationAuditor(auditor ImpersonationAuditor) *AdminHandler {
	if h != nil {
		h.impersonationAuditor = auditor
	}
	return h
}

func RespondIdentityAdminCommandError(c *gin.Context, err error) {
	respondIdentityAdminCommandError(c, err)
}

func (h *AdminHandler) requireRuntimeSettings(c *gin.Context) (*settings.RuntimeSettingsService, bool) {
	if h == nil || h.runtimeSettings == nil {
		errorrender.Render(c, http.StatusServiceUnavailable, codeIdentityServiceUnavailable, "系统设置服务未配置")
		return nil, false
	}
	return h.runtimeSettings, true
}

func (h *AdminHandler) requireIdentityAdminCommands(c *gin.Context) (ports.IdentityAdminCommands, bool) {
	if h == nil || h.identityCommands == nil {
		slog.Error("identity admin commands not configured")
		errorrender.Render(c, http.StatusInternalServerError, codeIdentityOperationFailed, "身份管理命令未配置")
		return nil, false
	}
	return h.identityCommands, true
}

func (h *AdminHandler) respondRuntimeSettingsReadError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	response.InternalError(c, 500, "读取系统设置失败", err)
	return true
}

func (h *AdminHandler) respondRuntimeSettingsWriteError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	if validation.Is(err) {
		writeIdentityRequestInvalid(c, identityValidationMessage(err, "参数错误"))
		return true
	}
	response.InternalError(c, 500, "保存设置失败", err)
	return true
}

// getTargetUserPublicIDParam enforces the public identity route contract.
// Internal database IDs and Auth IDs must never be accepted by admin routes.
func getTargetUserPublicIDParam(c *gin.Context) (string, bool) {
	publicID, err := uuid.Parse(strings.TrimSpace(c.Param("public_id")))
	if err != nil {
		errorrender.Render(c, http.StatusBadRequest, codeIdentityUserPublicIDInvalid, "无效的用户 public_id")
		return "", false
	}
	return publicID.String(), true
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
