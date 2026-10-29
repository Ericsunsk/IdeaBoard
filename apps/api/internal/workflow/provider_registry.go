package workflow

import (
	"strings"
)

// ProviderEntry is the display and routing metadata for one gateway provider.
type ProviderEntry struct {
	Slug             string
	DisplayName      string
	GatewayNamespace string
}

// FormatProviderDisplayName formats a provider slug into a human-readable display name
// using algorithmic transformation:
// 1. Trim and lowercase
// 2. Split by '-' or '_'
// 3. Capitalize each word (Title Case)
// 4. Transform known acronyms (e.g. AI, API, LLM, CF, UI) to uppercase
func FormatProviderDisplayName(slug string) string {
	clean := strings.ToLower(strings.TrimSpace(slug))
	if clean == "" {
		return ""
	}
	parts := strings.FieldsFunc(clean, func(r rune) bool {
		return r == '-' || r == '_'
	})
	words := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "ai", "api", "llm", "cf", "ui", "sdk", "tts", "asr", "ocr", "nlp", "url", "id", "db":
			words = append(words, strings.ToUpper(part))
		default:
			words = append(words, strings.ToUpper(part[:1])+part[1:])
		}
	}
	return strings.Join(words, " ")
}

// ResolveProvider returns display metadata and the normalized gateway namespace
// for a non-empty provider slug.
func ResolveProvider(slug string) (entry ProviderEntry, known bool) {
	clean := strings.ToLower(strings.TrimSpace(slug))
	if clean == "" {
		return ProviderEntry{}, false
	}
	return ProviderEntry{
		Slug:             clean,
		DisplayName:      FormatProviderDisplayName(clean),
		GatewayNamespace: clean,
	}, true
}

// IDPrefix renders the provider prefix used in a full model reference.
func (e ProviderEntry) IDPrefix() string {
	return e.Slug + "/"
}

// merged from internal/workflow/feature_rules.go (structural consolidation; symbols unchanged).
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
