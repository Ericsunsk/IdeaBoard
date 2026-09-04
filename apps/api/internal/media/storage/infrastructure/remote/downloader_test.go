package remote

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestDownloadImage(t *testing.T) {
	pngData := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9c, 0x63, 0x60, 0x60, 0xf8, 0x0f,
		0x00, 0x01, 0x05, 0x01, 0x02, 0xa5, 0x27, 0xde,
		0xfc, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
		0x44, 0xae, 0x42, 0x60, 0x82,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngData)
	}))
	defer server.Close()

	parsed, _ := url.Parse(server.URL)
	file, err := DownloadImage(context.Background(), server.URL+"/img.png", []string{parsed.Host})
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	defer file.Cleanup()

	if file.Size != int64(len(pngData)) {
		t.Fatalf("size = %d, want %d", file.Size, len(pngData))
	}
	if file.ContentType != "image/png" {
		t.Fatalf("content-type = %q, want image/png", file.ContentType)
	}
	if file.Width != 1 || file.Height != 1 {
		t.Fatalf("dimensions = %dx%d, want 1x1", file.Width, file.Height)
	}
}

func TestDownloadImageDetectsExtensionFromBytes(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)

	imageBytes := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9c, 0x63, 0x60, 0x60, 0xf8, 0x0f,
		0x00, 0x01, 0x05, 0x01, 0x02, 0xa5, 0x27, 0xde,
		0xfc, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
		0x44, 0xae, 0x42, 0x60, 0x82,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(imageBytes)
	}))
	defer server.Close()

	remoteFile, err := DownloadImage(context.Background(), server.URL+"/image.jpg", []string{server.URL})
	if err != nil {
		t.Fatalf("DownloadImage returned error: %v", err)
	}
	defer remoteFile.Cleanup()

	if !strings.HasSuffix(remoteFile.Filename, ".png") {
		t.Fatalf("filename = %q, want .png suffix", remoteFile.Filename)
	}
	if remoteFile.Size != int64(len(imageBytes)) {
		t.Fatalf("size = %d, want %d", remoteFile.Size, len(imageBytes))
	}
}

func TestDownloadImageRejectsDisguisedImage(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("<html><body>not an image</body></html>"))
	}))
	defer server.Close()

	_, err := DownloadImage(context.Background(), server.URL+"/image.png", []string{server.URL})
	if err == nil {
		t.Fatal("expected disguised image to be rejected")
	}
	if !strings.Contains(err.Error(), "远程图片校验失败") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRemoteDirEmpty(t, tmpDir)
}

func TestDownloadImageRejectsDimensionAndPixelLimitHeaders(t *testing.T) {
	for _, test := range []struct {
		name          string
		width, height uint32
		want          string
	}{
		{name: "dimension", width: 20000, height: 2, want: "图片尺寸超过限制"},
		{name: "pixel count", width: 9000, height: 8000, want: "图片总像素超过限制"},
	} {
		t.Run(test.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv("TMPDIR", tmpDir)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write(remotePNGConfigOnly(test.width, test.height))
			}))
			defer server.Close()

			_, err := DownloadImage(context.Background(), server.URL+"/image.png", []string{server.URL})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("DownloadImage() error = %v, want %q", err, test.want)
			}
			assertRemoteDirEmpty(t, tmpDir)
		})
	}
}

func TestDownloadImageCleansTempFileOnOversize(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)
	const testMaxSize = int64(64)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte{0x89, 0x50, 0x4E, 0x47})
		_, _ = io.Copy(w, io.LimitReader(zeroReader{}, testMaxSize+1))
	}))
	defer server.Close()

	_, err := DownloadImageWithLimit(context.Background(), server.URL+"/image.png", []string{server.URL}, testMaxSize)
	if err == nil {
		t.Fatal("expected oversize remote file to be rejected")
	}
	if !strings.Contains(err.Error(), "远程文件超过大小限制") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRemoteDirEmpty(t, tmpDir)
}

func remotePNGConfigOnly(width, height uint32) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a})
	payload := make([]byte, 13)
	binary.BigEndian.PutUint32(payload[0:4], width)
	binary.BigEndian.PutUint32(payload[4:8], height)
	payload[8], payload[9] = 8, 2
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(payload)))
	buf.Write(length[:])
	buf.WriteString("IHDR")
	buf.Write(payload)
	checksum := crc32.NewIEEE()
	_, _ = checksum.Write([]byte("IHDR"))
	_, _ = checksum.Write(payload)
	var sum [4]byte
	binary.BigEndian.PutUint32(sum[:], checksum.Sum32())
	buf.Write(sum[:])
	return buf.Bytes()
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func assertRemoteDirEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir failed: %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("expected dir to be empty, got: %s", strings.Join(names, ", "))
	}
}
