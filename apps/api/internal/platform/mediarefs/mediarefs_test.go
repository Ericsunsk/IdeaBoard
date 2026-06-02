package mediarefs

import (
	"strings"
	"testing"
)

func TestValidateUploadPath(t *testing.T) {
	if _, err := ValidateUploadPath("/uploads/workspace/OWNR1A/board-1/content/a.png"); err != nil {
		t.Fatalf("expected valid upload path, got %v", err)
	}
	if _, err := ValidateUploadPath("/uploads/2026/05/13/a.png"); err == nil {
		t.Fatalf("expected legacy generic upload path to fail")
	}
	if _, err := ValidateUploadPath("/tmp/a.png"); err == nil {
		t.Fatalf("expected invalid non-upload path")
	}
}

func TestParseDataImageURL(t *testing.T) {
	okData := "data:image/png;base64,aGVsbG8="
	parsed, err := ParseDataImageURL(okData, 10)
	if err != nil {
		t.Fatalf("expected parse success, got %v", err)
	}
	if parsed.Ext != ".png" || len(parsed.Data) == 0 {
		t.Fatalf("unexpected parse result: %+v", parsed)
	}

	tooLarge := "data:image/png;base64," + strings.Repeat("QQ==", 1024)
	if _, err := ParseDataImageURL(tooLarge, 8); err == nil {
		t.Fatalf("expected size limit error")
	}

	if _, err := ParseDataImageURL("data:image/svg+xml;base64,PHN2Zy8+", 1024); err == nil {
		t.Fatalf("expected svg data url to be rejected")
	}

	if _, err := ParseDataImageURL("data:image/heic;base64,aGVsbG8=", 1024); err == nil {
		t.Fatalf("expected heic data url to be rejected")
	}
}

func TestForEachParamMediaRef(t *testing.T) {
	params := map[string]interface{}{
		"image_url":            "https://a.example/1.png",
		"image_urls":           []interface{}{"https://a.example/2.png", "https://a.example/3.png"},
		"mask_image_urls":      []string{"https://a.example/m.png"},
		"reference_image_urls": []string{"https://a.example/r1.png", "https://a.example/r2.png"},
	}
	count := 0
	if err := ForEachParamMediaRef(params, func(key string, ref string) error {
		count++
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 6 {
		t.Fatalf("count=%d want=6", count)
	}

	bad := map[string]interface{}{"image_urls": []interface{}{"ok", 1}}
	if err := ForEachParamMediaRef(bad, func(key string, ref string) error { return nil }); err == nil {
		t.Fatalf("expected type error")
	}

	badReference := map[string]interface{}{"reference_image_urls": []interface{}{"ok", 1}}
	if err := ForEachParamMediaRef(badReference, func(key string, ref string) error { return nil }); err == nil {
		t.Fatalf("expected reference_image_urls type error")
	}
}
