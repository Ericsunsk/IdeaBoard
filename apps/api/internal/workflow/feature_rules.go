package workflow

import "strings"

const (
	ProviderOpenAICompatible = "openai-compatible"
	ProviderGoogleAI         = "google-ai"
	ProviderStability        = "stability"
)

func NormalizeProviderAPIType(apiType string, baseURL ...string) string {
	normalized := strings.ToLower(strings.TrimSpace(apiType))
	switch normalized {
	case ProviderOpenAICompatible:
		return ProviderOpenAICompatible
	case ProviderGoogleAI:
		return ProviderGoogleAI
	case ProviderStability:
		return ProviderStability
	}

	if len(baseURL) > 0 {
		return InferProviderAPITypeFromBaseURL(baseURL[0])
	}
	return ProviderOpenAICompatible
}

func InferProviderAPITypeFromBaseURL(rawBaseURL string) string {
	normalized := strings.ToLower(strings.TrimSpace(rawBaseURL))
	switch {
	case strings.Contains(normalized, "generativelanguage.googleapis.com"):
		return ProviderGoogleAI
	case strings.Contains(normalized, "api.stability.ai"):
		return ProviderStability
	default:
		return ProviderOpenAICompatible
	}
}

func IsGoogleNativeAPIType(apiType string) bool {
	return NormalizeProviderAPIType(apiType) == ProviderGoogleAI
}

func IsStabilityAPIType(apiType string) bool {
	return NormalizeProviderAPIType(apiType) == ProviderStability
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
