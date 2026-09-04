package domain_test

import (
	"testing"

	"ideaboard/api/internal/identity/users/domain"
)

func TestStorageUIDGenerationAndValidation(t *testing.T) {
	for i := 0; i < 100; i++ {
		uid, err := domain.NewStorageUID()
		if err != nil {
			t.Fatalf("NewStorageUID() error = %v", err)
		}
		if !domain.IsValidStorageUID(uid) {
			t.Fatalf("IsValidStorageUID(%q) = false, want true", uid)
		}
	}

	invalidUIDs := []string{
		"",
		"12345",
		"1234567",
		"abc123",
		"ABC 12",
		"ABC-12",
	}
	for _, bad := range invalidUIDs {
		if domain.IsValidStorageUID(bad) {
			t.Errorf("IsValidStorageUID(%q) = true, want false", bad)
		}
	}
}
