package wiring

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCopyMediaObject(t *testing.T) {
	for _, migration := range []mediaObjectMigration{
		{source: "ws-uploads/workspace/upload/image.png", target: "uploads/ws/workspace/upload/image.png", required: true},
		{source: "uploads/profile/user/avatar/avatar.png", target: "uploads/avatars/user/avatar.png", required: true},
	} {
		t.Run(migration.target, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, migration.source)), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, migration.source), []byte("original bytes"), 0600); err != nil {
				t.Fatal(err)
			}
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			for range 2 {
				if err := copyMediaObject(root, migration); err != nil {
					t.Fatal(err)
				}
			}
			for _, key := range []string{migration.source, migration.target} {
				actual, err := root.ReadFile(key)
				if err != nil || string(actual) != "original bytes" {
					t.Fatalf("%s = %q, err = %v", key, actual, err)
				}
			}
			if err := root.WriteFile(migration.target, []byte("conflicting bytes"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := copyMediaObject(root, migration); err == nil {
				t.Fatal("different destination bytes must block migration")
			}
		})
	}
}

func TestCopyMediaObjectMissingSource(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	migration := mediaObjectMigration{source: "ws-uploads/missing.png", target: "uploads/ws/missing.png"}
	if err := copyMediaObject(root, migration); err != nil {
		t.Fatalf("an unfilled upload session may have no bytes: %v", err)
	}
	migration.required = true
	if err := copyMediaObject(root, migration); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a missing asset must block migration, got %v", err)
	}
}

func TestCopyMediaObjectCannotEscapeRoot(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, key := range []string{"../private", "/tmp/private", "uploads/../private"} {
		if err := copyMediaObject(root, mediaObjectMigration{source: key, target: "uploads/ws/image.png", required: true}); err == nil {
			t.Errorf("invalid source %q must be rejected", key)
		}
	}
}

func TestCopyMediaObjectFromPersistentStaging(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := root.MkdirAll("uploads/ws-uploads/workspace/upload", 0700); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile("uploads/ws-uploads/workspace/upload/image.png", []byte("staged bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	migration := mediaObjectMigration{source: "ws-uploads/workspace/upload/image.png", target: "uploads/ws/workspace/upload/image.png", required: true}
	if err := copyMediaObject(root, migration); err != nil {
		t.Fatal(err)
	}
	actual, err := root.ReadFile(migration.target)
	if err != nil || string(actual) != "staged bytes" {
		t.Fatalf("staged object = %q, err = %v", actual, err)
	}
}
