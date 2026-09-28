package adminbranding

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgValidate          = "logo_key 不受支持"    // Validate @ brand_logo_overrides.go
	msgValidateTarget    = "scope 不受支持"       // ValidateTarget @ brand_logo_overrides.go
	msgValidateTarget2   = "target_key 格式无效"  // ValidateTarget @ brand_logo_overrides.go
	msgList              = "读取品牌 Logo 配置失败"   // List @ handler.go
	msgUpsert            = "参数错误"             // Upsert @ handler.go
	msgCheckAdmin        = "需要管理员权限"          // CheckAdmin @ handler.go
	msgRequireStore      = "品牌 Logo 配置服务未初始化" // RequireStore @ handler.go
	msgParseTarget       = "品牌 Logo 参数错误"     // ParseTarget @ handler.go
	msgRespondStoreError = "保存品牌 Logo 配置失败"   // RespondStoreError @ handler.go
)
