package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/errorrender"
	"ideaboard/api/internal/searchcatalog/application"
	"ideaboard/api/internal/searchcatalog/domain"

	"github.com/gin-gonic/gin"
)

type SearchHandler struct{ service *application.SearchService }

func NewSearchHandler(service *application.SearchService) *SearchHandler {
	return &SearchHandler{service: service}
}

func (h *SearchHandler) Search(c *gin.Context) {
	if h == nil || h.service == nil {
		errorrender.Render(c, http.StatusServiceUnavailable, string(contract.CodeSearchUnavailable), msgSearchUnavailable)
		return
	}
	limit := application.DefaultSearchLimit
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > application.MaxSearchLimit {
			errorrender.Render(c, http.StatusBadRequest, string(contract.CodeSearchLimitInvalid), msgSearchLimitInvalid)
			return
		}
		limit = parsed
	}
	result, err := h.service.Search(c.Request.Context(), c.Query("q"), c.Query("cursor"), limit)
	if err != nil {
		if errors.Is(err, application.ErrNoSearchProviders) {
			contract.RenderSuccess(c, domain.SearchResponse{Query: c.Query("q"), Results: []domain.Result{}, HasMore: false})
			return
		}
		msg := err.Error()
		if strings.Contains(msg, msgSearch) {
			errorrender.Render(c, http.StatusBadRequest, string(contract.CodeSearchQueryInvalid), fmt.Sprintf(msgSearchQueryInvalid, application.MaxSearchQueryRunes))
			return
		}
		if strings.Contains(msg, msgSearch2) {
			errorrender.Render(c, http.StatusBadRequest, string(contract.CodeSearchCursorInvalid), msgSearchCursorInvalid)
			return
		}
		contract.RenderError(c, http.StatusBadGateway, contract.CodeSearchUnavailable, msgSearchUnavailable2)
		return
	}
	contract.RenderSuccess(c, result)
}
