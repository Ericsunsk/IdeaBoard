package v1

// Workspace shared consumption budget surface (issue #945).
//
// A Billing Account holds the unified balance; a Workspace records the
// consumption location. The budget caps how much one Workspace's members
// may jointly consume (paid Task generations + Vision Search) per period:
// monthly (UTC calendar month) or unlimited (no cap, the default). There
// are no sub-wallets, no transfers, and no reservations: adjusting the
// limit never clears usage, and a monthly reset starts a new UTC month
// without deleting history.
//
// Agreed surface (same prefix/chain as the workspace billing group):
//
//	GET /workspaces/{workspace_id}/budget (any member reads the cap with
//	      live usage + remaining; block reasons surface on quote/submit
//	      as 402 task_insufficient_credits vs 403
//	      workspace_budget_exhausted vs 403 billing_wallet_restricted)
//	PUT /workspaces/{workspace_id}/budget (Billing Owner/Admin only:
//	      space-only Workspace admins render billing_account_forbidden,
//	      so a Workspace admin cannot raise a pool-granted budget; no
//	      Idempotency-Key, the set is naturally idempotent)
//
// Ownership: ONLY the budget row is written here (period + limit +
// updated_by). Usage is derived live from immutable settlement facts
// (task snapshots/settlements, vision operations, ledger charges); this
// file performs NO ledger writes.

import (
	"context"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/workspacebudgets"

	"github.com/gin-gonic/gin"
)

// workspaceBudgetStore persists the workspace-level cap and derives live
// usage. Production wires the workspacebudgets Store; tests stub this
// interface. Resolution joins the caller transaction inside the store for
// settlement checks; these handler reads use committed facts only.
type workspaceBudgetStore interface {
	GetBudgetRow(ctx context.Context, workspaceInternalID int64) (string, *int64, bool, error)
	SetBudget(ctx context.Context, workspaceInternalID int64, period string, limitCredits *int64, updatedByUserID int64) (workspacebudgets.SetResult, error)
	UsageForPeriod(ctx context.Context, workspaceInternalID int64, periodStart, periodEnd *time.Time) (int64, error)
}

type v1WorkspaceBudgetDTO struct {
	WorkspaceID      string  `json:"workspace_id"`
	BillingAccountID string  `json:"billing_account_id"`
	Period           string  `json:"period"`
	LimitCredits     *int64  `json:"limit_credits,omitempty"`
	UsedCredits      int64   `json:"used_credits"`
	RemainingCredits *int64  `json:"remaining_credits,omitempty"`
	PeriodStart      *string `json:"period_start,omitempty"`
	PeriodEnd        *string `json:"period_end,omitempty"`
}

func toWorkspaceBudgetDTO(workspacePublicID, billingAccountPublicID, period string, limitCredits *int64, used int64, periodStart, periodEnd *time.Time) v1WorkspaceBudgetDTO {
	dto := v1WorkspaceBudgetDTO{
		WorkspaceID:      workspacePublicID,
		BillingAccountID: billingAccountPublicID,
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

func (h *contractV1BillingHandler) workspaceBudgetReading(c *gin.Context, workspaceInternalID int64, period string, limitCredits *int64) (int64, *time.Time, *time.Time, bool) {
	var start, end *time.Time
	if !workspacebudgets.IsUnlimited(period, limitCredits) {
		boundStart, boundEnd := workspacebudgets.CurrentMonthBounds(time.Now().UTC())
		start, end = &boundStart, &boundEnd
	}
	used, err := h.deps.Budgets.UsageForPeriod(c.Request.Context(), workspaceInternalID, start, end)
	if err != nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
		c.Abort()
		return 0, nil, nil, false
	}
	return used, start, end, true
}

func (h *contractV1BillingHandler) getWorkspaceBudget(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx) {
		workspace := wctx.Workspace
		if h.deps.Budgets == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
			c.Abort()
			return
		}
		account, resolved := h.billingAccountForWorkspace(c, workspace)
		if !resolved {
			return
		}
		period, limit, _, err := h.deps.Budgets.GetBudgetRow(c.Request.Context(), workspace.InternalID)
		if err != nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
			c.Abort()
			return
		}
		if period == "" {
			period = workspacebudgets.PeriodUnlimited
		}
		used, start, end, ok := h.workspaceBudgetReading(c, workspace.InternalID, period, limit)
		if !ok {
			return
		}
		contract.RenderSuccess(c, toWorkspaceBudgetDTO(workspace.PublicID, account.PublicID, period, limit, used, start, end))
	})
}

type v1SetWorkspaceBudgetBody struct {
	Data struct {
		Period       string `json:"period"`
		LimitCredits *int64 `json:"limit_credits"`
	} `json:"data"`
}

func (h *contractV1BillingHandler) setWorkspaceBudget(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx) {
		workspace := wctx.Workspace
		if h.deps.Budgets == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
			c.Abort()
			return
		}
		// Financial management (#915): setting the budget requires
		// Billing Owner/Admin on the resolved account. Workspace
		// Owner/Admin standing alone (space-only admin) renders
		// billing_account_forbidden and cannot raise a pool-granted
		// budget. API and UI permissions agree: the frontend gates the
		// same capability off the billing-account read.
		account, ok := h.requireBillingManager(c, workspace, wctx.Principal.UserID())
		if !ok {
			return
		}
		var body v1SetWorkspaceBudgetBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
			return
		}
		period := strings.TrimSpace(body.Data.Period)
		fields := map[string]string{}
		switch period {
		case workspacebudgets.PeriodMonthly:
			if body.Data.LimitCredits == nil || *body.Data.LimitCredits <= 0 || *body.Data.LimitCredits > workspacebudgets.MaxLimitCredits {
				fields["limit_credits"] = "invalid"
			}
		case workspacebudgets.PeriodUnlimited:
			if body.Data.LimitCredits != nil {
				fields["limit_credits"] = "invalid"
			}
		default:
			fields["period"] = "invalid"
		}
		if len(fields) > 0 {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				runtimehttp.MsgSetWorkspaceBudget, contract.FieldErrors{Fields: fields})
			c.Abort()
			return
		}
		saved, err := h.deps.Budgets.SetBudget(c.Request.Context(), workspace.InternalID, period, body.Data.LimitCredits, wctx.Principal.UserID())
		if err != nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
			c.Abort()
			return
		}
		used, start, end, ok := h.workspaceBudgetReading(c, workspace.InternalID, saved.Period, saved.LimitCredits)
		if !ok {
			return
		}
		contract.RenderSuccess(c, toWorkspaceBudgetDTO(workspace.PublicID, account.PublicID, saved.Period, saved.LimitCredits, used, start, end))
	})
}
