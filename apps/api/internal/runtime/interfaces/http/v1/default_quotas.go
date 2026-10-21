package v1

// Account default new-member consumption quota surface (issue #947).
//
// A Billing Account holds the unified balance; a Workspace records the
// consumption location and the User records the real operator. The default
// seeds the per-member quota row on first join of the account consumption
// scope (same five periods as #946: daily / weekly / monthly UTC windows,
// fixed lifetime cap, or unlimited). Absent default means NOT CONFIGURED
// (fail closed: invites render 409 default_member_quota_not_configured,
// never implicit unlimited). Changing the default only affects later
// joins; existing per-member rows keep their cap and accrued usage
// (per-member SetQuota). Only Billing Owner/Admin writes (space-only
// Workspace admins render billing_account_forbidden); any member reads.
//
// Agreed surface (same prefix/chain as the workspace billing group):
//
//	GET /workspaces/{workspace_id}/default-member-quota (any member reads
//	      the default; absent renders 409 default_member_quota_not_configured)
//	PUT /workspaces/{workspace_id}/default-member-quota (Billing Owner/Admin
//	      only; no Idempotency-Key, the set is naturally idempotent)
//
// Ownership: ONLY the default row is written here (period + limit +
// updated_by). Usage is derived live from immutable settlement facts;
// this file performs NO ledger writes.

import (
	"net/http"
	"strings"

	"ideaboard/api/internal/memberquotas"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"

	"github.com/gin-gonic/gin"
)

type v1DefaultMemberQuotaDTO struct {
	BillingAccountID string `json:"billing_account_id"`
	WorkspaceID      string `json:"workspace_id"`
	Period           string `json:"period"`
	LimitCredits     *int64 `json:"limit_credits,omitempty"`
}

func toDefaultMemberQuotaDTO(billingAccountPublicID, workspacePublicID, period string, limitCredits *int64) v1DefaultMemberQuotaDTO {
	dto := v1DefaultMemberQuotaDTO{
		BillingAccountID: billingAccountPublicID,
		WorkspaceID:      workspacePublicID,
		Period:           period,
	}
	if limitCredits != nil {
		limit := *limitCredits
		dto.LimitCredits = &limit
	}
	return dto
}

func renderDefaultQuotaNotConfigured(c *gin.Context) {
	contract.RenderError(c, http.StatusConflict, contract.CodeDefaultMemberQuotaNotConfigured, runtimehttp.MsgDefaultMemberQuotaNotConfigured)
	c.Abort()
}

func (h *contractV1BillingHandler) getDefaultMemberQuota(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx) {
		workspace := wctx.Workspace
		if h.deps.Quotas == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
			c.Abort()
			return
		}
		account, resolved := h.billingAccountForWorkspace(c, workspace)
		if !resolved {
			return
		}
		period, limit, ok, err := h.deps.Quotas.GetDefaultPolicy(c.Request.Context(), account.InternalID)
		if err != nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
			c.Abort()
			return
		}
		if !ok {
			renderDefaultQuotaNotConfigured(c)
			return
		}
		if period == "" {
			period = memberquotas.PeriodUnlimited
		}
		contract.RenderSuccess(c, toDefaultMemberQuotaDTO(account.PublicID, workspace.PublicID, period, limit))
	})
}

type v1SetDefaultMemberQuotaBody struct {
	Data struct {
		Period       string `json:"period"`
		LimitCredits *int64 `json:"limit_credits"`
	} `json:"data"`
}

func (h *contractV1BillingHandler) setDefaultMemberQuota(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx) {
		workspace := wctx.Workspace
		if h.deps.Quotas == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
			c.Abort()
			return
		}
		// Financial management (#915): setting the default requires
		// Billing Owner/Admin on the resolved account. Workspace
		// Owner/Admin standing alone (space-only admin) renders
		// billing_account_forbidden and cannot raise the pool default.
		// API and UI permissions agree: the frontend gates the same
		// capability off the billing-account read.
		account, ok := h.requireBillingManager(c, workspace, wctx.Principal.UserID())
		if !ok {
			return
		}
		var body v1SetDefaultMemberQuotaBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
			return
		}
		period := strings.TrimSpace(body.Data.Period)
		fields := map[string]string{}
		switch period {
		case memberquotas.PeriodDaily, memberquotas.PeriodWeekly, memberquotas.PeriodMonthly, memberquotas.PeriodFixed:
			if body.Data.LimitCredits == nil || *body.Data.LimitCredits <= 0 || *body.Data.LimitCredits > memberquotas.MaxLimitCredits {
				fields["limit_credits"] = "invalid"
			}
		case memberquotas.PeriodUnlimited:
			if body.Data.LimitCredits != nil {
				fields["limit_credits"] = "invalid"
			}
		default:
			fields["period"] = "invalid"
		}
		if len(fields) > 0 {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				runtimehttp.MsgDefaultMemberQuotaInvalid, contract.FieldErrors{Fields: fields})
			c.Abort()
			return
		}
		saved, err := h.deps.Quotas.SetDefaultPolicy(c.Request.Context(), account.InternalID, period, body.Data.LimitCredits, wctx.Principal.UserID())
		if err != nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
			c.Abort()
			return
		}
		contract.RenderSuccess(c, toDefaultMemberQuotaDTO(account.PublicID, workspace.PublicID, saved.Period, saved.LimitCredits))
	})
}
