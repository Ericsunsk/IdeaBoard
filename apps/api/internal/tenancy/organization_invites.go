package tenancy

// Organization member invitation domain models and persistence seams (#957, ADR-0041).

import (
	"context"
	"time"
)

// OrganizationMemberInvite represents an invitation to join an organization.
type OrganizationMemberInvite struct {
	InternalID           int64
	PublicID             string
	OrganizationID       int64
	OrganizationPublicID string
	OrganizationName     string
	InviterUserID        int64
	InviteeEmail         string
	InviteeUserID        *int64
	Role                 string
	Status               string
	TokenHash            string
	ExpiresAt            time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
	RespondedAt          *time.Time
	RespondedByUserID    *int64
}

// OrganizationInviteStore groups organization invitation persistence.
type OrganizationInviteStore interface {
	CreateOrganizationMemberInvite(ctx context.Context, organizationPublicID string, inviterUserID int64, inviteeEmail, role string, inviteeUserID int64) (*OrganizationMemberInvite, string, error)
	ListOrganizationMemberInvites(ctx context.Context, organizationPublicID string) ([]*OrganizationMemberInvite, error)
	CancelOrganizationMemberInvite(ctx context.Context, organizationPublicID, invitePublicID string, callerUserID int64) error
	ListPendingOrganizationInvitesForUser(ctx context.Context, userID int64, userEmail string) ([]*OrganizationMemberInvite, error)
	GetOrganizationInviteByToken(ctx context.Context, rawToken string) (*OrganizationMemberInvite, error)
	GetOrganizationMemberInvite(ctx context.Context, invitePublicID string) (*OrganizationMemberInvite, error)
	// Accept returns the member plus the authoritative post-decision
	// invite; Decline returns the authoritative post-decision invite.
	// Callers forward the returned invite to the notification path without
	// a pre-read snapshot or post-write re-fetch (see the unified core in
	// member_invites.go / postgres/member_invites.go).
	AcceptOrganizationInvite(ctx context.Context, rawTokenOrPublicID string, recipientUserID int64, recipientEmail string) (*OrganizationMember, *OrganizationMemberInvite, error)
	DeclineOrganizationInvite(ctx context.Context, rawTokenOrPublicID string, recipientUserID int64, recipientEmail string) (*OrganizationMemberInvite, error)
}
