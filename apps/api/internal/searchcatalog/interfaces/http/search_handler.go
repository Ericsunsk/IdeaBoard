package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"ideaboard/api/internal/media/derivatives/domain"
	"ideaboard/api/internal/media/derivatives/infrastructure/preview"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/errorrender"
	"ideaboard/api/internal/searchcatalog/application"
	searchdomain "ideaboard/api/internal/searchcatalog/domain"

	"github.com/gin-gonic/gin"
)

type SearchHandler struct {
	service  *application.SearchService
	previews *preview.Service
}

func NewSearchHandler(service *application.SearchService, previews *preview.Service) *SearchHandler {
	return &SearchHandler{service: service, previews: previews}
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
