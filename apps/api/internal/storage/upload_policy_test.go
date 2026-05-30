package storage

import "testing"

func TestWorkspaceContentUploadDirPrefix(t *testing.T) {
	got, err := WorkspaceContentUploadDirPrefix("OWNR1A", "board-1")
	if err != nil {
		t.Fatalf("WorkspaceContentUploadDirPrefix returned error: %v", err)
	}
	want := "workspace/OWNR1A/board-1/content"
	if got != want {
		t.Fatalf("dir prefix = %q, want %q", got, want)
	}
}

func TestWorkspaceBoardUploadObjectPrefix(t *testing.T) {
	got, err := WorkspaceBoardUploadObjectPrefix("OWNR1A", "board-1")
	if err != nil {
		t.Fatalf("WorkspaceBoardUploadObjectPrefix returned error: %v", err)
	}
	want := "uploads/workspace/OWNR1A/board-1/"
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

func TestParseUploadResourceWorkspace(t *testing.T) {
	resource, err := ParseUploadResource("/uploads/workspace/OWNR1A/board-1/thumb/demo.txt")
	if err != nil {
		t.Fatalf("ParseUploadResource returned error: %v", err)
	}
	if resource.Kind != UploadResourceKindWorkspace {
		t.Fatalf("kind = %q, want %q", resource.Kind, UploadResourceKindWorkspace)
	}
	if resource.Workspace == nil {
		t.Fatalf("workspace metadata is nil")
	}
	if resource.Workspace.OwnerSegment != "OWNR1A" || resource.Workspace.BoardSegment != "board-1" {
		t.Fatalf("workspace = %+v, want owner=OWNR1A board=board-1", resource.Workspace)
	}
	if resource.Workspace.RelativePath != "thumb/demo.txt" {
		t.Fatalf("relative path = %q, want %q", resource.Workspace.RelativePath, "thumb/demo.txt")
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

func TestParseUploadResourceGenericAuthenticated(t *testing.T) {
	resource, err := ParseUploadResource("/uploads/2026/05/30/demo.png")
	if err != nil {
		t.Fatalf("ParseUploadResource returned error: %v", err)
	}
	if resource.Kind != UploadResourceKindAuthenticated {
		t.Fatalf("kind = %q, want %q", resource.Kind, UploadResourceKindAuthenticated)
	}
}

func TestParseUploadResourceRejectsTraversalOutsideUploads(t *testing.T) {
	if _, err := ParseUploadResource("/uploads/../secret.txt"); err == nil {
		t.Fatalf("expected traversal outside uploads to fail")
	}
}
