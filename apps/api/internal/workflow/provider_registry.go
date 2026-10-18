package workflow

import (
	"strings"
)

// ProviderEntry is the single enrichment record for one gateway provider.
// Membership is driven by the control plane; this table never gates
// discovery, it only enriches: display name, community catalog slice, and
// gateway namespace. GatewayNamespace is verified against CF provider docs;
// "" renders "custom-<slug>" for slugs with no native namespace.
type ProviderEntry struct {
	Slug               string
	DisplayName        string
	GatewayNamespace   string
	Aliases            []string
	CommunityPrefixes  []string
	CommunityProviders []string
}

// providerRegistry is the sole provider vocabulary: detection (community
// slice + display name), routing (gateway namespace) and payload shaping
// all resolve through it. Unknown slugs synthesize a default (prettified
// name, community guess, custom- namespace) instead of vanishing.
var providerRegistry = []ProviderEntry{
	{Slug: "openrouter", DisplayName: "OpenRouter", GatewayNamespace: "openrouter", CommunityPrefixes: []string{"openrouter"}, CommunityProviders: []string{"openrouter"}},
	{Slug: "openai", DisplayName: "OpenAI", GatewayNamespace: "openai", CommunityPrefixes: []string{"openai"}, CommunityProviders: []string{"openai"}},
	{Slug: "deepseek", DisplayName: "DeepSeek", GatewayNamespace: "deepseek", CommunityPrefixes: []string{"deepseek"}, CommunityProviders: []string{"deepseek"}},
	{Slug: "groq", DisplayName: "Groq", GatewayNamespace: "groq", CommunityPrefixes: []string{"groq"}, CommunityProviders: []string{"groq"}},
	{Slug: "mistral", DisplayName: "Mistral AI", GatewayNamespace: "mistral", CommunityPrefixes: []string{"mistral"}, CommunityProviders: []string{"mistral"}},
	{Slug: "grok", DisplayName: "xAI", GatewayNamespace: "grok", Aliases: []string{"xai"}, CommunityPrefixes: []string{"xai"}, CommunityProviders: []string{"xai"}},
	{Slug: "perplexity", DisplayName: "Perplexity", GatewayNamespace: "perplexity-ai", CommunityPrefixes: []string{"perplexity"}, CommunityProviders: []string{"perplexity"}},
	{Slug: "cerebras", DisplayName: "Cerebras", GatewayNamespace: "cerebras", CommunityPrefixes: []string{"cerebras"}, CommunityProviders: []string{"cerebras"}},
	{Slug: "anthropic", DisplayName: "Anthropic", GatewayNamespace: "anthropic", CommunityProviders: []string{"anthropic"}},
	{Slug: "cohere", DisplayName: "Cohere", GatewayNamespace: "cohere", CommunityPrefixes: []string{"cohere"}, CommunityProviders: []string{"cohere", "cohere_chat"}},
	{Slug: "google-ai-studio", DisplayName: "Google AI Studio", GatewayNamespace: "google-ai-studio", Aliases: []string{"googleaistudio", "gemini"}, CommunityPrefixes: []string{"gemini"}, CommunityProviders: []string{"gemini"}},
	{Slug: "workers-ai", DisplayName: "Workers AI", GatewayNamespace: "workers-ai", Aliases: []string{"workersai", "cf-workers-ai", "cloudflare"}, CommunityPrefixes: []string{"cloudflare"}, CommunityProviders: []string{"cloudflare"}},
	{Slug: "fal", DisplayName: "Fal AI", GatewayNamespace: "fal", Aliases: []string{"fal-ai", "fal_ai", "fal.ai"}, CommunityPrefixes: []string{"fal_ai"}, CommunityProviders: []string{"fal_ai"}},
	{Slug: "replicate", DisplayName: "Replicate", GatewayNamespace: "replicate", CommunityPrefixes: []string{"replicate"}, CommunityProviders: []string{"replicate"}},
	{Slug: "huggingface", DisplayName: "Hugging Face", GatewayNamespace: "huggingface", Aliases: []string{"hugging-face"}},
	{Slug: "elevenlabs", DisplayName: "ElevenLabs", GatewayNamespace: "elevenlabs", Aliases: []string{"eleven-labs"}},
	{Slug: "deepgram", DisplayName: "Deepgram", GatewayNamespace: "deepgram"},
	{Slug: "cartesia", DisplayName: "Cartesia", GatewayNamespace: "cartesia"},
	{Slug: "bedrock", DisplayName: "Amazon Bedrock", GatewayNamespace: "aws-bedrock", CommunityPrefixes: []string{"bedrock"}, CommunityProviders: []string{"bedrock", "bedrock_converse", "bedrock_mantle"}},
	{Slug: "azureopenai", DisplayName: "Azure OpenAI", GatewayNamespace: "azure-openai", Aliases: []string{"azure-openai"}, CommunityPrefixes: []string{"azure"}, CommunityProviders: []string{"azure"}},
	{Slug: "baseten", DisplayName: "Baseten", GatewayNamespace: "baseten", CommunityPrefixes: []string{"baseten"}, CommunityProviders: []string{"baseten"}},
	{Slug: "ideogram", DisplayName: "Ideogram", GatewayNamespace: "ideogram"},
	{Slug: "parallel", DisplayName: "Parallel", GatewayNamespace: "parallel", CommunityPrefixes: []string{"parallel_ai"}, CommunityProviders: []string{"parallel_ai"}},
	{Slug: "google-vertex-ai", DisplayName: "Google Vertex AI", GatewayNamespace: "google-vertex-ai", Aliases: []string{"vertex", "google-vertex", "vertex_ai"}, CommunityPrefixes: []string{"vertex_ai"}, CommunityProviders: []string{"vertex_ai", "vertex_ai-language-models", "vertex_ai-anthropic_models"}},
}

// ResolveProvider returns the registry entry for one slug (canonical or
// alias, case-insensitive). Unknown non-empty slugs synthesize a default:
// prettified display name, community guess (prefix and provider = slug),
// and empty GatewayNamespace meaning "custom-<slug>". known=false only
// for empty input (zero entry) or synthesized defaults.
func ResolveProvider(slug string) (entry ProviderEntry, known bool) {
	clean := strings.ToLower(strings.TrimSpace(slug))
	if clean == "" {
		return ProviderEntry{}, false
	}
	for _, entry := range providerRegistry {
		if entry.Slug == clean {
			return entry, true
		}
		for _, alias := range entry.Aliases {
			if alias == clean {
				return entry, true
			}
		}
	}
	return ProviderEntry{
		Slug:               clean,
		DisplayName:        prettifyProviderSlug(clean),
		CommunityPrefixes:  []string{clean},
		CommunityProviders: []string{clean},
	}, false
}

// IDPrefix renders the ID input hint for one entry: the primary community
// prefix (native key shape) or slug + "/" when the community indexes
// nothing under it.
func (e ProviderEntry) IDPrefix() string {
	if len(e.CommunityPrefixes) > 0 {
		return e.CommunityPrefixes[0] + "/"
	}
	return e.Slug + "/"
}

// ListProviders returns a copy of the registry in canonical order.
func ListProviders() []ProviderEntry {
	out := make([]ProviderEntry, len(providerRegistry))
	copy(out, providerRegistry)
	return out
}
func prettifyProviderSlug(slug string) string {
	parts := strings.Split(slug, "-")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
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
