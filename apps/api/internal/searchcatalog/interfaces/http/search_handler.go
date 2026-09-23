package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"ideaboard/api/internal/platform/response"
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
		response.MachineError(c, http.StatusServiceUnavailable, "search_unavailable", "搜索服务暂不可用")
		return
	}
	limit := application.DefaultSearchLimit
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > application.MaxSearchLimit {
			response.MachineError(c, http.StatusBadRequest, "search_limit_invalid", "limit 必须在 1 到 48 之间")
			return
		}
		limit = parsed
	}
	result, err := h.service.Search(c.Request.Context(), c.Query("q"), c.Query("cursor"), limit)
	if err != nil {
		if errors.Is(err, application.ErrNoSearchProviders) {
			response.SuccessDTO(c, domain.SearchResponse{Query: c.Query("q"), Results: []domain.Result{}, HasMore: false})
			return
		}
		msg := err.Error()
		if strings.Contains(msg, "搜索关键词") {
			response.MachineError(c, http.StatusBadRequest, "search_query_invalid", fmt.Sprintf("搜索关键词不能超过 %d 个字符", application.MaxSearchQueryRunes))
			return
		}
		if strings.Contains(msg, "分页游标") {
			response.MachineError(c, http.StatusBadRequest, "search_cursor_invalid", "分页游标无效")
			return
		}
		response.InternalError(c, http.StatusBadGateway, "搜索服务暂时不可用", err)
		return
	}
	response.SuccessDTO(c, result)
}
