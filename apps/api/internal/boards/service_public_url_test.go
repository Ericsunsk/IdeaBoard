package boards

import "testing"

func TestResolvePublicBaseURLUsesCanonicalEnv(t *testing.T) {
	t.Setenv("APP_PUBLIC_BASE_URL", "https://app.example.com/")

	got := ResolvePublicBaseURL("http", "request.example")
	if got != "https://app.example.com" {
		t.Fatalf("public base url = %q, want https://app.example.com", got)
	}
}

func TestResolvePublicBaseURLIgnoresLegacyEnvAliases(t *testing.T) {
	t.Setenv("APP_PUBLIC_BASE_URL", "")
	t.Setenv("APP_BASE_URL", "https://legacy-app.example.com")
	t.Setenv("PUBLIC_BASE_URL", "https://legacy-public.example.com")

	got := ResolvePublicBaseURL("https", "request.example")
	if got != "https://request.example" {
		t.Fatalf("public base url = %q, want request fallback", got)
	}
}
