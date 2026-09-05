package execution

import (
	"strings"
	"testing"
)

func TestRedactSignedURLStripsQueryString(t *testing.T) {
	raw := "https://bucket.s3.us-east-1.amazonaws.com/uploads/derivatives/asset_001/processing-input.webp?X-Amz-Signature=abcdef&X-Amz-Expires=600"
	got := RedactSignedURL(raw)
	if strings.Contains(got, "X-Amz-Signature") {
		t.Fatalf("signature leaked: %q", got)
	}
	if strings.Contains(got, "abcdef") {
		t.Fatalf("token leaked: %q", got)
	}
	if !strings.Contains(got, "signature=redacted") {
		t.Fatalf("redaction marker missing: %q", got)
	}
	// The host and path are preserved so the log reader can still
	// see the bucket and the key.
	if !strings.Contains(got, "bucket.s3.us-east-1.amazonaws.com") {
		t.Fatalf("host lost: %q", got)
	}
	if !strings.Contains(got, "uploads/derivatives/asset_001/processing-input.webp") {
		t.Fatalf("path lost: %q", got)
	}
}

func TestRedactSignedURLIsNoopForLocalUploadRef(t *testing.T) {
	raw := "/uploads/canvas/owner/board/content/image.png"
	got := RedactSignedURL(raw)
	if got != raw {
		t.Fatalf("local upload ref should not be redacted: %q != %q", got, raw)
	}
}

func TestRedactSignedURLIsNoopForEmpty(t *testing.T) {
	if got := RedactSignedURL(""); got != "" {
		t.Fatalf("empty URL should be empty: %q", got)
	}
	if got := RedactSignedURL("   "); got != "   " {
		t.Fatalf("whitespace-only URL should be preserved: %q", got)
	}
}

func TestRedactSignedURLRedactsInlineDataImage(t *testing.T) {
	raw := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="
	got := RedactSignedURL(raw)
	if strings.Contains(got, "iVBORw0KGgo") {
		t.Fatalf("data URL base64 leaked: %q", got)
	}
	if !strings.Contains(got, "[redacted]") {
		t.Fatalf("redaction marker missing: %q", got)
	}
}

func TestIsSignedURLClassifiesObjectStorageHosts(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"https://bucket.s3.us-east-1.amazonaws.com/uploads/derivatives/asset_001/processing-input.webp?X-Amz-Signature=abc", true},
		{"https://cdn.example.r2.cloudflarestorage.com/uploads/derivatives/foo.webp?signature=abc", true},
		{"https://oss-cn-hangzhou.aliyuncs.com/uploads/derivatives/foo.webp?Signature=abc", true},
		{"https://cos.ap-shanghai.myqcloud.com/uploads/derivatives/foo.webp?q-signature=abc", true},
		{"https://storage.googleapis.com/uploads/derivatives/foo.webp?X-Goog-Signature=abc", true},
		{"https://example.com/uploads/foo.png", false},
		{"https://example.com/uploads/foo.png?signature=abc", true},
		{"", false},
		{"data:image/png;base64,abc", false},
	}
	for _, c := range cases {
		if got := IsSignedURL(c.raw); got != c.want {
			t.Errorf("IsSignedURL(%q) = %v, want %v", c.raw, got, c.want)
		}
	}
}

func TestIsSignedObjectKeyClassifiesTiers(t *testing.T) {
	cases := map[string]bool{
		"uploads/derivatives/asset_001/processing-input.webp": true,
		"uploads/tasks/42/task-abc/image.png":                 true,
		"uploads/processing-input/asset_001.webp":             true,
		"uploads/canvas/owner/board/content/image.png":        false,
		"":                     false,
		"some/random/path.txt": false,
	}
	for key, want := range cases {
		if got := IsSignedObjectKey(key); got != want {
			t.Errorf("IsSignedObjectKey(%q) = %v, want %v", key, got, want)
		}
	}
}
