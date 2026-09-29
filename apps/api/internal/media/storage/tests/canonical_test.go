package tests

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMultipartUploadUsesDetectedFormatForKeyAndMetadata(t *testing.T) {
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })

	var payload bytes.Buffer
	if err := png.Encode(&payload, image.NewRGBA(image.Rect(0, 0, 3, 4))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	file := multipartHeader(t, "client.jpg", payload.Bytes())
	service := NewStorageService(nil, "")
	stored, err := service.UploadRefWithProfile(context.Background(), benchmarkLocalProfile(), file, StorageStoreOptions{DirPrefix: "profile/OWNR1A/avatar"})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if file.Filename != "client.png" || file.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("canonical multipart metadata = filename %q, content type %q", file.Filename, file.Header.Get("Content-Type"))
	}
	if !strings.HasSuffix(stored.StorageKey, ".png") || stored.MIMEType != "image/png" {
		t.Fatalf("stored metadata = %+v", stored)
	}
	if _, err := os.Stat(filepath.FromSlash(stored.StorageKey)); err != nil {
		t.Fatalf("stored object missing: %v", err)
	}
}

func TestStoreBytesUsesDetectedFormatForKeyAndMetadata(t *testing.T) {
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })

	payload := tinyPNGBytes(t)
	service := NewStorageService(nil, "")
	profile := service.DefaultLocalStorageProfile(true)
	ref, err := service.StoreBytesRefWithProfile(context.Background(), profile, "provider.jpg", payload, StorageStoreOptions{ObjectKey: "uploads/tasks/1/result.jpg"})
	if err != nil {
		t.Fatalf("store bytes: %v", err)
	}
	if ref.MIMEType != "image/png" || !strings.HasSuffix(ref.StorageKey, ".png") {
		t.Fatalf("stored metadata = %+v, want canonical PNG", ref)
	}
	if _, err := os.Stat(filepath.FromSlash(ref.StorageKey)); err != nil {
		t.Fatalf("stored object missing: %v", err)
	}
}

func multipartHeader(t *testing.T, filename string, data []byte) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name="file"; filename="` + filename + `"`},
		"Content-Type":        {"image/jpeg"},
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
