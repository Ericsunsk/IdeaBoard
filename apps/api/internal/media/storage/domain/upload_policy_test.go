package domain_test

import (
	"strings"
	"testing"

	"ideaboard/api/internal/media/storage/domain"
)

func TestAvatarUploadObjectKey(t *testing.T) {
	objectPrefix, err := domain.AvatarUploadObjectPrefix("OWNR1A")
	if err != nil {
		t.Fatalf("AvatarUploadObjectPrefix returned error: %v", err)
	}
	if objectPrefix != "uploads/avatars/OWNR1A/" {
		t.Fatalf("object prefix = %q, want %q", objectPrefix, "uploads/avatars/OWNR1A/")
	}

	key, err := domain.AvatarObjectKey("OWNR1A", "abcdef1234567890", ".png")
	if err != nil {
		t.Fatalf("AvatarObjectKey returned error: %v", err)
	}
	if key != "uploads/avatars/OWNR1A/abcdef1234567890.png" {
		t.Fatalf("object key = %q, want the canonical avatar key", key)
	}
	if _, err := domain.AvatarObjectKey("OWNR1A", "", ".png"); err == nil {
		t.Fatal("blank avatar hash should fail")
	}
	if _, err := domain.AvatarObjectKey("bad uid", "abcdef1234567890", ".png"); err == nil {
		t.Fatal("invalid storage UID should fail")
	}
}

func TestSiteAssetLogoObjectKey(t *testing.T) {
	if got := domain.SiteAssetLogoDirPrefix(); got != "uploads/site/assets/" {
		t.Fatalf("dir prefix = %q, want %q", got, "uploads/site/assets/")
	}
	if !domain.IsSiteAssetLogoKey("uploads/site/assets/logo_abcdef1234567890.png") {
		t.Fatal("canonical logo key should report as a site-asset logo key")
	}
	if domain.IsSiteAssetLogoKey("uploads/tasks/42/task-abc/demo.png") {
		t.Fatal("task output key must not report as a site-asset logo key")
	}
	key, err := domain.SiteAssetLogoObjectKey("abcdef1234567890", ".png")
	if err != nil {
		t.Fatalf("SiteAssetLogoObjectKey returned error: %v", err)
	}
	if key != "uploads/site/assets/logo_abcdef1234567890.png" {
		t.Fatalf("object key = %q, want the canonical logo key", key)
	}
	key, err = domain.SiteAssetLogoObjectKey("ABCDEF1234567890", "png")
	if err != nil || key != "uploads/site/assets/logo_abcdef1234567890.png" {
		t.Fatalf("object key = %q, %v; want lowercased digest with dotted ext", key, err)
	}
	if _, err := domain.SiteAssetLogoObjectKey("", ".png"); err == nil {
		t.Fatal("blank logo hash should fail")
	}
}

func TestTaskOutputUploadPrefixes(t *testing.T) {
	userPrefix, err := domain.TaskOutputUserUploadObjectPrefix(42)
	if err != nil {
		t.Fatalf("TaskOutputUserUploadObjectPrefix returned error: %v", err)
	}
	if userPrefix != "uploads/tasks/42/" {
		t.Fatalf("user object prefix = %q, want %q", userPrefix, "uploads/tasks/42/")
	}
}

func TestUserUploadObjectPrefixesIncludesAllUserOwnedUploadRoots(t *testing.T) {
	prefixes, err := domain.UserUploadObjectPrefixes(42, "OWNR1A")
	if err != nil {
		t.Fatalf("userUploadObjectPrefixes returned error: %v", err)
	}
	want := []string{
		"uploads/avatars/OWNR1A/",
		"uploads/tasks/42/",
		"uploads/library/42/",
	}
	if len(prefixes) != len(want) {
		t.Fatalf("prefixes = %#v, want %#v", prefixes, want)
	}
	for i := range want {
		if prefixes[i] != want[i] {
			t.Fatalf("prefixes[%d] = %q, want %q", i, prefixes[i], want[i])
		}
	}
}

func TestNormalizeUploadObjectKeyRejectsTraversalOutsideUploads(t *testing.T) {
	if _, err := domain.NormalizeUploadObjectKey("/uploads/../secret.txt"); err == nil {
		t.Fatalf("expected traversal outside uploads to fail")
	}
	if _, err := domain.NormalizeUploadObjectKey("/uploads/2026/05/30/demo.png"); err != nil {
		t.Fatalf("expected generic uploads key to normalize, got %v", err)
	}
}

func TestBuildObjectKeyRejectsTraversalDirPrefix(t *testing.T) {
	// Escaping prefixes still fail (anchored by NormalizeUploadObjectKey).
	if _, err := domain.BuildObjectKey("../private", ".png"); err == nil {
		t.Fatalf("expected escaping dir prefix to fail")
	}
	if _, err := domain.BuildObjectKey("../../private", ".png"); err == nil {
		t.Fatalf("expected escaping dir prefix to fail")
	}
	// Interior ".." that resolves inside the root normalizes instead of
	// failing (single-source traversal defense lives in Normalize*).
	key, err := domain.BuildObjectKey("tasks/OWNER/../../private", ".png")
	if err != nil {
		t.Fatalf("expected contained traversal to normalize, got %v", err)
	}
	if !strings.HasPrefix(key, "uploads/private/") {
		t.Fatalf("normalized key = %q, want uploads/private/ containment", key)
	}
}

func TestNormalizeUploadObjectKeySingleRoot(t *testing.T) {
	// Static retirement cutover: only the canonical uploads/ root
	// resolves (the ws-uploads/ dual-read retired with the static
	// /uploads/ handler, same window as the keyspace-unification
	// backfill).
	for _, key := range []string{
		"uploads/ws/abc123/def456/photo.png",
		"/uploads/ws/abc123/def456/photo.png",
		"uploads/tasks/42/task-abc/demo.png",
	} {
		if _, err := domain.NormalizeUploadObjectKey(key); err != nil {
			t.Fatalf("NormalizeUploadObjectKey(%q) = %v, want nil", key, err)
		}
	}
	for _, key := range []string{
		"ws-uploads/abc123/def456/photo.png",
		"/ws-uploads/abc123/def456/photo.png",
		"r2/tenant/photo.png",
	} {
		if _, err := domain.NormalizeUploadObjectKey(key); err == nil {
			t.Fatalf("NormalizeUploadObjectKey(%q) should fail, want rejection", key)
		}
	}
}
