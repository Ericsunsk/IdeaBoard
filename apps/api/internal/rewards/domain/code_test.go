package domain_test

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"ideaboard/api/internal/rewards/domain"
)

func TestParseCodeKeyringAcceptsCurrentAndPreviousKeys(t *testing.T) {
	current := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 32))
	previous := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x22}, 32))
	raw := `{"current":{"version":"v2","key":"` + current + `"},"previous":{"version":"v1","key":"` + previous + `"}}`

	keyring, err := domain.ParseCodeKeyring(raw)
	if err != nil {
		t.Fatalf("ParseCodeKeyring() error = %v", err)
	}
	if candidates := keyring.LookupDigests("IDEA2026"); len(candidates) != 2 ||
		candidates[0].Version != "v2" || candidates[1].Version != "v1" {
		t.Fatalf("lookup candidates = %+v, want current then previous", candidates)
	}
	if issued := keyring.IssueDigest("IDEA2026"); issued.Version != "v2" {
		t.Fatalf("issued digest version = %q, want current version v2", issued.Version)
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
			if _, err := domain.ParseCodeKeyring(raw); err == nil {
				t.Fatal("ParseCodeKeyring() succeeded, want fail-closed error")
			}
		})
	}
}

func TestParseCodeKeyringErrorsDoNotExposeSecrets(t *testing.T) {
	secret := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x7a}, 32))
	_, err := domain.ParseCodeKeyring(`{"current":{"version":"!","key":"` + secret + `"}}`)
	if err == nil {
		t.Fatal("ParseCodeKeyring() succeeded with invalid version")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("ParseCodeKeyring() error exposed the configured secret")
	}
}

func TestCodeKeyringRotationLooksUpPreviousAndIssuesWithCurrent(t *testing.T) {
	previous := domain.CodeKey{Version: "v1", Secret: bytes.Repeat([]byte{0x11}, 32)}
	oldKeyring, err := domain.NewCodeKeyring(previous, nil)
	if err != nil {
		t.Fatalf("NewCodeKeyring() old key error = %v", err)
	}
	previousIssued := oldKeyring.IssueDigest("IDEA2026")

	current := domain.CodeKey{Version: "v2", Secret: bytes.Repeat([]byte{0x22}, 32)}
	rotated, err := domain.NewCodeKeyring(current, &previous)
	if err != nil {
		t.Fatalf("NewCodeKeyring() rotated keys error = %v", err)
	}
	candidates := rotated.LookupDigests("IDEA2026")
	if len(candidates) != 2 || candidates[0].Version != "v2" || candidates[1].Version != "v1" {
		t.Fatalf("lookup candidates = %+v, want current then previous", candidates)
	}
	if !bytes.Equal(candidates[1].Digest, previousIssued.Digest) {
		t.Fatal("previous key candidate no longer matches a previously issued Code")
	}
	currentIssued := rotated.IssueDigest("IDEA2026")
	if currentIssued.Version != "v2" || !bytes.Equal(currentIssued.Digest, candidates[0].Digest) {
		t.Fatalf("new issuance = %+v, want current lookup candidate", currentIssued)
	}
	if bytes.Equal(currentIssued.Digest, previousIssued.Digest) {
		t.Fatal("rotated current key produced the previous key digest")
	}
}

func TestNormalizeCodeUsesCanonicalPublicPayload(t *testing.T) {
	normalized, err := domain.NormalizeCode("  idea-２０２６  ")
	if err != nil {
		t.Fatalf("NormalizeCode() error = %v", err)
	}
	if normalized != "IDEA2026" {
		t.Fatalf("NormalizeCode() = %q, want IDEA2026", normalized)
	}
}

func TestNormalizeCodeRemovesUnicodeWhitespaceAndHyphens(t *testing.T) {
	normalized, err := domain.NormalizeCode("\u2003ｉｄｅａ\u2010２\u00a0０\u2009２６\u3000")
	if err != nil {
		t.Fatalf("NormalizeCode() error = %v", err)
	}
	if normalized != "IDEA2026" {
		t.Fatalf("NormalizeCode() = %q, want IDEA2026", normalized)
	}
}

func TestNormalizeCodeBoundsRawBytesBeforeNormalization(t *testing.T) {
	atLimit := strings.Repeat("\u3000", 40) + "IDEA2026"
	if len(atLimit) != 128 {
		t.Fatalf("test fixture contains %d raw bytes, want 128", len(atLimit))
	}
	if normalized, err := domain.NormalizeCode(atLimit); err != nil || normalized != "IDEA2026" {
		t.Fatalf("NormalizeCode() at raw limit = %q, %v; want IDEA2026", normalized, err)
	}

	overLimit := strings.Repeat("\u3000", 41) + "IDEA2026"
	if _, err := domain.NormalizeCode(overLimit); err == nil {
		t.Fatal("NormalizeCode() accepted more than 128 raw bytes")
	}
}

func TestNormalizeCodeEnforcesPublicPayloadAlphabetAndLength(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "minimum length", raw: "ABCD-1234", want: "ABCD1234"},
		{name: "maximum length", raw: strings.Repeat("A", 32), want: strings.Repeat("A", 32)},
		{name: "too short", raw: "ABCD123"},
		{name: "too long", raw: strings.Repeat("A", 33)},
		{name: "non payload punctuation", raw: "ABCD_1234"},
		{name: "non ASCII letter", raw: "ABCD一二三四"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			normalized, err := domain.NormalizeCode(test.raw)
			if test.want == "" {
				if err == nil {
					t.Fatalf("NormalizeCode(%q) = %q, want rejection", test.raw, normalized)
				}
				return
			}
			if err != nil || normalized != test.want {
				t.Fatalf("NormalizeCode(%q) = %q, %v; want %q", test.raw, normalized, err, test.want)
			}
		})
	}
}
