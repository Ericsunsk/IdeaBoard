package library

import (
	"testing"
	"time"
)

func TestCursorRoundTrip(t *testing.T) {
	want := Cursor{CreatedAt: time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC), ID: 42}
	raw, err := EncodeCursor(want)
	if err != nil {
		t.Fatalf("EncodeCursor returned error: %v", err)
	}
	got, err := DecodeCursor(raw)
	if err != nil {
		t.Fatalf("DecodeCursor returned error: %v", err)
	}
	if *got != want {
		t.Fatalf("decoded cursor = %#v, want %#v", *got, want)
	}
}

func TestDecodeCursorRejectsMalformedValues(t *testing.T) {
	for _, raw := range []string{"not-base64", "", "e30"} {
		_, err := DecodeCursor(raw)
		if raw == "" {
			if err != nil {
				t.Fatalf("empty cursor error = %v, want nil", err)
			}
			continue
		}
		if err != ErrInvalidCursor {
			t.Fatalf("DecodeCursor(%q) error = %v, want %v", raw, err, ErrInvalidCursor)
		}
	}
}

func TestEncodeCursorRejectsIncompleteValues(t *testing.T) {
	for _, cursor := range []Cursor{{}, {ID: 1}, {CreatedAt: time.Now()}} {
		if _, err := EncodeCursor(cursor); err != ErrInvalidCursor {
			t.Fatalf("EncodeCursor(%#v) error = %v, want %v", cursor, err, ErrInvalidCursor)
		}
	}
}
