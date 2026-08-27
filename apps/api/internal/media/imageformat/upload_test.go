package imageformat

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"io"
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

func TestAVIFIsAllowedAndDetectedFromBytes(t *testing.T) {
	data := decodeTestAVIF(t)

	if ext, ok := NormalizeImageExtension("avif"); !ok || ext != ".avif" {
		t.Fatalf("expected avif extension to normalize, got %q, %v", ext, ok)
	}
	if mimeType, ok := NormalizeImageMIME("image/avif"); !ok || mimeType != "image/avif" {
		t.Fatalf("expected avif mime to normalize, got %q, %v", mimeType, ok)
	}
	if !UploadAllowedImageExtensions()[".avif"] {
		t.Fatal("expected avif uploads to be allowed")
	}
	if err := ValidateImageBytes(data, ".avif"); err != nil {
		t.Fatalf("expected valid avif bytes, got %v", err)
	}
	if ext := DetectImageExtensionFromBytes(data); ext != ".avif" {
		t.Fatalf("expected avif detection, got %q", ext)
	}
}

func TestInspectDetectedHandlesShortReads(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	facts, err := InspectDetected(&shortReadSeeker{Reader: *bytes.NewReader(encoded.Bytes())})
	if err != nil {
		t.Fatalf("expected short-read seeker to pass, got %v", err)
	}
	if facts.Extension != ".png" || facts.Width != 2 || facts.Height != 2 {
		t.Fatalf("unexpected facts: %+v", facts)
	}
}

type shortReadSeeker struct {
	bytes.Reader
}

func (r *shortReadSeeker) Read(p []byte) (int, error) {
	if len(p) > 2 {
		p = p[:2]
	}
	return r.Reader.Read(p)
}

var _ io.ReadSeeker = (*shortReadSeeker)(nil)

const testAVIFBase64 = "AAAAHGZ0eXBhdmlmAAAAAG1pZjFhdmlmbWlhZgAAAOBtZXRhAAAAAAAAACFoZGxyAAAAAAAAAABwaWN0AAAAAAAAAAAAAAAAAAAAAA5waXRtAAAAAAABAAAAImlsb2MAAAAAREAAAQABAAAAAAEEAAEAAAAAAAAAoQAAACNpaW5mAAAAAAABAAAAFWluZmUCAAAAAAEAAGF2MDEAAAAAYGlwcnAAAABBaXBjbwAAAAxhdjFDgURsAAAAABRpc3BlAAAAAAAAAoAAAAHgAAAAEHBpeGkAAAAAAwwMDAAAAAlpcm90AwAAABdpcG1hAAAAAAAAAAEAAQSBAgOEAAAAqW1kYXQSAAoKWSYn/vsaAhoNwjKQAUSAAOOOOEC1G3sKlYvaVm9zmuFenT+Jbbn7wihWiZQajoFIfo9fh4ogJHXITkDBR1Dq0X1/+mpbBz3ACJDYZ/cdkT+Tog4zAuH5JO6h8O5xxC/1ohrvPJmFOvChzKzeihbRYfk7eGF78rRrgaBFwNV/2xfMnjYEDVp+M3L2T+hJEoZnwaHPrB4G8pfTHWmAUA=="

func decodeTestAVIF(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(testAVIFBase64)
	if err != nil {
		t.Fatalf("decode avif fixture: %v", err)
	}
	return data
}
