package httpapi

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgSearchProviderUnavailable    = "搜索供应商配置不可用"          // CodeSearchProviderUnavailable
	msgList                         = "读取搜索供应商配置失败"         // List @ handler.go
	msgSearchProviderNotFound       = "搜索供应商不存在"            // CodeSearchProviderNotFound
	msgSearchRequestInvalid         = "参数错误"                // CodeSearchRequestInvalid
	msgSearchProviderTestRequired   = "启用前请先通过连接测试"         // CodeSearchProviderTestRequired
	msgSearchProviderAPIKeyInvalid  = "API Key 不能为空"        // CodeSearchProviderAPIKeyInvalid
	msgUpdate                       = "保存搜索供应商配置失败"         // Update @ handler.go
	msgSearchProviderUnavailable2   = "搜索供应商测试不可用"          // CodeSearchProviderUnavailable
	msgSearchProviderAPIKeyRequired = "请先保存 API Key"        // CodeSearchProviderAPIKeyRequired
	msgTest                         = "保存搜索供应商测试状态失败"       // Test @ handler.go
	msgSearchProviderTestFailed     = "搜索供应商连接测试失败"         // CodeSearchProviderTestFailed
	msgSearchUnavailable            = "搜索服务暂不可用"            // CodeSearchUnavailable
	msgSearchLimitInvalid           = "limit 必须在 1 到 48 之间" // CodeSearchLimitInvalid
	msgSearch                       = "搜索关键词"               // Search @ search_handler.go
	msgSearchQueryInvalid           = "搜索关键词不能超过 %d 个字符"    // CodeSearchQueryInvalid
	msgSearch2                      = "分页游标"                // Search @ search_handler.go
	msgSearchCursorInvalid          = "分页游标无效"              // CodeSearchCursorInvalid
	msgSearchUnavailable2           = "搜索服务暂不可用"            // CodeSearchUnavailable
	msgVisionSearchProviderError    = "视觉搜索暂不可用"            // CodeVisionSearchProviderError
	msgVisionSearchInputInvalid     = "请添加 1–4 张参考图"        // CodeVisionSearchInputInvalid
	msgVisionSearchImageInvalid     = "参考图格式、尺寸或总大小无效"      // CodeVisionSearchImageInvalid
	msgVisionSearchImageUnavailable = "参考图暂时无法读取"           // CodeVisionSearchImageUnavailable
	msgVisionSearchNotConfigured    = "视觉搜索暂未配置"            // CodeVisionSearchNotConfigured
	msgVisionSearchUnavailable      = "视觉搜索暂不可用"            // CodeVisionSearchUnavailable
	msgVisionSearchSchemaError      = "视觉搜索返回格式无效"          // CodeVisionSearchSchemaError
	msgVisionSearchCredentials      = "视觉搜索凭证无效"            // CodeVisionSearchCredentials
	msgVisionSearchQuota            = "视觉搜索权限或配额不可用"        // CodeVisionSearchQuota
	msgVisionSearchTimeout          = "视觉搜索超时，请重试"          // CodeVisionSearchTimeout
	msgVisionSearchNetwork          = "视觉搜索连接失败，请重试"        // CodeVisionSearchNetwork
)
