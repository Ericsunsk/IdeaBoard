package paramkeys

const (
	SchemaVersion = "schema_version"
	Feature       = "feature"
	Media         = "media"
	Controls      = "controls"
	Variation     = "variation"
	Extend        = "extend"
	HD            = "hd"
	Fusion        = "fusion"
	Extra         = "extra"
	Generation    = "generation"
	ExecutionPlan = "execution_plan"

	ImageURL           = "image_url"
	ImageURLs          = "image_urls"
	ReferenceImageURLs = "reference_image_urls"
	MaskImageURL       = "mask_image_url"
	MaskImageURLs      = "mask_image_urls"
	OriginalImageURL   = "original_image_url"
	UseMask            = "use_mask"

	CustomWidth         = "custom_width"
	CustomHeight        = "custom_height"
	OriginalImageWidth  = "original_image_width"
	OriginalImageHeight = "original_image_height"
	Ratio               = "ratio"
	Quality             = "quality"
	SampleCount         = "sample_count"
	OutputCount         = "output_count"
	OutputResolution    = "output_resolution"
	ImageCount          = "image_count"
	PromptUpsampling    = "prompt_upsampling"

	Strength      = "strength"
	TopPercent    = "top_percent"
	RightPercent  = "right_percent"
	BottomPercent = "bottom_percent"
	LeftPercent   = "left_percent"
	Scale         = "scale"
	Mode          = "mode"
	InputWeights  = "input_weights"

	VariationStrength  = "variation_strength"
	ExtendTop          = "extend_top"
	ExtendRight        = "extend_right"
	ExtendBottom       = "extend_bottom"
	ExtendLeft         = "extend_left"
	FusionInputWeights = "fusion_input_weights"
	OutputFormat       = "output_format"
	OutputQuality      = "output_quality"
	UpscaleMode        = "upscale_mode"
	UpscaleFactor      = "upscale_factor"

	PromptPlaceholderVariationStrength   = "{{" + VariationStrength + "}}"
	PromptPlaceholderExtendTopPercent    = "{{extend_top_percent}}"
	PromptPlaceholderExtendRightPercent  = "{{extend_right_percent}}"
	PromptPlaceholderExtendBottomPercent = "{{extend_bottom_percent}}"
	PromptPlaceholderExtendLeftPercent   = "{{extend_left_percent}}"
	PromptPlaceholderFusionInputWeights  = "{{" + FusionInputWeights + "}}"

	PromptPathFusionInputWeights = Fusion + "." + InputWeights
)

var canonicalTaskParamKeys = keySet(
	SchemaVersion,
	Feature,
	Media,
	Controls,
	Variation,
	Extend,
	HD,
	Fusion,
	Extra,
	Generation,
	ExecutionPlan,
)

var providerReservedKeys = keySet(
	SchemaVersion,
	Feature,
	Media,
	Controls,
	Variation,
	Extend,
	HD,
	Fusion,
	Extra,
	Generation,
	ExecutionPlan,
	ImageURL,
	ImageURLs,
	ReferenceImageURLs,
	MaskImageURL,
	MaskImageURLs,
	OriginalImageURL,
	UseMask,
	CustomWidth,
	CustomHeight,
	OriginalImageWidth,
	OriginalImageHeight,
	Ratio,
	Quality,
	OutputCount,
	OutputResolution,
	PromptUpsampling,
	VariationStrength,
	ExtendTop,
	ExtendRight,
	ExtendBottom,
	ExtendLeft,
	FusionInputWeights,
	OutputQuality,
	UpscaleMode,
	UpscaleFactor,
)

var submitTaskParamKeys = keySet(
	Feature,
	ImageURLs,
	ReferenceImageURLs,
	MaskImageURL,
	MaskImageURLs,
	OriginalImageURL,
	UseMask,
	CustomWidth,
	CustomHeight,
	OriginalImageWidth,
	OriginalImageHeight,
	Ratio,
	Quality,
	OutputCount,
	PromptUpsampling,
	Variation,
	Extend,
	HD,
	Fusion,
)

func IsCanonicalTaskParamKey(key string) bool {
	_, ok := canonicalTaskParamKeys[key]
	return ok
}

func IsProviderReservedKey(key string) bool {
	_, ok := providerReservedKeys[key]
	return ok
}

func IsKnownSubmitTaskParamKey(key string) bool {
	_, ok := submitTaskParamKeys[key]
	return ok
}

func keySet(keys ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		set[key] = struct{}{}
	}
	return set
}
