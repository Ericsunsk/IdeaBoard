package cursor_test

import (
	"errors"
	"testing"

	"ideaboard/api/internal/media/workspacemedia/cursor"
	"ideaboard/api/internal/platform/pagination"
)

func TestAssetCursorRoundTrip(t *testing.T) {
	raw, err := cursor.EncodeAssetCursor(25)
	if err != nil {
		t.Fatalf("EncodeAssetCursor: %v", err)
	}
	if raw == "" || raw == "25" {
		t.Fatalf("cursor must be opaque, got %q", raw)
	}
	offset, err := cursor.DecodeAssetCursor(raw)
	if err != nil || offset != 25 {
		t.Fatalf("DecodeAssetCursor = %d, %v; want 25, nil", offset, err)
	}
	if offset, err := cursor.DecodeAssetCursor(""); err != nil || offset != 0 {
		t.Fatalf("DecodeAssetCursor(\"\") = %d, %v; want 0, nil", offset, err)
	}
	for _, bad := range []string{"not-a-cursor", "25", "!!!"} {
		if _, err := cursor.DecodeAssetCursor(bad); err == nil {
			t.Fatalf("DecodeAssetCursor(%q) = nil error, want cursor error", bad)
		}
	}
	// A well-formed cursor with a negative offset must fail closed as a
	// cursor error, never as a raw-offset passthrough.
	neg, err := cursor.EncodeAssetCursor(-1)
	if err != nil {
		t.Fatalf("EncodeAssetCursor(-1): %v", err)
	}
	if _, err := cursor.DecodeAssetCursor(neg); err == nil {
		t.Fatal("DecodeAssetCursor(negative) = nil error, want cursor error")
	} else if !isCursorInvalid(err) {
		t.Fatalf("DecodeAssetCursor(negative) error = %v, want cursor taxonomy", err)
	}
}

func isCursorInvalid(err error) bool {
	return errors.Is(err, pagination.ErrInvalidCursor)
}
