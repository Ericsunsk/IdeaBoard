package http

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	storageports "ideaboard/api/internal/media/storage/ports"
	platformworkermedia "ideaboard/api/internal/platform/workermedia"

	"github.com/gin-gonic/gin"
)

type workerMediaTestObjects struct {
	data map[string][]byte
	ct   map[string]string
}

func (f *workerMediaTestObjects) ReadObjectAtProfile(_ context.Context, _ string, key string, _ int64) (*storageports.StoredUploadObject, error) {
	raw, ok := f.data[key]
	if !ok {
		return nil, errWorkerMediaMissing
	}
	return &storageports.StoredUploadObject{ObjectKey: key, ContentType: f.ct[key], Data: raw}, nil
}

func (f *workerMediaTestObjects) PutObjectAtProfile(_ context.Context, _ string, key string, data []byte, contentType string) error {
	if f.data == nil {
		f.data = map[string][]byte{}
	}
	if f.ct == nil {
		f.ct = map[string]string{}
	}
	f.data[key] = append([]byte(nil), data...)
	f.ct[key] = contentType
	return nil
}

type errWorkerMediaNotFound string

func (e errWorkerMediaNotFound) Error() string { return string(e) }

var errWorkerMediaMissing = errWorkerMediaNotFound("object not found")

func newWorkerMediaTestRouter(secret string, objects *workerMediaTestObjects) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := NewWorkerMediaHandler(secret, objects)
	router.GET("/api/v1/worker/media/*key", h.ServeGET)
	router.PUT("/api/v1/worker/media/*key", h.ServePUT)
	return router
}

func workerMediaSignedURL(secret, method, key, contentType string, ttl time.Duration) string {
	return platformworkermedia.SignURL("https://api.example.test", secret, method, key, contentType, time.Now().UTC().Add(ttl))
}

func workerMediaServe(t *testing.T, router *gin.Engine, method, rawURL string, body []byte, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	// Rewrite the absolute presigned URL to a relative request target.
	target := rawURL
	if idx := strings.Index(rawURL, "/api/v1/worker/media/"); idx >= 0 {
		target = rawURL[idx:]
	}
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestWorkerMediaHandoffLoopback(t *testing.T) {
	objects := &workerMediaTestObjects{
		data: map[string][]byte{"uploads/ws/test/in.png": {0x89, 0x50, 0x4E, 0x47}},
		ct:   map[string]string{"uploads/ws/test/in.png": "image/png"},
	}
	router := newWorkerMediaTestRouter("test-secret", objects)

	// GET input with a valid signature serves exact bytes.
	getURL := workerMediaSignedURL("test-secret", http.MethodGet, "uploads/ws/test/in.png", "image/png", 5*time.Minute)
	rec := workerMediaServe(t, router, http.MethodGet, getURL, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rec.Code)
	}
	if !bytes.Equal(rec.Body.Bytes(), []byte{0x89, 0x50, 0x4E, 0x47}) {
		t.Fatalf("GET bytes mismatch")
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("GET content-type = %q, want image/png", ct)
	}

	// PUT output with bound content-type lands exactly.
	key := "uploads/ws/abc/task-1/result-0.png"
	putURL := workerMediaSignedURL("test-secret", http.MethodPut, key, "image/png", 5*time.Minute)
	rec = workerMediaServe(t, router, http.MethodPut, putURL, []byte{1, 2, 3, 4}, "image/png")
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %q, want 200", rec.Code, rec.Body.String())
	}
	if !bytes.Equal(objects.data[key], []byte{1, 2, 3, 4}) || objects.ct[key] != "image/png" {
		t.Fatalf("landed object mismatch: %v %q", objects.data[key], objects.ct[key])
	}
}

func TestWorkerMediaRejectsBadSignatures(t *testing.T) {
	objects := &workerMediaTestObjects{data: map[string][]byte{}, ct: map[string]string{}}
	router := newWorkerMediaTestRouter("test-secret", objects)

	// Forged signature.
	rec := workerMediaServe(t, router, http.MethodGet,
		"https://api.example.test/api/v1/worker/media/uploads/ws/a/b.png?ct=image%2Fpng&exp=9999999999&sig=forged",
		nil, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("forged GET status = %d, want 403", rec.Code)
	}

	// Expired.
	expired := platformworkermedia.SignURL("https://api.example.test", "test-secret", http.MethodGet,
		"uploads/ws/a/b.png", "image/png", time.Now().UTC().Add(-time.Minute))
	rec = workerMediaServe(t, router, http.MethodGet, expired, nil, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expired GET status = %d, want 403", rec.Code)
	}

	// Cross-method replay: GET signature on PUT.
	key := "uploads/ws/abc/task-1/result-0.png"
	getURL := workerMediaSignedURL("test-secret", http.MethodGet, key, "image/png", 5*time.Minute)
	rec = workerMediaServe(t, router, http.MethodPut, getURL, []byte{1}, "image/png")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-method PUT status = %d, want 403", rec.Code)
	}

	// Content-type mismatch on PUT.
	putURL := workerMediaSignedURL("test-secret", http.MethodPut, key, "image/png", 5*time.Minute)
	rec = workerMediaServe(t, router, http.MethodPut, putURL, []byte{1}, "image/jpeg")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("ct-mismatch PUT status = %d, want 400", rec.Code)
	}

	// Traversal key never verifies.
	if platformworkermedia.ValidSlotKey("../evil.png") {
		t.Fatal("traversal key accepted, want rejection")
	}
	evil := platformworkermedia.SignURL("https://api.example.test", "test-secret", http.MethodPut,
		"../evil.png", "image/png", time.Now().UTC().Add(5*time.Minute))
	rec = workerMediaServe(t, router, http.MethodPut, evil, []byte{1}, "image/png")
	if rec.Code == http.StatusOK {
		t.Fatal("traversal PUT succeeded, want failure")
	}
}

func TestWorkerMediaMissingObjectIsNotFound(t *testing.T) {
	objects := &workerMediaTestObjects{data: map[string][]byte{}, ct: map[string]string{}}
	router := newWorkerMediaTestRouter("test-secret", objects)
	getURL := workerMediaSignedURL("test-secret", http.MethodGet, "uploads/ws/a/missing.png", "image/png", 5*time.Minute)
	rec := workerMediaServe(t, router, http.MethodGet, getURL, nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing GET status = %d, want 404", rec.Code)
	}
}
