package workflow

import "strings"

const (
	ProviderOpenAICompatible = "openai-compatible"
	ProviderGoogleAI         = "google-ai"
)

func IsSupportedProviderAPIType(apiType string) bool {
	switch strings.ToLower(strings.TrimSpace(apiType)) {
	case ProviderOpenAICompatible, ProviderGoogleAI:
		return true
	default:
		return false
	}
}

func ValidateProviderAPIType(apiType string, baseURL ...string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(apiType))
	if normalized == "" && len(baseURL) > 0 && strings.TrimSpace(baseURL[0]) != "" {
		return InferProviderAPITypeFromBaseURL(baseURL[0]), true
	}
	if IsSupportedProviderAPIType(normalized) {
		return normalized, true
	}
	return "", false
}

func InferProviderAPITypeFromBaseURL(rawBaseURL string) string {
	normalized := strings.ToLower(strings.TrimSpace(rawBaseURL))
	switch {
	case strings.Contains(normalized, "generativelanguage.googleapis.com"):
		return ProviderGoogleAI
	default:
		return ProviderOpenAICompatible
	}
}

func IsGoogleNativeAPIType(apiType string) bool {
	return strings.ToLower(strings.TrimSpace(apiType)) == ProviderGoogleAI
}

func IsReferenceImageFeature(featureSlug string) bool {
	switch featureSlug {
	case FeatureSlugStyleFusion,
		FeatureSlugFaceSwap,
		FeatureSlugMultiAngle:
		return true
	default:
		return false
	}
}
