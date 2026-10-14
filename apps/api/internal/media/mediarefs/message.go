package mediarefs

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgParseDataImageURL    = "仅支持 data:image"  // ParseDataImageURL @ mediarefs.go
	msgParseDataImageURL2   = "data URL 格式错误"   // ParseDataImageURL @ mediarefs.go
	msgParseDataImageURL3   = "data URL 超过大小限制" // ParseDataImageURL @ mediarefs.go
	msgParseDataImageURL4   = "不支持的图片格式"        // ParseDataImageURL @ mediarefs.go
	msgForEachParamMediaRef = "%s 必须是字符串"       // ForEachParamMediaRef @ mediarefs.go
	msgForEachStringList    = "%s 必须是字符串数组"     // ForEachStringList @ mediarefs.go
)
