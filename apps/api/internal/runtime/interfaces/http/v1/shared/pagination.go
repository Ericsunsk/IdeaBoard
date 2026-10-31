package shared

import (
	"net/http"
	"strings"

	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"

	"github.com/gin-gonic/gin"
)

type ListCursor struct {
	Offset int `json:"offset"`
}

// PageSize parses the page_size query param for a v1 list endpoint.
// ok=false means the 400 response is already rendered and the context
// aborted; callers return immediately.
func PageSize(c *gin.Context) (pageSize int, ok bool) {
	pageSize, err := contract.PageSizeFromGin(c)
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest31)
		c.Abort()
		return 0, false
	}
	return pageSize, true
}

// OffsetCursor decodes the opaque offset cursor for a v1 list endpoint.
// Empty means first page (offset 0). Malformed cursors and negative offsets
// converge to the frozen pagination_cursor_invalid 400; ok=false means the
// response is already rendered and the context aborted.
func OffsetCursor(c *gin.Context) (offset int, ok bool) {
	raw := strings.TrimSpace(c.Query(contract.CursorParam))
	if raw == "" {
		return 0, true
	}
	cursor, decodeErr := contract.DecodeCursor[ListCursor](raw)
	if decodeErr != nil || cursor == nil || cursor.Offset < 0 {
		contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, runtimehttp.MsgPaginationCursorInvalid3)
		c.Abort()
		return 0, false
	}
	return cursor.Offset, true
}

// OffsetMeta builds the frozen {next_cursor, has_more} meta for a v1
// offset page. hasMore=false renders the exhausted shape ("", false);
// otherwise the next offset is encoded opaquely. encodeFailMsg preserves
// the caller's frozen 503 message; ok=false means the 503 is rendered.
func OffsetMeta(c *gin.Context, nextOffset int, hasMore bool, encodeFailMsg string) (contract.PageMeta, bool) {
	if !hasMore {
		return contract.NewPageMeta("", false), true
	}
	next, err := contract.EncodeCursor(ListCursor{Offset: nextOffset})
	if err != nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, encodeFailMsg)
		c.Abort()
		return contract.PageMeta{}, false
	}
	return contract.NewPageMeta(next, true), true
}

// OffsetPage slices an in-memory ordered list through the shared
// offset-cursor contract.
func OffsetPage[T any](c *gin.Context, items []T, encodeFailMsg string) ([]T, contract.PageMeta, bool) {
	pageSize, ok := PageSize(c)
	if !ok {
		return nil, contract.PageMeta{}, false
	}
	offset, ok := OffsetCursor(c)
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
	meta, ok := OffsetMeta(c, end, end < len(items), encodeFailMsg)
	if !ok {
		return nil, contract.PageMeta{}, false
	}
	return items[offset:end], meta, true
}
