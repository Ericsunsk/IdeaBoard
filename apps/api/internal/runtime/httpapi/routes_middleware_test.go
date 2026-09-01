package httpapi

import (
	"net/http"
	"net/url"
	"testing"
)

func TestIsUploadRequestClassifiesURLPathsWithoutFilesystemCleaning(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "profile avatar", path: "/api/user/profile/avatar", want: true},
		{name: "admin upload", path: "/api/admin/upload", want: true},
		{name: "vision extract", path: "/api/user/search/vision-extract", want: true},
		{name: "board upload", path: "/api/user/boards/board-1/uploads", want: true},
		{name: "board traversal is not reclassified", path: "/api/user/boards/board-1/../uploads", want: false},
		{name: "board missing id", path: "/api/user/boards//uploads", want: false},
		{name: "board extra segment", path: "/api/user/boards/board-1/uploads/", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{Method: http.MethodPost, URL: &url.URL{Path: tt.path}}
			if got := isUploadRequest(req); got != tt.want {
				t.Fatalf("isUploadRequest(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsUploadRequestHandlesMalformedRequests(t *testing.T) {
	if isUploadRequest(nil) {
		t.Fatal("nil request should not be classified as upload")
	}
	if isUploadRequest(&http.Request{Method: http.MethodPost}) {
		t.Fatal("request without URL should not be classified as upload")
	}
	if isUploadRequest(&http.Request{Method: http.MethodGet, URL: &url.URL{Path: "/api/admin/upload"}}) {
		t.Fatal("non-POST request should not be classified as upload")
	}
}
