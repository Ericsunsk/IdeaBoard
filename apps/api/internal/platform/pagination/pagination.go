// Package pagination defines the shared opaque-cursor pagination contract for
// list APIs: a default page size, an upper bound, and strict base64url(JSON)
// cursor decoding.
//
// Rules:
//   - Cursors are opaque to callers. Domains validate their own payload fields
//     after decoding; this package only guarantees transport integrity
//     (length bound, canonical base64url alphabet, single JSON object).
//   - Limit bounds are wire-stable. Do not change DefaultLimit or MaxLimit
//     without coordinating the OpenAPI contract and the frontend callers.
//   - Keep the taxonomy flat: ErrInvalidLimit and ErrInvalidCursor are the
//     only outcomes. HTTP adapters map them to stable machine codes.
package pagination

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
)

const (
	// DefaultLimit is the page size when the caller omits limit.
	DefaultLimit = 50
	// MaxLimit bounds the number of rows returned by one page.
	MaxLimit = 200
	// MaxCursorLen bounds opaque cursor input before decoding.
	MaxCursorLen = 512
)

var (
	// ErrInvalidLimit reports an out-of-range or malformed limit.
	ErrInvalidLimit = errors.New("pagination limit invalid")
	// ErrInvalidCursor reports a malformed or oversized cursor.
	ErrInvalidCursor = errors.New("pagination cursor invalid")
)

// ParseLimit parses a raw query limit: empty means DefaultLimit, otherwise the
// value must be an integer in [1, MaxLimit].
func ParseLimit(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return DefaultLimit, nil
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil || value < 1 || value > MaxLimit {
		return 0, ErrInvalidLimit
	}
	return value, nil
}

// NormalizeLimit applies the same bounds to an already-parsed limit where 0
// means DefaultLimit. It keeps service-layer signatures stable for callers
// that do not carry the raw query string.
func NormalizeLimit(limit int) (int, error) {
	if limit == 0 {
		return DefaultLimit, nil
	}
	if limit < 1 || limit > MaxLimit {
		return 0, ErrInvalidLimit
	}
	return limit, nil
}

// EncodeCursor serializes a keyset position into an opaque cursor string.
func EncodeCursor[T any](value T) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// DecodeCursor parses an opaque cursor produced by EncodeCursor. An empty
// value means "first page" and returns (nil, nil). Unknown JSON fields,
// trailing payloads, and non-canonical base64url spellings are rejected so
// callers cannot smuggle semantics through the cursor.
func DecodeCursor[T any](value string) (*T, error) {
	if len(value) > MaxCursorLen {
		return nil, ErrInvalidCursor
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || base64.RawURLEncoding.EncodeToString(raw) != value {
		return nil, ErrInvalidCursor
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return nil, ErrInvalidCursor
	}
	var cursor T
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil {
		return nil, ErrInvalidCursor
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, ErrInvalidCursor
	}
	return &cursor, nil
}

// TruncateBeforeID applies the shared limit+1 truncation for id-ordered
// pages: callers query limit+1 rows, then converge to (page, next). When
// more rows exist, next is the idOf the last returned row (limit-1) and the
// page is truncated to limit; otherwise next is zero ("no more") and the
// page is returned whole. The off-by-one (limit-1, not limit) and the
// zero-means-exhausted semantics live here alone.
func TruncateBeforeID[T any, ID ~int64](items []T, limit int, idOf func(T) ID) ([]T, ID) {
	var zero ID
	if len(items) <= limit {
		return items, zero
	}
	next := idOf(items[limit-1])
	return items[:limit], next
}
