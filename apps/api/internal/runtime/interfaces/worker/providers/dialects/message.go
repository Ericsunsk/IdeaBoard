package dialects

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgEncode              = "序列化 chat 请求失败: %w"        // Encode @ chat.go
	msgInjectCustomParams  = "注入自定义参数失败: %w"            // InjectCustomParams @ common.go
	msgCheckVendorError    = "供应商返回错误: %s"              // CheckVendorError @ common.go
	msgTopLevelKeysError   = "未能从供应商响应中提取图片，返回字段: [%s]" // TopLevelKeysError @ common.go
	msgTopLevelKeysError2  = "供应商未返回有效图片数据"             // TopLevelKeysError @ common.go
	msgDecodeWithPaths     = "供应商返回空响应"                 // DecodeWithPaths @ common.go
	msgDecodeWithPaths2    = "供应商返回非标准响应内容 (格式: %s)"    // DecodeWithPaths @ common.go
	msgEncode2             = "序列化 fal 请求失败: %w"         // Encode @ fal.go
	msgEncode3             = "序列化图像请求失败: %w"            // Encode @ generic.go
	msgAdapterForOperation = "网关不支持操作 %q"               // AdapterForOperation @ routing.go
)
