package paramkeys

import "strings"

const (
	SchemaVersion = "schema_version"
	Service       = "service"
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
	MaskAssetID = "mask_asset_id"
	UseMask     = "use_mask"

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
	Service,
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
	Service,
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
	Service,
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
	// OutputResolution is the client resolution source of truth
	// (ADR-0012 rule 11): v1 submits carry it in params, the edge
	// checks it for exact agreement with the top-level billing
	// resolution, and canonical compilation normalizes it into the
	// execution plan (#851).
	OutputResolution,
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

// IsPersistedMediaURLKey identifies submit-time URL keys that must never
// appear in durable Task Params; durable media uses AssetID facts instead.
func IsPersistedMediaURLKey(key string) bool {
	switch key {
	case ImageURL, ImageURLs, ReferenceImageURLs, MaskImageURL, MaskImageURLs:
		return true
	case "original" + "_image_url":
		return true
	default:
		return false
	}
}

// persistedMediaURLMarkers are storage-internal substrings that must never
// persist in task params (canonical media carries AssetID + role facts
// only). Single owner for the marker list moved here from
// workspacetasks.containsStorageURL.
var persistedMediaURLMarkers = []string{"http://", "https://", "s3://", "storage_key", "ws-uploads", "uploads/ws", "builtin-local", "bucket"}

// IsPersistedMediaURLValue reports whether value carries a persisted-media
// URL or storage-internal marker, recursing through submit-time containers.
func IsPersistedMediaURLValue(value any) bool {
	switch typed := value.(type) {
	case string:
		lowered := strings.ToLower(strings.TrimSpace(typed))
		for _, marker := range persistedMediaURLMarkers {
			if strings.Contains(lowered, marker) {
				return true
			}
		}
		return false
	case []any:
		for _, item := range typed {
			if IsPersistedMediaURLValue(item) {
				return true
			}
		}
		return false
	case []string:
		for _, item := range typed {
			if IsPersistedMediaURLValue(item) {
				return true
			}
		}
		return false
	case map[string]any:
		for _, item := range typed {
			if IsPersistedMediaURLValue(item) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// IsPersistedMediaURL is the single submit-time predicate: a param entry
// is persisted-media-URL forbidden when its key is a URL key or its value
// carries URL/storage markers. Canonical owner for the rejectPersisted*
// (tasking/domain) + containsStorageURL (workspacetasks) convergence.
func IsPersistedMediaURL(key string, value any) bool {
	if IsPersistedMediaURLKey(key) {
		return true
	}
	return IsPersistedMediaURLValue(value)
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
