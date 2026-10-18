package http

// The legacy /api/user + /api/admin upload bypasses were deleted with the
// #809 hard cut: no POST path bypasses the global JSON body limit anymore
// (the v1 workspace upload/media surface is JSON session-based). These
// tests pin the always-false classifier, including its nil-safety.

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
		{name: "legacy profile avatar bypass is gone", path: "/api/user/profile/avatar", want: false},
		{name: "legacy admin upload bypass is gone", path: "/api/admin/upload", want: false},
		{name: "legacy vision extract bypass is gone", path: "/api/user/search/vision-extract", want: false},
		{name: "legacy board upload bypass is gone", path: "/api/user/boards/board-1/uploads", want: false},
		{name: "board traversal is not reclassified", path: "/api/user/boards/board-1/../uploads", want: false},
		{name: "board missing id", path: "/api/user/boards//uploads", want: false},
		{name: "board extra segment", path: "/api/user/boards/board-1/uploads/", want: false},
		{name: "v1 workspace upload honors the global limit", path: "/api/v1/workspaces/ws_123/uploads", want: false},
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
