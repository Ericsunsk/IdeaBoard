package http

import (
	"errors"
	"net/http"

	"ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/errorrender"
	"ideaboard/api/internal/platform/validation"

	"github.com/gin-gonic/gin"
)

// Identity admin/user HTTP wire codes are stable. Do not rename a code
// without coordinating frontend callers that branch on it. Messages stay
// intentionally safe for public display.
// Identity admin/user HTTP wire codes are stable. Do not rename a code
// without coordinating frontend callers that branch on it. Messages stay
// intentionally safe for public display.
//
// The values below alias the kernel registry
// (platform/contract.CodeIdentity*): the registry is the single source of
// truth, these names keep the edge diffs trivial. Wire values are
// unchanged.
const (
	codeIdentityRequestInvalid      = string(contract.CodeIdentityRequestInvalid)
	codeIdentityUnauthorized        = string(contract.CodeIdentityUnauthorized)
	codeIdentityUserPublicIDInvalid = string(contract.CodeIdentityUserPublicIDInvalid)
	codeIdentityUserNotFound        = string(contract.CodeIdentityUserNotFound)
	codeIdentityForbidden           = string(contract.CodeIdentityForbidden)
	codeIdentityConflict            = string(contract.CodeIdentityConflict)
	codeIdentityUpstreamRejected    = string(contract.CodeIdentityUpstreamRejected)
	codeIdentityUpstreamUnavailable = string(contract.CodeIdentityUpstreamUnavailable)
	codeIdentityOperationFailed     = string(contract.CodeIdentityOperationFailed)
	codeIdentityServiceUnavailable  = string(contract.CodeIdentityServiceUnavailable)
)

func writeIdentityRequestInvalid(c *gin.Context, message string) {
	if message == "" {
		message = msgRespondRuntimeSettingsWriteError
	}
	errorrender.Render(c, http.StatusBadRequest, codeIdentityRequestInvalid, message)
}

func identityValidationMessage(err error, fallback string) string {
	if validation.Is(err) && err.Error() != "" {
		return err.Error()
	}
	if fallback != "" {
		return fallback
	}
	return msgRespondRuntimeSettingsWriteError
}

// respondIdentityAdminCommandError renders typed admin command failures
// through the machine envelope. 4xx outcomes preserve the safe command
// message; 5xx outcomes log the cause and render the same safe message
// through the shared errorrender core so callers branch on stable codes.
func respondIdentityAdminCommandError(c *gin.Context, err error) {
	var commandErr *ports.IdentityAdminCommandError
	if err == nil {
		errorrender.Render(c, http.StatusInternalServerError, codeIdentityOperationFailed, msgIdentityOperationFailed3)
		return
	}
	if !errors.As(err, &commandErr) {
		errorrender.RenderDefault(c, "identity admin command failed",
			http.StatusInternalServerError, codeIdentityOperationFailed, msgIdentityOperationFailed3, err)
		return
	}
	safeMessage := commandErr.Message
	if safeMessage == "" {
		safeMessage = msgIdentityOperationFailed3
	}
	switch commandErr.Kind {
	case ports.IdentityAdminInvalid, ports.IdentityAdminTargetNotFound, ports.IdentityAdminAuthIdentityNotFound:
		errorrender.Render(c, http.StatusBadRequest, codeIdentityRequestInvalid, safeMessage)
	case ports.IdentityAdminConflict:
		errorrender.Render(c, http.StatusConflict, codeIdentityConflict, safeMessage)
	case ports.IdentityAdminForbidden:
		errorrender.Render(c, http.StatusForbidden, codeIdentityForbidden, safeMessage)
	case ports.IdentityAdminAuthFailure:
		errorrender.RenderDefault(c, "identity admin Auth failure",
			http.StatusBadGateway, codeIdentityUpstreamUnavailable, safeMessage, err)
	case ports.IdentityAdminBusinessFailure, ports.IdentityAdminReconciliationRequired, ports.IdentityAdminUnavailable:
		errorrender.RenderDefault(c, "identity admin operation failed",
			http.StatusInternalServerError, codeIdentityOperationFailed, safeMessage, err,
			"kind", string(commandErr.Kind))
	default:
		errorrender.RenderDefault(c, "identity admin operation failed",
			http.StatusInternalServerError, codeIdentityOperationFailed, msgIdentityOperationFailed3, err,
			"kind", string(commandErr.Kind))
	}
}

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgIdentityServiceUnavailable        = "系统设置服务未配置"                  // codeIdentityServiceUnavailable
	msgIdentityOperationFailed           = "身份管理命令未配置"                  // codeIdentityOperationFailed
	msgRespondRuntimeSettingsReadError   = "读取系统设置失败"                   // RespondRuntimeSettingsReadError @ admin.go
	msgRespondRuntimeSettingsWriteError  = "参数错误"                       // RespondRuntimeSettingsWriteError @ admin.go
	msgRespondRuntimeSettingsWriteError2 = "保存设置失败"                     // RespondRuntimeSettingsWriteError @ admin.go
	msgIdentityUserPublicIDInvalid       = "无效的用户 public_id"            // codeIdentityUserPublicIDInvalid
	msgImpersonateUser                   = "伪装登录失败"                     // ImpersonateUser @ admin_impersonate.go
	msgIdentityOperationFailed2          = "伪装登录服务未配置"                  // codeIdentityOperationFailed
	msgIdentityUserNotFound              = "用户不存在"                      // codeIdentityUserNotFound
	msgImpersonateUser2                  = "读取用户失败"                     // ImpersonateUser @ admin_impersonate.go
	msgImpersonateUser3                  = "不能模拟当前登录管理员自己"              // ImpersonateUser @ admin_impersonate.go
	msgImpersonateUser4                  = "不能模拟管理员账号"                  // ImpersonateUser @ admin_impersonate.go
	msgImpersonateUser5                  = "目标用户缺少 auth_user_id，无法模拟登录" // ImpersonateUser @ admin_impersonate.go
	msgIdentityUpstreamUnavailable       = "认证服务伪装登录失败"                 // codeIdentityUpstreamUnavailable
	msgIdentityUpstreamRejected          = "认证服务拒绝伪装登录"                 // codeIdentityUpstreamRejected
	msgIdentityRequestInvalid            = "目标用户无法进行伪装登录"               // codeIdentityRequestInvalid
	msgUpdateOutboundPolicy              = "参数错误，allow_hosts 为必填"       // UpdateOutboundPolicy @ admin_settings.go
	msgUpdateOutboundPolicy2             = "allow_hosts 不能为空"           // UpdateOutboundPolicy @ admin_settings.go
	msgListUsers                         = "获取用户列表失败"                   // ListUsers @ admin_users.go
	msgBanUser                           = "expires_in_days 不能小于 0"     // BanUser @ admin_users.go
	msgIdentityOperationFailed3          = "管理员操作失败"                    // codeIdentityOperationFailed
	msgUpdateLoginMetadata               = "更新登录信息失败"                   // UpdateLoginMetadata @ internal.go
)
