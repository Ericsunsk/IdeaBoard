package http

import (
	"fmt"
	"net/http"
	"strings"

	"ideaboard/api/internal/media/derivatives/domain"
	"ideaboard/api/internal/media/derivatives/infrastructure/preview"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/errorrender"
	platformhttp "ideaboard/api/internal/platform/http"
	"ideaboard/api/internal/searchcatalog/application/search"
	searchdomain "ideaboard/api/internal/searchcatalog/domain"

	"github.com/gin-gonic/gin"
)

type SearchHandler struct {
	service  *search.SearchService
	previews *preview.Service
}

func NewSearchHandler(service *search.SearchService, previews *preview.Service) *SearchHandler {
	return &SearchHandler{service: service, previews: previews}
}

func (h *SearchHandler) Search(c *gin.Context) {
	if !platformhttp.RequireDependencyCode(c, h != nil && h.service != nil, string(contract.CodeSearchUnavailable), errorrender.MsgSearchUnavailable) {
		return
	}
	limit := search.DefaultSearchLimit
	if raw := strings.TrimSpace(c.Query(contract.LimitParam)); raw != "" {
		parsed, err := contract.ParseSearchLimit(raw)
		if err != nil {
			errorrender.Render(c, http.StatusBadRequest, string(contract.CodeSearchLimitInvalid), errorrender.MsgSearchLimitInvalid)
			return
		}
		limit = parsed
	}
	result, err := h.service.Search(c.Request.Context(), c.Query("q"), c.Query("cursor"), limit)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, errorrender.MsgSearch) {
			errorrender.Render(c, http.StatusBadRequest, string(contract.CodeSearchQueryInvalid), fmt.Sprintf(errorrender.MsgSearchQueryInvalid, search.MaxSearchQueryRunes))
			return
		}
		if strings.Contains(msg, errorrender.MsgSearch2) {
			errorrender.Render(c, http.StatusBadRequest, string(contract.CodeSearchCursorInvalid), errorrender.MsgSearchCursorInvalid)
			return
		}
		contract.RenderError(c, http.StatusBadGateway, contract.CodeSearchUnavailable, errorrender.MsgSearchUnavailable2)
		return
	}
	type displayResult struct {
		searchdomain.Result
		Display *domain.Display `json:"display,omitempty"`
	}
	results := make([]displayResult, 0, len(result.Results))
	for _, item := range result.Results {
		results = append(results, displayResult{Result: item, Display: h.previews.SearchDisplay(string(item.Provider), item.ID, item.PreviewURL)})
	}
	contract.RenderSuccess(c, struct {
		Query      string          `json:"query"`
		Results    []displayResult `json:"results"`
		NextCursor string          `json:"next_cursor,omitempty"`
		HasMore    bool            `json:"has_more"`
	}{result.Query, results, result.NextCursor, result.HasMore})
}
