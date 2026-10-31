package billing

// Workspace Billing Account management (ADR-0038): fixed creation-scope
// resolution plus financial-manager membership.
//
// Funds identity (ba_) is stable and never derives from the current
// Workspace Owner's personal wallet. Workspace records the consumption
// location, User records the actual operator, and Task persists all three.
// Exactly one Personal account exists per user and one shared account per
// Organization; Workspace scope is fixed at creation and never switches.
//
// Financial management never grants Board content access, and Workspace
// Owner/Admin standing alone never grants financial management. Personal
// member quotas require the account holder; Organization quotas follow
// the Organization Owner/Admin role matrix in memberquotas.Policy.

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/billingoverview"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type v1BillingMemberDTO struct {
	UserID    string `json:"user_id"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at,omitempty"`
	// Email is the manager-visible member email for display assistance.
	// It is omitted when unavailable and never acts as a write identity.
	Email string `json:"email,omitempty"`
	// Avatar is the manager-visible member avatar public URL. It is
	// omitted when the member has no custom avatar or the provider is
	// unavailable, and never fails the request.
	Avatar string `json:"avatar,omitempty"`
}

type v1AddBillingMemberBody struct {
	Data struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	} `json:"data"`
}

type v1UpdateBillingMemberBody struct {
	Data struct {
		Role string `json:"role"`
	} `json:"data"`
}

func (h *contractV1BillingHandler) toBillingMemberDTO(c *gin.Context, m *tenancy.BillingAccountMember, display map[int64]V1MemberDisplay) (v1BillingMemberDTO, bool) {
	if h.deps.Users == nil {
		contract.RenderError(c, http.StatusNotImplemented,
			contract.CodeContractNotImplemented, "The v1 billing surface is not implemented yet")
		c.Abort()
		return v1BillingMemberDTO{}, false
	}
	publicID, err := h.deps.Users.GetUserPublicID(c.Request.Context(), m.UserID)
	if err != nil || publicID == uuid.Nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
		c.Abort()
		return v1BillingMemberDTO{}, false
	}
	dto := v1BillingMemberDTO{UserID: tenancy.FormatUserPublicID(publicID), Role: m.Role}
	if !m.CreatedAt.IsZero() {
		dto.CreatedAt = m.CreatedAt.UTC().Format(time.RFC3339)
	}
	if email, avatar := applyDisplayFact(display, m.UserID); email != "" || avatar != "" {
		dto.Email, dto.Avatar = email, avatar
	}
	return dto, true
}

// toBillingMemberDTOs builds the billing manager list in three round trips
// total: the store list (caller) plus one identity batch and one display
// batch, replacing the per-member GetUserPublicID N+1 loop. Unknown
// identities fail closed with 503, matching the single-row path; display
// facts stay best-effort omitted.
func (h *contractV1BillingHandler) toBillingMemberDTOs(ctx context.Context, c *gin.Context, members []tenancy.BillingAccountMember) ([]v1BillingMemberDTO, bool) {
	people, err := h.readBillingPeople(ctx, userIDsOfBillingMembers(members))
	if err != nil {
		renderUserPublicIDsError(c, err)
		return nil, false
	}
	return billingMemberDTOs(members, people), true
}

func billingMemberDTOs(members []tenancy.BillingAccountMember, people map[int64]billingoverview.Person) []v1BillingMemberDTO {
	page := make([]v1BillingMemberDTO, 0, len(members))
	for _, m := range members {
		dto := v1BillingMemberDTO{UserID: people[m.UserID].PublicID, Role: m.Role}
		if !m.CreatedAt.IsZero() {
			dto.CreatedAt = m.CreatedAt.UTC().Format(time.RFC3339)
		}
		dto.Email, dto.Avatar = people[m.UserID].Email, people[m.UserID].Avatar
		page = append(page, dto)
	}
	return page
}

func (h *contractV1BillingHandler) resolveBillingMemberUserID(c *gin.Context, raw string) (int64, bool) {
	trimmed := strings.TrimSpace(raw)
	if !contract.IsUserID(trimmed) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeBillingAccountNotFound, runtimehttp.MsgBillingAccountNotFound)
		c.Abort()
		return 0, false
	}
	if h.deps.Users == nil {
		contract.RenderError(c, http.StatusNotImplemented,
			contract.CodeContractNotImplemented, "The v1 billing surface is not implemented yet")
		c.Abort()
		return 0, false
	}
	parsed, err := tenancy.ParseUserPublicID(trimmed)
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeBillingAccountNotFound, runtimehttp.MsgBillingAccountNotFound)
		c.Abort()
		return 0, false
	}
	userID, err := h.deps.Users.ResolveUserID(c.Request.Context(), parsed)
	if err != nil || userID <= 0 {
		contract.RenderError(c, http.StatusNotFound, contract.CodeBillingAccountNotFound, runtimehttp.MsgBillingAccountNotFound)
		c.Abort()
		return 0, false
	}
	return userID, true
}

func (h *contractV1BillingHandler) listBillingAccountMembers(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx) {
		if h.deps.BillingAccounts == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
			c.Abort()
			return
		}
		account, resolved := h.billingAccountForWorkspace(c, wctx.Workspace)
		if !resolved {
			return
		}
		members, err := h.deps.BillingAccounts.ListBillingAccountMembers(
			c.Request.Context(), account.InternalID, wctx.Principal.UserID())
		if err != nil {
			renderBillingAccountError(c, err)
			return
		}
		page, ok := h.toBillingMemberDTOs(c.Request.Context(), c, members)
		if !ok {
			return
		}
		contract.RenderSuccess(c, page)
	})
}

func (h *contractV1BillingHandler) addBillingAccountMember(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx) {
		if h.deps.BillingAccounts == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
			c.Abort()
			return
		}
		key, err := contract.IdempotencyKeyFromHeader(c)
		if err != nil {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, runtimehttp.MsgIdempotencyKeyRequired2)
			c.Abort()
			return
		}
		var body v1AddBillingMemberBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
			return
		}
		newRole := strings.TrimSpace(body.Data.Role)
		if !tenancy.IsValidBillingRole(newRole) {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				runtimehttp.MsgValidationFailed, contract.FieldErrors{Fields: map[string]string{"role": "invalid"}})
			c.Abort()
			return
		}
		targetUserID, ok := h.resolveBillingMemberUserID(c, body.Data.UserID)
		if !ok {
			return
		}
		account, resolved := h.billingAccountForWorkspace(c, wctx.Workspace)
		if !resolved {
			return
		}
		externalUser := tenancy.FormatUserPublicID(wctx.Principal.PublicID())
		hash := contract.HashIdempotencyRequest("billing.account.member-add", wctx.Workspace.PublicID, body.Data.UserID, newRole)
		result, _, doErr := h.deps.Idempotency.Do(c.Request.Context(), contract.IdempotencyScope(externalUser, "billing.account.member-add"), wctx.Principal.UserID(), key, hash, func() (any, error) {
			return h.deps.BillingAccounts.AddBillingAccountMember(
				c.Request.Context(), account.InternalID, wctx.Principal.UserID(), targetUserID, newRole)
		})
		if doErr != nil {
			if contract.IsIdempotencyConflict(doErr) {
				contract.RenderError(c, http.StatusConflict, contract.CodeBillingAccountConflict, runtimehttp.MsgBillingAccountConflict)
				c.Abort()
				return
			}
			renderBillingAccountError(c, doErr)
			return
		}
		added, ok := result.(*tenancy.BillingAccountMember)
		if !ok || added == nil {
			renderBillingAccountError(c, errors.New("billing: member-add type mismatch"))
			return
		}
		display := batchDisplayFacts(c.Request.Context(), h.deps.MemberDisplay, []int64{added.UserID})
		dto, ok := h.toBillingMemberDTO(c, added, display)
		if !ok {
			return
		}
		contract.RenderCreated(c, dto)
	})
}

func (h *contractV1BillingHandler) updateBillingAccountMember(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx) {
		if h.deps.BillingAccounts == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
			c.Abort()
			return
		}
		var body v1UpdateBillingMemberBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
			return
		}
		newRole := strings.TrimSpace(body.Data.Role)
		if !tenancy.IsValidBillingRole(newRole) {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				runtimehttp.MsgValidationFailed, contract.FieldErrors{Fields: map[string]string{"role": "invalid"}})
			c.Abort()
			return
		}
		targetUserID, ok := h.resolveBillingMemberUserID(c, c.Param("member_id"))
		if !ok {
			return
		}
		account, resolved := h.billingAccountForWorkspace(c, wctx.Workspace)
		if !resolved {
			return
		}
		updated, err := h.deps.BillingAccounts.UpdateBillingAccountMemberRole(
			c.Request.Context(), account.InternalID, wctx.Principal.UserID(), targetUserID, newRole)
		if err != nil {
			renderBillingAccountError(c, err)
			return
		}
		display := batchDisplayFacts(c.Request.Context(), h.deps.MemberDisplay, []int64{updated.UserID})
		dto, ok := h.toBillingMemberDTO(c, updated, display)
		if !ok {
			return
		}
		contract.RenderSuccess(c, dto)
	})
}

// userIDsOfBillingMembers collects billing manager user ids.
func userIDsOfBillingMembers(members []tenancy.BillingAccountMember) []int64 {
	ids := make([]int64, 0, len(members))
	for _, m := range members {
		if m.UserID <= 0 {
			continue
		}
		ids = append(ids, m.UserID)
	}
	return ids
}
