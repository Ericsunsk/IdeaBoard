package searchcatalog

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type SearchHandler struct{ service *SearchService }

func NewSearchHandler(service *SearchService) *SearchHandler { return &SearchHandler{service: service} }

func (h *SearchHandler) Search(c *gin.Context) {
	if _, ok := principal.Require(c); !ok {
		return
	}
	limit := DefaultSearchLimit
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > MaxSearchLimit {
			response.Error(c, http.StatusBadRequest, "limit 必须在 1 到 48 之间")
			return
		}
		limit = parsed
	}
	result, err := h.service.Search(c.Request.Context(), c.Query("q"), c.Query("cursor"), limit, SearchRefinements{Palette: c.QueryArray("palette"), AtmosphereTags: c.QueryArray("atmosphere_tag")})
	if err != nil {
		if errors.Is(err, ErrNoSearchProviders) {
			response.Error(c, http.StatusServiceUnavailable, "暂无可用的搜索供应商")
			return
		}
		if strings.Contains(err.Error(), "搜索关键词") || strings.Contains(err.Error(), "分页游标") {
			response.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		response.InternalError(c, http.StatusBadGateway, "搜索服务暂时不可用", err)
		return
	}
	response.SuccessDTO(c, result)
}
