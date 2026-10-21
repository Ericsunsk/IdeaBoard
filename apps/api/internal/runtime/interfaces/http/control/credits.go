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
	UserID             string                     `json:"user_id,omitempty"`
	DisplayName        string                     `json:"display_name,omitempty"`
	BillingAccountID   string                     `json:"billing_account_id,omitempty"`
	OwnerType          string                     `json:"owner_type,omitempty"`
	AvailableCredits   int64                      `json:"available_credits"`
	ReservedCredits    int64                      `json:"reserved_credits"`
	Status             string                     `json:"status"`
	ActiveRestrictions []controlWalletRestriction `json:"active_restrictions"`
}

type controlWalletRestriction struct {
	RestrictionID    string                          `json:"restriction_id"`
	UserID           string                          `json:"user_id,omitempty"`
	DisplayName      string                          `json:"display_name,omitempty"`
	BillingAccountID string                          `json:"billing_account_id,omitempty"`
	Type             string                          `json:"type"`
	Source           string                          `json:"source"`
	RelatedObject    creditsdomain.BusinessReference `json:"related_object"`
	Reason           string                          `json:"reason"`
	CreatedBy        creditsdomain.Actor             `json:"created_by"`
	CreatedAt        time.Time                       `json:"created_at"`
}

func newControlCreditResult(result creditsports.CommandResult, userID uuid.UUID, displayName, billingAccountPublicID string) controlCreditResult {
	return controlCreditResult{
		TransactionID: result.TransactionID,
		CommandType:   string(result.CommandType),
		AmountCredits: result.Amount,
		Replayed:      result.Replayed,
		CreatedAt:     result.CreatedAt,
		Wallet: controlCreditWallet{
			UserID:             tenancy.FormatUserPublicID(userID),
			DisplayName:        displayName,
			BillingAccountID:   billingAccountPublicID,
			AvailableCredits:   result.Wallet.Available,
			ReservedCredits:    result.Wallet.Reserved,
			ActiveRestrictions: []controlWalletRestriction{},
		},
	}
}

// newControlCreditAccountResult projects a fund command on an explicitly
// addressed Billing Account (#923): funds identity is the ba_ account and
// the operator stays on the Ledger Actor; no single-user linkage is
// synthesized for shared/organization accounts.
func newControlCreditAccountResult(result creditsports.CommandResult, target *controlCreditTarget) controlCreditResult {
	return controlCreditResult{
		TransactionID: result.TransactionID,
		CommandType:   string(result.CommandType),
		AmountCredits: result.Amount,
		Replayed:      result.Replayed,
		CreatedAt:     result.CreatedAt,
		Wallet: controlCreditWallet{
			BillingAccountID:   target.billingAccountPublicID,
			OwnerType:          target.ownerType,
			AvailableCredits:   result.Wallet.Available,
			ReservedCredits:    result.Wallet.Reserved,
			ActiveRestrictions: []controlWalletRestriction{},
		},
	}
}

// controlBillingAccountIdentity resolves the ba_ display id plus the owner
// type for an account wallet. Unknown accounts render empty (fail-closed
// reads already gated the operation); the numeric key never crosses.
func (d *ContractControlDeps) controlBillingAccountIdentity(c *gin.Context, billingAccountInternalID int64) (publicID, ownerType string) {
	if d == nil || d.BillingAccounts == nil || billingAccountInternalID <= 0 {
		return "", ""
	}
	account, err := d.BillingAccounts.GetBillingAccountByInternalID(c.Request.Context(), billingAccountInternalID)
	if err != nil {
		return "", ""
	}
	return account.PublicID, account.OwnerType
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
	// UserID addresses the target user's deterministic initial personal
	// Billing Account (#916). Exactly one of UserID / BillingAccountID is
	// required: the ba_ form addresses an explicit shared/organization
	// account with no single-user linkage synthesized.
	UserID              string `json:"user_id,omitempty"`
	BillingAccountID    string `json:"billing_account_id,omitempty"`
	AmountCredits       int64  `json:"amount_credits"`
	BusinessReferenceID string `json:"business_reference_id"`
}

type controlCreditLinkRequest struct {
	controlCreditAmountRequest
	ChargeTransactionID   string `json:"charge_transaction_id"`
	OriginalTransactionID string `json:"original_transaction_id"`
}

// controlCreditTarget is the resolved fund-operation target: one explicit
// Billing Account plus the optional personal-account user linkage. Funds
// identity and operator are never conflated: the Ledger Actor is always
// the platform-Admin operator (see controlCreditMetadata).
type controlCreditTarget struct {
	billingAccountID       int64
	billingAccountPublicID string
	ownerType              string
	userPublicID           uuid.UUID
	displayName            string
	hasUser                bool
}

// resolveControlCreditTarget maps the XOR-addressed request body to one
// explicit Billing Account. Missing/ambiguous addressing fails closed with
// 400; unknown users or accounts surface as 404 from the seams.
func (d *ContractControlDeps) resolveControlCreditTarget(c *gin.Context, rawUserID, rawBillingAccountID string) (*controlCreditTarget, bool) {
	rawUserID = strings.TrimSpace(rawUserID)
	rawBillingAccountID = strings.TrimSpace(rawBillingAccountID)
	if (rawUserID == "") == (rawBillingAccountID == "") {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest37)
		return nil, false
	}
	if rawBillingAccountID != "" {
		publicID, ok := parseControlBillingAccountID(c, rawBillingAccountID)
		if !ok {
			return nil, false
		}
		if d == nil || d.BillingAccounts == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable8)
			return nil, false
		}
		account, err := d.BillingAccounts.GetBillingAccountByPublicID(c.Request.Context(), publicID)
		if err != nil {
			writeControlBillingAccountError(c, err)
			return nil, false
		}
		return &controlCreditTarget{
			billingAccountID:       account.InternalID,
			billingAccountPublicID: account.PublicID,
			ownerType:              account.OwnerType,
		}, true
	}
	userID, resolvedPublicID, resolvedDisplayName, resolved := d.resolveControlCreditUser(c, rawUserID)
	if !resolved {
		return nil, false
	}
	if d == nil || d.Credits == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable8)
		return nil, false
	}
	accountID, err := d.Credits.ResolvePersonalAccount(c.Request.Context(), userID)
	if err != nil {
		writeControlCreditError(c, err)
		return nil, false
	}
	publicID, ownerType := d.controlBillingAccountIdentity(c, accountID)
	return &controlCreditTarget{
		billingAccountID:       accountID,
		billingAccountPublicID: publicID,
		ownerType:              ownerType,
		userPublicID:           resolvedPublicID,
		displayName:            resolvedDisplayName,
		hasUser:                true,
	}, true
}

// prepareControlCreditCommand decodes and validates the shared credit
// mutation body (header-only idempotency key; body carries no key). The
// target is exactly one explicit Billing Account: usr_-addressed requests
// resolve the target user's deterministic initial personal account (#916),
// ba_-addressed requests name the funds identity directly. Fund operations
// address the account with the admin operator as Actor, never a user wallet.
func (d *ContractControlDeps) prepareControlCreditCommand(c *gin.Context) (target *controlCreditTarget, amount creditsdomain.CreditAmount, chargeID, originalID, businessRefID string, ok bool) {
	var req controlCreditLinkRequest
	if err := contract.BindStrictJSON(c, &req); err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest4)
		return nil, 0, "", "", "", false
	}
	resolved, resolvedOK := d.resolveControlCreditTarget(c, req.UserID, req.BillingAccountID)
	if !resolvedOK {
		return nil, 0, "", "", "", false
	}
	parsed, err := creditsdomain.NewCreditAmount(req.AmountCredits)
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest10)
		return nil, 0, "", "", "", false
	}
	return resolved,
		parsed,
		strings.TrimSpace(req.ChargeTransactionID), strings.TrimSpace(req.OriginalTransactionID),
		strings.TrimSpace(req.BusinessReferenceID), true
}

// renderControlCreditResult projects a fund command result: user-addressed
// calls keep the usr_ identity linkage, account-addressed calls project the
// ba_ funds identity with no synthesized user.
func renderControlCreditResult(c *gin.Context, result creditsports.CommandResult, target *controlCreditTarget) {
	if target.hasUser {
		contract.RenderSuccess(c, newControlCreditResult(result, target.userPublicID, target.displayName, target.billingAccountPublicID))
		return
	}
	contract.RenderSuccess(c, newControlCreditAccountResult(result, target))
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

func (d *ContractControlDeps) grantControlCredits(c *gin.Context, ctx controlContext) {
	if d == nil || d.Credits == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable8)
		return
	}
	target, amount, _, _, businessRefID, ok := d.prepareControlCreditCommand(c)
	if !ok {
		return
	}
	result, err := d.Credits.Grant(c.Request.Context(), creditsports.GrantCommand{
		BillingAccountID: target.billingAccountID, Amount: amount,
		Metadata: controlCreditMetadata(ctx, "control_grant", businessRefID),
	})
	if err != nil {
		writeControlCreditError(c, err)
		return
	}
	renderControlCreditResult(c, result, target)
}

func (d *ContractControlDeps) deductControlCredits(c *gin.Context, ctx controlContext) {
	if d == nil || d.Credits == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable8)
		return
	}
	target, amount, _, _, businessRefID, ok := d.prepareControlCreditCommand(c)
	if !ok {
		return
	}
	result, err := d.Credits.Deduct(c.Request.Context(), creditsports.DeductionCommand{
		BillingAccountID: target.billingAccountID, Amount: amount,
		Metadata: controlCreditMetadata(ctx, "control_deduction", businessRefID),
	})
	if err != nil {
		writeControlCreditError(c, err)
		return
	}
	renderControlCreditResult(c, result, target)
}

func (d *ContractControlDeps) refundControlCredits(c *gin.Context, ctx controlContext) {
	if d == nil || d.Credits == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable8)
		return
	}
	target, amount, chargeID, _, businessRefID, ok := d.prepareControlCreditCommand(c)
	if !ok {
		return
	}
	if chargeID == "" {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest11)
		return
	}
	result, err := d.Credits.Refund(c.Request.Context(), creditsports.RefundCommand{
		BillingAccountID: target.billingAccountID, ChargeTransactionID: chargeID, Amount: amount,
		Metadata: controlCreditMetadata(ctx, "control_refund", businessRefID),
	})
	if err != nil {
		writeControlCreditError(c, err)
		return
	}
	renderControlCreditResult(c, result, target)
}

func (d *ContractControlDeps) correctControlCredits(c *gin.Context, ctx controlContext) {
	if d == nil || d.Credits == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable8)
		return
	}
	target, amount, _, originalID, businessRefID, ok := d.prepareControlCreditCommand(c)
	if !ok {
		return
	}
	if originalID == "" {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest12)
		return
	}
	result, err := d.Credits.Correct(c.Request.Context(), creditsports.CorrectionCommand{
		BillingAccountID: target.billingAccountID, OriginalTransactionID: originalID, Amount: amount,
		Metadata: controlCreditMetadata(ctx, "control_correction", businessRefID),
	})
	if err != nil {
		writeControlCreditError(c, err)
		return
	}
	renderControlCreditResult(c, result, target)
}

func (d *ContractControlDeps) getControlCreditWallet(c *gin.Context, _ controlContext) {
	if d == nil || d.CreditReads == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable8)
		return
	}
	userID, targetPublicID, targetDisplayName, ok := d.resolveControlCreditUser(c, c.Param("user_id"))
	if !ok {
		return
	}
	accountID, err := d.CreditReads.ResolvePersonalAccount(c.Request.Context(), userID)
	if err != nil {
		writeControlCreditError(c, err)
		return
	}
	wallet, err := d.CreditReads.GetWallet(c.Request.Context(), accountID)
	if err != nil {
		writeControlCreditError(c, err)
		return
	}
	contract.RenderSuccess(c, newControlCreditWalletView(c, d, wallet, targetPublicID, targetDisplayName))
}

func newControlCreditWalletView(c *gin.Context, d *ContractControlDeps, wallet creditsdomain.WalletDetails, targetPublicID uuid.UUID, targetDisplayName string) controlCreditWallet {
	publicID, ownerType := d.controlBillingAccountIdentity(c, wallet.BillingAccountID)
	view := controlCreditWallet{
		UserID:             tenancy.FormatUserPublicID(targetPublicID),
		DisplayName:        targetDisplayName,
		BillingAccountID:   publicID,
		OwnerType:          ownerType,
		AvailableCredits:   wallet.Available,
		ReservedCredits:    wallet.Reserved,
		Status:             string(wallet.Status),
		ActiveRestrictions: make([]controlWalletRestriction, 0, len(wallet.ActiveRestrictions)),
	}
	for _, restriction := range wallet.ActiveRestrictions {
		view.ActiveRestrictions = append(view.ActiveRestrictions, controlWalletRestriction{
			RestrictionID:    restriction.RestrictionID,
			UserID:           tenancy.FormatUserPublicID(targetPublicID),
			DisplayName:      targetDisplayName,
			BillingAccountID: publicID,
			Type:             restriction.Type,
			Source:           restriction.Source,
			RelatedObject:    restriction.RelatedObject,
			Reason:           restriction.Reason,
			CreatedBy:        restriction.CreatedBy,
			CreatedAt:        restriction.CreatedAt,
		})
	}
	return view
}

// newControlBillingAccountWalletView projects an explicitly addressed
// account wallet (#923): funds identity is the ba_ account with its owner
// type; no single-user linkage is attached.
func newControlBillingAccountWalletView(c *gin.Context, d *ContractControlDeps, wallet creditsdomain.WalletDetails, publicID, ownerType string) controlCreditWallet {
	view := controlCreditWallet{
		BillingAccountID:   publicID,
		OwnerType:          ownerType,
		AvailableCredits:   wallet.Available,
		ReservedCredits:    wallet.Reserved,
		Status:             string(wallet.Status),
		ActiveRestrictions: make([]controlWalletRestriction, 0, len(wallet.ActiveRestrictions)),
	}
	for _, restriction := range wallet.ActiveRestrictions {
		view.ActiveRestrictions = append(view.ActiveRestrictions, controlWalletRestriction{
			RestrictionID:    restriction.RestrictionID,
			BillingAccountID: publicID,
			Type:             restriction.Type,
			Source:           restriction.Source,
			RelatedObject:    restriction.RelatedObject,
			Reason:           restriction.Reason,
			CreatedBy:        restriction.CreatedBy,
			CreatedAt:        restriction.CreatedAt,
		})
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
	accountID, err := d.CreditReads.ResolvePersonalAccount(c.Request.Context(), userID)
	if err != nil {
		writeControlCreditError(c, err)
		return
	}
	d.listControlLedgerForAccount(c, accountID)
}

// getControlBillingAccountWallet reads one explicitly addressed account
// wallet by its ba_ id (#923). It is the Admin repair/audit read for
// shared and organization accounts that no single usr_ names.
func (d *ContractControlDeps) getControlBillingAccountWallet(c *gin.Context, _ controlContext) {
	if d == nil || d.CreditReads == nil || d.BillingAccounts == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable8)
		return
	}
	publicID, ok := parseControlBillingAccountID(c, c.Param("billing_account_id"))
	if !ok {
		return
	}
	account, err := d.BillingAccounts.GetBillingAccountByPublicID(c.Request.Context(), publicID)
	if err != nil {
		writeControlBillingAccountError(c, err)
		return
	}
	wallet, err := d.CreditReads.GetWallet(c.Request.Context(), account.InternalID)
	if err != nil {
		writeControlCreditError(c, err)
		return
	}
	contract.RenderSuccess(c, newControlBillingAccountWalletView(c, d, wallet, account.PublicID, account.OwnerType))
}

// listControlBillingAccountLedger pages the Ledger of one explicitly
// addressed account by its ba_ id (#923).
func (d *ContractControlDeps) listControlBillingAccountLedger(c *gin.Context, _ controlContext) {
	if d == nil || d.CreditReads == nil || d.BillingAccounts == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable8)
		return
	}
	publicID, ok := parseControlBillingAccountID(c, c.Param("billing_account_id"))
	if !ok {
		return
	}
	account, err := d.BillingAccounts.GetBillingAccountByPublicID(c.Request.Context(), publicID)
	if err != nil {
		writeControlBillingAccountError(c, err)
		return
	}
	d.listControlLedgerForAccount(c, account.InternalID)
}

func (d *ContractControlDeps) listControlLedgerForAccount(c *gin.Context, accountID int64) {
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
		BillingAccountID: accountID,
		BeforeSequence:   before,
		Limit:            limit,
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
