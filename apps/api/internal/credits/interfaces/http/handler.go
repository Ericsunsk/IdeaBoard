package http

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"ideaboard/api/internal/credits/application/health"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/adminaction"
	"ideaboard/api/internal/platform/jsonlimit"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AdminCommandService interface {
	Grant(context.Context, ports.GrantCommand) (ports.CommandResult, error)
	Deduct(context.Context, ports.DeductionCommand) (ports.CommandResult, error)
	Correct(context.Context, ports.CorrectionCommand) (ports.CommandResult, error)
	Refund(context.Context, ports.RefundCommand) (ports.CommandResult, error)
}

type UserWalletService interface {
	GetWallet(context.Context, int64) (domain.WalletDetails, error)
}

type AdminReadService interface {
	GetWallet(context.Context, int64) (domain.WalletDetails, error)
	ListLedger(context.Context, domain.ListLedgerInput) (domain.LedgerPage, error)
}

type ReconciliationService interface {
	GetCreditHealth(context.Context) (health.CreditHealth, error)
	StartReconciliation(context.Context, ports.StartManualReconciliationInput) (ports.ReconciliationRun, error)
	GetReconciliationRun(context.Context, int64) (ports.ReconciliationRun, error)
	ListReconciliationRuns(context.Context, ports.ListReconciliationInput) (ports.ReconciliationRunPage, error)
	ListGlobalReconciliationFindings(context.Context, ports.ReconciliationFindingStatus, ports.ListReconciliationInput) (ports.ReconciliationFindingPage, error)
}

type UserHandler struct {
	service      UserWalletService
	userResolver UserIDResolver
}

type AdminCommandHandler struct {
	service      AdminCommandService
	userResolver UserPublicIDResolver
}

type AdminReadHandler struct {
	service      AdminReadService
	userResolver AdminReadIdentityResolver
}

type ReconciliationHandler struct {
	service      ReconciliationService
	userResolver UserIDResolver
}

// UserIdentity is the HTTP identity projection shared by Credits inbound
// adapters. The resolver boundary keeps Credits independent from the users
// lifecycle package (which itself depends on Credits for Wallet lifecycle).
type UserIdentity struct {
	ID          int64
	PublicID    uuid.UUID
	DisplayName string
}

type UserPublicIDResolver interface {
	ResolvePublicID(context.Context, string) (UserIdentity, error)
}

type UserIDResolver interface {
	ResolveID(context.Context, int64) (UserIdentity, error)
}

type AdminReadIdentityResolver interface {
	UserPublicIDResolver
	UserIDResolver
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

func NewUserHandler(service UserWalletService, resolver UserIDResolver) *UserHandler {
	return &UserHandler{service: service, userResolver: resolver}
}

func NewAdminCommandHandler(service AdminCommandService, resolver UserPublicIDResolver) *AdminCommandHandler {
	return &AdminCommandHandler{service: service, userResolver: resolver}
}

func NewAdminReadHandler(service AdminReadService, resolver AdminReadIdentityResolver) *AdminReadHandler {
	return &AdminReadHandler{service: service, userResolver: resolver}
}

func NewReconciliationHandler(service ReconciliationService, resolver UserIDResolver) *ReconciliationHandler {
	return &ReconciliationHandler{service: service, userResolver: resolver}
}

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

func (h *ReconciliationHandler) GetCreditHealth(c *gin.Context) {
	if _, ok := requireAdmin(c, h != nil && h.service != nil); !ok {
		return
	}
	health, err := h.service.GetCreditHealth(c.Request.Context())
	if err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, health)
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

func (h *UserHandler) GetCurrentWallet(c *gin.Context) {
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
	if err := enrichWallet(c.Request.Context(), h.userResolver, &wallet); err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, wallet.WalletBalance)
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

func authenticatedUserID(c *gin.Context) (int64, bool) {
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return 0, false
	}
	return identityPrincipal.UserID(), true
}

func (h *ReconciliationHandler) StartReconciliation(c *gin.Context) {
	adminID, ok := requireAdmin(c, h != nil && h.service != nil)
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
	run, err := h.service.StartReconciliation(c.Request.Context(), ports.StartManualReconciliationInput{
		ActorID: action.ActorID(),
		Reason:  action.Reason(),
	})
	if err != nil {
		writeCreditError(c, err)
		return
	}
	if err := enrichReconciliationRun(c.Request.Context(), h.userResolver, &run); err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, run)
}

func (h *ReconciliationHandler) GetReconciliationRun(c *gin.Context) {
	if _, ok := requireAdmin(c, h != nil && h.service != nil); !ok {
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
	if err := enrichReconciliationRun(c.Request.Context(), h.userResolver, &run); err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, run)
}

func (h *ReconciliationHandler) ListReconciliationRuns(c *gin.Context) {
	if _, ok := requireAdmin(c, h != nil && h.service != nil); !ok {
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
	if err := enrichReconciliationRuns(c.Request.Context(), h.userResolver, &page); err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, page)
}

func (h *ReconciliationHandler) ListGlobalReconciliationFindings(c *gin.Context) {
	if _, ok := requireAdmin(c, h != nil && h.service != nil); !ok {
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
	if err := enrichReconciliationFindings(c.Request.Context(), h.userResolver, &page); err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, page)
}

func parseReconciliationFindingStatus(value string) (ports.ReconciliationFindingStatus, bool) {
	status := ports.ReconciliationFindingStatus(strings.TrimSpace(value))
	return status, status == ports.ReconciliationFindingOpen || status == ports.ReconciliationFindingResolved
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

func enrichWallet(ctx context.Context, userResolver UserIDResolver, wallet *domain.WalletDetails) error {
	if userResolver == nil || wallet == nil || wallet.UserID <= 0 {
		return nil
	}
	user, err := userResolver.ResolveID(ctx, wallet.UserID)
	if err != nil {
		return fmt.Errorf("resolve credit wallet identity: %w", err)
	}
	wallet.PublicID = user.PublicID
	wallet.DisplayName = user.DisplayName
	for index := range wallet.ActiveRestrictions {
		wallet.ActiveRestrictions[index].PublicID = user.PublicID
		wallet.ActiveRestrictions[index].DisplayName = user.DisplayName
	}
	return nil
}

func enrichReconciliationRun(ctx context.Context, userResolver UserIDResolver, run *ports.ReconciliationRun) error {
	if userResolver == nil || run == nil || run.RequestedBy == nil {
		return nil
	}
	identity, err := userResolver.ResolveID(ctx, *run.RequestedBy)
	if err != nil {
		return fmt.Errorf("resolve reconciliation actor identity: %w", err)
	}
	publicID := identity.PublicID
	run.RequestedByPublicID = &publicID
	run.RequestedByName = identity.DisplayName
	return nil
}

func enrichReconciliationRuns(ctx context.Context, userResolver UserIDResolver, page *ports.ReconciliationRunPage) error {
	if page == nil {
		return nil
	}
	for index := range page.Runs {
		if err := enrichReconciliationRun(ctx, userResolver, &page.Runs[index]); err != nil {
			return err
		}
	}
	return nil
}

func enrichReconciliationFindings(ctx context.Context, userResolver UserIDResolver, page *ports.ReconciliationFindingPage) error {
	if userResolver == nil || page == nil {
		return nil
	}
	for index := range page.Findings {
		finding := &page.Findings[index]
		if finding.WalletUserID == nil {
			continue
		}
		identity, err := userResolver.ResolveID(ctx, *finding.WalletUserID)
		if err != nil {
			return fmt.Errorf("resolve reconciliation wallet identity: %w", err)
		}
		publicID := identity.PublicID
		finding.WalletPublicID = &publicID
		finding.WalletDisplayName = identity.DisplayName
	}
	return nil
}

func parseReconciliationPage(c *gin.Context) (ports.ListReconciliationInput, bool) {
	input := ports.ListReconciliationInput{}
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
