package rewards

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestParseCodeKeyringAcceptsCurrentAndPreviousKeys(t *testing.T) {
	current := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 32))
	previous := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x22}, 32))
	raw := `{"current":{"version":"v2","key":"` + current + `"},"previous":{"version":"v1","key":"` + previous + `"}}`

	keyring, err := ParseCodeKeyring(raw)
	if err != nil {
		t.Fatalf("ParseCodeKeyring() error = %v", err)
	}
	if candidates := keyring.lookupDigests("IDEA2026"); len(candidates) != 2 ||
		candidates[0].version != "v2" || candidates[1].version != "v1" {
		t.Fatalf("lookup candidates = %+v, want current then previous", candidates)
	}
}

func TestParseCodeKeyringFailsClosed(t *testing.T) {
	validKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 32))
	tests := map[string]string{
		"empty":             "",
		"short key":         `{"current":{"version":"v1","key":"c2hvcnQ="}}`,
		"duplicate version": `{"current":{"version":"v1","key":"` + validKey + `"},"previous":{"version":"v1","key":"` + validKey + `"}}`,
		"unknown field":     `{"current":{"version":"v1","key":"` + validKey + `"},"extra":true}`,
		"trailing JSON":     `{"current":{"version":"v1","key":"` + validKey + `"}} {}`,
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseCodeKeyring(raw); err == nil {
				t.Fatal("ParseCodeKeyring() succeeded, want fail-closed error")
			}
		})
	}
}

func TestNormalizeCodeUsesCanonicalPublicPayload(t *testing.T) {
	normalized, err := normalizeCode("  idea-２０２６  ")
	if err != nil {
		t.Fatalf("normalizeCode() error = %v", err)
	}
	if normalized != "IDEA2026" {
		t.Fatalf("normalizeCode() = %q, want IDEA2026", normalized)
	}
}
