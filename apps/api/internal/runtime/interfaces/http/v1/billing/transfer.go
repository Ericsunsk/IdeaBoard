package billing

// Studio handover invitations (#948, invite-style ownership transfer).
//
// Only a Billing Owner may initiate; the recipient accepts or rejects, and
// the initiator may cancel while pending. Acceptance hands management to
// the recipient (new Owner, previous Owner steps down to Admin) and grants
// the recipient Workspace Owner on every non-Default business Workspace
// associated with the account; the Default Workspace never transfers.
// Identity, balance, Ledger, and history stay stable; no funds move to the
// recipient's other accounts, quotas never reset, and no welcome grant is
// minted. Row-pinned settlement keeps existing Task/Search/payment/refund
// attribution on the original account.
//
// Authorization: every invite route rides the shared workspace prelude
// (member-only visibility; hidden workspaces converge to 404
// workspace_not_found). The recipient must already hold workspace
// membership to accept/reject (add them via the workspace member invite
// first); acceptance then promotes them to Workspace Owner on every
// associated business Workspace.

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
)

type v1InitiateTransferInviteBody struct {
	Data struct {
		NewOwnerUserID string `json:"new_owner_user_id"`
	} `json:"data"`
}

type v1DetachedBillingAccountMemberDTO struct {
	Removed bool `json:"removed"`
}

type v1TransferInviteDTO struct {
	PublicID          string   `json:"public_id"`
	BillingAccountID  string   `json:"billing_account_id"`
	FromUserID        string   `json:"from_user_id"`
	ToUserID          string   `json:"to_user_id"`
	FromEmail         string   `json:"from_email,omitempty"`
	FromAvatar        string   `json:"from_avatar,omitempty"`
	ToEmail           string   `json:"to_email,omitempty"`
	ToAvatar          string   `json:"to_avatar,omitempty"`
	Status            string   `json:"status"`
	ScopeWorkspaceIDs []string `json:"scope_workspace_ids"`
	CreatedAt         string   `json:"created_at,omitempty"`
	ExpiresAt         string   `json:"expires_at,omitempty"`
	DecidedAt         string   `json:"decided_at,omitempty"`
	DecidedBy         string   `json:"decided_by,omitempty"`
	// DecidedByEmail/DecidedByAvatar mirror the decided_by identity
	// display; both stay omitted while undecided or unavailable.
	DecidedByEmail  string `json:"decided_by_email,omitempty"`
	DecidedByAvatar string `json:"decided_by_avatar,omitempty"`
}

// toTransferInviteDTOs builds transfer-invite pages in three round trips
// total: the store page (caller) plus one identity batch and one display
// batch, replacing the per-invite from/to/decidedBy GetUserPublicID N+1
// loop. Unknown identities fail closed with 503, matching the previous
// per-invite behavior; display facts stay best-effort omitted.
func (h *contractV1BillingHandler) toTransferInviteDTOs(ctx context.Context, c *gin.Context, invites []*tenancy.BillingTransferInvite, accountPublicID string) ([]v1TransferInviteDTO, bool) {
	userIDs := make([]int64, 0, 3*len(invites))
	for _, invite := range invites {
		if invite == nil {
			continue
		}
		userIDs = append(userIDs, invite.FromUserID, invite.ToUserID)
		if invite.DecidedBy != nil && *invite.DecidedBy > 0 {
			userIDs = append(userIDs, *invite.DecidedBy)
		}
	}
	people, err := h.readBillingPeople(ctx, userIDs)
	if err != nil {
		renderUserPublicIDsError(c, err)
		return nil, false
	}
	return transferInviteDTOs(invites, accountPublicID, people), true
}

func transferInviteDTOs(invites []*tenancy.BillingTransferInvite, accountPublicID string, people map[int64]billingoverview.Person) []v1TransferInviteDTO {
	page := make([]v1TransferInviteDTO, 0, len(invites))
	for _, invite := range invites {
		if invite == nil {
			continue
		}
		dto := v1TransferInviteDTO{
			PublicID:          invite.PublicID,
			BillingAccountID:  accountPublicID,
			FromUserID:        people[invite.FromUserID].PublicID,
			ToUserID:          people[invite.ToUserID].PublicID,
			Status:            invite.Status,
			ScopeWorkspaceIDs: append([]string(nil), invite.ScopeWorkspacePublicIDs...),
		}
		if dto.ScopeWorkspaceIDs == nil {
			dto.ScopeWorkspaceIDs = []string{}
		}
		dto.FromEmail, dto.FromAvatar = people[invite.FromUserID].Email, people[invite.FromUserID].Avatar
		dto.ToEmail, dto.ToAvatar = people[invite.ToUserID].Email, people[invite.ToUserID].Avatar
		if !invite.CreatedAt.IsZero() {
			dto.CreatedAt = invite.CreatedAt.UTC().Format(time.RFC3339)
		}
		if !invite.ExpiresAt.IsZero() {
			dto.ExpiresAt = invite.ExpiresAt.UTC().Format(time.RFC3339)
		}
		if invite.DecidedAt != nil && !invite.DecidedAt.IsZero() {
			dto.DecidedAt = invite.DecidedAt.UTC().Format(time.RFC3339)
		}
		if invite.DecidedBy != nil && *invite.DecidedBy > 0 {
			dto.DecidedBy = people[*invite.DecidedBy].PublicID
			dto.DecidedByEmail, dto.DecidedByAvatar = people[*invite.DecidedBy].Email, people[*invite.DecidedBy].Avatar
		}
		page = append(page, dto)
	}
	return page
}

// toTransferInviteDTO renders one invite through the same batched shape
// (batch of one, same 501/503 precedence) for the single-invite writes.
func (h *contractV1BillingHandler) toTransferInviteDTO(c *gin.Context, invite *tenancy.BillingTransferInvite, accountPublicID string) (v1TransferInviteDTO, bool) {
	if h.deps.Users == nil {
		contract.RenderError(c, http.StatusNotImplemented,
			contract.CodeContractNotImplemented, "The v1 billing surface is not implemented yet")
		c.Abort()
		return v1TransferInviteDTO{}, false
	}
	page, ok := h.toTransferInviteDTOs(c.Request.Context(), c, []*tenancy.BillingTransferInvite{invite}, accountPublicID)
	if !ok || len(page) != 1 {
		return v1TransferInviteDTO{}, false
	}
	return page[0], true
}

// initiateTransferInvite creates a pending handover invitation. Only the
// current Billing Owner may initiate; the previous Owner steps down only
// on acceptance. Requires the Idempotency-Key header: same key+hash
// replays the first invite, a different request renders 409
// billing_account_conflict. At most one pending invite per account.
func (h *contractV1BillingHandler) initiateTransferInvite(c *gin.Context) {
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
		var body v1InitiateTransferInviteBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
			return
		}
		newOwnerID, ok := h.resolveBillingMemberUserID(c, body.Data.NewOwnerUserID)
		if !ok {
			return
		}
		account, resolved := h.billingAccountForWorkspace(c, wctx.Workspace)
		if !resolved {
			return
		}
		externalUser := tenancy.FormatUserPublicID(wctx.Principal.PublicID())
		hash := contract.HashIdempotencyRequest("billing.account.transfer-invite", wctx.Workspace.PublicID, body.Data.NewOwnerUserID)
		result, _, doErr := h.deps.Idempotency.Do(c.Request.Context(), contract.IdempotencyScope(externalUser, "billing.account.transfer-invite"), wctx.Principal.UserID(), key, hash, func() (any, error) {
			return h.deps.Invitations.InitiateTransferInvite(
				c.Request.Context(), account.InternalID, wctx.Principal.UserID(), newOwnerID, key,
				tenancy.BillingTransferInitiatedParams{
					FromUserPublicID:  tenancy.FormatUserPublicID(wctx.Principal.PublicID()),
					WorkspacePublicID: wctx.Workspace.PublicID,
					WorkspaceName:     wctx.Workspace.Name,
				})
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
		invited, ok := result.(*tenancy.BillingTransferInvite)
		if !ok || invited == nil {
			renderBillingAccountError(c, errors.New("billing: transfer-invite type mismatch"))
			return
		}

		dto, ok := h.toTransferInviteDTO(c, invited, account.PublicID)
		if !ok {
			return
		}
		contract.RenderCreated(c, dto)
	})
}

// listTransferInvites pages handover history newest-first for the
// account's financial managers, or their own invitations for recipients.
// No Idempotency-Key: read-only, safe to retry.
func (h *contractV1BillingHandler) listTransferInvites(c *gin.Context) {
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
		invites, err := h.deps.BillingAccounts.ListBillingTransferInvitesForAccount(
			c.Request.Context(), account.InternalID, wctx.Principal.UserID(), 0, 50)
		if err != nil {
			renderBillingAccountError(c, err)
			return
		}
		page, ok := h.toTransferInviteDTOs(c.Request.Context(), c, invites, account.PublicID)
		if !ok {
			return
		}
		contract.RenderSuccess(c, page)
	})
}

// getTransferInvite fetches one invite by its bti_ id for the account's
// financial managers. Hidden ids converge to billing_account_not_found.
func (h *contractV1BillingHandler) getTransferInvite(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx) {
		if h.deps.BillingAccounts == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
			c.Abort()
			return
		}
		inviteID := strings.TrimSpace(c.Param("invite_id"))
		if !contract.IsBillingTransferInviteID(inviteID) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeBillingAccountNotFound, runtimehttp.MsgBillingAccountNotFound)
			c.Abort()
			return
		}
		account, resolved := h.billingAccountForWorkspace(c, wctx.Workspace)
		if !resolved {
			return
		}
		// The Workspace prelude establishes membership; financial managers
		// see account history and recipients see only their own invitations.
		role, _, err := h.deps.BillingAccounts.GetBillingAccountRole(c.Request.Context(), account.InternalID, wctx.Principal.UserID())
		if err != nil {
			renderBillingAccountError(c, err)
			return
		}
		invite, err := h.deps.BillingAccounts.GetBillingTransferInvite(c.Request.Context(), inviteID)
		if err != nil {
			renderBillingAccountError(c, err)
			return
		}
		if invite.BillingAccountID != account.InternalID || (!tenancy.IsBillingManager(role) && invite.ToUserID != wctx.Principal.UserID()) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeBillingAccountNotFound, runtimehttp.MsgBillingAccountNotFound)
			c.Abort()
			return
		}
		dto, ok := h.toTransferInviteDTO(c, invite, account.PublicID)
		if !ok {
			return
		}
		contract.RenderSuccess(c, dto)
	})
}

// decideTransferInvite runs the shared accept/reject/cancel sequencing: only
// the operation name and the store decision differ. Requires the
// Idempotency-Key header for exactly-once decisions under retry.
func (h *contractV1BillingHandler) decideTransferInvite(c *gin.Context, operation string, decide func(ctx context.Context, accountInternalID int64, inviteID string, actorUserID int64) (*tenancy.BillingTransferInvite, error)) {
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
		inviteID := strings.TrimSpace(c.Param("invite_id"))
		if !contract.IsBillingTransferInviteID(inviteID) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeBillingAccountNotFound, runtimehttp.MsgBillingAccountNotFound)
			c.Abort()
			return
		}
		account, resolved := h.billingAccountForWorkspace(c, wctx.Workspace)
		if !resolved {
			return
		}
		externalUser := tenancy.FormatUserPublicID(wctx.Principal.PublicID())
		hash := contract.HashIdempotencyRequest(operation, wctx.Workspace.PublicID, inviteID)
		result, _, doErr := h.deps.Idempotency.Do(c.Request.Context(), contract.IdempotencyScope(externalUser, operation), wctx.Principal.UserID(), key, hash, func() (any, error) {
			return decide(c.Request.Context(), account.InternalID, inviteID, wctx.Principal.UserID())
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
		decided, ok := result.(*tenancy.BillingTransferInvite)
		if !ok || decided == nil {
			renderBillingAccountError(c, errors.New("billing: transfer-invite type mismatch"))
			return
		}
		dto, ok := h.toTransferInviteDTO(c, decided, account.PublicID)
		if !ok {
			return
		}
		contract.RenderSuccess(c, dto)
	})
}

// acceptTransferInvite completes the handover: only the recipient may
// accept a pending invite. Requires the Idempotency-Key header for
// exactly-once acceptance under retry.
func (h *contractV1BillingHandler) acceptTransferInvite(c *gin.Context) {
	h.decideTransferInvite(c, "billing.account.transfer-accept", func(ctx context.Context, accountInternalID int64, inviteID string, actorUserID int64) (*tenancy.BillingTransferInvite, error) {
		return h.deps.BillingAccounts.AcceptBillingTransferInvite(ctx, accountInternalID, inviteID, actorUserID)
	})
}

// rejectTransferInvite declines a pending invite: only the recipient may
// reject. Requires the Idempotency-Key header.
func (h *contractV1BillingHandler) rejectTransferInvite(c *gin.Context) {
	h.decideTransferInvite(c, "billing.account.transfer-reject", func(ctx context.Context, accountInternalID int64, inviteID string, actorUserID int64) (*tenancy.BillingTransferInvite, error) {
		return h.deps.BillingAccounts.RejectBillingTransferInvite(ctx, accountInternalID, inviteID, actorUserID)
	})
}

// cancelTransferInvite withdraws a pending invite: only the initiator may
// cancel. Requires the Idempotency-Key header.
func (h *contractV1BillingHandler) cancelTransferInvite(c *gin.Context) {
	h.decideTransferInvite(c, "billing.account.transfer-cancel", func(ctx context.Context, accountInternalID int64, inviteID string, actorUserID int64) (*tenancy.BillingTransferInvite, error) {
		return h.deps.BillingAccounts.CancelBillingTransferInvite(ctx, accountInternalID, inviteID, actorUserID)
	})
}

// removeBillingAccountMember detaches one financial manager without
// deleting or freezing shared funds (exit path): only the management row
// leaves, the Wallet/Ledger/history stay readable. The last Owner cannot
// be removed. No Idempotency-Key: idempotent detach by value, safe to retry.
func (h *contractV1BillingHandler) removeBillingAccountMember(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx) {
		if h.deps.BillingAccounts == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
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
		if err := h.deps.BillingAccounts.RemoveBillingAccountMember(
			c.Request.Context(), account.InternalID, wctx.Principal.UserID(), targetUserID); err != nil {
			renderBillingAccountError(c, err)
			return
		}
		contract.RenderSuccess(c, v1DetachedBillingAccountMemberDTO{Removed: true})
	})
}
