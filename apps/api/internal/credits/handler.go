package credits

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/adminaction"
	"ideaboard/api/internal/platform/jsonlimit"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AdminService interface {
	Grant(context.Context, GrantCommand) (CommandResult, error)
	Deduct(context.Context, DeductionCommand) (CommandResult, error)
	Correct(context.Context, CorrectionCommand) (CommandResult, error)
	Refund(context.Context, RefundCommand) (CommandResult, error)
	GetWallet(context.Context, int64) (WalletDetails, error)
	ListLedger(context.Context, ListLedgerInput) (LedgerPage, error)
	GetCreditHealth(context.Context) (CreditHealth, error)
	StartReconciliation(context.Context, StartManualReconciliationInput) (ReconciliationRun, error)
	GetReconciliationRun(context.Context, int64) (ReconciliationRun, error)
	ListReconciliationRuns(context.Context, ListReconciliationInput) (ReconciliationRunPage, error)
	ListGlobalReconciliationFindings(context.Context, ReconciliationFindingStatus, ListReconciliationInput) (ReconciliationFindingPage, error)
}

type Handler struct {
	service      AdminService
	userResolver UserResolver
}

// UserIdentity is the public identity projection needed by admin credit reads.
// The resolver boundary keeps the credits package independent from the users
// lifecycle package (which itself depends on credits for wallet lifecycle).
type UserIdentity struct {
	ID          int64
	PublicID    uuid.UUID
	DisplayName string
}

type UserResolver interface {
	ResolvePublicID(context.Context, string) (UserIdentity, error)
	ResolveID(context.Context, int64) (UserIdentity, error)
}

type adminGrantRequest struct {
	UserPublicID        string `json:"user_public_id"`
	AmountCredits       int64  `json:"amount_credits" binding:"required"`
	IdempotencyKey      string `json:"idempotency_key" binding:"required"`
	Reason              string `json:"reason" binding:"required"`
	BusinessReferenceID string `json:"business_reference_id" binding:"required"`
}

type adminDeductionRequest adminGrantRequest

type adminRefundRequest struct {
	UserPublicID        string `json:"user_public_id"`
	ChargeTransactionID string `json:"charge_transaction_id"`
	AmountCredits       int64  `json:"amount_credits"`
	IdempotencyKey      string `json:"idempotency_key"`
	Reason              string `json:"reason"`
	BusinessReferenceID string `json:"business_reference_id"`
}

type adminCorrectionRequest struct {
	UserPublicID          string `json:"user_public_id"`
	OriginalTransactionID string `json:"original_transaction_id"`
	AmountCredits         int64  `json:"amount_credits"`
	IdempotencyKey        string `json:"idempotency_key"`
	Reason                string `json:"reason"`
	BusinessReferenceID   string `json:"business_reference_id"`
}

type adminCreditCommandRequest interface {
	creditAmount() int64
	publicUserID() string
}

func (r *adminGrantRequest) creditAmount() int64       { return r.AmountCredits }
func (r *adminDeductionRequest) creditAmount() int64   { return r.AmountCredits }
func (r *adminRefundRequest) creditAmount() int64      { return r.AmountCredits }
func (r *adminCorrectionRequest) creditAmount() int64  { return r.AmountCredits }
func (r *adminGrantRequest) publicUserID() string      { return r.UserPublicID }
func (r *adminDeductionRequest) publicUserID() string  { return r.UserPublicID }
func (r *adminRefundRequest) publicUserID() string     { return r.UserPublicID }
func (r *adminCorrectionRequest) publicUserID() string { return r.UserPublicID }

type adminReconciliationRequest struct {
	Reason string `json:"reason"`
}

const maxAdminCreditRequestBytes = 16 << 10

func NewHandler(service AdminService, resolver ...UserResolver) *Handler {
	var userResolver UserResolver
	if len(resolver) > 0 {
		userResolver = resolver[0]
	}
	return &Handler{service: service, userResolver: userResolver}
}

func (h *Handler) Grant(c *gin.Context) {
	var request adminGrantRequest
	adminID, userID, amount, ok := h.prepareAdminCreditCommand(c, &request)
	if !ok {
		return
	}
	h.writeAdminCreditCommand(c, func(ctx context.Context) (CommandResult, error) {
		return h.service.Grant(ctx, GrantCommand{
			UserID: userID, Amount: amount,
			Metadata: adminCommandMetadata(adminID, request.IdempotencyKey, "admin_grant", request.Reason, request.BusinessReferenceID),
		})
	})
}

func (h *Handler) Deduct(c *gin.Context) {
	var request adminDeductionRequest
	adminID, userID, amount, ok := h.prepareAdminCreditCommand(c, &request)
	if !ok {
		return
	}
	h.writeAdminCreditCommand(c, func(ctx context.Context) (CommandResult, error) {
		return h.service.Deduct(ctx, DeductionCommand{
			UserID: userID, Amount: amount,
			Metadata: adminCommandMetadata(adminID, request.IdempotencyKey, "admin_deduction", request.Reason, request.BusinessReferenceID),
		})
	})
}

func (h *Handler) Refund(c *gin.Context) {
	var request adminRefundRequest
	adminID, userID, amount, ok := h.prepareAdminCreditCommand(c, &request)
	if !ok {
		return
	}
	h.writeAdminCreditCommand(c, func(ctx context.Context) (CommandResult, error) {
		return h.service.Refund(ctx, RefundCommand{
			UserID: userID, ChargeTransactionID: request.ChargeTransactionID, Amount: amount,
			Metadata: adminCommandMetadata(adminID, request.IdempotencyKey, "admin_refund", request.Reason, request.BusinessReferenceID),
		})
	})
}

func (h *Handler) Correct(c *gin.Context) {
	var request adminCorrectionRequest
	adminID, userID, amount, ok := h.prepareAdminCreditCommand(c, &request)
	if !ok {
		return
	}
	h.writeAdminCreditCommand(c, func(ctx context.Context) (CommandResult, error) {
		return h.service.Correct(ctx, CorrectionCommand{
			UserID: userID, OriginalTransactionID: request.OriginalTransactionID, Amount: amount,
			Metadata: adminCommandMetadata(adminID, request.IdempotencyKey, "admin_correction", request.Reason, request.BusinessReferenceID),
		})
	})
}

func (h *Handler) prepareAdminCreditCommand(c *gin.Context, request adminCreditCommandRequest) (int64, int64, CreditAmount, bool) {
	adminID, ok := h.requireAdmin(c)
	if !ok {
		return 0, 0, 0, false
	}
	if err := decodeAdminCreditRequest(c, request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return 0, 0, 0, false
	}
	userID, ok := h.resolveUserID(c, request.publicUserID())
	if !ok {
		return 0, 0, 0, false
	}
	amount, err := NewCreditAmount(request.creditAmount())
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Idea数量必须是有效正整数")
		return 0, 0, 0, false
	}
	return adminID, userID, amount, true
}

func (h *Handler) writeAdminCreditCommand(c *gin.Context, execute func(context.Context) (CommandResult, error)) {
	result, err := execute(c.Request.Context())
	if err != nil {
		writeCreditError(c, err)
		return
	}
	if err := h.enrichCommandResult(c.Request.Context(), &result); err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, result)
}

func (h *Handler) requireAdmin(c *gin.Context) (int64, bool) {
	identityPrincipal, ok := principal.RequireAdmin(c)
	if !ok {
		return 0, false
	}
	if h == nil || h.service == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "Idea服务暂不可用", nil)
		return 0, false
	}
	return identityPrincipal.UserID(), true
}

func (h *Handler) GetCreditHealth(c *gin.Context) {
	if _, ok := h.requireAdmin(c); !ok {
		return
	}
	health, err := h.service.GetCreditHealth(c.Request.Context())
	if err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, health)
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
	if _, ok := h.requireAdmin(c); !ok {
		return
	}
	userID, ok := h.resolveUserID(c, c.Param("public_id"))
	if !ok {
		return
	}
	wallet, err := h.service.GetWallet(c.Request.Context(), userID)
	if err != nil {
		writeCreditError(c, err)
		return
	}
	if err := h.enrichWallet(c.Request.Context(), &wallet); err != nil {
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
	if err := h.enrichWallet(c.Request.Context(), &wallet); err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, wallet.WalletBalance)
}

func (h *Handler) ListLedger(c *gin.Context) {
	if _, ok := h.requireAdmin(c); !ok {
		return
	}
	userID, ok := h.resolveUserID(c, c.Param("public_id"))
	if !ok {
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
	adminID, ok := h.requireAdmin(c)
	if !ok {
		return
	}
	var request adminReconciliationRequest
	if err := decodeAdminCreditRequest(c, &request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	action, err := adminaction.New(adminID, request.Reason)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "reason 无效")
		return
	}
	run, err := h.service.StartReconciliation(c.Request.Context(), StartManualReconciliationInput{
		Action: action,
	})
	if err != nil {
		writeCreditError(c, err)
		return
	}
	if err := h.enrichReconciliationRun(c.Request.Context(), &run); err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, run)
}

func (h *Handler) GetReconciliationRun(c *gin.Context) {
	if _, ok := h.requireAdmin(c); !ok {
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
	if err := h.enrichReconciliationRun(c.Request.Context(), &run); err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, run)
}

func (h *Handler) ListReconciliationRuns(c *gin.Context) {
	if _, ok := h.requireAdmin(c); !ok {
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
	if err := h.enrichReconciliationRuns(c.Request.Context(), &page); err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, page)
}

func (h *Handler) ListGlobalReconciliationFindings(c *gin.Context) {
	if _, ok := h.requireAdmin(c); !ok {
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
	page, err := h.service.ListGlobalReconciliationFindings(c.Request.Context(), status, input)
	if err != nil {
		writeCreditError(c, err)
		return
	}
	if err := h.enrichReconciliationFindings(c.Request.Context(), &page); err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, page)
}

func parseReconciliationFindingStatus(value string) (ReconciliationFindingStatus, bool) {
	status := ReconciliationFindingStatus(strings.TrimSpace(value))
	return status, status == ReconciliationFindingOpen || status == ReconciliationFindingResolved
}

func (h *Handler) resolveUserID(c *gin.Context, rawPublicID string) (int64, bool) {
	if h.userResolver == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "Idea服务暂不可用", nil)
		return 0, false
	}
	if strings.TrimSpace(rawPublicID) == "" {
		response.Error(c, http.StatusBadRequest, "用户 public_id 无效")
		return 0, false
	}
	user, err := h.userResolver.ResolvePublicID(c.Request.Context(), rawPublicID)
	if err != nil || user.ID <= 0 {
		response.Error(c, http.StatusNotFound, "用户不存在")
		return 0, false
	}
	return user.ID, true
}

func (h *Handler) enrichCommandResult(ctx context.Context, result *CommandResult) error {
	if h == nil || h.userResolver == nil || result == nil || result.Wallet.UserID <= 0 {
		return nil
	}
	user, err := h.userResolver.ResolveID(ctx, result.Wallet.UserID)
	if err != nil {
		return fmt.Errorf("resolve credit wallet identity: %w", err)
	}
	result.Wallet.PublicID = user.PublicID
	result.Wallet.DisplayName = user.DisplayName
	return nil
}

func (h *Handler) enrichWallet(ctx context.Context, wallet *WalletDetails) error {
	if h == nil || h.userResolver == nil || wallet == nil || wallet.WalletBalance.UserID <= 0 {
		return nil
	}
	user, err := h.userResolver.ResolveID(ctx, wallet.WalletBalance.UserID)
	if err != nil {
		return fmt.Errorf("resolve credit wallet identity: %w", err)
	}
	wallet.WalletBalance.PublicID = user.PublicID
	wallet.WalletBalance.DisplayName = user.DisplayName
	for index := range wallet.ActiveRestrictions {
		wallet.ActiveRestrictions[index].PublicID = user.PublicID
		wallet.ActiveRestrictions[index].DisplayName = user.DisplayName
	}
	return nil
}

func (h *Handler) enrichReconciliationRun(ctx context.Context, run *ReconciliationRun) error {
	if h == nil || h.userResolver == nil || run == nil || run.RequestedBy == nil {
		return nil
	}
	identity, err := h.userResolver.ResolveID(ctx, *run.RequestedBy)
	if err != nil {
		return fmt.Errorf("resolve reconciliation actor identity: %w", err)
	}
	publicID := identity.PublicID
	run.RequestedByPublicID = &publicID
	run.RequestedByName = identity.DisplayName
	return nil
}

func (h *Handler) enrichReconciliationRuns(ctx context.Context, page *ReconciliationRunPage) error {
	if page == nil {
		return nil
	}
	for index := range page.Runs {
		if err := h.enrichReconciliationRun(ctx, &page.Runs[index]); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) enrichReconciliationFindings(ctx context.Context, page *ReconciliationFindingPage) error {
	if h == nil || h.userResolver == nil || page == nil {
		return nil
	}
	for index := range page.Findings {
		finding := &page.Findings[index]
		if finding.WalletUserID == nil {
			continue
		}
		identity, err := h.userResolver.ResolveID(ctx, *finding.WalletUserID)
		if err != nil {
			return fmt.Errorf("resolve reconciliation wallet identity: %w", err)
		}
		publicID := identity.PublicID
		finding.WalletPublicID = &publicID
		finding.WalletDisplayName = identity.DisplayName
	}
	return nil
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
	case errors.Is(err, ErrWalletRestricted):
		response.Error(c, http.StatusLocked, "Idea钱包当前受限")
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
