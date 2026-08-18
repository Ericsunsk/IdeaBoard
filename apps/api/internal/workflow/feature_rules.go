package workflow

import "strings"

const (
	ProviderOpenAICompatible = "openai-compatible"
	ProviderSelfHosted       = "self-hosted"
	ProviderGoogleAI         = "google-ai"
	ProviderGoogleCloud      = "google-cloud"
)

func IsSupportedProviderAPIType(apiType string) bool {
	switch strings.ToLower(strings.TrimSpace(apiType)) {
	case ProviderOpenAICompatible, ProviderSelfHosted, ProviderGoogleAI, ProviderGoogleCloud:
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
	case strings.Contains(normalized, "aiplatform.googleapis.com"):
		return ProviderGoogleCloud
	default:
		return ProviderOpenAICompatible
	}
}

func IsGoogleNativeAPIType(apiType string) bool {
	switch strings.ToLower(strings.TrimSpace(apiType)) {
	case ProviderGoogleAI, ProviderGoogleCloud:
		return true
	default:
		return false
	}
}

func IsGoogleCloudAPIType(apiType string) bool {
	return strings.ToLower(strings.TrimSpace(apiType)) == ProviderGoogleCloud
}
