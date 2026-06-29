package storage

import "testing"

func TestCanvasContentUploadDirPrefix(t *testing.T) {
	got, err := CanvasContentUploadDirPrefix("OWNR1A", "board-1")
	if err != nil {
		t.Fatalf("CanvasContentUploadDirPrefix returned error: %v", err)
	}
	want := "canvas/OWNR1A/board-1/content"
	if got != want {
		t.Fatalf("dir prefix = %q, want %q", got, want)
	}
}

func TestCanvasBoardUploadObjectPrefix(t *testing.T) {
	got, err := CanvasBoardUploadObjectPrefix("OWNR1A", "board-1")
	if err != nil {
		t.Fatalf("CanvasBoardUploadObjectPrefix returned error: %v", err)
	}
	want := "uploads/canvas/OWNR1A/board-1/"
	if got != want {
		t.Fatalf("object prefix = %q, want %q", got, want)
	}
}

func TestProfileAvatarUploadPrefixes(t *testing.T) {
	dirPrefix, err := ProfileAvatarUploadDirPrefix("OWNR1A")
	if err != nil {
		t.Fatalf("ProfileAvatarUploadDirPrefix returned error: %v", err)
	}
	if dirPrefix != "profile/OWNR1A/avatar" {
		t.Fatalf("dir prefix = %q, want %q", dirPrefix, "profile/OWNR1A/avatar")
	}

	objectPrefix, err := ProfileAvatarUploadObjectPrefix("OWNR1A")
	if err != nil {
		t.Fatalf("ProfileAvatarUploadObjectPrefix returned error: %v", err)
	}
	if objectPrefix != "uploads/profile/OWNR1A/avatar/" {
		t.Fatalf("object prefix = %q, want %q", objectPrefix, "uploads/profile/OWNR1A/avatar/")
	}
}

func TestParseUploadResourceCanvas(t *testing.T) {
	resource, err := ParseUploadResource("/uploads/canvas/OWNR1A/board-1/thumb/demo.txt")
	if err != nil {
		t.Fatalf("ParseUploadResource returned error: %v", err)
	}
	if resource.Kind != UploadResourceKindCanvas {
		t.Fatalf("kind = %q, want %q", resource.Kind, UploadResourceKindCanvas)
	}
	if resource.Canvas == nil {
		t.Fatalf("canvas metadata is nil")
	}
	if resource.Canvas.OwnerSegment != "OWNR1A" || resource.Canvas.BoardSegment != "board-1" {
		t.Fatalf("canvas = %+v, want owner=OWNR1A board=board-1", resource.Canvas)
	}
	if resource.Canvas.RelativePath != "thumb/demo.txt" {
		t.Fatalf("relative path = %q, want %q", resource.Canvas.RelativePath, "thumb/demo.txt")
	}
}

func TestParseUploadResourceProfileAvatar(t *testing.T) {
	resource, err := ParseUploadResource("uploads/profile/OWNR1A/avatar/avatar.txt")
	if err != nil {
		t.Fatalf("ParseUploadResource returned error: %v", err)
	}
	if resource.Kind != UploadResourceKindProfileAvatar {
		t.Fatalf("kind = %q, want %q", resource.Kind, UploadResourceKindProfileAvatar)
	}
	if resource.ProfileAvatar == nil {
		t.Fatalf("profile avatar metadata is nil")
	}
	if resource.ProfileAvatar.OwnerSegment != "OWNR1A" {
		t.Fatalf("owner segment = %q, want %q", resource.ProfileAvatar.OwnerSegment, "OWNR1A")
	}
}

func TestParseUploadResourceTaskOutput(t *testing.T) {
	resource, err := ParseUploadResource("/uploads/tasks/42/task-abc/demo.png")
	if err != nil {
		t.Fatalf("ParseUploadResource returned error: %v", err)
	}
	if resource.Kind != UploadResourceKindTaskOutput || resource.TaskOutput == nil {
		t.Fatalf("kind = %q taskOutput=%#v, want task output", resource.Kind, resource.TaskOutput)
	}
	if resource.TaskOutput.UserSegment != "42" || resource.TaskOutput.TaskSegment != "task-abc" {
		t.Fatalf("task output = %#v", resource.TaskOutput)
	}
}

func TestParseUploadResourceSiteAsset(t *testing.T) {
	resource, err := ParseUploadResource("/uploads/site/assets/logo.png")
	if err != nil {
		t.Fatalf("ParseUploadResource returned error: %v", err)
	}
	if resource.Kind != UploadResourceKindSiteAsset || resource.SiteAsset == nil {
		t.Fatalf("kind = %q siteAsset=%#v, want site asset", resource.Kind, resource.SiteAsset)
	}
	if resource.SiteAsset.RelativePath != "logo.png" {
		t.Fatalf("relative path = %q, want %q", resource.SiteAsset.RelativePath, "logo.png")
	}
}

func TestTaskOutputUploadPrefixes(t *testing.T) {
	dirPrefix, err := TaskOutputUploadDirPrefix(42, "task-abc")
	if err != nil {
		t.Fatalf("TaskOutputUploadDirPrefix returned error: %v", err)
	}
	if dirPrefix != "tasks/42/task-abc" {
		t.Fatalf("dir prefix = %q, want %q", dirPrefix, "tasks/42/task-abc")
	}

	userPrefix, err := TaskOutputUserUploadObjectPrefix(42)
	if err != nil {
		t.Fatalf("TaskOutputUserUploadObjectPrefix returned error: %v", err)
	}
	if userPrefix != "uploads/tasks/42/" {
		t.Fatalf("user object prefix = %q, want %q", userPrefix, "uploads/tasks/42/")
	}

	taskPrefix, err := TaskOutputUploadObjectPrefix(42, "task-abc")
	if err != nil {
		t.Fatalf("TaskOutputUploadObjectPrefix returned error: %v", err)
	}
	if taskPrefix != "uploads/tasks/42/task-abc/" {
		t.Fatalf("task object prefix = %q, want %q", taskPrefix, "uploads/tasks/42/task-abc/")
	}
}

func TestSiteAssetUploadPrefixes(t *testing.T) {
	if got := SiteAssetUploadDirPrefix(); got != "site/assets" {
		t.Fatalf("dir prefix = %q, want %q", got, "site/assets")
	}
	if got := SiteAssetUploadObjectPrefix(); got != "uploads/site/assets/" {
		t.Fatalf("object prefix = %q, want %q", got, "uploads/site/assets/")
	}
}

func TestUserUploadObjectPrefixesIncludesAllUserOwnedUploadRoots(t *testing.T) {
	prefixes, err := userUploadObjectPrefixes(42, "OWNR1A")
	if err != nil {
		t.Fatalf("userUploadObjectPrefixes returned error: %v", err)
	}
	want := []string{
		"uploads/canvas/OWNR1A/",
		"uploads/profile/OWNR1A/avatar/",
		"uploads/tasks/42/",
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

func TestParseUploadResourceRejectsGenericPath(t *testing.T) {
	if _, err := ParseUploadResource("/uploads/2026/05/30/demo.png"); err == nil {
		t.Fatalf("expected generic upload path to fail")
	}
}

func TestParseUploadResourceRejectsUnsupportedSitePath(t *testing.T) {
	if _, err := ParseUploadResource("/uploads/site/private/logo.png"); err == nil {
		t.Fatalf("expected unsupported site upload path to fail")
	}
}

func TestParseUploadResourceRejectsTraversalOutsideUploads(t *testing.T) {
	if _, err := ParseUploadResource("/uploads/../secret.txt"); err == nil {
		t.Fatalf("expected traversal outside uploads to fail")
	}
}

func TestBuildObjectKeyRejectsTraversalDirPrefix(t *testing.T) {
	if _, err := buildObjectKey("../private", ".png"); err == nil {
		t.Fatalf("expected traversal dir prefix to fail")
	}
	if _, err := buildObjectKey("canvas/OWNER/../../private", ".png"); err == nil {
		t.Fatalf("expected nested traversal dir prefix to fail")
	}
}
