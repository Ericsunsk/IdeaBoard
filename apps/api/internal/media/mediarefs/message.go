package mediarefs

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgValidateUploadPath   = "不支持的上传资源路径"                         // ValidateUploadPath @ mediarefs.go
	msgNormalizeUploadPath  = "仅支持 /uploads/ 相对路径"                 // NormalizeUploadPath @ mediarefs.go
	msgUploadPathParts      = "文件路径不能为空"                           // UploadPathParts @ mediarefs.go
	msgParseDataImageURL    = "仅支持 data:image"                     // ParseDataImageURL @ mediarefs.go
	msgParseDataImageURL2   = "data URL 格式错误"                      // ParseDataImageURL @ mediarefs.go
	msgParseDataImageURL3   = "data URL 超过大小限制"                    // ParseDataImageURL @ mediarefs.go
	msgParseDataImageURL4   = "不支持的图片格式"                           // ParseDataImageURL @ mediarefs.go
	msgValidateMediaRef     = "地址不能为空"                             // ValidateMediaRef @ mediarefs.go
	msgValidateMediaRef2    = "不支持协议相对地址"                          // ValidateMediaRef @ mediarefs.go
	msgValidateMediaRef3    = "仅支持 http(s)、/uploads/ 或 data:image" // ValidateMediaRef @ mediarefs.go
	msgForEachParamMediaRef = "%s 必须是字符串"                          // ForEachParamMediaRef @ mediarefs.go
	msgForEachStringList    = "%s 必须是字符串数组"                        // ForEachStringList @ mediarefs.go
)
