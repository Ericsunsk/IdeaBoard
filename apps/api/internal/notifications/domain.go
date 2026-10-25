package notifications

import (
	"errors"
	"time"
)

var (
	// ErrNotFound is returned when a notification does not exist or does not belong to the user.
	ErrNotFound = errors.New("notifications: not found")
)

// NotificationType identifies the semantic category of notification.
type NotificationType string

const (
	TypeWorkspaceInvite            NotificationType = "workspace_invite"
	TypeWorkspaceMemberJoined      NotificationType = "workspace_member_joined"
	TypeWorkspaceInviteDeclined    NotificationType = "workspace_invite_declined"
	TypeOrganizationInvite         NotificationType = "organization_invite"
	TypeOrganizationMemberJoined   NotificationType = "organization_member_joined"
	TypeOrganizationInviteDeclined NotificationType = "organization_invite_declined"
	TypeStudioHandover             NotificationType = "studio_handover"
	TypeBillingCreditLow           NotificationType = "billing_credit_low"
	TypeBillingRechargeSuccess     NotificationType = "billing_recharge_success"
)

// NotificationStatus represents the lifecycle state of a notification.
type NotificationStatus string

const (
	StatusUnread   NotificationStatus = "unread"
	StatusRead     NotificationStatus = "read"
	StatusArchived NotificationStatus = "archived"
)

// ActionPayload holds structured context and metadata for inline client actions.
//
// Typed constructors live below (InviteActionPayload, HandoverActionPayload,
// BillingActionPayload): the dispatcher must build those values and call
// ToActionPayload instead of hand-writing map keys. The JSON keys are the
// wire contract read by the frontend notificationActionRegistry.
type ActionPayload map[string]any

// Wire keys shared by the dispatcher, the invite_status write path
// (MarkInviteResolved), and the frontend registry. invite_id is the member
// invite family key; transfer_invite_id is intentionally distinct for the
// studio handover family (billing transfer invites are not member invites,
// so the invite_status merge never touches them).
const (
	PayloadKeyInviteID         = "invite_id"
	PayloadKeyInviteStatus     = "invite_status"
	PayloadKeyRole             = "role"
	PayloadKeyWorkspaceID      = "workspace_id"
	PayloadKeyWorkspaceName    = "workspace_name"
	PayloadKeyOrganizationID   = "organization_id"
	PayloadKeyOrganizationName = "organization_name"
	PayloadKeyMemberUserID     = "member_user_id"
	PayloadKeyMemberEmail      = "member_email"
	PayloadKeyInviteeEmail     = "invitee_email"

	PayloadKeyTransferInviteID = "transfer_invite_id"
	PayloadKeyFromUserID       = "from_user_id"
	PayloadKeyBillingAccountID = "billing_account_id"
)

// InviteActionPayload is the typed invite-family payload (workspace_invite,
// organization_invite, and the inviter-side joined/declined notices).
// ID/Status/Role/Names converge here; scope keys are omitempty so workspace
// rows carry workspace_* and organization rows carry organization_*.
type InviteActionPayload struct {
	InviteID         string `json:"invite_id"`
	InviteStatus     string `json:"invite_status,omitempty"`
	Role             string `json:"role,omitempty"`
	WorkspaceID      string `json:"workspace_id,omitempty"`
	WorkspaceName    string `json:"workspace_name,omitempty"`
	OrganizationID   string `json:"organization_id,omitempty"`
	OrganizationName string `json:"organization_name,omitempty"`
	MemberUserID     string `json:"member_user_id,omitempty"`
	MemberEmail      string `json:"member_email,omitempty"`
	InviteeEmail     string `json:"invitee_email,omitempty"`
}

// ToActionPayload converts the typed invite value to the wire map. Empty
// fields are omitted so decided notices stay minimal.
func (p InviteActionPayload) ToActionPayload() ActionPayload {
	out := ActionPayload{PayloadKeyInviteID: p.InviteID}
	if p.InviteStatus != "" {
		out[PayloadKeyInviteStatus] = p.InviteStatus
	}
	if p.Role != "" {
		out[PayloadKeyRole] = p.Role
	}
	if p.WorkspaceID != "" {
		out[PayloadKeyWorkspaceID] = p.WorkspaceID
	}
	if p.WorkspaceName != "" {
		out[PayloadKeyWorkspaceName] = p.WorkspaceName
	}
	if p.OrganizationID != "" {
		out[PayloadKeyOrganizationID] = p.OrganizationID
	}
	if p.OrganizationName != "" {
		out[PayloadKeyOrganizationName] = p.OrganizationName
	}
	if p.MemberUserID != "" {
		out[PayloadKeyMemberUserID] = p.MemberUserID
	}
	if p.MemberEmail != "" {
		out[PayloadKeyMemberEmail] = p.MemberEmail
	}
	if p.InviteeEmail != "" {
		out[PayloadKeyInviteeEmail] = p.InviteeEmail
	}
	return out
}

// HandoverActionPayload is the typed studio-handover payload
// (studio_handover). See PayloadKeyTransferInviteID for why the handover
// key stays distinct from the member invite_id family.
type HandoverActionPayload struct {
	TransferInviteID string `json:"transfer_invite_id"`
	WorkspaceID      string `json:"workspace_id,omitempty"`
	WorkspaceName    string `json:"workspace_name,omitempty"`
	FromUserID       string `json:"from_user_id,omitempty"`
	BillingAccountID string `json:"billing_account_id,omitempty"`
}

// ToActionPayload converts the typed handover value to the wire map.
func (p HandoverActionPayload) ToActionPayload() ActionPayload {
	out := ActionPayload{PayloadKeyTransferInviteID: p.TransferInviteID}
	if p.WorkspaceID != "" {
		out[PayloadKeyWorkspaceID] = p.WorkspaceID
	}
	if p.WorkspaceName != "" {
		out[PayloadKeyWorkspaceName] = p.WorkspaceName
	}
	if p.FromUserID != "" {
		out[PayloadKeyFromUserID] = p.FromUserID
	}
	if p.BillingAccountID != "" {
		out[PayloadKeyBillingAccountID] = p.BillingAccountID
	}
	return out
}

// BillingActionPayload is the reserved typed billing-family payload
// (billing_credit_low, billing_recharge_success). Reserved: no producer
// emits these types yet; when one lands it must converge here instead of
// hand-writing keys.
type BillingActionPayload struct {
	BillingAccountID string `json:"billing_account_id,omitempty"`
	WorkspaceID      string `json:"workspace_id,omitempty"`
	WorkspaceName    string `json:"workspace_name,omitempty"`
}

// ToActionPayload converts the typed billing value to the wire map.
func (p BillingActionPayload) ToActionPayload() ActionPayload {
	out := ActionPayload{}
	if p.BillingAccountID != "" {
		out[PayloadKeyBillingAccountID] = p.BillingAccountID
	}
	if p.WorkspaceID != "" {
		out[PayloadKeyWorkspaceID] = p.WorkspaceID
	}
	if p.WorkspaceName != "" {
		out[PayloadKeyWorkspaceName] = p.WorkspaceName
	}
	return out
}

// Notification mirrors the user_notifications row (#958, ADR-0040).
type Notification struct {
	InternalID    int64              `json:"-"`
	PublicID      string             `json:"id"`
	UserID        int64              `json:"-"`
	Type          NotificationType   `json:"type"`
	Title         string             `json:"title"`
	Body          string             `json:"body"`
	ActionPayload ActionPayload      `json:"action_payload"`
	Status        NotificationStatus `json:"status"`
	ReadAt        *time.Time         `json:"read_at,omitempty"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
}
