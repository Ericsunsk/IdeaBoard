package auth

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgAuthMiddleware                = "认证服务未配置"                         // AuthMiddleware @ middleware.go
	msgAuthMiddleware2               = "未登录"                             // AuthMiddleware @ middleware.go
	msgAuthMiddleware3               = "会话已失效，请重新登录"                     // AuthMiddleware @ middleware.go
	msgAuthMiddleware4               = "认证服务暂不可用"                        // AuthMiddleware @ middleware.go
	msgAuthMiddleware5               = "用户不存在或已停用"                       // AuthMiddleware @ middleware.go
	msgAuthMiddleware6               = "用户服务暂不可用"                        // AuthMiddleware @ middleware.go
	msgAdminMiddleware               = "需要管理员权限"                         // AdminMiddleware @ middleware.go
	msgImpersonateUser               = "认证用户 ID 不能为空"                    // ImpersonateUser @ service_client.go
	msgImpersonateUser2              = "认证服务暂不可用"                        // ImpersonateUser @ service_client.go
	msgImpersonateUser3              = "读取认证服务响应失败"                      // ImpersonateUser @ service_client.go
	msgImpersonateUser4              = "认证服务拒绝伪装登录"                      // ImpersonateUser @ service_client.go
	msgImpersonateUser5              = "认证服务未返回伪装 Session"               // ImpersonateUser @ service_client.go
	msgMailConfigContractFailure     = "认证服务返回无效响应: %s"                  // MailConfigContractFailure @ service_client.go
	msgMailConfigContractFailure2    = "认证服务返回无效错误状态: code=%d status=%d" // MailConfigContractFailure @ service_client.go
	msgSocialProviderContractFailure = "认证服务返回无效响应"                      // SocialProviderContractFailure @ service_client.go
)
