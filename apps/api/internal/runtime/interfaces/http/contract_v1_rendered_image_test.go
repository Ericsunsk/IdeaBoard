package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"

	"github.com/gin-gonic/gin"
)

func TestRenderedImageProjection(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"ready", nil, http.StatusOK},
		{"source", imgproxy.ErrSource, http.StatusConflict},
		{"missing", imgproxy.ErrNotFound, http.StatusNotFound},
		{"unavailable", imgproxy.ErrUnavailable, http.StatusServiceUnavailable},
		{"deadline", context.DeadlineExceeded, http.StatusServiceUnavailable},
		{"internal cancellation", context.Canceled, http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := gin.New()
			engine.GET("/image", func(c *gin.Context) {
				writeRenderedImage(c, imgproxy.Image{Data: []byte("rendered image"), MIMEType: "image/webp"}, tc.err, "private, max-age=300")
			})
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest("GET", "/image", nil))
			if response.Code != tc.want {
				t.Fatalf("status = %d, want %d", response.Code, tc.want)
			}
			if tc.err != nil {
				if response.Body.String() != http.StatusText(tc.want) || response.Header().Get("Cache-Control") != "no-store" {
					t.Fatalf("error response = %q %v", response.Body.String(), response.Header())
				}
				return
			}
			if response.Body.String() != "rendered image" || response.Header().Get("Cache-Control") != "private, max-age=300" || response.Header().Get("Content-Type") != "image/webp" || response.Header().Get("Content-Disposition") != "inline" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("image response = %q %v", response.Body.String(), response.Header())
			}
		})
	}
}

func TestRenderedImageRequestCancellation(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
		body string
	}{
		{"client canceled", context.Canceled, 499, ""},
		{"upstream failed", imgproxy.ErrUnavailable, http.StatusServiceUnavailable, "Service Unavailable"},
		{"deadline exceeded", context.DeadlineExceeded, http.StatusServiceUnavailable, "Service Unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := gin.New()
			engine.GET("/image", func(c *gin.Context) {
				writeRenderedImage(c, imgproxy.Image{}, tc.err, "private, max-age=300")
			})
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			request := httptest.NewRequest(http.MethodGet, "/image", nil).WithContext(ctx)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if response.Code != tc.want || response.Body.String() != tc.body {
				t.Fatalf("canceled response = %d %q, want %d %q", response.Code, response.Body.String(), tc.want, tc.body)
			}
		})
	}
}
