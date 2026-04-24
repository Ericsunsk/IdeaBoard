package workflow

import "strings"

func IsStabilityTextToImagePlatform(platform string) bool {
	normalizedPlatform := strings.ToLower(strings.TrimSpace(platform))
	return normalizedPlatform == "stability" ||
		normalizedPlatform == "stability ai" ||
		strings.Contains(normalizedPlatform, "stable diffusion")
}

func IsReferenceImageFeature(featureSlug string) bool {
	switch featureSlug {
	case FeatureSlugStyleFusion,
		FeatureSlugFaceSwap,
		FeatureSlugFabricOnBody,
		FeatureSlugMultiAngle:
		return true
	default:
		return false
	}
}
