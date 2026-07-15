package credits

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/adminaction"
	"ideaboard/api/internal/platform/jsonlimit"
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
	StartReconciliation(context.Context, StartReconciliationInput) (ReconciliationRun, error)
	GetReconciliationRun(context.Context, int64) (ReconciliationRun, error)
	ListReconciliationRuns(context.Context, ListReconciliationInput) (ReconciliationRunPage, error)
	ListReconciliationFindings(context.Context, int64, ReconciliationFindingStatus, ListReconciliationInput) (ReconciliationFindingPage, error)
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

type adminCreditCommandRequest interface {
	creditAmount() int64
}

func (r *adminGrantRequest) creditAmount() int64      { return r.AmountCredits }
func (r *adminDeductionRequest) creditAmount() int64  { return r.AmountCredits }
func (r *adminRefundRequest) creditAmount() int64     { return r.AmountCredits }
func (r *adminCorrectionRequest) creditAmount() int64 { return r.AmountCredits }

type adminReconciliationRequest struct {
	StaleHoldAfterSeconds int64  `json:"stale_hold_after_seconds"`
	Reason                string `json:"reason"`
}

const maxAdminCreditRequestBytes = 16 << 10

func NewHandler(service AdminService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Grant(c *gin.Context) {
	var request adminGrantRequest
	adminID, amount, ok := h.prepareAdminCreditCommand(c, &request)
	if !ok {
		return
	}
	writeAdminCreditCommand(c, func(ctx context.Context) (CommandResult, error) {
		return h.service.Grant(ctx, GrantCommand{
			UserID: request.UserID, Amount: amount,
			Metadata: adminCommandMetadata(adminID, request.IdempotencyKey, "admin_grant", request.Reason, request.BusinessReferenceID),
		})
	})
}

func (h *Handler) Deduct(c *gin.Context) {
	var request adminDeductionRequest
	adminID, amount, ok := h.prepareAdminCreditCommand(c, &request)
	if !ok {
		return
	}
	writeAdminCreditCommand(c, func(ctx context.Context) (CommandResult, error) {
		return h.service.Deduct(ctx, DeductionCommand{
			UserID: request.UserID, Amount: amount,
			Metadata: adminCommandMetadata(adminID, request.IdempotencyKey, "admin_deduction", request.Reason, request.BusinessReferenceID),
		})
	})
}

func (h *Handler) Refund(c *gin.Context) {
	var request adminRefundRequest
	adminID, amount, ok := h.prepareAdminCreditCommand(c, &request)
	if !ok {
		return
	}
	writeAdminCreditCommand(c, func(ctx context.Context) (CommandResult, error) {
		return h.service.Refund(ctx, RefundCommand{
			UserID: request.UserID, ChargeTransactionID: request.ChargeTransactionID, Amount: amount,
			Metadata: adminCommandMetadata(adminID, request.IdempotencyKey, "admin_refund", request.Reason, request.BusinessReferenceID),
		})
	})
}

func (h *Handler) Correct(c *gin.Context) {
	var request adminCorrectionRequest
	adminID, amount, ok := h.prepareAdminCreditCommand(c, &request)
	if !ok {
		return
	}
	writeAdminCreditCommand(c, func(ctx context.Context) (CommandResult, error) {
		return h.service.Correct(ctx, CorrectionCommand{
			UserID: request.UserID, OriginalTransactionID: request.OriginalTransactionID, Amount: amount,
			Metadata: adminCommandMetadata(adminID, request.IdempotencyKey, "admin_correction", request.Reason, request.BusinessReferenceID),
		})
	})
}

func (h *Handler) prepareAdminCreditCommand(c *gin.Context, request adminCreditCommandRequest) (int64, CreditAmount, bool) {
	if h == nil || h.service == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "Idea服务暂不可用", nil)
		return 0, 0, false
	}
	adminID, ok := authenticatedAdminID(c)
	if !ok {
		return 0, 0, false
	}
	if err := decodeAdminCreditRequest(c, request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return 0, 0, false
	}
	amount, err := NewCreditAmount(request.creditAmount())
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Idea数量必须是有效正整数")
		return 0, 0, false
	}
	return adminID, amount, true
}

func writeAdminCreditCommand(c *gin.Context, execute func(context.Context) (CommandResult, error)) {
	result, err := execute(c.Request.Context())
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
	return jsonlimit.DecodeStrictRequest(c.Writer, c.Request, maxAdminCreditRequestBytes, destination)
}

func (h *Handler) GetWallet(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "Idea服务暂不可用", nil)
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

func (h *Handler) GetCurrentWallet(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "Idea服务暂不可用", nil)
		return
	}
	userID, ok := authenticatedUserID(c)
	if !ok {
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
		response.InternalError(c, http.StatusServiceUnavailable, "Idea服务暂不可用", nil)
		return
	}
	userID, ok := parsePositiveInt64(c.Param("user_id"))
	if !ok {
		response.Error(c, http.StatusBadRequest, "用户 ID 无效")
		return
	}
	h.listLedger(c, userID)
}

func (h *Handler) listLedger(c *gin.Context, userID int64) {
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

func authenticatedUserID(c *gin.Context) (int64, bool) {
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return 0, false
	}
	return identityPrincipal.UserID(), true
}

func (h *Handler) StartReconciliation(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "Idea对账服务暂不可用", nil)
		return
	}
	adminID, ok := authenticatedAdminID(c)
	if !ok {
		return
	}
	var request adminReconciliationRequest
	if err := decodeAdminCreditRequest(c, &request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	if request.StaleHoldAfterSeconds == 0 {
		request.StaleHoldAfterSeconds = 86400
	}
	if request.StaleHoldAfterSeconds < 60 || request.StaleHoldAfterSeconds > 31536000 {
		response.Error(c, http.StatusBadRequest, "stale_hold_after_seconds 无效")
		return
	}
	action, err := adminaction.New(adminID, request.Reason)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "reason 无效")
		return
	}
	run, err := h.service.StartReconciliation(c.Request.Context(), StartReconciliationInput{
		Action: action, StaleHoldAfter: time.Duration(request.StaleHoldAfterSeconds) * time.Second,
	})
	if err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, run)
}

func (h *Handler) GetReconciliationRun(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "Idea对账服务暂不可用", nil)
		return
	}
	runID, ok := parsePositiveInt64(c.Param("run_id"))
	if !ok {
		response.Error(c, http.StatusBadRequest, "run_id 无效")
		return
	}
	run, err := h.service.GetReconciliationRun(c.Request.Context(), runID)
	if err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, run)
}

func (h *Handler) ListReconciliationRuns(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "Idea对账服务暂不可用", nil)
		return
	}
	input, ok := parseReconciliationPage(c)
	if !ok {
		return
	}
	page, err := h.service.ListReconciliationRuns(c.Request.Context(), input)
	if err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, page)
}

func (h *Handler) ListReconciliationFindings(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "Idea对账服务暂不可用", nil)
		return
	}
	runID, ok := parsePositiveInt64(c.Param("run_id"))
	if !ok {
		response.Error(c, http.StatusBadRequest, "run_id 无效")
		return
	}
	input, ok := parseReconciliationPage(c)
	if !ok {
		return
	}
	status, ok := parseReconciliationFindingStatus(c.Query("status"))
	if !ok {
		response.Error(c, http.StatusBadRequest, "status 无效")
		return
	}
	page, err := h.service.ListReconciliationFindings(c.Request.Context(), runID, status, input)
	if err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, page)
}

func parseReconciliationFindingStatus(value string) (ReconciliationFindingStatus, bool) {
	status := ReconciliationFindingStatus(strings.TrimSpace(value))
	return status, status == "" || status == ReconciliationFindingOpen || status == ReconciliationFindingResolved
}

func parseReconciliationPage(c *gin.Context) (ListReconciliationInput, bool) {
	input := ListReconciliationInput{}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 200 {
			response.Error(c, http.StatusBadRequest, "limit 无效")
			return input, false
		}
		input.Limit = limit
	}
	if raw := strings.TrimSpace(c.Query("before_id")); raw != "" {
		before, ok := parsePositiveInt64(raw)
		if !ok {
			response.Error(c, http.StatusBadRequest, "before_id 无效")
			return input, false
		}
		input.BeforeID = before
	}
	return input, true
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
		response.Error(c, http.StatusConflict, "幂等键已用于不同的Idea操作")
	case errors.Is(err, ErrWalletNotFound):
		response.Error(c, http.StatusNotFound, "Idea钱包不存在")
	case errors.Is(err, ErrWalletFrozen):
		response.Error(c, http.StatusLocked, "Idea钱包已冻结")
	case errors.Is(err, ErrInsufficientCredits):
		response.Error(c, http.StatusUnprocessableEntity, "Idea不足")
	case errors.Is(err, ErrChargeNotFound):
		response.Error(c, http.StatusNotFound, "原扣款记录不存在")
	case errors.Is(err, ErrRefundExceedsCharge):
		response.Error(c, http.StatusUnprocessableEntity, "退款金额超过原扣款剩余额度")
	case errors.Is(err, ErrReconciliationNotFound):
		response.Error(c, http.StatusNotFound, "Idea对账任务不存在")
	case errors.Is(err, ErrReconciliationUnavailable):
		response.InternalError(c, http.StatusServiceUnavailable, "Idea对账服务暂不可用", err)
	default:
		response.InternalError(c, http.StatusInternalServerError, "Idea操作失败", err)
	}
}
