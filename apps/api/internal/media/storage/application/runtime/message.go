package runtime

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgUploadRefAtProfile                   = "读取媒体存储配置失败: %w"         // UploadRefAtProfile @ service.go
	msgPrepareUploadRefFromFile             = "读取上传文件 checksum 失败: %w" // PrepareUploadRefFromFile @ service.go
	msgPrepareUploadRefFromFile2            = "上传对象路径无效: %w"           // PrepareUploadRefFromFile @ service.go
	msgChecksumMultipartFile                = "文件内容不合法"                // ChecksumMultipartFile @ service.go
	msgPutObjectAtProfile                   = "读取存储配置失败: %w"           // PutObjectAtProfile @ service.go
	msgResolveProviderDerivativeURL         = "媒体衍生图解析器未配置"            // ResolveProviderDerivativeURL @ service.go
	msgResolveProviderDerivativeURL2        = "定位媒体衍生图失败: %w"          // ResolveProviderDerivativeURL @ service.go
	msgResolveProviderDerivativeURL3        = "媒体衍生图尚未就绪"              // ResolveProviderDerivativeURL @ service.go
	msgResolveProviderDerivativeURL4        = "媒体衍生图存储路径无效: %w"        // ResolveProviderDerivativeURL @ service.go
	msgResolveProviderDerivativeURL5        = "读取媒体衍生图存储配置失败: %w"      // ResolveProviderDerivativeURL @ service.go
	msgResolveStoredUploadTarget            = "定位媒体资产失败: %w"           // ResolveStoredUploadTarget @ service.go
	msgResolveStoredUploadTarget2           = "媒体资产存储路径无效: %w"         // ResolveStoredUploadTarget @ service.go
	msgResolveStoredUploadTarget3           = "媒体资产存储路径与请求不一致"         // ResolveStoredUploadTarget @ service.go
	msgResolveStoredUploadTarget4           = "读取媒体资产存储配置失败: %w"       // ResolveStoredUploadTarget @ service.go
	msgResolveProfileFromUploadURL          = "图片地址格式错误"               // ResolveProfileFromUploadURL @ service.go
	msgFindS3ProfileForUploadURL            = "图片地址匹配多个存储配置"           // FindS3ProfileForUploadURL @ service.go
	msgFindS3ProfileForUploadURL2           = "未找到图片所属的存储配置"           // FindS3ProfileForUploadURL @ service.go
	msgExtractObjectKeyFromUserUploadURL    = "图片地址不能为空"               // ExtractObjectKeyFromUserUploadURL @ service.go
	msgExtractObjectKeyFromUserUploadURL2   = "不支持的存储类型"               // ExtractObjectKeyFromUserUploadURL @ service.go
	msgExtractObjectKeyFromAccessDomainURL  = "访问域名配置无效"               // ExtractObjectKeyFromAccessDomainURL @ service.go
	msgExtractObjectKeyFromAccessDomainURL2 = "仅允许删除当前存储域名下的文件"        // ExtractObjectKeyFromAccessDomainURL @ service.go
	msgUploadObjectKeyFromURL               = "不支持的上传资源路径"             // UploadObjectKeyFromURL @ service.go
	msgCanonicalizeMultipartImageWithFacts  = "图片校验失败: %w"             // CanonicalizeMultipartImageWithFacts @ service.go
	msgDecodeDataURL                        = "无效的 base64 图片格式"        // DecodeDataURL @ service.go
)
