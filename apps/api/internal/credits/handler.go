package credits

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type AdminService interface {
	Grant(context.Context, GrantCommand) (CommandResult, error)
	Deduct(context.Context, DeductionCommand) (CommandResult, error)
	Correct(context.Context, CorrectionCommand) (CommandResult, error)
	Refund(context.Context, RefundCommand) (CommandResult, error)
	GetWallet(context.Context, int64) (WalletDetails, error)
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

type adminDeductionRequest adminGrantRequest

type adminRefundRequest struct {
	UserID              int64  `json:"user_id"`
	ChargeTransactionID string `json:"charge_transaction_id"`
	AmountCredits       int64  `json:"amount_credits"`
	IdempotencyKey      string `json:"idempotency_key"`
	Reason              string `json:"reason"`
	BusinessReferenceID string `json:"business_reference_id"`
}

type adminCorrectionRequest struct {
	UserID                int64  `json:"user_id"`
	OriginalTransactionID string `json:"original_transaction_id"`
	AmountCredits         int64  `json:"amount_credits"`
	IdempotencyKey        string `json:"idempotency_key"`
	Reason                string `json:"reason"`
	BusinessReferenceID   string `json:"business_reference_id"`
}

const maxAdminCreditRequestBytes = 16 << 10

func NewHandler(service AdminService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Grant(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "点子服务暂不可用", nil)
		return
	}
	adminID, ok := authenticatedAdminID(c)
	if !ok {
		return
	}
	var request adminGrantRequest
	if err := decodeAdminCreditRequest(c, &request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	amount, err := NewCreditAmount(request.AmountCredits)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "点子数量必须是有效正整数")
		return
	}
	result, err := h.service.Grant(c.Request.Context(), GrantCommand{
		UserID:   request.UserID,
		Amount:   amount,
		Metadata: adminCommandMetadata(adminID, request.IdempotencyKey, "admin_grant", request.Reason, request.BusinessReferenceID),
	})
	if err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, result)
}

func (h *Handler) Deduct(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "点子服务暂不可用", nil)
		return
	}
	adminID, ok := authenticatedAdminID(c)
	if !ok {
		return
	}
	var request adminDeductionRequest
	if err := decodeAdminCreditRequest(c, &request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	amount, err := NewCreditAmount(request.AmountCredits)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "点子数量必须是有效正整数")
		return
	}
	result, err := h.service.Deduct(c.Request.Context(), DeductionCommand{
		UserID: request.UserID, Amount: amount,
		Metadata: adminCommandMetadata(adminID, request.IdempotencyKey, "admin_deduction", request.Reason, request.BusinessReferenceID),
	})
	if err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, result)
}

func (h *Handler) Refund(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "点子服务暂不可用", nil)
		return
	}
	adminID, ok := authenticatedAdminID(c)
	if !ok {
		return
	}
	var request adminRefundRequest
	if err := decodeAdminCreditRequest(c, &request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	amount, err := NewCreditAmount(request.AmountCredits)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "点子数量必须是有效正整数")
		return
	}
	result, err := h.service.Refund(c.Request.Context(), RefundCommand{
		UserID: request.UserID, ChargeTransactionID: request.ChargeTransactionID, Amount: amount,
		Metadata: adminCommandMetadata(adminID, request.IdempotencyKey, "admin_refund", request.Reason, request.BusinessReferenceID),
	})
	if err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, result)
}

func (h *Handler) Correct(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "点子服务暂不可用", nil)
		return
	}
	adminID, ok := authenticatedAdminID(c)
	if !ok {
		return
	}
	var request adminCorrectionRequest
	if err := decodeAdminCreditRequest(c, &request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	amount, err := NewCreditAmount(request.AmountCredits)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "点子数量必须是有效正整数")
		return
	}
	result, err := h.service.Correct(c.Request.Context(), CorrectionCommand{
		UserID: request.UserID, OriginalTransactionID: request.OriginalTransactionID, Amount: amount,
		Metadata: adminCommandMetadata(adminID, request.IdempotencyKey, "admin_correction", request.Reason, request.BusinessReferenceID),
	})
	if err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, result)
}

func authenticatedAdminID(c *gin.Context) (int64, bool) {
	adminID := c.GetInt64("user_id")
	if adminID <= 0 {
		response.Error(c, http.StatusUnauthorized, "未登录")
		return 0, false
	}
	return adminID, true
}

func adminCommandMetadata(adminID int64, idempotencyKey, source, reason, businessReferenceID string) CommandMetadata {
	return CommandMetadata{
		IdempotencyKey: idempotencyKey,
		Actor:          Actor{Type: ActorAdmin, ID: strconv.FormatInt(adminID, 10)},
		Source:         source, Reason: reason,
		BusinessReference: BusinessReference{Type: "admin_adjustment", ID: businessReferenceID},
	}
}

func decodeAdminCreditRequest(c *gin.Context, destination any) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAdminCreditRequestBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request must contain one JSON object")
	}
	return nil
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
	case errors.Is(err, ErrChargeNotFound):
		response.Error(c, http.StatusNotFound, "原扣款记录不存在")
	case errors.Is(err, ErrRefundExceedsCharge):
		response.Error(c, http.StatusUnprocessableEntity, "退款金额超过原扣款剩余额度")
	default:
		response.InternalError(c, http.StatusInternalServerError, "点子操作失败", err)
	}
}
