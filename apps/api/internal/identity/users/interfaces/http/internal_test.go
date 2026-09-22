package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type stubLoginMetadataUpdater struct {
	calls int
	err   error
}

func (s *stubLoginMetadataUpdater) UpdateLastLoginMetadata(ctx context.Context, authUserID, ip, country string) error {
	s.calls++
	return s.err
}

func newInternalTestRouter(handler *InternalHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/internal/users/login-metadata", handler.UpdateLoginMetadata)
	return r
}

func serveInternalRequest(t *testing.T, r *gin.Engine, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/internal/users/login-metadata", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("X-Internal-Key", key)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestUpdateLoginMetadataRequiresInternalKey(t *testing.T) {
	const key = "test-internal-key-with-sufficient-entropy"
	validBody := `{"auth_user_id":"user-1","ip":"127.0.0.1","country":"CN"}`

	t.Run("missing key is unauthorized", func(t *testing.T) {
		updater := &stubLoginMetadataUpdater{}
		w := serveInternalRequest(t, newInternalTestRouter(NewInternalHandler(updater, key)), "", validBody)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
		}
		if updater.calls != 0 {
			t.Fatalf("updater calls = %d, want 0", updater.calls)
		}
	})

	t.Run("wrong key is unauthorized", func(t *testing.T) {
		updater := &stubLoginMetadataUpdater{}
		w := serveInternalRequest(t, newInternalTestRouter(NewInternalHandler(updater, key)), "wrong-key-with-same-length-xxxx", validBody)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
		}
		if updater.calls != 0 {
			t.Fatalf("updater calls = %d, want 0", updater.calls)
		}
	})

	t.Run("correct key updates metadata", func(t *testing.T) {
		updater := &stubLoginMetadataUpdater{}
		w := serveInternalRequest(t, newInternalTestRouter(NewInternalHandler(updater, key)), key, validBody)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
		}
		if updater.calls != 1 {
			t.Fatalf("updater calls = %d, want 1", updater.calls)
		}
	})

	t.Run("correct key with invalid body is bad request", func(t *testing.T) {
		updater := &stubLoginMetadataUpdater{}
		w := serveInternalRequest(t, newInternalTestRouter(NewInternalHandler(updater, key)), key, `{"auth_user_id":"","ip":"not-an-ip"}`)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
		if updater.calls != 0 {
			t.Fatalf("updater calls = %d, want 0", updater.calls)
		}
	})
}
