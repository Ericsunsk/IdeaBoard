package tenancy

import "context"

// BillingTransferInitiatedParams is the fixed handover context for
// OnBillingTransferInitiated. BillingAccountPublicID carries the ba_ funds
// identity snapshot (omitempty on the wire); empty means the dispatcher
// omits billing_account_id instead of emitting an empty key.
type BillingTransferInitiatedParams struct {
	// NotificationKey identifies one durable initiate request, including reissues.
	NotificationKey        string
	FromUserPublicID       string
	WorkspacePublicID      string
	WorkspaceName          string
	BillingAccountPublicID string
}

// CollaborationEventDispatcher coordinates domain lifecycle side-effects
// (notifications, emails) for workspace invitations, membership decisions,
// and studio handover transitions (#957, #958, #948).
type CollaborationEventDispatcher interface {
	OnWorkspaceMemberInvited(ctx context.Context, invite *WorkspaceMemberInvite, rawToken string) error
	OnWorkspaceMemberInviteAccepted(ctx context.Context, invite *WorkspaceMemberInvite, member *WorkspaceMember, recipientEmail string) error
	OnWorkspaceMemberInviteDeclined(ctx context.Context, invite *WorkspaceMemberInvite) error
	OnMemberInviteCancelled(ctx context.Context, inviteID string) error
	OnOrganizationMemberInvited(ctx context.Context, invite *OrganizationMemberInvite) error
	OnOrganizationMemberInviteAccepted(ctx context.Context, invite *OrganizationMemberInvite, member *OrganizationMember, recipientEmail string) error
	OnOrganizationMemberInviteDeclined(ctx context.Context, invite *OrganizationMemberInvite) error
	OnBillingTransferInitiated(ctx context.Context, transfer *BillingTransferInvite, params BillingTransferInitiatedParams) error
}

// WorkspaceInviteEmailSender stages durable workspace invitation email delivery
// in the caller transaction. External delivery happens only in the River worker.
type WorkspaceInviteEmailSender interface {
	SendWorkspaceInviteEmail(ctx context.Context, invite *WorkspaceMemberInvite, rawToken string) error
}
