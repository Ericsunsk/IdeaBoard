package v1

// Member cross-workspace consumption quota surface (issue #946, #953).
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
// New members default to unlimited (absent row reads as unlimited);
// invites never gate on a default (#953 retires the #947 default surface).
// Block reasons surface on quote/submit as 402 task_insufficient_credits
// vs 403 member_quota_exhausted vs 403 billing_wallet_restricted.
//
// Role matrix (#953, ADR-0038): Organization Owner is always unlimited
// (the settlement check short-circuits, and capped writes for an Owner
// render 403); Owner may set Admin quotas; Owner/Admin may set Member
// quotas; Admin cannot edit Owner/Admin quotas (403). Personal scopes
// aggregate by (Personal Billing Account, User) with the account holder
// as manager (Billing Owner gate; space-only admins render
// billing_account_forbidden).
//
// Agreed surface (same prefix/chain as the workspace billing group):
//
//	GET /workspaces/{workspace_id}/member-quotas (managers only:
//	      Organization Owner/Admin on org scopes, Billing Owner on
//	      personal scopes; lists every explicitly set quota on the resolved
//	      account with live usage + remaining; other members render
//	      billing_account_forbidden)
//	GET /workspaces/{workspace_id}/member-quotas/self (any member reads
//	      their own quota with live usage + remaining on the resolved
//	      account; absent means unlimited; Organization Owners always read
//	      unlimited)
//	PUT /workspaces/{workspace_id}/member-quotas/{user_id} (managers only
//	      per the same matrix; Admin editing Owner/Admin renders 403;
//	      capped writes for an Owner render 403; no Idempotency-Key, the
//	      set is naturally idempotent)
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
// New members default to unlimited (absent row reads as unlimited);
// invites never gate on a default (#953 retires the #947 default surface).
type memberQuotaStore interface {
	memberquotas.ReadStore
	SetQuota(ctx context.Context, billingAccountID, userID int64, period string, limitCredits *int64, updatedByUserID int64) (memberquotas.SetResult, error)
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

func quotaReadingDTO(billingAccountPublicID, userPublicID string, reading memberquotas.Reading) v1MemberQuotaDTO {
	dto := v1MemberQuotaDTO{
		BillingAccountID: billingAccountPublicID,
		UserID:           userPublicID,
		Period:           reading.Period,
		UsedCredits:      reading.Used,
	}
	if reading.Limit != nil {
		limit := *reading.Limit
		dto.LimitCredits = &limit
		remaining := limit - reading.Used
		if remaining < 0 {
			remaining = 0
		}
		dto.RemainingCredits = &remaining
	}
	if reading.Start != nil && !reading.Start.IsZero() {
		start := reading.Start.UTC().Format(time.RFC3339)
		dto.PeriodStart = &start
	}
	if reading.End != nil && !reading.End.IsZero() {
		end := reading.End.UTC().Format(time.RFC3339)
		dto.PeriodEnd = &end
	}
	return dto
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

func (h *contractV1BillingHandler) quotaPolicy() memberquotas.Policy {
	return memberquotas.Policy{OrganizationRoles: h.deps.Tenancy}
}

func (h *contractV1BillingHandler) getMemberQuotaSelf(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx) {
		workspace := wctx.Workspace
		if h.deps.Quotas == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
			c.Abort()
			return
		}
		dto, ok := h.readMemberQuotaSelf(c, workspace, wctx.Principal.UserID())
		if ok {
			contract.RenderSuccess(c, dto)
		}
	})
}

// requireQuotaManager enforces the #953 write/list matrix and returns the
// resolved account. Personal scopes require the Billing Owner gate
// (the holder manages collaborators); Organization scopes require Org
// Owner/Admin (Admins need not hold a Billing grant to manage Members).
func (h *contractV1BillingHandler) requireQuotaManager(c *gin.Context, workspace *tenancy.Workspace, actorUserID int64) (tenancy.BillingAccount, bool) {
	account, resolved := h.billingAccountForWorkspace(c, workspace)
	if !resolved {
		return tenancy.BillingAccount{}, false
	}
	allowed, err := h.canManageMemberQuotas(c.Request.Context(), workspace, account.InternalID, actorUserID)
	if err != nil {
		renderBillingAccountError(c, err)
		return tenancy.BillingAccount{}, false
	}
	if !allowed {
		contract.RenderError(c, http.StatusForbidden, contract.CodeBillingAccountForbidden, runtimehttp.MsgBillingAccountForbidden)
		c.Abort()
		return tenancy.BillingAccount{}, false
	}
	return account, true
}

// canManageMemberQuotas supplies the same capability to the account projection
// and the quota endpoint gate; consumption visibility is not authorization.
func (h *contractV1BillingHandler) canManageMemberQuotas(ctx context.Context, workspace *tenancy.Workspace, accountID, actorUserID int64) (bool, error) {
	personalOwner := false
	if workspace.OrganizationID == nil {
		role, member, err := h.deps.BillingAccounts.GetBillingAccountRole(ctx, accountID, actorUserID)
		if err != nil {
			return false, err
		}
		personalOwner = member && role == tenancy.BillingRoleOwner
	}
	return h.quotaPolicy().CanManage(ctx, memberquotas.ScopeForWorkspace(workspace), actorUserID, personalOwner)
}

// checkQuotaTargetRole delegates the #953 write matrix to the quota policy
// and maps its result to the HTTP contract. requireQuotaManager supplies
// the Personal holder fact; period is the validated request period.
func (h *contractV1BillingHandler) checkQuotaTargetRole(c *gin.Context, workspace *tenancy.Workspace, actorUserID, targetUserID int64, period string) bool {
	// requireQuotaManager already established the Personal holder fact.
	allowed, err := h.quotaPolicy().CanSet(c.Request.Context(), memberquotas.ScopeForWorkspace(workspace), actorUserID, targetUserID, period, workspace.OrganizationID == nil)
	if err != nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
		c.Abort()
		return false
	}
	if !allowed {
		contract.RenderError(c, http.StatusForbidden, contract.CodeBillingAccountForbidden, runtimehttp.MsgBillingAccountForbidden)
		c.Abort()
		return false
	}
	return true
}

// quotaPageDTOs maps the shared normalized read to the wire contract.
func (h *contractV1BillingHandler) quotaPageDTOs(ctx context.Context, c *gin.Context, scope memberquotas.Scope, account tenancy.BillingAccount, rows []memberquotas.QuotaRow) ([]v1MemberQuotaDTO, bool) {
	readings, err := (memberquotas.Reader{Store: h.deps.Quotas, Policy: h.quotaPolicy()}).Page(ctx, scope, account.InternalID, rows)
	if err != nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
		c.Abort()
		return nil, false
	}
	ids := make([]int64, 0, len(readings))
	for _, reading := range readings {
		ids = append(ids, reading.UserID)
	}
	publicIDs, ok := batchUserPublicIDs(ctx, c, h.deps.Users, ids)
	if !ok {
		return nil, false
	}
	page := make([]v1MemberQuotaDTO, 0, len(readings))
	for _, reading := range readings {
		page = append(page, quotaReadingDTO(account.PublicID, publicIDs[reading.UserID], reading))
	}
	return page, true
}

func (h *contractV1BillingHandler) listMemberQuotas(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx) {
		workspace := wctx.Workspace
		if h.deps.Quotas == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
			c.Abort()
			return
		}
		// ?scope=self shortcut: any member reads only their own quota
		// through the list envelope (single-item page), without the
		// manager gate the full list requires. Unknown scopes render
		// 400 bad_request (canonical generic bad params).
		if scope := strings.TrimSpace(c.Query("scope")); scope != "" {
			if scope != "self" {
				contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeBadRequest), contract.CodeBadRequest,
					runtimehttp.MsgBadRequest4, contract.FieldErrors{Fields: map[string]string{"scope": "invalid"}})
				c.Abort()
				return
			}
			dto, ok := h.readMemberQuotaSelf(c, workspace, wctx.Principal.UserID())
			if ok {
				contract.RenderSuccess(c, []v1MemberQuotaDTO{dto})
			}
			return
		}
		account, ok := h.requireQuotaManager(c, workspace, wctx.Principal.UserID())
		if !ok {
			return
		}
		rows, err := h.deps.Quotas.ListQuotas(c.Request.Context(), account.InternalID)
		if err != nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
			c.Abort()
			return
		}
		page, ok := h.quotaPageDTOs(c.Request.Context(), c, memberquotas.ScopeForWorkspace(workspace), account, rows)
		if !ok {
			return
		}
		contract.RenderSuccess(c, page)
	})
}

func (h *contractV1BillingHandler) readMemberQuotaSelf(c *gin.Context, workspace *tenancy.Workspace, userID int64) (v1MemberQuotaDTO, bool) {
	account, resolved := h.billingAccountForWorkspace(c, workspace)
	if !resolved {
		return v1MemberQuotaDTO{}, false
	}
	reading, err := (memberquotas.Reader{Store: h.deps.Quotas, Policy: h.quotaPolicy()}).Self(c.Request.Context(), memberquotas.ScopeForWorkspace(workspace), account.InternalID, userID)
	if err != nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
		c.Abort()
		return v1MemberQuotaDTO{}, false
	}
	userPublicID, ok := h.memberQuotaPublicID(c, userID)
	if !ok {
		return v1MemberQuotaDTO{}, false
	}
	return quotaReadingDTO(account.PublicID, userPublicID, reading), true
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
		// Role matrix (#953): Personal scopes require the Billing Owner
		// gate (the holder manages collaborators; space-only admins render
		// billing_account_forbidden). Organization scopes require Org
		// Owner/Admin. API and UI permissions agree off the same read.
		account, ok := h.requireQuotaManager(c, workspace, wctx.Principal.UserID())
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
		if !h.checkQuotaTargetRole(c, workspace, wctx.Principal.UserID(), targetUserID, period) {
			return
		}
		saved, err := h.deps.Quotas.SetQuota(c.Request.Context(), account.InternalID, targetUserID, period, body.Data.LimitCredits, wctx.Principal.UserID())
		if err != nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
			c.Abort()
			return
		}
		reading, err := (memberquotas.Reader{Store: h.deps.Quotas}).Read(c.Request.Context(), account.InternalID, targetUserID, saved.Period, saved.LimitCredits)
		if err != nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
			c.Abort()
			return
		}
		userPublicID, ok := h.memberQuotaPublicID(c, targetUserID)
		if !ok {
			return
		}
		contract.RenderSuccess(c, quotaReadingDTO(account.PublicID, userPublicID, reading))
	})
}
