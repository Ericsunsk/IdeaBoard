package imgproxy

import (
	"strings"
	"testing"
)

func dicebearTestConfig() Config {
	return Config{BaseURL: "http://127.0.0.1:8080", KeyHex: strings.Repeat("ab", 16), SaltHex: strings.Repeat("cd", 16)}
}

func TestProxyDicebearAvatarURLRewritesSeededURL(t *testing.T) {
	cfg := dicebearTestConfig()
	raw := "https://api.dicebear.com/9.x/big-ears-neutral/svg?seed=auth-2"
	first, ok, err := cfg.ProxyDicebearAvatarURL(raw, DefaultAvatarSize)
	if err != nil || !ok {
		t.Fatalf("proxy = %q %v %v, want rewrite", first, ok, err)
	}
	// The seed survives the rewrite so the avatar stays derivable.
	if !strings.Contains(first, "auth-2") {
		t.Fatalf("proxied URL %q dropped the seed", first)
	}
	// No expiry segment: the seed is permanent, so the URL is stable.
	second, ok, err := cfg.ProxyDicebearAvatarURL(raw, DefaultAvatarSize)
	if err != nil || !ok || second != first {
		t.Fatalf("proxy is not deterministic: %q vs %q (%v %v)", first, second, ok, err)
	}
	if strings.Contains(first, "/exp:") {
		t.Fatalf("proxied URL %q must not carry an expiry", first)
	}
	if !strings.Contains(first, "/rs:fill:64:64/plain/") || !strings.HasSuffix(first, "@webp") {
		t.Fatalf("proxied URL %q must mirror the avatar fill geometry", first)
	}
	// The signature verifies against the exact served path.
	sig, signedPath, cut := strings.Cut(strings.TrimPrefix(first, "http://127.0.0.1:8080/"), "/")
	if !cut {
		t.Fatalf("proxied URL %q has no signature segment", first)
	}
	signedPath = "/" + signedPath
	wantSig, err := cfg.SignPath(signedPath)
	if err != nil || sig != wantSig {
		t.Fatalf("proxied URL has an invalid signature")
	}
}

func TestProxyDicebearAvatarURLPassthrough(t *testing.T) {
	cfg := dicebearTestConfig()
	for name, raw := range map[string]string{
		"empty":          "",
		"blank":          "   ",
		"oauth picture":  "https://cdn.discordapp.com/avatar.png",
		"lookalike host": "https://api.dicebear.com.evil.com/9.x/big-ears-neutral/svg?seed=auth-2",
		"plain http":     "http://api.dicebear.com/9.x/big-ears-neutral/svg?seed=auth-2",
		"relative route": "/api/v1/public/users/AVTR01/avatar?v=abc123",
		"storage key":    "uploads/avatars/AVTR01/abcdef1234567890.png",
		"not a url":      "::::",
		"userinfo":       "https://user@api.dicebear.com/9.x/big-ears-neutral/svg?seed=auth-2",
	} {
		if _, ok, err := cfg.ProxyDicebearAvatarURL(raw, DefaultAvatarSize); err != nil || ok {
			t.Fatalf("%s (%q): proxy = %v %v, want passthrough", name, raw, ok, err)
		}
	}
}

func TestProxyDicebearAvatarURLHostMatchIsCaseInsensitive(t *testing.T) {
	cfg := dicebearTestConfig()
	if _, ok, err := cfg.ProxyDicebearAvatarURL("https://API.DICEBEAR.COM/9.x/big-ears-neutral/svg?seed=auth-2", DefaultAvatarSize); err != nil || !ok {
		t.Fatalf("uppercase host: proxy = %v %v, want rewrite", ok, err)
	}
}

func TestProxyDicebearAvatarURLRejectsWeakConfig(t *testing.T) {
	bad := Config{BaseURL: "http://127.0.0.1:8080", KeyHex: "ab", SaltHex: strings.Repeat("cd", 16)}
	if _, ok, err := bad.ProxyDicebearAvatarURL("https://api.dicebear.com/9.x/big-ears-neutral/svg?seed=auth-2", DefaultAvatarSize); err == nil || ok {
		t.Fatalf("weak config: proxy = %v %v, want an error", ok, err)
	}
}

func TestProxyDicebearAvatarURLRejectsUnknownSize(t *testing.T) {
	if _, ok, err := dicebearTestConfig().ProxyDicebearAvatarURL("https://api.dicebear.com/9.x/big-ears-neutral/svg?seed=x", 100); err == nil || ok {
		t.Fatalf("unknown avatar size: proxy = %v %v, want an error", ok, err)
	}
}
