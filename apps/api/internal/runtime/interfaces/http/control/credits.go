package control

// Control financial operations delegate to credits.Commands (Grant, Deduct,
// Correct, Refund), never direct ledger writes. The header Idempotency-Key is
// also the durable ledger key. Metadata records the admin actor's public ID
// and a control_* source.

import (
	"errors"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"net/http"
	"strconv"
	"strings"
	"time"

	creditsdomain "ideaboard/api/internal/credits/domain"
	creditsports "ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// controlCreditResult is the Control Plane credit command projection:
// identifiers are usr_ public ids; the internal wallet FK never crosses.
type controlCreditResult struct {
	TransactionID string              `json:"transaction_id"`
	CommandType   string              `json:"command_type"`
	AmountCredits int64               `json:"amount_credits"`
	Replayed      bool                `json:"replayed"`
	CreatedAt     time.Time           `json:"created_at"`
	Wallet        controlCreditWallet `json:"wallet"`
}

type controlCreditWallet struct {
	UserID             string                     `json:"user_id"`
	DisplayName        string                     `json:"display_name"`
	AvailableCredits   int64                      `json:"available_credits"`
	ReservedCredits    int64                      `json:"reserved_credits"`
	Status             string                     `json:"status"`
	ActiveRestrictions []controlWalletRestriction `json:"active_restrictions"`
}

type controlWalletRestriction struct {
	RestrictionID string                          `json:"restriction_id"`
	UserID        string                          `json:"user_id"`
	DisplayName   string                          `json:"display_name"`
	Type          string                          `json:"type"`
	Source        string                          `json:"source"`
	RelatedObject creditsdomain.BusinessReference `json:"related_object"`
	Reason        string                          `json:"reason"`
	CreatedBy     creditsdomain.Actor             `json:"created_by"`
	CreatedAt     time.Time                       `json:"created_at"`
}

func newControlCreditResult(result creditsports.CommandResult, userID uuid.UUID, displayName string) controlCreditResult {
	return controlCreditResult{
		TransactionID: result.TransactionID,
		CommandType:   string(result.CommandType),
		AmountCredits: result.Amount,
		Replayed:      result.Replayed,
		CreatedAt:     result.CreatedAt,
		Wallet: controlCreditWallet{
			UserID:             tenancy.FormatUserPublicID(userID),
			DisplayName:        displayName,
			AvailableCredits:   result.Wallet.Available,
			ReservedCredits:    result.Wallet.Reserved,
			ActiveRestrictions: []controlWalletRestriction{},
		},
	}
}

// writeControlCreditError maps credit domain failures to contract codes.
// Financial denials reuse the frozen billing codes so frontend branches on
// billing_wallet_restricted and idempotency_conflict uniformly.
func writeControlCreditError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, creditsdomain.ErrInvalidCommand):
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest4)
	case errors.Is(err, creditsdomain.ErrIdempotencyConflict):
		contract.RenderError(c, http.StatusConflict, contract.CodeIdempotencyConflict, runtimehttp.MsgIdempotencyConflict2)
	case errors.Is(err, creditsdomain.ErrWalletNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, runtimehttp.MsgControlNotFound2)
	case errors.Is(err, creditsdomain.ErrWalletFrozen),
		errors.Is(err, creditsdomain.ErrWalletRestricted):
		contract.RenderError(c, http.StatusForbidden, contract.CodeBillingWalletRestricted, runtimehttp.MsgBillingWalletRestricted)
	case errors.Is(err, creditsdomain.ErrInsufficientCredits):
		contract.RenderError(c, http.StatusUnprocessableEntity, contract.CodeValidationFailed, runtimehttp.MsgValidationFailed2)
	case errors.Is(err, creditsdomain.ErrChargeNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, runtimehttp.MsgControlNotFound3)
	case errors.Is(err, creditsdomain.ErrRefundExceedsCharge):
		contract.RenderError(c, http.StatusUnprocessableEntity, contract.CodeValidationFailed, runtimehttp.MsgValidationFailed3)
	case errors.Is(err, creditsdomain.ErrReconciliationNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, runtimehttp.MsgControlNotFound4)
	case errors.Is(err, creditsdomain.ErrReconciliationUnavailable):
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable6)
	default:
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable7)
	}
}

type controlCreditAmountRequest struct {
	UserID              string `json:"user_id"`
	AmountCredits       int64  `json:"amount_credits"`
	BusinessReferenceID string `json:"business_reference_id"`
}

type controlCreditLinkRequest struct {
	controlCreditAmountRequest
	ChargeTransactionID   string `json:"charge_transaction_id"`
	OriginalTransactionID string `json:"original_transaction_id"`
}

// resolveControlCreditUser maps a usr_ user_id to the internal identity
// the credits seam consumes. Only usr_ shapes are accepted.
func (d *ContractControlDeps) resolveControlCreditUser(c *gin.Context, rawUserID string) (userID int64, publicID uuid.UUID, displayName string, ok bool) {
	parsed, ok := parseControlUserPublicID(c, rawUserID)
	if !ok {
		return 0, uuid.Nil, "", false
	}
	if d == nil || d.CreditUsers == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable8)
		return 0, uuid.Nil, "", false
	}
	identity, err := d.CreditUsers.ResolvePublicID(c.Request.Context(), parsed.String())
	if err != nil || identity.ID <= 0 {
		contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, runtimehttp.MsgControlNotFound5)
		return 0, uuid.Nil, "", false
	}
	return identity.ID, identity.PublicID, identity.DisplayName, true
}

// controlCreditMetadata builds the ledger command metadata with the
// explicit control operator, header idempotency key, and control_*
// source provenance.
func controlCreditMetadata(ctx controlContext, source, businessReferenceID string) creditsdomain.CommandMetadata {
	_, operatorID := controlOperatorIdentity(ctx.Operator)
	return creditsdomain.CommandMetadata{
		IdempotencyKey: ctx.IdemKey,
		Actor:          creditsdomain.Actor{Type: creditsdomain.ActorAdmin, ID: operatorID},
		Source:         source,
		BusinessReference: creditsdomain.BusinessReference{
			Type: "control_adjustment",
			ID:   strings.TrimSpace(businessReferenceID),
		},
	}
}

// prepareControlCreditCommand decodes and validates the shared credit
// mutation body (header-only idempotency key; body carries no key).
func (d *ContractControlDeps) prepareControlCreditCommand(c *gin.Context) (userID int64, publicID uuid.UUID, displayName string, amount creditsdomain.CreditAmount, chargeID, originalID, businessRefID string, ok bool) {
	var req controlCreditLinkRequest
	if err := contract.BindStrictJSON(c, &req); err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest4)
		return 0, uuid.Nil, "", 0, "", "", "", false
	}
	userID, publicID, displayName, ok = d.resolveControlCreditUser(c, req.UserID)
	if !ok {
		return 0, uuid.Nil, "", 0, "", "", "", false
	}
	parsed, err := creditsdomain.NewCreditAmount(req.AmountCredits)
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest10)
		return 0, uuid.Nil, "", 0, "", "", "", false
	}
	return userID, publicID, displayName, parsed,
		strings.TrimSpace(req.ChargeTransactionID), strings.TrimSpace(req.OriginalTransactionID),
		strings.TrimSpace(req.BusinessReferenceID), true
}

func (d *ContractControlDeps) grantControlCredits(c *gin.Context, ctx controlContext) {
	if d == nil || d.Credits == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable8)
		return
	}
	userID, publicID, displayName, amount, _, _, businessRefID, ok := d.prepareControlCreditCommand(c)
	if !ok {
		return
	}
	result, err := d.Credits.Grant(c.Request.Context(), creditsports.GrantCommand{
		UserID: userID, Amount: amount,
		Metadata: controlCreditMetadata(ctx, "control_grant", businessRefID),
	})
	if err != nil {
		writeControlCreditError(c, err)
		return
	}
	contract.RenderSuccess(c, newControlCreditResult(result, publicID, displayName))
}

func (d *ContractControlDeps) deductControlCredits(c *gin.Context, ctx controlContext) {
	if d == nil || d.Credits == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable8)
		return
	}
	userID, publicID, displayName, amount, _, _, businessRefID, ok := d.prepareControlCreditCommand(c)
	if !ok {
		return
	}
	result, err := d.Credits.Deduct(c.Request.Context(), creditsports.DeductionCommand{
		UserID: userID, Amount: amount,
		Metadata: controlCreditMetadata(ctx, "control_deduction", businessRefID),
	})
	if err != nil {
		writeControlCreditError(c, err)
		return
	}
	contract.RenderSuccess(c, newControlCreditResult(result, publicID, displayName))
}

func (d *ContractControlDeps) refundControlCredits(c *gin.Context, ctx controlContext) {
	if d == nil || d.Credits == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable8)
		return
	}
	userID, publicID, displayName, amount, chargeID, _, businessRefID, ok := d.prepareControlCreditCommand(c)
	if !ok {
		return
	}
	if chargeID == "" {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest11)
		return
	}
	result, err := d.Credits.Refund(c.Request.Context(), creditsports.RefundCommand{
		UserID: userID, ChargeTransactionID: chargeID, Amount: amount,
		Metadata: controlCreditMetadata(ctx, "control_refund", businessRefID),
	})
	if err != nil {
		writeControlCreditError(c, err)
		return
	}
	contract.RenderSuccess(c, newControlCreditResult(result, publicID, displayName))
}

func (d *ContractControlDeps) correctControlCredits(c *gin.Context, ctx controlContext) {
	if d == nil || d.Credits == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable8)
		return
	}
	userID, publicID, displayName, amount, _, originalID, businessRefID, ok := d.prepareControlCreditCommand(c)
	if !ok {
		return
	}
	if originalID == "" {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest12)
		return
	}
	result, err := d.Credits.Correct(c.Request.Context(), creditsports.CorrectionCommand{
		UserID: userID, OriginalTransactionID: originalID, Amount: amount,
		Metadata: controlCreditMetadata(ctx, "control_correction", businessRefID),
	})
	if err != nil {
		writeControlCreditError(c, err)
		return
	}
	contract.RenderSuccess(c, newControlCreditResult(result, publicID, displayName))
}

func (d *ContractControlDeps) getControlCreditWallet(c *gin.Context, _ controlContext) {
	if d == nil || d.CreditReads == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable8)
		return
	}
	userID, _, _, ok := d.resolveControlCreditUser(c, c.Param("user_id"))
	if !ok {
		return
	}
	wallet, err := d.CreditReads.GetWallet(c.Request.Context(), userID)
	if err != nil {
		writeControlCreditError(c, err)
		return
	}
	contract.RenderSuccess(c, newControlCreditWalletView(c, d, wallet))
}

func newControlCreditWalletView(c *gin.Context, d *ContractControlDeps, wallet creditsdomain.WalletDetails) controlCreditWallet {
	view := controlCreditWallet{
		UserID:             tenancy.FormatUserPublicID(wallet.PublicID),
		DisplayName:        wallet.DisplayName,
		AvailableCredits:   wallet.Available,
		ReservedCredits:    wallet.Reserved,
		Status:             string(wallet.Status),
		ActiveRestrictions: make([]controlWalletRestriction, 0, len(wallet.ActiveRestrictions)),
	}
	for _, restriction := range wallet.ActiveRestrictions {
		view.ActiveRestrictions = append(view.ActiveRestrictions, controlWalletRestriction{
			RestrictionID: restriction.RestrictionID,
			UserID:        tenancy.FormatUserPublicID(restriction.PublicID),
			DisplayName:   restriction.DisplayName,
			Type:          restriction.Type,
			Source:        restriction.Source,
			RelatedObject: restriction.RelatedObject,
			Reason:        restriction.Reason,
			CreatedBy:     restriction.CreatedBy,
			CreatedAt:     restriction.CreatedAt,
		})
	}
	if d == nil || d.CreditUsers == nil || wallet.UserID <= 0 {
		return view
	}
	if identity, err := d.CreditUsers.ResolveID(c.Request.Context(), wallet.UserID); err == nil && identity.ID > 0 {
		view.UserID = tenancy.FormatUserPublicID(identity.PublicID)
		view.DisplayName = identity.DisplayName
	}
	return view
}

// controlLedgerCursor is the opaque cursor over before_sequence.
type controlLedgerCursor struct {
	Before int64 `json:"before"`
}

func (d *ContractControlDeps) listControlCreditLedger(c *gin.Context, _ controlContext) {
	if d == nil || d.CreditReads == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable8)
		return
	}
	userID, _, _, ok := d.resolveControlCreditUser(c, c.Param("user_id"))
	if !ok {
		return
	}
	cursor, ok := parseControlPageCursor(c, func(cursor *controlLedgerCursor) bool { return cursor.Before > 0 })
	if !ok {
		return
	}
	var before int64
	if cursor != nil {
		before = cursor.Before
	}
	limit, ok := parseControlPageSize(c, contract.DefaultPageSize)
	if !ok {
		return
	}
	page, err := d.CreditReads.ListLedger(c.Request.Context(), creditsdomain.ListLedgerInput{
		UserID:         userID,
		BeforeSequence: before,
		Limit:          limit,
	})
	if err != nil {
		writeControlCreditError(c, err)
		return
	}
	transactions := page.Transactions
	if transactions == nil {
		transactions = []creditsdomain.LedgerTransaction{}
	}
	hasMore := len(transactions) == limit && page.NextBeforeSequence > 0
	var nextCursor string
	if hasMore {
		encoded, err := contract.EncodeCursor(controlLedgerCursor{Before: page.NextBeforeSequence})
		if err != nil {
			contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable9)
			return
		}
		nextCursor = encoded
	}
	contract.RenderPaginated(c, transactions, contract.NewPageMeta(nextCursor, hasMore))
}

func (d *ContractControlDeps) getControlCreditHealth(c *gin.Context, _ controlContext) {
	if d == nil || d.Reconciliation == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable8)
		return
	}
	health, err := d.Reconciliation.GetCreditHealth(c.Request.Context())
	if err != nil {
		writeControlCreditError(c, err)
		return
	}
	contract.RenderSuccess(c, health)
}

func (d *ContractControlDeps) startControlReconciliation(c *gin.Context, ctx controlContext) {
	if d == nil || d.Reconciliation == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable8)
		return
	}
	run, err := d.Reconciliation.StartReconciliation(c.Request.Context(), creditsports.StartManualReconciliationInput{
		ActorID: ctx.Operator.UserID(),
	})
	if err != nil {
		writeControlCreditError(c, err)
		return
	}
	contract.RenderSuccess(c, enrichControlReconciliationRun(c, d, run))
}

func (d *ContractControlDeps) listControlReconciliationRuns(c *gin.Context, _ controlContext) {
	if d == nil || d.Reconciliation == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable8)
		return
	}
	input, ok := parseControlReconciliationPage(c)
	if !ok {
		return
	}
	page, err := d.Reconciliation.ListReconciliationRuns(c.Request.Context(), input)
	if err != nil {
		writeControlCreditError(c, err)
		return
	}
	for i := range page.Runs {
		page.Runs[i] = enrichControlReconciliationRun(c, d, page.Runs[i])
	}
	contract.RenderSuccess(c, page)
}

func (d *ContractControlDeps) getControlReconciliationRun(c *gin.Context, _ controlContext) {
	if d == nil || d.Reconciliation == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable8)
		return
	}
	// Reconciliation runs use bigint IDs and numeric before_id cursors.
	// They are operator-only records with no public enumeration surface.
	runID, err := strconv.ParseInt(strings.TrimSpace(c.Param("run_id")), 10, 64)
	if err != nil || runID <= 0 {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest15)
		return
	}
	run, err := d.Reconciliation.GetReconciliationRun(c.Request.Context(), runID)
	if err != nil {
		writeControlCreditError(c, err)
		return
	}
	contract.RenderSuccess(c, enrichControlReconciliationRun(c, d, run))
}

func (d *ContractControlDeps) listControlReconciliationFindings(c *gin.Context, _ controlContext) {
	if d == nil || d.Reconciliation == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable8)
		return
	}
	input, ok := parseControlReconciliationPage(c)
	if !ok {
		return
	}
	status := creditsports.ReconciliationFindingStatus(strings.TrimSpace(c.Query("status")))
	if status != creditsports.ReconciliationFindingOpen && status != creditsports.ReconciliationFindingResolved {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest16)
		return
	}
	page, err := d.Reconciliation.ListGlobalReconciliationFindings(c.Request.Context(), status, input)
	if err != nil {
		writeControlCreditError(c, err)
		return
	}
	contract.RenderSuccess(c, page)
}

func parseControlReconciliationPage(c *gin.Context) (creditsports.ListReconciliationInput, bool) {
	input := creditsports.ListReconciliationInput{}
	if raw := strings.TrimSpace(c.Query(contract.LimitParam)); raw != "" {
		parsed, err := contract.ParseLimit(raw)
		if err != nil {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest17)
			return input, false
		}
		input.Limit = parsed
	}
	if raw := strings.TrimSpace(c.Query(contract.BeforeIDParam)); raw != "" {
		before, err := contract.ParseBeforeID(raw)
		if err != nil {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest18)
			return input, false
		}
		input.BeforeID = before
	}
	return input, true
}

func enrichControlReconciliationRun(c *gin.Context, d *ContractControlDeps, run creditsports.ReconciliationRun) creditsports.ReconciliationRun {
	if d == nil || d.CreditUsers == nil || run.RequestedBy == nil {
		return run
	}
	identity, err := d.CreditUsers.ResolveID(c.Request.Context(), *run.RequestedBy)
	if err != nil {
		return run
	}
	run.RequestedByPublicID = &identity.PublicID
	run.RequestedByName = identity.DisplayName
	return run
}
