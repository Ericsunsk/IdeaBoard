package runtime

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgUploadRefAtProfile                  = "读取媒体存储配置失败: %w"         // UploadRefAtProfile @ service.go
	msgPrepareUploadRefFromFile            = "读取上传文件 checksum 失败: %w" // PrepareUploadRefFromFile @ service.go
	msgPrepareUploadRefFromFile2           = "上传对象路径无效: %w"           // PrepareUploadRefFromFile @ service.go
	msgChecksumMultipartFile               = "文件内容不合法"                // ChecksumMultipartFile @ service.go
	msgPutObjectAtProfile                  = "读取存储配置失败: %w"           // PutObjectAtProfile @ service.go
	msgCanonicalizeMultipartImageWithFacts = "图片校验失败: %w"             // CanonicalizeMultipartImageWithFacts @ service.go
	msgDecodeDataURL                       = "无效的 base64 图片格式"        // DecodeDataURL @ service.go
)
