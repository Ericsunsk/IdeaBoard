package control

// Platform-Admin Billing Account locate/audit surface (#923).
//
// A Billing Account holds the unified balance; many Workspaces may share one
// account (org pool or explicit studio link). The platform-Admin operator
// (admin business role on a human/worker session, enforced by the guard)
// audits any stable ba_ account here. Customer-side Billing Owner/Admin
// standing manages recharge/bills/quotas on its own accounts only and never
// implies platform-Admin access or Board content access.

import (
	"errors"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"net/http"
	"strings"
	"time"

	creditsdomain "ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"

	"github.com/gin-gonic/gin"
)

// controlBillingAccountCursor pages the Admin account list newest-first.
type controlBillingAccountCursor struct {
	Before int64 `json:"before"`
}

type controlBillingAccountItem struct {
	BillingAccountID string    `json:"billing_account_id"`
	OwnerType        string    `json:"owner_type"`
	WorkspaceID      string    `json:"workspace_id,omitempty"`
	OrganizationID   string    `json:"organization_id,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type controlBillingAccountMember struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

type controlLinkedWorkspace struct {
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
}

type controlBillingAccountWallet struct {
	AvailableCredits int64  `json:"available_credits"`
	ReservedCredits  int64  `json:"reserved_credits"`
	Status           string `json:"status"`
}

type controlBillingAccountDetail struct {
	BillingAccountID string                        `json:"billing_account_id"`
	OwnerType        string                        `json:"owner_type"`
	WorkspaceID      string                        `json:"workspace_id,omitempty"`
	OrganizationID   string                        `json:"organization_id,omitempty"`
	LinkedWorkspaces []controlLinkedWorkspace      `json:"linked_workspaces"`
	Members          []controlBillingAccountMember `json:"members"`
	Wallet           *controlBillingAccountWallet  `json:"wallet"`
	CreatedAt        time.Time                     `json:"created_at"`
	UpdatedAt        time.Time                     `json:"updated_at"`
}

// parseControlBillingAccountID validates a ba_ path/body id. Malformed
// shapes fail with 400; unknown accounts surface as 404 from the seam.
func parseControlBillingAccountID(c *gin.Context, raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if !contract.IsBillingAccountID(trimmed) {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest36)
		return "", false
	}
	return trimmed, true
}

// writeControlBillingAccountError maps tenancy billing failures to contract
// codes. Hidden ba_ ids converge to 404 (never existence-oracle 403s).
func writeControlBillingAccountError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, tenancy.ErrBillingNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, runtimehttp.MsgControlNotFound9)
	case errors.Is(err, tenancy.ErrBillingForbidden):
		contract.RenderError(c, http.StatusForbidden, contract.CodeForbidden, runtimehttp.MsgForbidden)
	case errors.Is(err, tenancy.ErrBillingWorkspaceInvalid):
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest4)
	default:
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable2)
	}
}

func newControlBillingAccountItem(account tenancypg.AdminBillingAccount) controlBillingAccountItem {
	return controlBillingAccountItem{
		BillingAccountID: account.PublicID,
		OwnerType:        account.OwnerType,
		WorkspaceID:      account.WorkspacePublicID,
		OrganizationID:   account.OrganizationPublicID,
		CreatedAt:        account.CreatedAt,
		UpdatedAt:        account.UpdatedAt,
	}
}

// listControlBillingAccounts serves both the workspace resolve (existing
// workspace_id contract) and the Admin account list (#923): without
// workspace_id it pages stable accounts newest-first with the opaque
// cursor/page_size dialect plus an optional owner_scope filter
// (all|organization|workspace, default all).
func (d *ContractControlDeps) listControlBillingAccounts(c *gin.Context, _ controlContext) {
	if d == nil || d.BillingAccounts == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable)
		return
	}
	if workspaceID := strings.TrimSpace(c.Query("workspace_id")); workspaceID != "" {
		d.resolveControlBillingAccountForWorkspace(c, workspaceID)
		return
	}
	ownerScope := strings.TrimSpace(c.Query("owner_scope"))
	if ownerScope == "" {
		ownerScope = "all"
	}
	cursor, ok := parseControlPageCursor(c, func(cursor *controlBillingAccountCursor) bool { return cursor.Before >= 0 })
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
	accounts, err := d.BillingAccounts.ListBillingAccountsForAdmin(c.Request.Context(), before, ownerScope, limit)
	if err != nil {
		writeControlBillingAccountError(c, err)
		return
	}
	items := make([]controlBillingAccountItem, 0, len(accounts))
	for _, account := range accounts {
		items = append(items, newControlBillingAccountItem(account))
	}
	hasMore := len(accounts) == limit && len(accounts) > 0
	var nextCursor string
	if hasMore {
		encoded, err := contract.EncodeCursor(controlBillingAccountCursor{Before: accounts[len(accounts)-1].InternalID})
		if err != nil {
			contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable9)
			return
		}
		nextCursor = encoded
	}
	contract.RenderPaginated(c, items, contract.NewPageMeta(nextCursor, hasMore))
}

func (d *ContractControlDeps) resolveControlBillingAccountForWorkspace(c *gin.Context, workspaceID string) {
	if d == nil || d.Tenancy == nil || d.BillingAccounts == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable)
		return
	}
	if !contract.IsWorkspaceID(workspaceID) {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest3)
		return
	}
	workspace, err := d.Tenancy.GetWorkspace(c.Request.Context(), workspaceID)
	if err != nil {
		writeControlTenancyError(c, err, runtimehttp.MsgListControlBillingAccounts)
		return
	}
	identity, err := d.BillingAccounts.ResolveFunds(c.Request.Context(), workspace.PublicID)
	if err != nil {
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable2)
		return
	}
	account := identity.ToBillingAccount()
	items := []controlBillingAccountItem{{
		BillingAccountID: account.PublicID,
		OwnerType:        account.OwnerType,
		WorkspaceID:      account.WorkspacePublicID,
		CreatedAt:        account.CreatedAt,
		UpdatedAt:        account.UpdatedAt,
	}}
	contract.RenderPaginated(c, items, contract.NewPageMeta("", false))
}

// getControlBillingAccount reads one stable account by its ba_ id with
// linkage, financial managers, and the wallet summary (#923). It is a
// platform-Admin audit read: member identities resolve to usr_ public ids
// for display, and a missing wallet renders null (never synthesized).
func (d *ContractControlDeps) getControlBillingAccount(c *gin.Context, _ controlContext) {
	if d == nil || d.BillingAccounts == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable)
		return
	}
	publicID, ok := parseControlBillingAccountID(c, c.Param("billing_account_id"))
	if !ok {
		return
	}
	account, err := d.BillingAccounts.GetAdminBillingAccountByPublicID(c.Request.Context(), publicID)
	if err != nil {
		writeControlBillingAccountError(c, err)
		return
	}
	detail := controlBillingAccountDetail{
		BillingAccountID: account.PublicID,
		OwnerType:        account.OwnerType,
		LinkedWorkspaces: []controlLinkedWorkspace{},
		Members:          []controlBillingAccountMember{},
		CreatedAt:        account.CreatedAt,
		UpdatedAt:        account.UpdatedAt,
	}
	if account.WorkspacePublicID != "" {
		detail.WorkspaceID = account.WorkspacePublicID
	}
	if account.OrganizationPublicID != "" {
		detail.OrganizationID = account.OrganizationPublicID
	}
	if linked, err := d.BillingAccounts.ListExplicitlyLinkedWorkspaces(c.Request.Context(), account.InternalID); err == nil {
		for _, workspace := range linked {
			detail.LinkedWorkspaces = append(detail.LinkedWorkspaces, controlLinkedWorkspace{
				WorkspaceID: workspace.PublicID,
				Name:        workspace.Name,
			})
		}
	} else {
		writeControlBillingAccountError(c, err)
		return
	}
	if d.CreditUsers != nil {
		if members, err := d.BillingAccounts.ListBillingAccountMembersForAdmin(c.Request.Context(), account.InternalID); err == nil {
			for _, member := range members {
				identity, err := d.CreditUsers.ResolveID(c.Request.Context(), member.UserID)
				if err != nil {
					continue
				}
				detail.Members = append(detail.Members, controlBillingAccountMember{
					UserID:      tenancy.FormatUserPublicID(identity.PublicID),
					DisplayName: identity.DisplayName,
					Role:        member.Role,
				})
			}
		} else {
			writeControlBillingAccountError(c, err)
			return
		}
	}
	if d.CreditReads != nil {
		if wallet, err := d.CreditReads.GetWallet(c.Request.Context(), account.InternalID); err == nil {
			detail.Wallet = &controlBillingAccountWallet{
				AvailableCredits: wallet.Available,
				ReservedCredits:  wallet.Reserved,
				Status:           string(wallet.Status),
			}
		} else if !errors.Is(err, creditsdomain.ErrWalletNotFound) {
			writeControlCreditError(c, err)
			return
		}
	}
	contract.RenderSuccess(c, detail)
}
