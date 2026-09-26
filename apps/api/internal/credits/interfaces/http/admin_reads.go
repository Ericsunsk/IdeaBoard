package http

import (
	"net/http"
	"strconv"
	"strings"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type AdminReadHandler struct {
	service      AdminReadService
	userResolver AdminReadIdentityResolver
}

func NewAdminReadHandler(service AdminReadService, resolver AdminReadIdentityResolver) *AdminReadHandler {
	return &AdminReadHandler{service: service, userResolver: resolver}
}

func (h *AdminReadHandler) GetWallet(c *gin.Context) {
	if _, ok := requireAdmin(c, h != nil && h.service != nil); !ok {
		return
	}
	user, ok := resolveUser(c, h.userResolver, c.Param("public_id"))
	if !ok {
		return
	}
	wallet, err := h.service.GetWallet(c.Request.Context(), user.ID)
	if err != nil {
		writeCreditError(c, err)
		return
	}
	if err := enrichWallet(c.Request.Context(), h.userResolver, &wallet); err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, wallet)
}

func (h *AdminReadHandler) ListLedger(c *gin.Context) {
	if _, ok := requireAdmin(c, h != nil && h.service != nil); !ok {
		return
	}
	user, ok := resolveUser(c, h.userResolver, c.Param("public_id"))
	if !ok {
		return
	}
	h.listLedger(c, user.ID)
}

func (h *AdminReadHandler) listLedger(c *gin.Context, userID int64) {
	limit := 0
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 || parsed > 200 {
			response.Error(c, http.StatusBadRequest, "limit 无效")
			return
		}
		limit = parsed
	}
	beforeSequence := int64(0)
	if rawBefore := strings.TrimSpace(c.Query("before_sequence")); rawBefore != "" {
		parsed, ok := parsePositiveInt64(rawBefore)
		if !ok {
			response.Error(c, http.StatusBadRequest, "before_sequence 无效")
			return
		}
		beforeSequence = parsed
	}
	page, err := h.service.ListLedger(c.Request.Context(), domain.ListLedgerInput{
		UserID:         userID,
		BeforeSequence: beforeSequence,
		Limit:          limit,
	})
	if err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, page)
}
