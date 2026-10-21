package v1

// Member cross-workspace consumption quota surface (issue #946).
//
// A Billing Account holds the unified balance; a Workspace records the
// consumption location and the User records the real operator. The quota
// caps one member's settled + pending consumption across every Workspace
// sharing the account (paid Task generations + Vision Search) per window:
// daily / weekly / monthly (UTC calendar day, Monday-start week, calendar
// month), fixed (lifetime cumulative cap), or unlimited (no cap, the
// default). There are no sub-wallets, no transfers, and no reservations:
// adjusting the limit never clears usage, unused periodic quota never
// rolls into the next window, and joining or rejoining never re-issues.
// Block reasons surface on quote/submit as 402 task_insufficient_credits
// vs 403 workspace_budget_exhausted vs 403 member_quota_exhausted vs 403
// billing_wallet_restricted.
//
// Agreed surface (same prefix/chain as the workspace billing group):
//
//	GET /workspaces/{workspace_id}/member-quotas (Billing Owner/Admin only:
//	      lists every explicitly set quota on the resolved account with
//	      live usage + remaining; other members render
//	      billing_account_forbidden)
//	GET /workspaces/{workspace_id}/member-quotas/self (any member reads
//	      their own quota with live usage + remaining on the resolved
//	      account; absent means unlimited)
//	PUT /workspaces/{workspace_id}/member-quotas/{user_id} (Billing
//	      Owner/Admin only: space-only Workspace admins render
//	      billing_account_forbidden, so a Workspace admin cannot raise a
//	      pool-granted quota; no Idempotency-Key, the set is naturally
//	      idempotent)
//
// Ownership: ONLY the quota row is written here (period + limit +
// updated_by). Usage is derived live from immutable settlement facts
// (task snapshots/settlements, vision operations, ledger charges); this
// file performs NO ledger writes.

import (
	"context"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/memberquotas"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

// memberQuotaStore persists the member-level caps and derives live usage.
// Production wires the memberquotas Store; tests stub this interface.
type memberQuotaStore interface {
	GetQuotaRow(ctx context.Context, billingAccountID, userID int64) (string, *int64, bool, error)
	SetQuota(ctx context.Context, billingAccountID, userID int64, period string, limitCredits *int64, updatedByUserID int64) (memberquotas.SetResult, error)
	ListQuotas(ctx context.Context, billingAccountID int64) ([]memberquotas.QuotaRow, error)
	UsageForPeriod(ctx context.Context, billingAccountID, userID int64, periodStart, periodEnd *time.Time) (int64, error)
	// GetDefaultPolicy reads the account default new-member policy (#947).
	// ok=false means NOT CONFIGURED (fail closed, never implicit unlimited).
	GetDefaultPolicy(ctx context.Context, billingAccountID int64) (string, *int64, bool, error)
	// SetDefaultPolicy upserts the account default (#947, Billing Owner/Admin
	// only). Changing it never touches existing per-member rows.
	SetDefaultPolicy(ctx context.Context, billingAccountID int64, period string, limitCredits *int64, updatedByUserID int64) (memberquotas.SetResult, error)
}

type v1MemberQuotaDTO struct {
	BillingAccountID string  `json:"billing_account_id"`
	UserID           string  `json:"user_id"`
	Period           string  `json:"period"`
	LimitCredits     *int64  `json:"limit_credits,omitempty"`
	UsedCredits      int64   `json:"used_credits"`
	RemainingCredits *int64  `json:"remaining_credits,omitempty"`
	PeriodStart      *string `json:"period_start,omitempty"`
	PeriodEnd        *string `json:"period_end,omitempty"`
}

func toMemberQuotaDTO(billingAccountPublicID, userPublicID, period string, limitCredits *int64, used int64, periodStart, periodEnd *time.Time) v1MemberQuotaDTO {
	dto := v1MemberQuotaDTO{
		BillingAccountID: billingAccountPublicID,
		UserID:           userPublicID,
		Period:           period,
		UsedCredits:      used,
	}
	if limitCredits != nil {
		limit := *limitCredits
		dto.LimitCredits = &limit
		remaining := limit - used
		if remaining < 0 {
			remaining = 0
		}
		dto.RemainingCredits = &remaining
	}
	if periodStart != nil && !periodStart.IsZero() {
		start := periodStart.UTC().Format(time.RFC3339)
		dto.PeriodStart = &start
	}
	if periodEnd != nil && !periodEnd.IsZero() {
		end := periodEnd.UTC().Format(time.RFC3339)
		dto.PeriodEnd = &end
	}
	return dto
}

// quotaWindowFor returns the live usage window for a quota period at now.
// Fixed quotas are unbounded (all time); unlimited carries no window.
func quotaWindowFor(period string, limitCredits *int64) (start, end *time.Time, bounded bool) {
	if memberquotas.IsUnlimited(period, limitCredits) {
		return nil, nil, false
	}
	windowStart, windowEnd, ok := memberquotas.CurrentBounds(period, time.Now().UTC())
	if !ok {
		return nil, nil, false
	}
	return windowStart, windowEnd, true
}

func (h *contractV1BillingHandler) memberQuotaReading(c *gin.Context, billingAccountID, userID int64, period string, limitCredits *int64) (int64, *time.Time, *time.Time, bool) {
	start, end, _ := quotaWindowFor(period, limitCredits)
	used, err := h.deps.Quotas.UsageForPeriod(c.Request.Context(), billingAccountID, userID, start, end)
	if err != nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
		c.Abort()
		return 0, nil, nil, false
	}
	return used, start, end, true
}

func (h *contractV1BillingHandler) memberQuotaPublicID(c *gin.Context, userID int64) (string, bool) {
	if h.deps.Users == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
		c.Abort()
		return "", false
	}
	publicID, err := h.deps.Users.GetUserPublicID(c.Request.Context(), userID)
	if err != nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
		c.Abort()
		return "", false
	}
	return tenancy.FormatUserPublicID(publicID), true
}

func (h *contractV1BillingHandler) getMemberQuotaSelf(c *gin.Context) {
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
		userID := wctx.Principal.UserID()
		period, limit, _, err := h.deps.Quotas.GetQuotaRow(c.Request.Context(), account.InternalID, userID)
		if err != nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
			c.Abort()
			return
		}
		if period == "" {
			period = memberquotas.PeriodUnlimited
		}
		used, start, end, ok := h.memberQuotaReading(c, account.InternalID, userID, period, limit)
		if !ok {
			return
		}
		userPublicID, ok := h.memberQuotaPublicID(c, userID)
		if !ok {
			return
		}
		contract.RenderSuccess(c, toMemberQuotaDTO(account.PublicID, userPublicID, period, limit, used, start, end))
	})
}

func (h *contractV1BillingHandler) listMemberQuotas(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx) {
		workspace := wctx.Workspace
		if h.deps.Quotas == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
			c.Abort()
			return
		}
		// Financial management (#915): listing every member's quota
		// requires Billing Owner/Admin on the resolved account. Members
		// read only their own row via the self endpoint; workspace
		// standing alone never grants the account-wide view.
		account, ok := h.requireBillingManager(c, workspace, wctx.Principal.UserID())
		if !ok {
			return
		}
		rows, err := h.deps.Quotas.ListQuotas(c.Request.Context(), account.InternalID)
		if err != nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
			c.Abort()
			return
		}
		page := make([]v1MemberQuotaDTO, 0, len(rows))
		for _, row := range rows {
			period := row.Period
			if period == "" {
				period = memberquotas.PeriodUnlimited
			}
			used, start, end, ok := h.memberQuotaReading(c, account.InternalID, row.UserID, period, row.LimitCredits)
			if !ok {
				return
			}
			userPublicID, ok := h.memberQuotaPublicID(c, row.UserID)
			if !ok {
				return
			}
			page = append(page, toMemberQuotaDTO(account.PublicID, userPublicID, period, row.LimitCredits, used, start, end))
		}
		contract.RenderSuccess(c, page)
	})
}

type v1SetMemberQuotaBody struct {
	Data struct {
		Period       string `json:"period"`
		LimitCredits *int64 `json:"limit_credits"`
	} `json:"data"`
}

func (h *contractV1BillingHandler) setMemberQuota(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx) {
		workspace := wctx.Workspace
		if h.deps.Quotas == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
			c.Abort()
			return
		}
		// Financial management (#915): setting a quota requires
		// Billing Owner/Admin on the resolved account. Workspace
		// Owner/Admin standing alone (space-only admin) renders
		// billing_account_forbidden and cannot raise a pool-granted
		// quota. API and UI permissions agree: the frontend gates the
		// same capability off the billing-account read.
		account, ok := h.requireBillingManager(c, workspace, wctx.Principal.UserID())
		if !ok {
			return
		}
		targetUserID, ok := h.resolveBillingMemberUserID(c, c.Param("user_id"))
		if !ok {
			return
		}
		var body v1SetMemberQuotaBody
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
				runtimehttp.MsgSetMemberQuota, contract.FieldErrors{Fields: fields})
			c.Abort()
			return
		}
		saved, err := h.deps.Quotas.SetQuota(c.Request.Context(), account.InternalID, targetUserID, period, body.Data.LimitCredits, wctx.Principal.UserID())
		if err != nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
			c.Abort()
			return
		}
		used, start, end, ok := h.memberQuotaReading(c, account.InternalID, targetUserID, saved.Period, saved.LimitCredits)
		if !ok {
			return
		}
		userPublicID, ok := h.memberQuotaPublicID(c, targetUserID)
		if !ok {
			return
		}
		contract.RenderSuccess(c, toMemberQuotaDTO(account.PublicID, userPublicID, saved.Period, saved.LimitCredits, used, start, end))
	})
}
