package contract

import (
	"errors"
	"strconv"
	"strings"

	"ideaboard/api/internal/platform/pagination"

	"github.com/gin-gonic/gin"
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

// Legacy limit/before_id dialects (P0-5 convergence).
//
// The Public API exposes only cursor + page_size (parsed by ParsePageSize
// above). The rows below are the single home for the pre-contract dialects
// that operator/edge surfaces still carry on the wire:
//
//   - limit (control plane + legacy edges): 50/200, pagination.ParseLimit
//     bounds, pagination.ErrInvalidLimit taxonomy.
//   - search limit (searchcatalog edge): 24/48 strict validation.
//   - reward limit (rewards edges): 20/100 strict validation.
//   - before_id (id-ordered stores: reconciliation, reward redemptions):
//     positive int64, absent means first page.
//
// New endpoints must use cursor + page_size; these helpers exist so the
// dialects parse in exactly one place instead of per-handler Atoi blocks.
const (
	// LimitParam is the legacy page-size query param on control/edge lists.
	LimitParam = "limit"
	// BeforeIDParam is the store-ordered cursor query param on id-ordered lists.
	BeforeIDParam = "before_id"
	// DefaultLimit mirrors pagination.DefaultLimit (control-plane default).
	DefaultLimit = pagination.DefaultLimit
	// MaxLimit mirrors pagination.MaxLimit (control-plane bound).
	MaxLimit = pagination.MaxLimit
	// DefaultSearchLimit is the search edge default page size.
	DefaultSearchLimit = 24
	// MaxSearchLimit is the search edge page-size bound.
	MaxSearchLimit = 48
	// DefaultRewardLimit is the rewards edge default page size.
	DefaultRewardLimit = 20
	// MaxRewardLimit is the rewards edge page-size bound.
	MaxRewardLimit = 100
)

// ErrInvalidBeforeID reports a malformed or non-positive before_id.
var ErrInvalidBeforeID = errors.New("contract before_id invalid")

// ErrInvalidSearchLimit reports an out-of-range or malformed search limit.
var ErrInvalidSearchLimit = errors.New("contract search limit invalid")

// ErrInvalidRewardLimit reports an out-of-range or malformed reward limit.
var ErrInvalidRewardLimit = errors.New("contract reward limit invalid")

// ParseLimit parses a raw legacy limit value: empty means DefaultLimit,
// otherwise the value must be an integer in [1, MaxLimit].
func ParseLimit(raw string) (int, error) {
	return pagination.ParseLimit(raw)
}

// NormalizeLimit applies the legacy limit bounds to an already-parsed
// value where 0 means DefaultLimit.
func NormalizeLimit(limit int) (int, error) {
	return pagination.NormalizeLimit(limit)
}

// ParseSearchLimit parses a raw search limit: empty means
// DefaultSearchLimit, otherwise the value must be an integer in
// [1, MaxSearchLimit].
func ParseSearchLimit(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return DefaultSearchLimit, nil
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil || value < 1 || value > MaxSearchLimit {
		return 0, ErrInvalidSearchLimit
	}
	return value, nil
}

// ParseRewardLimit parses a raw rewards limit: empty means
// DefaultRewardLimit, otherwise the value must be an integer in
// [1, MaxRewardLimit].
func ParseRewardLimit(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return DefaultRewardLimit, nil
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil || value < 1 || value > MaxRewardLimit {
		return 0, ErrInvalidRewardLimit
	}
	return value, nil
}

// NormalizeRewardLimit applies the rewards limit bounds to an
// already-parsed value (e.g. JSON body fields) where 0 means
// DefaultRewardLimit.
func NormalizeRewardLimit(limit int) (int, error) {
	if limit == 0 {
		return DefaultRewardLimit, nil
	}
	if limit < 1 || limit > MaxRewardLimit {
		return 0, ErrInvalidRewardLimit
	}
	return limit, nil
}

// ParseBeforeID parses a raw store-ordered cursor: empty means first page
// (0), otherwise the value must be a positive int64.
func ParseBeforeID(raw string) (int64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || value <= 0 {
		return 0, ErrInvalidBeforeID
	}
	return value, nil
}

// PageSizeFromGin reads the page_size query param through ParsePageSize.
// It only parses; rendering stays at the call site so each surface keeps
// its frozen error code/message.
func PageSizeFromGin(c *gin.Context) (int, error) {
	if c == nil {
		return 0, ErrInvalidPageSize
	}
	return ParsePageSize(c.Query(PageSizeParam))
}

// LimitFromGin reads the legacy limit query param through ParseLimit.
// Absent means DefaultLimit, matching the control-plane edge contract.
func LimitFromGin(c *gin.Context) (int, error) {
	if c == nil {
		return DefaultLimit, nil
	}
	return ParseLimit(strings.TrimSpace(c.Query(LimitParam)))
}

// SearchLimitFromGin reads the search limit query param through
// ParseSearchLimit.
func SearchLimitFromGin(c *gin.Context) (int, error) {
	if c == nil {
		return DefaultSearchLimit, nil
	}
	return ParseSearchLimit(c.Query(LimitParam))
}

// RewardLimitFromGin reads the rewards limit query param through
// ParseRewardLimit.
func RewardLimitFromGin(c *gin.Context) (int, error) {
	if c == nil {
		return DefaultRewardLimit, nil
	}
	return ParseRewardLimit(c.Query(LimitParam))
}

// BeforeIDFromGin reads the store-ordered before_id query param through
// ParseBeforeID. Absent means first page (0).
func BeforeIDFromGin(c *gin.Context) (int64, error) {
	if c == nil {
		return 0, nil
	}
	return ParseBeforeID(c.Query(BeforeIDParam))
}
