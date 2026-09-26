package http

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/jsonlimit"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type AdminCommandHandler struct {
	service      AdminCommandService
	userResolver UserPublicIDResolver
}

func NewAdminCommandHandler(service AdminCommandService, resolver UserPublicIDResolver) *AdminCommandHandler {
	return &AdminCommandHandler{service: service, userResolver: resolver}
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

const maxAdminCreditRequestBytes = 16 << 10

func (h *AdminCommandHandler) Grant(c *gin.Context) {
	var request adminGrantRequest
	adminID, user, amount, ok := h.prepareAdminCreditCommand(c, &request)
	if !ok {
		return
	}
	h.writeAdminCreditCommand(c, user, func(ctx context.Context) (ports.CommandResult, error) {
		return h.service.Grant(ctx, ports.GrantCommand{
			UserID: user.ID, Amount: amount,
			Metadata: adminCommandMetadata(adminID, request.IdempotencyKey, "admin_grant", request.Reason, request.BusinessReferenceID),
		})
	})
}

func (h *AdminCommandHandler) Deduct(c *gin.Context) {
	var request adminDeductionRequest
	adminID, user, amount, ok := h.prepareAdminCreditCommand(c, &request)
	if !ok {
		return
	}
	h.writeAdminCreditCommand(c, user, func(ctx context.Context) (ports.CommandResult, error) {
		return h.service.Deduct(ctx, ports.DeductionCommand{
			UserID: user.ID, Amount: amount,
			Metadata: adminCommandMetadata(adminID, request.IdempotencyKey, "admin_deduction", request.Reason, request.BusinessReferenceID),
		})
	})
}

func (h *AdminCommandHandler) Refund(c *gin.Context) {
	var request adminRefundRequest
	adminID, user, amount, ok := h.prepareAdminCreditCommand(c, &request)
	if !ok {
		return
	}
	h.writeAdminCreditCommand(c, user, func(ctx context.Context) (ports.CommandResult, error) {
		return h.service.Refund(ctx, ports.RefundCommand{
			UserID: user.ID, ChargeTransactionID: request.ChargeTransactionID, Amount: amount,
			Metadata: adminCommandMetadata(adminID, request.IdempotencyKey, "admin_refund", request.Reason, request.BusinessReferenceID),
		})
	})
}

func (h *AdminCommandHandler) Correct(c *gin.Context) {
	var request adminCorrectionRequest
	adminID, user, amount, ok := h.prepareAdminCreditCommand(c, &request)
	if !ok {
		return
	}
	h.writeAdminCreditCommand(c, user, func(ctx context.Context) (ports.CommandResult, error) {
		return h.service.Correct(ctx, ports.CorrectionCommand{
			UserID: user.ID, OriginalTransactionID: request.OriginalTransactionID, Amount: amount,
			Metadata: adminCommandMetadata(adminID, request.IdempotencyKey, "admin_correction", request.Reason, request.BusinessReferenceID),
		})
	})
}

func (h *AdminCommandHandler) prepareAdminCreditCommand(c *gin.Context, request adminCreditCommandRequest) (int64, UserIdentity, domain.CreditAmount, bool) {
	adminID, ok := requireAdmin(c, h != nil && h.service != nil)
	if !ok {
		return 0, UserIdentity{}, 0, false
	}
	if err := decodeAdminCreditRequest(c, request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return 0, UserIdentity{}, 0, false
	}
	user, ok := resolveUser(c, h.userResolver, request.publicUserID())
	if !ok {
		return 0, UserIdentity{}, 0, false
	}
	amount, err := domain.NewCreditAmount(request.creditAmount())
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Idea数量必须是有效正整数")
		return 0, UserIdentity{}, 0, false
	}
	return adminID, user, amount, true
}

func (h *AdminCommandHandler) writeAdminCreditCommand(c *gin.Context, user UserIdentity, execute func(context.Context) (ports.CommandResult, error)) {
	result, err := execute(c.Request.Context())
	if err != nil {
		writeCreditError(c, err)
		return
	}
	result.Wallet.PublicID = user.PublicID
	result.Wallet.DisplayName = user.DisplayName
	response.SuccessDTO(c, result)
}

func requireAdmin(c *gin.Context, serviceConfigured bool) (int64, bool) {
	identityPrincipal, ok := principal.RequireAdmin(c)
	if !ok {
		return 0, false
	}
	if !serviceConfigured {
		response.InternalError(c, http.StatusServiceUnavailable, "Idea服务暂不可用", nil)
		return 0, false
	}
	return identityPrincipal.UserID(), true
}

func adminCommandMetadata(adminID int64, idempotencyKey, source, reason, businessReferenceID string) domain.CommandMetadata {
	return domain.CommandMetadata{
		IdempotencyKey: idempotencyKey,
		Actor:          domain.Actor{Type: domain.ActorAdmin, ID: strconv.FormatInt(adminID, 10)},
		Source:         source, Reason: reason,
		BusinessReference: domain.BusinessReference{Type: "admin_adjustment", ID: businessReferenceID},
	}
}

func decodeAdminCreditRequest(c *gin.Context, destination any) error {
	return jsonlimit.DecodeStrictRequest(c.Writer, c.Request, maxAdminCreditRequestBytes, destination)
}

func resolveUser(c *gin.Context, userResolver UserPublicIDResolver, rawPublicID string) (UserIdentity, bool) {
	if userResolver == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "Idea服务暂不可用", nil)
		return UserIdentity{}, false
	}
	if strings.TrimSpace(rawPublicID) == "" {
		response.Error(c, http.StatusBadRequest, "用户 public_id 无效")
		return UserIdentity{}, false
	}
	user, err := userResolver.ResolvePublicID(c.Request.Context(), rawPublicID)
	if err != nil || user.ID <= 0 {
		response.Error(c, http.StatusNotFound, "用户不存在")
		return UserIdentity{}, false
	}
	return user, true
}

func parsePositiveInt64(value string) (int64, bool) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed, err == nil && parsed > 0
}
