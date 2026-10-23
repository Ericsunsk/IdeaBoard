package tenancy

// Billing-account resolution types: stable funds identity and handover invites.

import (
	"strings"
	"time"
)

// Billing Account financial roles (ADR-0038). Financial management (recharge,
// bills, member quotas) is separate from Workspace content membership. Owner
// and Admin both manage recharge/bills/quotas; only Owner transfers
// ownership. Financial management never grants Board content access.
const (
	BillingRoleOwner = "owner"
	BillingRoleAdmin = "admin"
)

// MaxBillingAccountMembers bounds explicit financial managers per account.
// One User may manage many independent accounts; this cap is per account.
const MaxBillingAccountMembers = 10

// BillingAccountMember is one billing_account_members row.
type BillingAccountMember struct {
	BillingAccountID int64
	UserID           int64
	Role             string
	CreatedAt        time.Time
}

// Billing transfer invite lifecycle (#948, invite-style studio handover).
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
const (
	BillingTransferPending   = "pending"
	BillingTransferAccepted  = "accepted"
	BillingTransferRejected  = "rejected"
	BillingTransferCancelled = "cancelled"
)

// BillingTransferInvite is one billing_account_transfer_invites row
// projected for the API. BillingAccountPublicID is the stable ba_ funds
// identity snapshot; ScopeWorkspacePublicIDs is the non-Default business
// Workspace scope granted at acceptance (empty until accepted). ExpiresAt is
// the 7-day lapse: a pending invite at or past ExpiresAt never moves funds
// or membership, decisions on it fail closed, and the owner may re-issue.
type BillingTransferInvite struct {
	InternalID              int64
	PublicID                string
	BillingAccountID        int64
	BillingAccountPublicID  string
	FromUserID              int64
	ToUserID                int64
	Status                  string
	ScopeWorkspacePublicIDs []string
	CreatedAt               time.Time
	ExpiresAt               time.Time
	DecidedAt               *time.Time
	DecidedBy               *int64
}

// Expired reports whether a pending invite has lapsed at now. Only pending
// invites lapse; terminal invites carry their decided outcome regardless of
// ExpiresAt. A zero ExpiresAt (predates the expiry column) never lapses.
func (invite *BillingTransferInvite) Expired(now time.Time) bool {
	if invite == nil || invite.Status != BillingTransferPending || invite.ExpiresAt.IsZero() {
		return false
	}
	return !now.Before(invite.ExpiresAt)
}

// IsValidBillingTransferStatus reports whether status is a known invite state.
func IsValidBillingTransferStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case BillingTransferPending, BillingTransferAccepted, BillingTransferRejected, BillingTransferCancelled:
		return true
	default:
		return false
	}
}
