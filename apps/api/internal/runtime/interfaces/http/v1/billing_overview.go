package v1

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"ideaboard/api/internal/billingoverview"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"
	usageapplication "ideaboard/api/internal/usage/application"
	usagedomain "ideaboard/api/internal/usage/domain"

	"github.com/gin-gonic/gin"
)

type v1BillingOverviewDTO struct {
	BillingAccount v1BillingAccountDTO        `json:"billing_account"`
	Balance        v1BillingCreditsBalanceDTO `json:"balance"`
	Usage          usageapplication.Usage     `json:"usage"`
	// Quota is the caller's own quota with live usage (always present).
	Quota v1MemberQuotaDTO `json:"quota"`
	// Quotas is the full manager quota list with live usage, present only
	// for quota managers (same gate as listMemberQuotas).
	Quotas []v1MemberQuotaDTO `json:"quotas,omitempty"`
	// BillingMembers is the financial manager list with display facts,
	// present only for billing managers (same gate as
	// listBillingAccountMembers).
	BillingMembers []v1BillingMemberDTO `json:"billing_members,omitempty"`
	// MembersCount is the workspace member count (any member may read it,
	// same visibility as listWorkspaceMembers).
	MembersCount int `json:"members_count"`
	// LiveInvites carries pending transfer invites with the store's own
	// recipient scoping (managers see all, others see their own).
	LiveInvites []v1TransferInviteDTO `json:"live_invites,omitempty"`
}

func (h *contractV1BillingHandler) getBillingOverview(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx) {
		usageRange, err := usagedomain.ParseRange(strings.TrimSpace(c.Query("range")))
		if err != nil {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				runtimehttp.MsgRenderV1UsageError, contract.FieldErrors{Fields: map[string]string{"range": "invalid"}})
			c.Abort()
			return
		}
		reader := billingoverview.Reader{
			Accounts: h.deps.BillingAccounts, Tenancy: h.deps.Tenancy, Wallets: h.deps.Wallets,
			Usage: h.deps.Usage, Quotas: h.deps.Quotas, People: h.readBillingPeople, Snapshot: h.deps.Snapshot,
		}
		result, err := reader.Read(c.Request.Context(), billingoverview.Input{
			Workspace: wctx.Workspace, WorkspaceRole: wctx.Role, ActorUserID: wctx.Principal.UserID(), Range: usageRange,
		})
		if err != nil {
			renderBillingOverviewError(c, err)
			return
		}
		quotas := make([]v1MemberQuotaDTO, 0, len(result.Quotas))
		for _, reading := range result.Quotas {
			quotas = append(quotas, quotaReadingDTO(result.Account.PublicID, result.People[reading.UserID].PublicID, reading))
		}
		contract.RenderSuccess(c, v1BillingOverviewDTO{
			BillingAccount: toBillingAccountDTO(wctx.Workspace, result.Account, result.Scope, result.CanManageQuotas),
			Balance:        billingToBalanceDTO(result.Account.PublicID, wctx.Workspace.PublicID, result.Wallet),
			Usage:          result.Usage,
			Quota:          quotaReadingDTO(result.Account.PublicID, tenancy.FormatUserPublicID(wctx.Principal.PublicID()), result.Quota),
			Quotas:         quotas, BillingMembers: billingMemberDTOs(result.BillingMembers, result.People),
			MembersCount: result.MembersCount, LiveInvites: transferInviteDTOs(result.LiveInvites, result.Account.PublicID, result.People),
		})
	})
}

// readBillingPeople adapts identity and display owners to the read module.
// It also feeds the single-read projections, with no response rendering.
func (h *contractV1BillingHandler) readBillingPeople(ctx context.Context, ids []int64) (map[int64]billingoverview.Person, error) {
	publicIDs, err := readUserPublicIDs(ctx, h.deps.Users, ids)
	if err != nil {
		return nil, err
	}
	display := batchDisplayFacts(ctx, h.deps.MemberDisplay, ids)
	people := make(map[int64]billingoverview.Person, len(publicIDs))
	for id, publicID := range publicIDs {
		email, avatar := applyDisplayFact(display, id)
		people[id] = billingoverview.Person{PublicID: publicID, Email: email, Avatar: avatar}
	}
	return people, nil
}

func renderBillingOverviewError(c *gin.Context, err error) {
	var readErr *billingoverview.Error
	if errors.As(err, &readErr) {
		switch readErr.Stage {
		case billingoverview.Account:
			renderBillingAccountError(c, readErr.Err)
			return
		case billingoverview.WorkspaceMembers:
			contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, runtimehttp.MsgListControlBillingAccounts)
			c.Abort()
			return
		case billingoverview.People:
			renderUserPublicIDsError(c, readErr.Err)
			return
		case billingoverview.Wallet:
			billingRenderWalletGate(c, readErr.Err)
			return
		case billingoverview.Usage:
			renderV1UsageError(c, readErr.Err)
			return
		}
	}
	contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable39)
	c.Abort()
}
