package contract

import (
	"errors"
	"strconv"
	"strings"

	"ideaboard/api/internal/platform/pagination"
)

// Contract pagination freezes the Baseline 5.5 wire bounds: page_size
// default 20, max 100, opaque cursor, next_cursor/has_more meta. It is a
// thin wrapper over platform/pagination (transport integrity: length
// bound, canonical base64url, single JSON object, unknown-field
// rejection) with the kernel's own size bounds.
//
// DEPRECATED dialects (comment only; the legacy package is untouched):
// offset `page`/`page_number` params, `before_id`/`before_seq` cursors,
// and the legacy `limit` param are not part of the Public API. List
// endpoints expose only `cursor` + `page_size`.
const (
	// DefaultPageSize is the page size when page_size is omitted.
	DefaultPageSize = 20
	// MaxPageSize bounds the number of rows returned by one page.
	MaxPageSize = 100
	// PageSizeParam is the only page-size query param in the Public API.
	PageSizeParam = "page_size"
	// CursorParam is the only cursor query param in the Public API.
	CursorParam = "cursor"
)

// ErrInvalidPageSize reports an out-of-range or malformed page_size.
// Adapters render it as 400 (CodeBadRequest); only cursor failures use
// CodePaginationCursorInvalid.
var ErrInvalidPageSize = errors.New("contract page_size invalid")

// ParsePageSize parses a raw page_size query value: empty means
// DefaultPageSize, otherwise the value must be an integer in
// [1, MaxPageSize]. It intentionally does not inherit
// platform/pagination DefaultLimit/MaxLimit (50/200, legacy wire).
func ParsePageSize(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return DefaultPageSize, nil
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil || value < 1 || value > MaxPageSize {
		return 0, ErrInvalidPageSize
	}
	return value, nil
}

// EncodeCursor serializes a keyset position into an opaque cursor string.
func EncodeCursor[T any](value T) (string, error) {
	return pagination.EncodeCursor(value)
}

// DecodeCursor parses an opaque cursor produced by EncodeCursor. Empty
// means "first page" and returns (nil, nil). Malformed cursors surface
// pagination.ErrInvalidCursor; use IsCursorInvalid to classify them into
// CodePaginationCursorInvalid.
func DecodeCursor[T any](value string) (*T, error) {
	return pagination.DecodeCursor[T](value)
}

// IsCursorInvalid reports whether err is a malformed/oversized cursor
// (vs a domain payload validation failure after decoding).
func IsCursorInvalid(err error) bool {
	return errors.Is(err, pagination.ErrInvalidCursor)
}

// NewPageMeta builds the required list meta: stable ordering is the
// caller's responsibility; cursors stay bound to principal, filters, and
// sort order (Baseline 5.5).
func NewPageMeta(nextCursor string, hasMore bool) PageMeta {
	return PageMeta{NextCursor: nextCursor, HasMore: hasMore}
}
