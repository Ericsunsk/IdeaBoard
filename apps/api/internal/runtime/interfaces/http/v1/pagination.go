package v1

// Single gin helper for v1 list page-size parsing (P0-5 convergence).
//
// All v1 list endpoints expose only cursor + page_size from
// platform/contract (default 20, max 100). This helper is the only
// call-site shape: it parses page_size, renders the frozen 400
// BadRequest outcome, and aborts. Cursor decoding already converges on
// contract.DecodeCursor at each site; only the size block needed a seam.

import (
	"net/http"
	"strings"

	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"

	"github.com/gin-gonic/gin"
)

// v1PageSize parses the page_size query param for a v1 list endpoint.
// ok=false means the 400 response is already rendered and the context
// aborted; callers return immediately.
func v1PageSize(c *gin.Context) (pageSize int, ok bool) {
	pageSize, err := contract.PageSizeFromGin(c)
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest31)
		c.Abort()
		return 0, false
	}
	return pageSize, true
}

// v1OffsetCursor decodes the opaque offset cursor for a v1 list endpoint.
// Empty means first page (offset 0). Malformed cursors and negative offsets
// converge to the frozen pagination_cursor_invalid 400; ok=false means the
// response is already rendered and the context aborted.
func v1OffsetCursor(c *gin.Context) (offset int, ok bool) {
	raw := strings.TrimSpace(c.Query(contract.CursorParam))
	if raw == "" {
		return 0, true
	}
	cursor, decodeErr := contract.DecodeCursor[v1ListCursor](raw)
	if decodeErr != nil || cursor == nil || cursor.Offset < 0 {
		contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, runtimehttp.MsgPaginationCursorInvalid3)
		c.Abort()
		return 0, false
	}
	return cursor.Offset, true
}

// v1OffsetMeta builds the frozen {next_cursor, has_more} meta for a v1
// offset page. hasMore=false renders the exhausted shape ("", false);
// otherwise the next offset is encoded opaquely. encodeFailMsg preserves
// the caller's frozen 503 message; ok=false means the 503 is rendered.
func v1OffsetMeta(c *gin.Context, nextOffset int, hasMore bool, encodeFailMsg string) (contract.PageMeta, bool) {
	if !hasMore {
		return contract.NewPageMeta("", false), true
	}
	next, err := contract.EncodeCursor(v1ListCursor{Offset: nextOffset})
	if err != nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, encodeFailMsg)
		c.Abort()
		return contract.PageMeta{}, false
	}
	return contract.NewPageMeta(next, true), true
}

// v1OffsetPage slices an in-memory ordered list through the shared
// offset-cursor contract: page_size + opaque cursor decode, over-range
// offsets converge to pagination_cursor_invalid, and the exhausted page
// renders ("", false). encodeFailMsg preserves the caller's frozen 503
// message for the (unreachable in practice) cursor-encode failure.
func v1OffsetPage[T any](c *gin.Context, items []T, encodeFailMsg string) ([]T, contract.PageMeta, bool) {
	pageSize, ok := v1PageSize(c)
	if !ok {
		return nil, contract.PageMeta{}, false
	}
	offset, ok := v1OffsetCursor(c)
	if !ok {
		return nil, contract.PageMeta{}, false
	}
	if offset > len(items) {
		contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, runtimehttp.MsgPaginationCursorInvalid3)
		c.Abort()
		return nil, contract.PageMeta{}, false
	}
	end := offset + pageSize
	if end > len(items) {
		end = len(items)
	}
	meta, ok := v1OffsetMeta(c, end, end < len(items), encodeFailMsg)
	if !ok {
		return nil, contract.PageMeta{}, false
	}
	return items[offset:end], meta, true
}
