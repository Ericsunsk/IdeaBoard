package paramkeys

const (
	SchemaVersion = "schema_version"
	Feature       = "feature"
	Media         = "media"
	Controls      = "controls"
	Variation     = "variation"
	Outpaint      = "outpaint"
	Upscale       = "upscale"
	Fusion        = "fusion"
	Extra         = "extra"
	Generation    = "generation"
	ExecutionPlan = "execution_plan"
	WorkspaceID   = "__workspace_id"

	ImageURL               = "image_url"
	ImageURLs              = "image_urls"
	ReferenceImageURLs     = "reference_image_urls"
	MaskImageURL           = "mask_image_url"
	MaskImageURLs          = "mask_image_urls"
	ImageAssetIDs          = "image_asset_ids"
	ReferenceImageAssetIDs = "reference_image_asset_ids"
	OriginalImageAssetID   = "original_image_asset_id"
	MaskImageAssetID       = "mask_image_asset_id"
	// MaskAssetID carries a workspace public asset_... mask reference for
	// v1 inpaint submits. Unlike MaskImageAssetID (legacy numeric asset id)
	// and MaskImageURL(s) (legacy URLs, never persisted), MaskAssetID is
	// the durable contract: the mask bytes arrive via the existing
	// upload-session flow and the task links the resulting asset id.
	MaskAssetID    = "mask_asset_id"
	CanvasImageRef = "canvas_image_ref"
	UseMask        = "use_mask"

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

	Strength     = "strength"
	TargetWidth  = "target_width"
	TargetHeight = "target_height"
	ImageOffsetX = "image_offset_x"
	ImageOffsetY = "image_offset_y"
	Scale        = "scale"
	Mode         = "mode"
	InputWeights = "input_weights"

	VariationStrength  = "variation_strength"
	FusionInputWeights = "fusion_input_weights"
	OutputFormat       = "output_format"
	OutputQuality      = "output_quality"
	UpscaleMode        = "upscale_mode"
	UpscaleFactor      = "upscale_factor"

	PromptPlaceholderVariationStrength  = "{{" + VariationStrength + "}}"
	PromptPlaceholderFusionInputWeights = "{{" + FusionInputWeights + "}}"

	PromptPathFusionInputWeights = Fusion + "." + InputWeights
)

var canonicalTaskParamKeys = keySet(
	SchemaVersion,
	Feature,
	Media,
	Controls,
	Variation,
	Outpaint,
	Upscale,
	Fusion,
	Extra,
	Generation,
	ExecutionPlan,
	WorkspaceID,
)

var providerReservedKeys = keySet(
	SchemaVersion,
	Feature,
	Media,
	Controls,
	Variation,
	Outpaint,
	Upscale,
	Fusion,
	Extra,
	Generation,
	ExecutionPlan,
	WorkspaceID,
	ImageURL,
	ImageURLs,
	ReferenceImageURLs,
	MaskImageURL,
	MaskImageURLs,
	ImageAssetIDs,
	ReferenceImageAssetIDs,
	OriginalImageAssetID,
	MaskImageAssetID,
	CanvasImageRef,
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
	ImageAssetIDs,
	ReferenceImageAssetIDs,
	OriginalImageAssetID,
	MaskImageAssetID,
	MaskAssetID,
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
	Outpaint,
	Upscale,
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

// IsLegacyPersistedMediaKey identifies submit-time URL keys that must never
// appear in durable Task Params after the AssetID media contract cutover.
func IsLegacyPersistedMediaKey(key string) bool {
	switch key {
	case ImageURL, ImageURLs, ReferenceImageURLs, MaskImageURL, MaskImageURLs:
		return true
	case "original" + "_image_url":
		return true
	default:
		return false
	}
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
