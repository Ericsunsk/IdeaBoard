package settings

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgUpdateOutboundPolicy           = "管理员身份无效"                                 // UpdateOutboundPolicy @ outbound_policy.go
	msgNormalizeOutboundPolicySetting = "allow_hosts 最多支持 128 条"                  // NormalizeOutboundPolicySetting @ outbound_policy.go
	msgNormalizeOutboundAllowHost     = "allow_hosts 仅支持 hostname、IP 或 host:port" // NormalizeOutboundAllowHost @ outbound_policy.go
	msgNormalizeOutboundAllowHost2    = "allow_hosts 端口必须在 1 到 65535 之间"          // NormalizeOutboundAllowHost @ outbound_policy.go
	msgNormalizeOutboundAllowHost3    = "allow_hosts 包含无效 IP 地址"                  // NormalizeOutboundAllowHost @ outbound_policy.go
	msgNormalizeOutboundAllowHost4    = "allow_hosts 包含空主机"                       // NormalizeOutboundAllowHost @ outbound_policy.go
	msgNormalizeOutboundAllowHost5    = "allow_hosts 包含无效 hostname"               // NormalizeOutboundAllowHost @ outbound_policy.go
	msgUpdateRateLimit                = "window_seconds 必须在 1 到 3600 秒之间"         // UpdateRateLimit @ runtime_settings.go
	msgUpdateRateLimit2               = "max_requests 必须在 1 到 10000 之间"           // UpdateRateLimit @ runtime_settings.go
	msgUpdateLoginLockout             = "threshold 必须在 3 到 100 之间"                // UpdateLoginLockout @ runtime_settings.go
	msgUpdateLoginLockout2            = "duration_seconds 必须在 60 到 86400 秒之间"     // UpdateLoginLockout @ runtime_settings.go
	msgNormalizeMediaUploadSetting    = "upload_max_concurrent 必须在 1 到 64 之间"     // NormalizeMediaUploadSetting @ runtime_settings.go
	msgNormalizeBranding              = "project_name 不能为空"                       // NormalizeBranding @ runtime_settings.go
	msgNormalizeBranding2             = "project_name 不能超过 40 个字符"                // NormalizeBranding @ runtime_settings.go
	msgNormalizeBranding3             = "logo_url 不能超过 2048 个字符"                  // NormalizeBranding @ runtime_settings.go
	msgNormalizeBranding4             = "logo_url 必须为空、站内 Logo 链接或 http(s) URL"   // NormalizeBranding @ runtime_settings.go
	msgNormalizeSupportChannels       = "客服渠道最多支持 20 条"
	msgNormalizeSupportChannel        = "客服渠道配置无效"
	msgNormalizeSupportChannel2       = "客服渠道 ID 不能重复"
)
