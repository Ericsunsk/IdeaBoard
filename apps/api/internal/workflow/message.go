package workflow

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgValidateWorkflowFeatureSlug       = "feature %q 不支持"                     // ValidateWorkflowFeatureSlug @ features.go
	msgLabel                             = "Custom Target Route (自定义)"          // Label @ gateway_path.go
	msgLabel2                            = "OpenRouter Chat (多模态消息)"            // Label @ gateway_path.go
	msgLabel3                            = "Anthropic Messages (文本与视觉)"         // Label @ gateway_path.go
	msgLabel4                            = "Fal.ai Media Pipeline (图像/抠图)"      // Label @ gateway_path.go
	msgLabel5                            = "OpenAI Chat Completions (多模态)"      // Label @ gateway_path.go
	msgLabel6                            = "OpenAI Images API (生成)"             // Label @ gateway_path.go
	msgValidateGenerationSubmitContract  = "generation 不接受显式 %s，生成模式由参考图推导"     // ValidateGenerationSubmitContract @ generation_submit_contract.go
	msgValidateGenerationSubmitContract2 = "generation 参考图必须只使用 image_urls"     // ValidateGenerationSubmitContract @ generation_submit_contract.go
	msgValidateGenerationSubmitContract3 = "generation 最多支持 %d 张参考图"            // ValidateGenerationSubmitContract @ generation_submit_contract.go
	msgNormalizeVariationsContract       = "variation.strength 必须是数字"           // NormalizeVariationsContract @ prompt_contract.go
	msgNormalizeOutpaintContract         = "outpaint.%s 必须是数字"                  // NormalizeOutpaintContract @ prompt_contract.go
	msgNormalizeOutpaintContract2        = "outpaint.%s 必须是整数"                  // NormalizeOutpaintContract @ prompt_contract.go
	msgNormalizeOutpaintContract3        = "outpaint.%s 必须是正整数"                 // NormalizeOutpaintContract @ prompt_contract.go
	msgNormalizeOutpaintContract4        = "outpaint.%s 不能为负数"                  // NormalizeOutpaintContract @ prompt_contract.go
	msgNormalizeUpscaleContract          = "upscale.scale 必须是数字"                // NormalizeUpscaleContract @ prompt_contract.go
	msgNormalizeUpscaleContract2         = "upscale.scale 必须是 1.0-4.0 且步长为 0.1" // NormalizeUpscaleContract @ prompt_contract.go
	msgNormalizeFusionContract           = "fusion.input_weights 至少需要一个权重"      // NormalizeFusionContract @ prompt_contract.go
	msgRequiredWorkflowObjectParam       = "%s 参数缺失或格式错误"                       // RequiredWorkflowObjectParam @ prompt_contract.go
)
