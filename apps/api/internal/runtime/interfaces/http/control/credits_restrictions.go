package control

// Platform-Admin wallet restriction operations on explicit Billing Accounts
// (#923).
//
// Restriction writes go through the typed credits.WalletRestrictionCommands
// seam only (the same seam Reconciliation uses for automatic isolation):
// no handler touches credit_wallet_restrictions directly. Creation binds
// one (Billing Account, type, source, related object); release addresses
// exactly one restriction_id, so resolving one Finding never clears another
// source's restriction and never touches sibling accounts of the same Owner.
// The Ledger Actor on both paths is the platform-Admin operator.

import (
	"errors"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"net/http"
	"strings"

	creditsdomain "ideaboard/api/internal/credits/domain"
	creditsports "ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/platform/contract"

	"github.com/gin-gonic/gin"
)

type controlRestrictWalletRequest struct {
	BillingAccountID  string `json:"billing_account_id"`
	Type              string `json:"type"`
	Source            string `json:"source"`
	RelatedObjectType string `json:"related_object_type"`
	RelatedObjectID   string `json:"related_object_id"`
	Reason            string `json:"reason"`
}

type controlReleaseWalletRestrictionRequest struct {
	Reason string `json:"reason"`
}

type controlWalletRestrictionResult struct {
	RestrictionID    string `json:"restriction_id"`
	Replayed         bool   `json:"replayed"`
	BillingAccountID string `json:"billing_account_id,omitempty"`
}

// writeControlRestrictionError maps restriction domain failures to contract
// codes. Unknown accounts and restrictions converge to 404 with
// account-scoped copy (never the personal-wallet wording).
func writeControlRestrictionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, creditsdomain.ErrInvalidCommand):
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest4)
	case errors.Is(err, creditsdomain.ErrWalletNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, runtimehttp.MsgControlNotFound9)
	case errors.Is(err, creditsdomain.ErrWalletRestrictionNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, runtimehttp.MsgControlNotFound10)
	default:
		writeControlCreditError(c, err)
	}
}

// restrictControlWallet creates one reason-coded spending limit on the
// explicitly addressed Billing Account.
func (d *ContractControlDeps) restrictControlWallet(c *gin.Context, ctx controlContext) {
	if d == nil || d.Restrictions == nil || d.BillingAccounts == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable8)
		return
	}
	var req controlRestrictWalletRequest
	if err := contract.BindStrictJSON(c, &req); err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest4)
		return
	}
	publicID, ok := parseControlBillingAccountID(c, req.BillingAccountID)
	if !ok {
		return
	}
	account, err := d.BillingAccounts.GetBillingAccountByPublicID(c.Request.Context(), publicID)
	if err != nil {
		writeControlBillingAccountError(c, err)
		return
	}
	_, operatorID := controlOperatorIdentity(ctx.Operator)
	change, err := d.Restrictions.RestrictWallet(c.Request.Context(), creditsports.RestrictWalletCommand{
		BillingAccountID: account.InternalID,
		Type:             strings.TrimSpace(req.Type),
		Source:           strings.TrimSpace(req.Source),
		RelatedObject: creditsdomain.BusinessReference{
			Type: strings.TrimSpace(req.RelatedObjectType),
			ID:   strings.TrimSpace(req.RelatedObjectID),
		},
		Reason: strings.TrimSpace(req.Reason),
		Actor:  creditsdomain.Actor{Type: creditsdomain.ActorAdmin, ID: operatorID},
	})
	if err != nil {
		writeControlRestrictionError(c, err)
		return
	}
	contract.RenderSuccess(c, controlWalletRestrictionResult{
		RestrictionID:    change.RestrictionID,
		Replayed:         change.Replayed,
		BillingAccountID: account.PublicID,
	})
}

// releaseControlWalletRestriction releases exactly the addressed
// restriction_id, keeping its history. Sibling restrictions on the same
// account and restrictions on other accounts are untouched.
func (d *ContractControlDeps) releaseControlWalletRestriction(c *gin.Context, ctx controlContext) {
	if d == nil || d.Restrictions == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable8)
		return
	}
	restrictionID := strings.TrimSpace(c.Param("restriction_id"))
	if restrictionID == "" {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest38)
		return
	}
	var req controlReleaseWalletRestrictionRequest
	if err := contract.BindStrictJSON(c, &req); err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest4)
		return
	}
	_, operatorID := controlOperatorIdentity(ctx.Operator)
	change, err := d.Restrictions.ReleaseWalletRestriction(c.Request.Context(), creditsports.ReleaseWalletRestrictionCommand{
		RestrictionID: restrictionID,
		Reason:        strings.TrimSpace(req.Reason),
		Actor:         creditsdomain.Actor{Type: creditsdomain.ActorAdmin, ID: operatorID},
	})
	if err != nil {
		writeControlRestrictionError(c, err)
		return
	}
	contract.RenderSuccess(c, controlWalletRestrictionResult{
		RestrictionID: change.RestrictionID,
		Replayed:      change.Replayed,
	})
}
