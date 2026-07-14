package billingrecords

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type Reader interface {
	List(context.Context, ListInput) (Page, error)
}

type Handler struct {
	reader Reader
}

func NewHandler(reader Reader) *Handler {
	return &Handler{reader: reader}
}

func (h *Handler) List(c *gin.Context) {
	if h == nil || h.reader == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "账单服务暂不可用", nil)
		return
	}
	userID := c.GetInt64("user_id")
	if userID <= 0 {
		response.Error(c, http.StatusUnauthorized, "未登录")
		return
	}
	input, ok := parseListInput(c, userID)
	if !ok {
		return
	}
	page, err := h.reader.List(c.Request.Context(), input)
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, "加载账单记录失败", err)
		return
	}
	response.SuccessDTO(c, page)
}

func parseListInput(c *gin.Context, userID int64) (ListInput, bool) {
	input := ListInput{UserID: userID, Limit: DefaultPageLimit}
	if raw := strings.TrimSpace(c.Query("before_sequence")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value <= 0 {
			response.Error(c, http.StatusBadRequest, "before_sequence 无效")
			return ListInput{}, false
		}
		input.BeforeSequence = value
	}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > MaxPageLimit {
			response.Error(c, http.StatusBadRequest, "limit 无效")
			return ListInput{}, false
		}
		input.Limit = value
	}
	return input, true
}
