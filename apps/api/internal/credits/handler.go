package credits

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type AdminService interface {
	Grant(context.Context, GrantCommand) (CommandResult, error)
	GetWallet(context.Context, int64) (WalletBalance, error)
	ListLedger(context.Context, ListLedgerInput) (LedgerPage, error)
}

type Handler struct {
	service AdminService
}

type adminGrantRequest struct {
	UserID              int64  `json:"user_id" binding:"required"`
	AmountCredits       int64  `json:"amount_credits" binding:"required"`
	IdempotencyKey      string `json:"idempotency_key" binding:"required"`
	Reason              string `json:"reason" binding:"required"`
	BusinessReferenceID string `json:"business_reference_id" binding:"required"`
}

func NewHandler(service AdminService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Grant(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "点子服务暂不可用", nil)
		return
	}
	adminID := c.GetInt64("user_id")
	if adminID <= 0 {
		response.Error(c, http.StatusUnauthorized, "未登录")
		return
	}
	var request adminGrantRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	amount, err := NewCreditAmount(request.AmountCredits)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "点子数量必须是有效正整数")
		return
	}
	result, err := h.service.Grant(c.Request.Context(), GrantCommand{
		UserID: request.UserID,
		Amount: amount,
		Metadata: CommandMetadata{
			IdempotencyKey: request.IdempotencyKey,
			Actor: Actor{
				Type: ActorAdmin,
				ID:   strconv.FormatInt(adminID, 10),
			},
			Source: "admin_grant",
			Reason: request.Reason,
			BusinessReference: BusinessReference{
				Type: "admin_adjustment",
				ID:   request.BusinessReferenceID,
			},
		},
	})
	if err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, result)
}

func (h *Handler) GetWallet(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "点子服务暂不可用", nil)
		return
	}
	userID, ok := parsePositiveInt64(c.Param("user_id"))
	if !ok {
		response.Error(c, http.StatusBadRequest, "用户 ID 无效")
		return
	}
	wallet, err := h.service.GetWallet(c.Request.Context(), userID)
	if err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, wallet)
}

func (h *Handler) ListLedger(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "点子服务暂不可用", nil)
		return
	}
	userID, ok := parsePositiveInt64(c.Param("user_id"))
	if !ok {
		response.Error(c, http.StatusBadRequest, "用户 ID 无效")
		return
	}
	limit := 0
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 {
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
	page, err := h.service.ListLedger(c.Request.Context(), ListLedgerInput{
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

func parsePositiveInt64(value string) (int64, bool) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed, err == nil && parsed > 0
}

func writeCreditError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidCommand):
		response.Error(c, http.StatusBadRequest, "参数错误")
	case errors.Is(err, ErrIdempotencyConflict):
		response.Error(c, http.StatusConflict, "幂等键已用于不同的点子操作")
	case errors.Is(err, ErrWalletNotFound):
		response.Error(c, http.StatusNotFound, "点子钱包不存在")
	case errors.Is(err, ErrWalletFrozen):
		response.Error(c, http.StatusLocked, "点子钱包已冻结")
	case errors.Is(err, ErrInsufficientCredits):
		response.Error(c, http.StatusUnprocessableEntity, "点子不足")
	default:
		response.InternalError(c, http.StatusInternalServerError, "点子操作失败", err)
	}
}
