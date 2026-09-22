package pagination

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

type testCursor struct {
	OccurredAt string `json:"occurred_at"`
	OrderID    string `json:"order_id"`
}

func TestParseLimitDefaultsAndBounds(t *testing.T) {
	if got, err := ParseLimit(""); err != nil || got != DefaultLimit {
		t.Fatalf("ParseLimit(\"\") = %d, %v, want %d, nil", got, err, DefaultLimit)
	}
	if got, err := ParseLimit("  "); err != nil || got != DefaultLimit {
		t.Fatalf("ParseLimit(blank) = %d, %v, want %d, nil", got, err, DefaultLimit)
	}
	if got, err := ParseLimit("20"); err != nil || got != 20 {
		t.Fatalf("ParseLimit(20) = %d, %v, want 20, nil", got, err)
	}
	if got, err := ParseLimit("200"); err != nil || got != MaxLimit {
		t.Fatalf("ParseLimit(200) = %d, %v, want %d, nil", got, err, MaxLimit)
	}
	for _, raw := range []string{"0", "-1", "201", "1000", "x", "1.5", "20,20"} {
		if _, err := ParseLimit(raw); !errors.Is(err, ErrInvalidLimit) {
			t.Fatalf("ParseLimit(%q) error = %v, want ErrInvalidLimit", raw, err)
		}
	}
}

func TestNormalizeLimitDefaultsAndBounds(t *testing.T) {
	if got, err := NormalizeLimit(0); err != nil || got != DefaultLimit {
		t.Fatalf("NormalizeLimit(0) = %d, %v, want %d, nil", got, err, DefaultLimit)
	}
	if got, err := NormalizeLimit(10); err != nil || got != 10 {
		t.Fatalf("NormalizeLimit(10) = %d, %v, want 10, nil", got, err)
	}
	for _, limit := range []int{-1, 201} {
		if _, err := NormalizeLimit(limit); !errors.Is(err, ErrInvalidLimit) {
			t.Fatalf("NormalizeLimit(%d) error = %v, want ErrInvalidLimit", limit, err)
		}
	}
}

func TestCursorRoundTrip(t *testing.T) {
	occurredAt := time.Date(2026, 8, 4, 12, 0, 0, 123456789, time.UTC).Format(time.RFC3339Nano)
	encoded, err := EncodeCursor(testCursor{OccurredAt: occurredAt, OrderID: "0198cd29-f457-7e4b-b95e-6cfde41b8333"})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCursor[testCursor](encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded == nil || decoded.OccurredAt != occurredAt || decoded.OrderID != "0198cd29-f457-7e4b-b95e-6cfde41b8333" {
		t.Fatalf("decoded cursor = %+v", decoded)
	}
}

func TestDecodeCursorRejectsInvalidInput(t *testing.T) {
	for _, value := range []string{
		"bad",
		strings.Repeat("a", MaxCursorLen+1),
		// Valid base64url of a JSON object with an unknown field.
		mustEncodeBase64(t, `{"occurred_at":"2026-08-04T12:00:00Z","order_id":"x","unknown":1}`),
		// Two concatenated JSON objects.
		mustEncodeBase64(t, `{"occurred_at":"2026-08-04T12:00:00Z","order_id":"x"} {}`),
		// Not a JSON object.
		mustEncodeBase64(t, `[1,2]`),
		mustEncodeBase64(t, `null`),
	} {
		if _, err := DecodeCursor[testCursor](value); !errors.Is(err, ErrInvalidCursor) {
			t.Fatalf("DecodeCursor(%q) error = %v, want ErrInvalidCursor", value, err)
		}
	}
}

func mustEncodeBase64(t *testing.T, raw string) string {
	t.Helper()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func TestDecodeCursorEmptyMeansFirstPage(t *testing.T) {
	for _, value := range []string{"", "  "} {
		decoded, err := DecodeCursor[testCursor](value)
		if err != nil || decoded != nil {
			t.Fatalf("DecodeCursor(%q) = %+v, %v, want nil, nil", value, decoded, err)
		}
	}
}
