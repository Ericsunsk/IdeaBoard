package httpapi

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	"ideaboard/api/internal/media/derivatives/infrastructure/preview"

	"github.com/gin-gonic/gin"
)

func expiredPublicPreviewPath(t *testing.T, config imgproxy.Config, path string) string {
	t.Helper()
	parts := strings.Split(path, "/")
	key, _ := hex.DecodeString(config.KeyHex)
	salt, _ := hex.DecodeString(config.SaltHex)
	mac := hmac.New(sha256.New, key)
	mac.Write(salt)
	purpose := []byte("ideaboard.external-preview.v1")
	mac.Write(purpose)
	block, err := aes.NewCipher(mac.Sum(nil))
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[4])
	if err != nil {
		t.Fatal(err)
	}
	payload, err := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], purpose)
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	claims["expires"] = time.Now().Add(-time.Minute).Unix()
	payload, err = json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	parts[4] = base64.RawURLEncoding.EncodeToString(aead.Seal(raw[:aead.NonceSize()], raw[:aead.NonceSize()], payload, purpose))
	return strings.Join(parts, "/")
}

func TestPublicMediaPreviewRoutesWithoutSearchOrWorkspace(t *testing.T) {
	config := imgproxy.Config{BaseURL: "http://unused.test", KeyHex: strings.Repeat("ab", 32), SaltHex: strings.Repeat("cd", 32)}
	previews, err := preview.NewService(config, t.TempDir(), imgproxy.NewRenderer(config, nil))
	if err != nil {
		t.Fatal(err)
	}
	display := previews.SearchDisplay("unsplash", "route-preview", "https://images.unsplash.com/preview")
	if display == nil {
		t.Fatal("valid search result lacks a capability")
	}
	path := display.Variants[0].URL
	router := gin.New()
	RegisterRoutes(router, Dependencies{ContractV1: ContractV1Deps{MediaPreviews: previews}}, nil, FrontendRouteOptions{})
	for _, tt := range []struct {
		name   string
		path   string
		status int
	}{
		{"invalid capability", "/api/media/previews/invalid/thumb", http.StatusNotFound},
		{"tampered capability", strings.Replace(path, "/thumb", "x/thumb", 1), http.StatusNotFound},
		{"expired capability", expiredPublicPreviewPath(t, config, path), http.StatusNotFound},
		{"invalid tier", strings.TrimSuffix(path, "/thumb") + "/original", http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != tt.status || recorder.Header().Get("Cache-Control") != "no-store" || recorder.Body.String() != http.StatusText(tt.status) {
				t.Fatalf("response = %d %q; want plain %d with no-store", recorder.Code, recorder.Header().Get("Cache-Control"), tt.status)
			}
		})
	}
	// Exercise cancellation at the Media route; global middleware may stop
	// already-cancelled requests before dispatching any handler.
	isolated := gin.New()
	registerPublicMediaRoutes(isolated, previews)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	recorder := httptest.NewRecorder()
	isolated.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx))
	if recorder.Code != http.StatusServiceUnavailable || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("cancelled Media render did not fail unavailable")
	}
}

func TestPublicPreviewProjectionCacheAndBytes(t *testing.T) {
	image := imgproxy.Image{MIMEType: "image/webp", Data: []byte("rendered bytes")}
	for _, tt := range []struct {
		name   string
		life   time.Duration
		minAge int
		maxAge int
	}{
		{"cap long lifetime", 24 * time.Hour, 300, 300},
		{"bound remaining lifetime", 90 * time.Second, 85, 90},
		{"no negative age", -time.Second, 0, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			writePublicPreview(ctx, image, time.Now().Add(tt.life), nil)
			var maxAge int
			if _, err := fmt.Sscanf(recorder.Header().Get("Cache-Control"), "public, max-age=%d", &maxAge); err != nil || maxAge < tt.minAge || maxAge > tt.maxAge {
				t.Fatalf("cache age = %q", recorder.Header().Get("Cache-Control"))
			}
			if recorder.Code != http.StatusOK || recorder.Body.String() != string(image.Data) || recorder.Header().Get("Content-Type") != image.MIMEType || recorder.Header().Get("Content-Disposition") != "inline" || recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("rendered bytes projection lost headers or body")
			}
		})
	}
}

func TestRenderedImagePublicPrivateErrorPolicy(t *testing.T) {
	for _, tt := range []struct {
		err     error
		public  int
		private int
	}{
		{imgproxy.ErrSource, http.StatusBadRequest, http.StatusConflict},
		{imgproxy.ErrNotFound, http.StatusNotFound, http.StatusNotFound},
		{preview.ErrCapability, http.StatusNotFound, http.StatusServiceUnavailable},
		{errors.New("render failed"), http.StatusServiceUnavailable, http.StatusServiceUnavailable},
	} {
		for _, isPublic := range []bool{true, false} {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			want := tt.private
			if isPublic {
				want = tt.public
				writePublicPreview(ctx, imgproxy.Image{}, time.Time{}, tt.err)
			} else {
				writeRenderedImage(ctx, imgproxy.Image{}, tt.err, "private, max-age=300")
			}
			if recorder.Code != want || recorder.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("public=%v status=%d, want %d with no-store", isPublic, recorder.Code, want)
			}
		}
	}
}
