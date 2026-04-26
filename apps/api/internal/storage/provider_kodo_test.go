package storage

import "testing"

func TestNormalizeKodoDeletePrefix(t *testing.T) {
	t.Run("rejects empty prefix", func(t *testing.T) {
		if _, err := normalizeKodoDeletePrefix("   "); err == nil {
			t.Fatal("expected empty prefix to be rejected")
		}
	})

	t.Run("trims and accepts prefix", func(t *testing.T) {
		got, err := normalizeKodoDeletePrefix(" uploads/workspace/U12345/ ")
		if err != nil {
			t.Fatalf("normalizeKodoDeletePrefix returned error: %v", err)
		}
		want := "uploads/workspace/U12345/"
		if got != want {
			t.Fatalf("prefix = %q, want %q", got, want)
		}
	})
}
