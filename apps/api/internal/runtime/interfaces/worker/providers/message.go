package providers

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgMarshalProviderTaskResult          = "序列化任务结果失败 (task_id=%s): %w"                                                      // MarshalProviderTaskResult @ executor.go
	msgMaterializeProviderResultImages    = "持久化上游图片失败: 存储服务未初始化"                                                             // MaterializeProviderResultImages @ executor.go
	msgMaterializeProviderResultImages2   = "持久化上游图片失败: %w"                                                                   // MaterializeProviderResultImages @ executor.go
	msgMaterializeProviderResultImages3   = "持久化上游图片失败: materializer returned no image URLs"                                  // MaterializeProviderResultImages @ executor.go
	msgMaterializeProviderResultImages4   = "持久化上游图片失败: materializer returned inconsistent result counts: source=%d items=%d" // MaterializeProviderResultImages @ executor.go
	msgCallGatewayModelImage              = "网关"                                                                              // CallGatewayModelImage @ gateway_adapters.go
	msgResolveDialectImageURLs            = "输入图像"                                                                            // ResolveDialectImageURLs @ gateway_adapters.go
	msgBuildUpscaleGatewayPayload         = "高清放大请求缺少原图"                                                                      // BuildUpscaleGatewayPayload @ gateway_adapters.go
	msgBuildUpscaleGatewayPayload2        = "高清放大"                                                                            // BuildUpscaleGatewayPayload @ gateway_adapters.go
	msgBuildUpscaleGatewayPayload3        = "高清放大 scale 必须是 1.0-4.0 且步长为 0.1"                                                 // BuildUpscaleGatewayPayload @ gateway_adapters.go
	msgBuildCutoutGatewayPayload          = "抠图"                                                                              // BuildCutoutGatewayPayload @ gateway_adapters.go
	msgCallGatewayImageBytes              = "序列化%s请求失败: %w"                                                                   // CallGatewayImageBytes @ gateway_adapters.go
	msgCallGatewayImageBytes2             = "%s响应解析失败: %w"                                                                    // CallGatewayImageBytes @ gateway_adapters.go
	msgDoGatewayImageCall                 = "%s网关请求失败: %w"                                                                    // DoGatewayImageCall @ gateway_adapters.go
	msgDoGatewayImageCall2                = "%s响应为空"                                                                          // DoGatewayImageCall @ gateway_adapters.go
	msgCallGatewayAPI                     = "任务未分配模型，请先在后台配置功能模型"                                                             // CallGatewayAPI @ gateway_execute.go
	msgCallGatewayAPI2                    = "读取模型路由配置失败: %w"                                                                  // CallGatewayAPI @ gateway_execute.go
	msgCallGatewayAPI3                    = "调用 API 失败 (模型: %s): %w"                                                          // CallGatewayAPI @ gateway_execute.go
	msgCallGatewayAPI4                    = "API 未返回图片"                                                                       // CallGatewayAPI @ gateway_execute.go
	msgCallGatewayAPI5                    = "finalizer 未返回图片"                                                                 // CallGatewayAPI @ gateway_execute.go
	msgNormalizeOpenAIEditMultipartImage  = "解码图像失败: %w"                                                                      // NormalizeOpenAIEditMultipartImage @ image.go
	msgNormalizeOpenAIEditMultipartImage2 = "编码 PNG 失败: %w"                                                                   // NormalizeOpenAIEditMultipartImage @ image.go
)
