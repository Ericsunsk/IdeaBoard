package shared

import (
	"context"

	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/tenancy/funds"
)

// AccountStore resolves the stable funds identity plus financial
// management. Production uses tenancypg.BillingAccountStore via the deep
// funds kernel; tests stub this interface.
// Resolution joins the caller transaction inside the deep module (funds
// borrow-or-begin); handlers pass only the ws_ public id and receive the
// ba_ identity, never branching on the workspace/org mapping.
//
// It lives in shared so the tenancy and billing subpackages depend only
// on shared+kernel, never on each other (billing keeps a compat alias).
type AccountStore interface {
	ResolveFunds(ctx context.Context, workspacePublicID string) (funds.Identity, error)
	GetBillingAccountByPublicID(ctx context.Context, publicID string) (tenancy.BillingAccount, error)
	// GetBillingAccountByInternalID resolves the stable ba_ funds identity
	// for a beneficiary account (#921): reward redemption credits the
	// caller's initial personal account, and the edge maps it to ba_ so
	// the UI can name the actual credited account.
	GetBillingAccountByInternalID(ctx context.Context, billingAccountInternalID int64) (tenancy.BillingAccount, error)
	GetBillingAccountRole(ctx context.Context, billingAccountInternalID, userID int64) (string, bool, error)
	ListBillingAccountMembers(ctx context.Context, billingAccountInternalID, actorUserID int64) ([]tenancy.BillingAccountMember, error)
	AddBillingAccountMember(ctx context.Context, billingAccountInternalID, actorUserID, targetUserID int64, role string) (*tenancy.BillingAccountMember, error)
	UpdateBillingAccountMemberRole(ctx context.Context, billingAccountInternalID, actorUserID, targetUserID int64, newRole string) (*tenancy.BillingAccountMember, error)
	// Studio handover invitations (#948, invite-style ownership transfer).
	InitiateBillingTransferInvite(ctx context.Context, billingAccountInternalID, fromUserID, toUserID int64, idempotencyKey string) (*tenancy.BillingTransferInvite, error)
	GetBillingTransferInvite(ctx context.Context, publicID string) (*tenancy.BillingTransferInvite, error)
	ListBillingTransferInvitesForAccount(ctx context.Context, billingAccountInternalID, actorUserID, beforeID int64, limit int) ([]*tenancy.BillingTransferInvite, error)
	AcceptBillingTransferInvite(ctx context.Context, billingAccountInternalID int64, invitePublicID string, recipientUserID int64) (*tenancy.BillingTransferInvite, error)
	RejectBillingTransferInvite(ctx context.Context, billingAccountInternalID int64, invitePublicID string, recipientUserID int64) (*tenancy.BillingTransferInvite, error)
	CancelBillingTransferInvite(ctx context.Context, billingAccountInternalID int64, invitePublicID string, initiatorUserID int64) (*tenancy.BillingTransferInvite, error)
	// RemoveBillingAccountMember detaches one financial manager without
	// deleting or freezing shared funds (exit path): only the management
	// row leaves, the Wallet/Ledger/history stay readable.
	RemoveBillingAccountMember(ctx context.Context, billingAccountInternalID, actorUserID, targetUserID int64) error
}
