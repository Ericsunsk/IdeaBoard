package storage

import (
	"bytes"
	"context"
	"encoding/base64"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ideaboard/api/internal/platform/config"
)

func BenchmarkStorageStoreBytesLocal(b *testing.B) {
	service := newBenchmarkStorageService(b)
	profile := benchmarkLocalProfile()
	payload := benchmarkPayload(1 << 20)
	opts := StorageStoreOptions{DirPrefix: "bench/store-bytes"}
	adapter, err := service.adapterRegistry.get(StorageProviderLocal)
	if err != nil {
		b.Fatalf("local storage adapter missing: %v", err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		url, err := adapter.StoreBytes(context.Background(), &profile, "bench.png", payload, opts)
		if err != nil {
			b.Fatalf("StoreBytes returned error: %v", err)
		}
		removeBenchmarkUpload(b, url)
	}
}

func BenchmarkStorageUploadLocalMultipart(b *testing.B) {
	service := newBenchmarkStorageService(b)
	profile := benchmarkLocalProfile()
	payload := benchmarkPayload(1 << 20)
	file := benchmarkMultipartFileHeader(b, "bench.png", payload)
	opts := StorageStoreOptions{DirPrefix: "bench/upload"}

	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ref, err := service.uploadRefWithProfile(context.Background(), profile, file, opts)
		if err != nil {
			b.Fatalf("UploadWithOptions returned error: %v", err)
		}
		removeBenchmarkUpload(b, ref.URL)
	}
}

func BenchmarkStorageStoreRemoteLocal(b *testing.B) {
	service := newBenchmarkStorageService(b)
	profile := benchmarkLocalProfile()
	payload := benchmarkPNGPayload(1 << 20)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(payload)
	}))
	b.Cleanup(server.Close)
	opts := StorageStoreOptions{
		DirPrefix:  "bench/store-remote",
		AllowHosts: []string{server.URL},
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		url, err := service.storeRemoteWithProfile(context.Background(), profile, server.URL+"/image.png", opts)
		if err != nil {
			b.Fatalf("StoreRemote returned error: %v", err)
		}
		removeBenchmarkUpload(b, url)
	}
}

func newBenchmarkStorageService(b *testing.B) *StorageService {
	b.Helper()

	originalWD, err := os.Getwd()
	if err != nil {
		b.Fatalf("get cwd failed: %v", err)
	}
	tmpDir := b.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		b.Fatalf("chdir temp dir failed: %v", err)
	}
	b.Cleanup(func() {
		_ = os.Chdir(originalWD)
	})

	return NewStorageService(nil, config.PublicOriginsConfig{})
}

func benchmarkLocalProfile() StorageProfile {
	return StorageProfile{
		ID:        builtinLocalProfileID(),
		Name:      "Local",
		Provider:  StorageProviderLocal,
		Enabled:   true,
		IsCurrent: true,
		Builtin:   true,
		Config:    map[string]interface{}{},
	}
}

func benchmarkMultipartFileHeader(b *testing.B, filename string, data []byte) *multipart.FileHeader {
	b.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		b.Fatalf("create multipart part failed: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		b.Fatalf("write multipart part failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		b.Fatalf("close multipart writer failed: %v", err)
	}

	reader := multipart.NewReader(&body, writer.Boundary())
	form, err := reader.ReadForm(int64(len(data) + 1024))
	if err != nil {
		b.Fatalf("read multipart form failed: %v", err)
	}
	b.Cleanup(func() {
		_ = form.RemoveAll()
	})

	files := form.File["file"]
	if len(files) == 0 {
		b.Fatal("multipart file missing")
	}
	return files[0]
}

func benchmarkPayload(size int) []byte {
	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i)
	}
	return payload
}

func benchmarkPNGPayload(size int) []byte {
	tinyPNG, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aCwkAAAAASUVORK5CYII=")
	if err != nil {
		panic(err)
	}
	if size <= len(tinyPNG) {
		return tinyPNG
	}
	payload := make([]byte, size)
	copy(payload, tinyPNG)
	for i := len(tinyPNG); i < len(payload); i++ {
		payload[i] = byte(i)
	}
	return payload
}

func removeBenchmarkUpload(b *testing.B, url string) {
	b.Helper()
	if !strings.HasPrefix(url, "/uploads/") {
		b.Fatalf("unexpected benchmark upload url: %s", url)
	}
	if err := os.Remove(filepath.FromSlash(strings.TrimPrefix(url, "/"))); err != nil {
		b.Fatalf("remove benchmark upload failed: %v", err)
	}
}
