package workermedia

import (
	"strings"
	"testing"
	"time"
)

func TestMintOutputKeyAlignsWithMediaScheme(t *testing.T) {
	if got := MintOutputKey("ws_abc", "task-1"); got != "uploads/ws/abc/task-1/result-0.png" {
		t.Fatalf("key = %q, want uploads/ws/abc/task-1/result-0.png", got)
	}
	if got := MintOutputKey("", ""); !strings.HasPrefix(got, "uploads/ws/") || !strings.HasSuffix(got, "/result-0.png") {
		t.Fatalf("fallback key = %q, want uploads/ws/.../result-0.png", got)
	}
	for _, segment := range strings.Split(MintOutputKey("ws_../evil", "task-1"), "/") {
		if segment == ".." {
			t.Fatal("traversal key accepted, want contained key")
		}
	}
}

func TestSignVerifyRoundTrip(t *testing.T) {
	expires := time.Now().UTC().Add(SlotTTL)
	raw := SignURL("https://api.example.test", "s3cr3t", "PUT", "uploads/ws/abc/task-1/result-0.png", SlotContentType, expires)
	verified, err := VerifyURL("s3cr3t", raw, "PUT")
	if err != nil {
		t.Fatalf("VerifyURL() error = %v", err)
	}
	if verified.Key != "uploads/ws/abc/task-1/result-0.png" || verified.ContentType != SlotContentType {
		t.Fatalf("verified = %+v", verified)
	}
	if _, err := VerifyURL("s3cr3t", raw, "GET"); err == nil {
		t.Fatal("cross-method signature accepted, want error")
	}
	parsed, err := ParseKey(raw + "&extra=1")
	if err != nil || parsed != verified.Key {
		t.Fatalf("ParseKey = %q, %v; want %q", parsed, err, verified.Key)
	}
}

func TestVerifyRejectsTampering(t *testing.T) {
	good := SignURL("https://api.example.test", "s3cr3t", "GET", "uploads/ws/a/b.png", "image/png", time.Now().UTC().Add(time.Minute))
	if _, err := VerifyURL("wrong", good, "GET"); err == nil {
		t.Fatal("wrong secret accepted, want error")
	}
	tampered := strings.Replace(good, "image%2Fpng", "image%2Fjpeg", 1)
	if _, err := VerifyURL("s3cr3t", tampered, "GET"); err == nil {
		t.Fatal("tampered content-type accepted, want error")
	}
	if _, err := VerifyURL("s3cr3t", "https://api.example.test/other/uploads/ws/a/b.png?ct=image%2Fpng&exp=9999999999&sig=x", "GET"); err == nil {
		t.Fatal("off-route URL accepted, want error")
	}
	if _, err := VerifyURL("s3cr3t", SignURL("https://api.example.test", "s3cr3t", "GET", "../evil.png", "image/png", time.Now().UTC().Add(time.Minute)), "GET"); err == nil {
		t.Fatal("traversal key accepted, want error")
	}
}

func TestSlotConstants(t *testing.T) {
	if SlotContentType != "image/png" {
		t.Fatalf("slot content-type = %q, want image/png", SlotContentType)
	}
	if SlotTTL != 5*time.Minute {
		t.Fatalf("slot ttl = %s, want short 5m validity", SlotTTL)
	}
}
