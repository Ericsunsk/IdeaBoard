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
