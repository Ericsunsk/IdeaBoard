package profiles

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgDefaultProviderMeta          = "本地存储"                                                               // DefaultProviderMeta @ meta.go
	msgDefaultProviderMeta2         = "使用服务器本地 /uploads 目录保存文件。"                                           // DefaultProviderMeta @ meta.go
	msgDefaultProviderMeta3         = "S3 兼容存储"                                                            // DefaultProviderMeta @ meta.go
	msgDefaultProviderMeta4         = "统一接入 Amazon S3、阿里云 OSS、腾讯云 COS、Cloudflare R2、MinIO 等 S3 兼容对象存储。"    // DefaultProviderMeta @ meta.go
	msgDefaultProviderMeta5         = "请输入 Access Key ID"                                                  // DefaultProviderMeta @ meta.go
	msgDefaultProviderMeta6         = "请输入 Secret Access Key"                                              // DefaultProviderMeta @ meta.go
	msgDefaultProviderMeta7         = "Bucket 名称"                                                          // DefaultProviderMeta @ meta.go
	msgDefaultProviderMeta8         = "请输入 Bucket 名称"                                                      // DefaultProviderMeta @ meta.go
	msgDefaultProviderMeta9         = "例如：s3.us-east-1.amazonaws.com / <account>.r2.cloudflarestorage.com" // DefaultProviderMeta @ meta.go
	msgDefaultProviderMeta10        = "地域 / Region"                                                        // DefaultProviderMeta @ meta.go
	msgDefaultProviderMeta11        = "例如：us-east-1，可留空"                                                   // DefaultProviderMeta @ meta.go
	msgDefaultProviderMeta12        = "公开访问域名"                                                             // DefaultProviderMeta @ meta.go
	msgDefaultProviderMeta13        = "例如：https://bucket.s3.us-east-1.amazonaws.com"                       // DefaultProviderMeta @ meta.go
	msgDefaultProviderMeta14        = "直接拼接对象 Key；若服务需要 Bucket 路径，请在域名中包含 /bucket。"                        // DefaultProviderMeta @ meta.go
	msgDefaultProviderMeta15        = "部分兼容存储需要使用 path-style 访问。"                                          // DefaultProviderMeta @ meta.go
	msgSaveProfile                  = "存储配置不能为空"                                                           // SaveProfile @ service.go
	msgResolveProfileSaveTarget     = "不支持的存储类型"                                                           // ResolveProfileSaveTarget @ service.go
	msgResolveProfileSaveTarget2    = "不允许修改存储类型"                                                          // ResolveProfileSaveTarget @ service.go
	msgResolveProfileSaveTarget3    = "本地存储不支持新增额外卡片"                                                      // ResolveProfileSaveTarget @ service.go
	msgPrepareProfileSave           = "当前存储不允许停用"                                                          // PrepareProfileSave @ service.go
	msgDeleteProfile                = "内置存储不允许删除"                                                          // DeleteProfile @ service.go
	msgDeleteProfile2               = "当前存储不允许删除"                                                          // DeleteProfile @ service.go
	msgEnsureProfileReadyForRuntime = "存储未启用或未配置完整"                                                        // EnsureProfileReadyForRuntime @ service.go
	msgResolveActiveProfile         = "未找到可用存储配置"                                                          // ResolveActiveProfile @ service.go
	msgNormalizeProfile             = "存储"                                                                 // NormalizeProfile @ service.go
	msgValidateProfile              = "存储名称不能为空"                                                           // ValidateProfile @ service.go
)
