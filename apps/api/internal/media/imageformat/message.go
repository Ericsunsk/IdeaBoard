package imageformat

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgInspectDetectedBytes     = "文件内容不合法"           // InspectDetectedBytes @ dimensions.go
	msgInspect                  = "不支持的图片格式: %s"      // Inspect @ dimensions.go
	msgInspect2                 = "重置图片读取位置失败: %w"    // Inspect @ dimensions.go
	msgInspect3                 = "读取图片失败: %w"        // Inspect @ dimensions.go
	msgDecodeImageConfig        = "读取图片尺寸失败: %w"      // DecodeImageConfig @ dimensions.go
	msgValidateImageDimensions  = "图片尺寸或格式无效"         // ValidateImageDimensions @ dimensions.go
	msgValidateImageDimensions2 = "图片尺寸超过限制: %dx%d"   // ValidateImageDimensions @ dimensions.go
	msgValidateImageDimensions3 = "图片总像素超过限制: %dx%d"  // ValidateImageDimensions @ dimensions.go
	msgInspectGIFAnimation      = "读取 GIF 动画信息失败: %w" // InspectGIFAnimation @ dimensions.go
	msgInspectFileHeader        = "读取文件失败: %w"        // InspectFileHeader @ upload.go
	msgInspectFileHeader2       = "图片内容无法完整解码: %w"    // InspectFileHeader @ upload.go
	msgValidateImageMagic       = "文件内容与扩展名不匹配"       // ValidateImageMagic @ upload.go
)
