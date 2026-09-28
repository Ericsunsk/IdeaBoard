package s3

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgParseS3Endpoint            = "无效的 Endpoint"                                                                                                                // ParseS3Endpoint @ config.go
	msgNormalizePublicDomain      = "未配置访问域名"                                                                                                                     // NormalizePublicDomain @ config.go
	msgNormalizePublicDomain2     = "无效的访问域名"                                                                                                                     // NormalizePublicDomain @ config.go
	msgValidateS3EndpointPolicy   = "endpoint 不可用，仅支持可达的公网 http(s) 地址"                                                                                            // ValidateS3EndpointPolicy @ config.go
	msgError                      = "S3 权限探测失败"                                                                                                                   // Error @ probe.go
	msgError2                     = "Bucket ARN 添加 %s"                                                                                                            // Error @ probe.go
	msgError3                     = "Object ARN 添加 %s"                                                                                                            // Error @ probe.go
	msgError4                     = "Cloudflare R2 凭证缺少权限：%s。请确认 R2 API Token 使用 Object Read & Write、包含当前 Bucket，且 Access Key ID 与 Secret Access Key 来自同一个 Token" // Error @ probe.go
	msgError5                     = "S3 凭证缺少权限：%s。请在%s策略中授予对应权限"                                                                                                  // Error @ probe.go
	msgError6                     = "探测 %s 失败: %s: %v"                                                                                                            // Error @ probe.go
	msgError7                     = "探测 %s 失败: %v"                                                                                                                // Error @ probe.go
	msgUserMessage                = "%s 公开访问域名读取失败：请确认 Access Domain 已绑定当前 Bucket 且允许公网读取"                                                                        // UserMessage @ probe.go
	msgUserMessage2               = "Cloudflare R2 请求配置错误：Region 必须为 auto，Endpoint 必须为 <account-id>.r2.cloudflarestorage.com"                                     // UserMessage @ probe.go
	msgUserMessage3               = "S3 请求配置错误：请检查 Region 与 Endpoint 是否匹配"                                                                                        // UserMessage @ probe.go
	msgUserMessage4               = "%s 凭证校验失败：请检查 Endpoint、Region，并确认 Access Key ID 与 Secret Access Key 匹配"                                                      // UserMessage @ probe.go
	msgUserMessage5               = "%s Bucket 不存在或不属于当前账号"                                                                                                       // UserMessage @ probe.go
	msgUserMessage6               = "%s 请求失败（%s），请检查 Endpoint、Region 和凭证配置"                                                                                       // UserMessage @ probe.go
	msgProbeCapabilitiesOnce      = "公开访问域名返回的探测对象内容不一致"                                                                                                          // ProbeCapabilitiesOnce @ probe.go
	msgEnsureS3BucketAccessible   = "S3 客户端未初始化"                                                                                                                  // EnsureS3BucketAccessible @ probe.go
	msgEnsureS3BucketAccessible2  = "验证 Bucket 失败: %w"                                                                                                            // EnsureS3BucketAccessible @ probe.go
	msgEnsureS3BucketAccessible3  = "bucket 不存在或当前凭证无权访问"                                                                                                         // EnsureS3BucketAccessible @ probe.go
	msgUploadRefWithConfig        = "打开文件失败: %w"                                                                                                                  // UploadRefWithConfig @ service.go
	msgUploadBytesWithConfigRef   = "读取图片信息失败: %w"                                                                                                                // UploadBytesWithConfigRef @ service.go
	msgPutBytesAtKeyWithConfig    = "文件路径不能为空"                                                                                                                    // PutBytesAtKeyWithConfig @ service.go
	msgPutBytesAtKeyWithConfig2   = "读取对象 generation 失败: %w"                                                                                                      // PutBytesAtKeyWithConfig @ service.go
	msgPutBytesAtKeyWithConfig3   = "上传派生图片失败: %w"                                                                                                                // PutBytesAtKeyWithConfig @ service.go
	msgUploadReaderWithConfigRef  = "S3 兼容存储配置未启用或不完整"                                                                                                            // UploadReaderWithConfigRef @ service.go
	msgUploadReaderWithConfigRef2 = "文件内容不能为空"                                                                                                                    // UploadReaderWithConfigRef @ service.go
	msgUploadReaderWithConfigRef3 = "文件大小无效"                                                                                                                      // UploadReaderWithConfigRef @ service.go
	msgPutReaderWithConfigRef     = "计算上传文件 checksum 失败: %w"                                                                                                      // PutReaderWithConfigRef @ service.go
	msgPutReaderWithConfigRef2    = "上传失败: %w"                                                                                                                    // PutReaderWithConfigRef @ service.go
	msgPutObjectWithRetry         = "重试上传前重置文件失败: %w"                                                                                                             // PutObjectWithRetry @ service.go
	msgDeleteWithConfig           = "删除失败: %w"                                                                                                                    // DeleteWithConfig @ service.go
	msgReadDeliveryObject         = "读取文件失败: %w"                                                                                                                  // ReadDeliveryObject @ service.go
	msgReadDeliveryObject2        = "读取 signed 文件失败: %w"                                                                                                          // ReadDeliveryObject @ service.go
	msgReadDeliveryObject3        = "读取文件失败: 空响应"                                                                                                                 // ReadDeliveryObject @ service.go
	msgReadDeliveryObject4        = "读取文件失败: 存储服务返回 HTTP %d"                                                                                                      // ReadDeliveryObject @ service.go
	msgDeletePrefixWithConfig     = "文件前缀不能为空"                                                                                                                    // DeletePrefixWithConfig @ service.go
	msgDeletePrefixWithConfig2    = "删除前缀对象失败: %w"                                                                                                                // DeletePrefixWithConfig @ service.go
	msgSignedURLWithConfig        = "生成 S3 signed URL 失败: %w"                                                                                                     // SignedURLWithConfig @ service.go
	msgVerifyObjectGeneration     = "%w: S3 兼容存储配置未启用或不完整"                                                                                                        // VerifyObjectGeneration @ service.go
	msgValidateBaseConfig         = "配置不能为空"                                                                                                                      // ValidateBaseConfig @ service.go
	msgValidateBaseConfig2        = "请先填写 Access Key ID / Secret Access Key / Bucket 名称"                                                                          // ValidateBaseConfig @ service.go
	msgValidateBaseConfig3        = "请先填写 Endpoint"                                                                                                               // ValidateBaseConfig @ service.go
	msgClientWithConfig           = "初始化 S3 客户端失败: %w"                                                                                                            // ClientWithConfig @ service.go
	msgS3ClientTransport          = "初始化 R2 HTTP 传输失败: %w"                                                                                                        // S3ClientTransport @ service.go
	msgReadStorageObject          = "文件超过大小限制 (%d MB)"                                                                                                            // ReadStorageObject @ service.go
	msgReadStorageObject2         = "文件内容为空"                                                                                                                      // ReadStorageObject @ service.go
)
