package media

import (
	"bytes"
	"strings"
	"testing"
)

func TestValidateImageMagicAcceptsWebPWithWebPChunk(t *testing.T) {
	buf := []byte{
		0x52, 0x49, 0x46, 0x46,
		0x24, 0x00, 0x00, 0x00,
		0x57, 0x45, 0x42, 0x50,
		0x56, 0x50, 0x38, 0x20,
	}

	if err := validateImageMagic(buf, ".webp"); err != nil {
		t.Fatalf("expected valid webp header, got %v", err)
	}
}

func TestValidateImageMagicAcceptsBmpHeader(t *testing.T) {
	buf := []byte{0x42, 0x4D, 0x46, 0x00}

	if err := validateImageMagic(buf, ".bmp"); err != nil {
		t.Fatalf("expected valid bmp header, got %v", err)
	}
}

func TestValidateImageMagicRejectsRiffWithoutWebPChunk(t *testing.T) {
	buf := []byte{
		0x52, 0x49, 0x46, 0x46,
		0x24, 0x00, 0x00, 0x00,
		0x41, 0x56, 0x49, 0x20,
		0x4c, 0x49, 0x53, 0x54,
	}

	if err := validateImageMagic(buf, ".webp"); err == nil {
		t.Fatal("expected non-webp RIFF payload to be rejected")
	}
}

func TestValidateImageBytesRejectsHTMLPayload(t *testing.T) {
	buf := []byte("<html><body>not an image</body></html>")

	if err := ValidateImageBytes(buf, ".png"); err == nil {
		t.Fatal("expected html payload to be rejected for png extension")
	}
}

func TestValidateImageReaderUsesOnlySignatureBytes(t *testing.T) {
	buf := bytes.NewReader([]byte{
		0x52, 0x49, 0x46, 0x46,
		0x24, 0x00, 0x00, 0x00,
		0x57, 0x45, 0x42, 0x50,
		0x56, 0x50, 0x38, 0x20,
	})

	if err := ValidateImageReader(buf, ".webp"); err != nil {
		t.Fatalf("expected valid webp reader to pass, got %v", err)
	}
}

func TestValidateImageReaderRejectsShortPayload(t *testing.T) {
	err := ValidateImageReader(strings.NewReader("\x89PNG"), ".webp")
	if err == nil {
		t.Fatal("expected short reader to be rejected")
	}
}

func TestNormalizeImageExtensionRejectsSVG(t *testing.T) {
	if _, ok := NormalizeImageExtension(".svg"); ok {
		t.Fatal("expected svg to be rejected")
	}
}
