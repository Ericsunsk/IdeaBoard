package runtime

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"mime/multipart"
	"net/textproto"
	"testing"

	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/ports"
)

type stubDriverResolver struct{}

func (d stubDriverResolver) GetDriver(provider domain.StorageProvider) (ports.StorageDriver, error) {
	return nil, nil
}

func TestPublicUploadURL(t *testing.T) {
	svc := NewService(nil, stubDriverResolver{}, "https://ideaboard.art")
	url := svc.PublicUploadURL("uploads/site/assets/sample.png")
	if url != "https://ideaboard.art/uploads/site/assets/sample.png" {
		t.Fatalf("public URL = %q, want https://ideaboard.art/uploads/site/assets/sample.png", url)
	}

	svcNoOrigin := NewService(nil, stubDriverResolver{}, "")
	urlNoOrigin := svcNoOrigin.PublicUploadURL("uploads/site/assets/sample.png")
	if urlNoOrigin != "/uploads/site/assets/sample.png" {
		t.Fatalf("public URL = %q, want /uploads/site/assets/sample.png", urlNoOrigin)
	}
}

func TestChecksumMultipartFile(t *testing.T) {
	payload := []byte("deduplicated-media-payload")
	file := multipartHeaderForTest(t, "client.png", payload)
	checksum, err := checksumMultipartFile(file)
	if err != nil {
		t.Fatalf("checksum upload: %v", err)
	}
	expectedBytes := sha256.Sum256(payload)
	if checksum != hex.EncodeToString(expectedBytes[:]) {
		t.Fatalf("checksum = %q, want %q", checksum, hex.EncodeToString(expectedBytes[:]))
	}
}

func multipartHeaderForTest(t *testing.T, filename string, data []byte) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name="file"; filename="` + filename + `"`},
		"Content-Type":        {"image/png"},
	})
	if err != nil {
		t.Fatalf("create part: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	form, err := multipart.NewReader(&body, writer.Boundary()).ReadForm(int64(len(data) + 1024))
	if err != nil {
		t.Fatalf("read form: %v", err)
	}
	t.Cleanup(func() { _ = form.RemoveAll() })
	return form.File["file"][0]
}
