package tenancy

// Member invitation lifecycle deep module (#957, ADR-0039, ADR-0041).
//
// Workspace and organization invites are one lifecycle parameterized by a
// scope policy: token mint/hash, pending-only reads, expiry, anti-enumeration
// (email/user mismatch converges to ErrNotFound), InviteeUserID persistence,
// and the status audit shape are identical. Scopes differ only in policy
// hooks:
//
//   - capacity: workspace enforces active + pending <= MaxWorkspaceMembers;
//     organization enforces the live single-organization limit at accept.
//   - closure: workspace writes reject closed owning organizations via
//     CheckWorkspaceWritable; organization writes lock the live organization.
//   - member write target: workspace_memberships vs organization_memberships.
//   - Default protection applies to workspace scope only.
//
// The postgres and in-memory adapters implement the same sequencing against
// per-scope tables (workspace_member_invites / organization_member_invites,
// each with its own FOR UPDATE row lock and partial-unique pending index).
// WorkspaceMemberInvite / OrganizationMemberInvite carry each scope's facts
// at the persistence and application boundaries; both adapters delegate the
// common lifecycle rules to this module.

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// InviteScope identifies which membership family an invitation belongs to.
type InviteScope string

const (
	InviteScopeWorkspace    InviteScope = "workspace"
	InviteScopeOrganization InviteScope = "organization"
)

// Member invitation statuses shared by both scopes.
const (
	MemberInviteStatusPending   = "pending"
	MemberInviteStatusAccepted  = "accepted"
	MemberInviteStatusDeclined  = "declined"
	MemberInviteStatusCancelled = "cancelled"
	MemberInviteStatusExpired   = "expired"
)

// MemberInviteTTL is the invitation lifetime (7 days, both scopes).
const MemberInviteTTL = 7 * 24 * time.Hour

// MemberInvite is the scope-agnostic invitation fact. ScopePublicID/ScopeName
// carry the workspace or organization identity depending on Scope.
type MemberInvite struct {
	Scope             InviteScope
	InternalID        int64
	PublicID          string
	ScopeInternalID   int64
	ScopePublicID     string
	ScopeName         string
	InviterUserID     int64
	InviteeEmail      string
	InviteeUserID     *int64
	Role              string
	Status            string
	TokenHash         string
	ExpiresAt         time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
	RespondedAt       *time.Time
	RespondedByUserID *int64
}

// MemberInvitePolicy parameterizes the shared lifecycle per scope.
// ConflictErr/ExpiredErr identify scope-specific decision errors.
type MemberInvitePolicy struct {
	Scope          InviteScope
	PublicIDPrefix string
	ConflictErr    error
	ExpiredErr     error
	NewPublicID    func() (string, error)
}

// CheckInviterRole applies the organization inviter rules. Workspace
// authorization is enforced by its HTTP/application boundary.
func (p MemberInvitePolicy) CheckInviterRole(actorRole, grantedRole string) error {
	if p.Scope != InviteScopeOrganization {
		return nil
	}
	if actorRole != RoleOwner && actorRole != RoleAdmin {
		return ErrForbidden
	}
	if grantedRole == RoleAdmin && actorRole != RoleOwner {
		return ErrForbidden
	}
	return nil
}

// CheckCreateCapacity reserves workspace capacity for active members and
// pending invitations. Organization membership has no create-time cap.
func (p MemberInvitePolicy) CheckCreateCapacity(active, pending int64) error {
	if p.Scope == InviteScopeWorkspace && active+pending >= MaxWorkspaceMembers {
		return ErrWorkspaceMemberLimitReached
	}
	return nil
}

// TransferInviteLapsed reports the transfer 7-day lapse (inclusive): a
// pending invite at or past ExpiresAt never moves funds or membership.
// Member invites use MemberInviteExpired (strict After); the boundary
// differs by one instant and is preserved here, so this helper must not
// replace MemberInviteExpired for member scopes.
func TransferInviteLapsed(expiresAt, now time.Time) bool {
	if expiresAt.IsZero() {
		return false
	}
	return !now.Before(expiresAt)
}

// ValidateTransferDecisionInput validates transfer accept/reject/cancel
// inputs with the same shape as ValidateMemberInviteDecisionInput (trimmed
// ref + positive actor) but maps to the billing error domain
// (ErrBillingWorkspaceInvalid, not ErrForbidden).
func ValidateTransferDecisionInput(billingAccountInternalID int64, invitePublicID string, actorUserID int64) error {
	if billingAccountInternalID <= 0 || strings.TrimSpace(invitePublicID) == "" || actorUserID <= 0 {
		return ErrBillingWorkspaceInvalid
	}
	return nil
}

// TransferNotificationKey builds the durable handover dedup key. It reuses
// the invite-notification value-object shape (invite public id + stable
// lifecycle identity) so reissues and retries collapse to one notification.
func TransferNotificationKey(transferPublicID, idempotencyKey string) string {
	return fmt.Sprintf("handover:%s:%x", transferPublicID, sha256.Sum256([]byte(idempotencyKey)))
}

// WorkspaceInvitePolicy returns the workspace-scope lifecycle policy.
func WorkspaceInvitePolicy() MemberInvitePolicy {
	return MemberInvitePolicy{
		Scope:          InviteScopeWorkspace,
		PublicIDPrefix: WorkspaceInvitePrefix,
		ConflictErr:    ErrWorkspaceInviteConflict,
		ExpiredErr:     ErrWorkspaceInviteExpired,
		NewPublicID:    NewWorkspaceInvitePublicID,
	}
}

// OrganizationInvitePolicy returns the organization-scope lifecycle policy.
func OrganizationInvitePolicy() MemberInvitePolicy {
	return MemberInvitePolicy{
		Scope:          InviteScopeOrganization,
		PublicIDPrefix: OrganizationInvitePrefix,
		ConflictErr:    ErrOrganizationInviteConflict,
		ExpiredErr:     ErrOrganizationInviteExpired,
		NewPublicID:    NewOrganizationInvitePublicID,
	}
}

// NormalizeMemberInviteEmail lowercases and trims an invitee address.
func NormalizeMemberInviteEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// NormalizeMemberInviteRole defaults a blank invite role to member.
func NormalizeMemberInviteRole(role string) string {
	role = strings.TrimSpace(role)
	if role == "" {
		return RoleMember
	}
	return role
}

// ValidateMemberInviteRole accepts member/admin on the invite path for both
// scopes (owner grants never flow through invites).
func ValidateMemberInviteRole(role string) error {
	if role != RoleMember && role != RoleAdmin {
		return ErrForbidden
	}
	return nil
}

// ValidateMemberInviteCreateInput normalizes email/role for creation.
// inviteeUserID carries the resolved registered user (>0); 0 leaves
// invitee_user_id NULL for unregistered invitees.
func ValidateMemberInviteCreateInput(email, role string) (string, string, error) {
	email = NormalizeMemberInviteEmail(email)
	if email == "" {
		return "", "", ErrForbidden
	}
	role = NormalizeMemberInviteRole(role)
	if err := ValidateMemberInviteRole(role); err != nil {
		return "", "", err
	}
	return email, role, nil
}

// MintMemberInviteToken mints a 32-byte crypto token and its SHA-256 hex hash.
// Only the hash is persisted; the raw token is the unguessable capability.
func MintMemberInviteToken() (rawToken, tokenHash string, err error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", err
	}
	rawToken = hex.EncodeToString(tokenBytes)
	return rawToken, MemberInviteTokenHash(rawToken), nil
}

// MemberInviteTokenHash hashes a raw token for storage/lookup.
func MemberInviteTokenHash(rawToken string) string {
	hash := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(hash[:])
}

// IsMemberInviteRef reports whether ref is a public-id reference for the
// scope prefix (vs a raw token capability).
func IsMemberInviteRef(ref, prefix string) bool {
	return strings.HasPrefix(ref, prefix)
}

// ValidateMemberInviteDecisionInput guards accept/decline inputs.
func ValidateMemberInviteDecisionInput(rawTokenOrPublicID string, recipientUserID int64) error {
	if strings.TrimSpace(rawTokenOrPublicID) == "" || recipientUserID <= 0 {
		return ErrForbidden
	}
	return nil
}

// CheckMemberInviteRecipient enforces anti-enumeration: an email/user mismatch
// converges to ErrNotFound (not ErrForbidden) so invite existence is not
// leaked. A missing email is safe only when a matching user id binds the invite.
func CheckMemberInviteRecipient(inviteeEmail string, inviteeUserID *int64, recipientUserID int64, recipientEmail string) error {
	recipientEmail = NormalizeMemberInviteEmail(recipientEmail)
	if recipientEmail == "" && inviteeUserID == nil {
		return ErrNotFound
	}
	if recipientEmail != "" && !strings.EqualFold(inviteeEmail, recipientEmail) {
		return ErrNotFound
	}
	if inviteeUserID != nil && *inviteeUserID != recipientUserID {
		return ErrNotFound
	}
	return nil
}

// MemberInviteExpired reports whether a pending invite is past expiry.
func MemberInviteExpired(expiresAt, now time.Time) bool {
	return now.After(expiresAt)
}

// CopyMemberInvite deep-copies invitee/responded pointers.
func CopyMemberInvite(inv *MemberInvite) *MemberInvite {
	if inv == nil {
		return nil
	}
	out := *inv
	if inv.InviteeUserID != nil {
		uid := *inv.InviteeUserID
		out.InviteeUserID = &uid
	}
	if inv.RespondedAt != nil {
		t := *inv.RespondedAt
		out.RespondedAt = &t
	}
	if inv.RespondedByUserID != nil {
		uid := *inv.RespondedByUserID
		out.RespondedByUserID = &uid
	}
	return &out
}

// WithMemberInviteInviteeUserID attaches the resolved invitee (>0) or nil.
func WithMemberInviteInviteeUserID(inv *MemberInvite, inviteeUserID int64) *MemberInvite {
	if inv != nil && inviteeUserID > 0 {
		uid := inviteeUserID
		inv.InviteeUserID = &uid
	}
	return inv
}

// MemberInviteFromWorkspace projects workspace facts into the shared lifecycle.
func MemberInviteFromWorkspace(inv *WorkspaceMemberInvite) *MemberInvite {
	if inv == nil {
		return nil
	}
	return &MemberInvite{
		Scope:             InviteScopeWorkspace,
		InternalID:        inv.InternalID,
		PublicID:          inv.PublicID,
		ScopeInternalID:   inv.WorkspaceID,
		ScopePublicID:     inv.WorkspacePublicID,
		ScopeName:         inv.WorkspaceName,
		InviterUserID:     inv.InviterUserID,
		InviteeEmail:      inv.InviteeEmail,
		InviteeUserID:     inv.InviteeUserID,
		Role:              inv.Role,
		Status:            inv.Status,
		TokenHash:         inv.TokenHash,
		ExpiresAt:         inv.ExpiresAt,
		CreatedAt:         inv.CreatedAt,
		UpdatedAt:         inv.UpdatedAt,
		RespondedAt:       inv.RespondedAt,
		RespondedByUserID: inv.RespondedByUserID,
	}
}

// WorkspaceInviteFromMember projects lifecycle facts into a workspace invite.
func WorkspaceInviteFromMember(inv *MemberInvite) *WorkspaceMemberInvite {
	if inv == nil {
		return nil
	}
	return &WorkspaceMemberInvite{
		InternalID:        inv.InternalID,
		PublicID:          inv.PublicID,
		WorkspaceID:       inv.ScopeInternalID,
		WorkspacePublicID: inv.ScopePublicID,
		WorkspaceName:     inv.ScopeName,
		InviterUserID:     inv.InviterUserID,
		InviteeEmail:      inv.InviteeEmail,
		InviteeUserID:     inv.InviteeUserID,
		Role:              inv.Role,
		Status:            inv.Status,
		TokenHash:         inv.TokenHash,
		ExpiresAt:         inv.ExpiresAt,
		CreatedAt:         inv.CreatedAt,
		UpdatedAt:         inv.UpdatedAt,
		RespondedAt:       inv.RespondedAt,
		RespondedByUserID: inv.RespondedByUserID,
	}
}

// MemberInviteFromOrganization projects organization facts into the shared lifecycle.
func MemberInviteFromOrganization(inv *OrganizationMemberInvite) *MemberInvite {
	if inv == nil {
		return nil
	}
	return &MemberInvite{
		Scope:             InviteScopeOrganization,
		InternalID:        inv.InternalID,
		PublicID:          inv.PublicID,
		ScopeInternalID:   inv.OrganizationID,
		ScopePublicID:     inv.OrganizationPublicID,
		ScopeName:         inv.OrganizationName,
		InviterUserID:     inv.InviterUserID,
		InviteeEmail:      inv.InviteeEmail,
		InviteeUserID:     inv.InviteeUserID,
		Role:              inv.Role,
		Status:            inv.Status,
		TokenHash:         inv.TokenHash,
		ExpiresAt:         inv.ExpiresAt,
		CreatedAt:         inv.CreatedAt,
		UpdatedAt:         inv.UpdatedAt,
		RespondedAt:       inv.RespondedAt,
		RespondedByUserID: inv.RespondedByUserID,
	}
}

// OrganizationInviteFromMember projects lifecycle facts into an organization invite.
func OrganizationInviteFromMember(inv *MemberInvite) *OrganizationMemberInvite {
	if inv == nil {
		return nil
	}
	return &OrganizationMemberInvite{
		InternalID:           inv.InternalID,
		PublicID:             inv.PublicID,
		OrganizationID:       inv.ScopeInternalID,
		OrganizationPublicID: inv.ScopePublicID,
		OrganizationName:     inv.ScopeName,
		InviterUserID:        inv.InviterUserID,
		InviteeEmail:         inv.InviteeEmail,
		InviteeUserID:        inv.InviteeUserID,
		Role:                 inv.Role,
		Status:               inv.Status,
		TokenHash:            inv.TokenHash,
		ExpiresAt:            inv.ExpiresAt,
		CreatedAt:            inv.CreatedAt,
		UpdatedAt:            inv.UpdatedAt,
		RespondedAt:          inv.RespondedAt,
		RespondedByUserID:    inv.RespondedByUserID,
	}
}

// MarkMemberInviteExpiredCopy marks a copy expired when pending and past
// expiry. Read paths (get/list) use this: expiry is visible immediately but
// persisted lazily on write paths (accept/decline/cancel), see ADR-0039.
func MarkMemberInviteExpiredCopy(inv *MemberInvite, now time.Time) *MemberInvite {
	if inv != nil && inv.Status == MemberInviteStatusPending && MemberInviteExpired(inv.ExpiresAt, now) {
		cp := CopyMemberInvite(inv)
		cp.Status = MemberInviteStatusExpired
		return cp
	}
	return inv
}
