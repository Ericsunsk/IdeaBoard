package tenancy

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"ideaboard/api/internal/platform/contract"
)

// Public identifier prefixes. These alias the contract kernel so the two
// can never drift; the middleware seam validates authoritatively via
// contract.IsWorkspaceID etc.
const (
	UserPrefix         = contract.PrefixUser
	OrganizationPrefix = contract.PrefixOrganization
	WorkspacePrefix    = contract.PrefixWorkspace
	// BillingAccountPrefix aliases the contract kernel so the two can
	// never drift; billing_accounts.public_id carries ba_ per #804.
	BillingAccountPrefix = contract.PrefixBillingAccount
	// BillingTransferInvitePrefix aliases the contract kernel so the two
	// can never drift; billing_account_transfer_invites.public_id carries
	// bti_ per #948 (invite-style studio handover).
	BillingTransferInvitePrefix = contract.PrefixBillingTransferInvite
	// WorkspaceInvitePrefix aliases the contract kernel so the two can never
	// drift; workspace_member_invites.public_id carries wsi_ per #957.
	WorkspaceInvitePrefix = contract.PrefixWorkspaceInvite
	// NotificationPrefix aliases the contract kernel so the two can never
	// drift; user_notifications.public_id carries notif_ per #958.
	NotificationPrefix = contract.PrefixNotification
	// OrganizationInvitePrefix aliases the contract kernel so the two can never
	// drift; organization_member_invites.public_id carries osi_ per #957 / ADR-0041.
	OrganizationInvitePrefix = contract.PrefixOrganizationInvite
)

// FormatUserPublicID renders the external user identifier. The users table
// keeps its UUID internal key; only this translator exposes usr_<hex> and
// it must stay the single place that does so (do NOT change users table).
func FormatUserPublicID(id uuid.UUID) string {
	return UserPrefix + strings.ReplaceAll(id.String(), "-", "")
}

// ParseUserPublicID parses an external usr_<hex> identifier back to UUID.
// It accepts dashed or undashed hex after the prefix.
func ParseUserPublicID(external string) (uuid.UUID, error) {
	if !strings.HasPrefix(external, UserPrefix) {
		return uuid.Nil, fmt.Errorf("%w: %q", ErrNotUserPublicID, external)
	}
	raw := strings.TrimPrefix(external, UserPrefix)
	raw = strings.ReplaceAll(raw, "-", "")
	if len(raw) != 32 {
		return uuid.Nil, fmt.Errorf("%w: %q", ErrMalformedUserPublicID, external)
	}
	dashed := raw[:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:]
	id, err := uuid.Parse(dashed)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: %q", ErrMalformedUserPublicID, external)
	}
	return id, nil
}

// NewWorkspacePublicID mints an opaque ws_ public identifier.
func NewWorkspacePublicID() (string, error) {
	return contract.MintPublicID(WorkspacePrefix, 12)
}

// NewOrganizationPublicID mints an opaque org_ public identifier.
func NewOrganizationPublicID() (string, error) {
	return contract.MintPublicID(OrganizationPrefix, 12)
}

// NewBillingAccountPublicID mints an opaque ba_ public identifier for the
// workspace-owned billing account (#804).
func NewBillingAccountPublicID() (string, error) {
	return contract.MintPublicID(BillingAccountPrefix, 12)
}

// NewBillingTransferInvitePublicID mints an opaque bti_ public identifier
// for a studio handover invitation (#948).
func NewBillingTransferInvitePublicID() (string, error) {
	return contract.MintPublicID(BillingTransferInvitePrefix, 12)
}

// NewWorkspaceInvitePublicID mints an opaque wsi_ public identifier
// for a workspace member invitation (#957).
func NewWorkspaceInvitePublicID() (string, error) {
	return contract.MintPublicID(WorkspaceInvitePrefix, 12)
}

// NewOrganizationInvitePublicID mints an opaque osi_ public identifier
// for an organization member invitation (ADR-0041).
func NewOrganizationInvitePublicID() (string, error) {
	return contract.MintPublicID(OrganizationInvitePrefix, 12)
}
