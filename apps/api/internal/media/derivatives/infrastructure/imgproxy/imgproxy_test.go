package imgproxy

import (
	"strings"
	"testing"

	"ideaboard/api/internal/media/derivatives/domain"
)

func TestSignPathMatchesDocumentedVector(t *testing.T) {
	cfg := Config{BaseURL: "http://imgproxy.example.com", KeyHex: "736563726574", SaltHex: "68656C6C6F"}
	got, err := cfg.SignPath("/rs:fill:300:400:0/g:sm/aHR0cDovL2V4YW1w/bGUuY29tL2ltYWdl/cy9jdXJpb3NpdHku/anBn.png")
	if err != nil {
		t.Fatal(err)
	}
	if want := "oKfUtW34Dvo2BGQehJFR4Nr0_rIjOtdtzJ3QFsUcXH8"; got != want {
		t.Fatalf("signature = %s, want %s", got, want)
	}
}

func TestTierURLGeometry(t *testing.T) {
	cfg := Config{BaseURL: "http://127.0.0.1:8080", KeyHex: strings.Repeat("ab", 16), SaltHex: strings.Repeat("cd", 16)}
	url, err := cfg.TierURL(domain.TierThumb, "local:///uploads/canvas/u/b/content/image.png")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(url, "http://127.0.0.1:8080/") || !strings.Contains(url, "/rs:fit:256:256/plain/local:///uploads/canvas/u/b/content/image.png@webp") {
		t.Fatalf("unexpected tier URL %s", url)
	}
	if _, err := cfg.TierURL("nope", "local:///x.png"); err == nil {
		t.Fatal("unknown tier should fail")
	}
}

func TestValidateRejectsWeakBindings(t *testing.T) {
	base := Config{BaseURL: "http://127.0.0.1:8080", KeyHex: strings.Repeat("ab", 16), SaltHex: strings.Repeat("cd", 16)}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	for name, mutate := range map[string]func(*Config){
		"empty base":  func(c *Config) { c.BaseURL = "" },
		"bad scheme":  func(c *Config) { c.BaseURL = "ftp://x" },
		"short key":   func(c *Config) { c.KeyHex = "ab" },
		"bad key hex": func(c *Config) { c.KeyHex = "zz" },
		"short salt":  func(c *Config) { c.SaltHex = "cd" },
	} {
		cfg := base
		mutate(&cfg)
		if err := cfg.Validate(); err == nil {
			t.Fatalf("%s should fail validation", name)
		}
	}
}

func TestLocalSource(t *testing.T) {
	got, err := LocalSource("uploads/canvas/u/b/content/image.png")
	if err != nil || got != "local:///uploads/canvas/u/b/content/image.png" {
		t.Fatalf("got %q err %v", got, err)
	}
	if _, err := LocalSource("  "); err == nil {
		t.Fatal("blank key should fail")
	}
}
