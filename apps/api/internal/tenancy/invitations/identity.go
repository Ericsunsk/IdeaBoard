package invitations

import (
	"context"
	"errors"

	"ideaboard/api/internal/tenancy"
)

// IdentityReader supplies trusted identity facts; callers never provide the
// actor's email or construct an invitee's internal identity themselves.
type IdentityReader interface {
	Email(context.Context, int64) (string, error)
	FindUser(context.Context, string) (int64, error)
}

var errIdentityUnavailable = errors.New("invitations: identity reader unavailable")

func (s *Service) actorEmail(ctx context.Context, actorID int64) (string, error) {
	if s == nil || s.Identities == nil {
		return "", errIdentityUnavailable
	}
	email, err := s.Identities.Email(ctx, actorID)
	if err != nil {
		// A bound invite can still identify its recipient during an Auth
		// outage. The Store rejects an empty email without a matching user ID.
		return "", nil
	}
	return tenancy.NormalizeMemberInviteEmail(email), nil
}

func (s *Service) inviteeID(ctx context.Context, email string) (int64, error) {
	if s == nil || s.Identities == nil {
		return 0, errIdentityUnavailable
	}
	id, err := s.Identities.FindUser(ctx, tenancy.NormalizeMemberInviteEmail(email))
	if err != nil || id <= 0 {
		// Unknown users, provisioning gaps and lookup outages keep the
		// email-addressed invitation path; they never invent a user binding.
		return 0, nil
	}
	return id, nil
}

func (s *Service) decisionEmail(ctx context.Context, scope tenancy.InviteScope, ref string, actorID int64) (string, error) {
	if err := tenancy.ValidateMemberInviteDecisionInput(ref, actorID); err != nil {
		return "", err
	}
	// Unknown in-app references retain their 404 anti-enumeration result
	// before entering the decision transaction. Known rows remain subject to
	// the Store's locked recipient, lifecycle and permission checks.
	var err error
	if scope == tenancy.InviteScopeOrganization && tenancy.IsMemberInviteRef(ref, tenancy.OrganizationInvitePrefix) {
		_, err = s.Store.GetOrganizationMemberInvite(ctx, ref)
	} else if scope == tenancy.InviteScopeWorkspace && tenancy.IsMemberInviteRef(ref, tenancy.WorkspaceInvitePrefix) {
		_, err = s.Store.GetWorkspaceMemberInvite(ctx, ref)
	}
	if errors.Is(err, tenancy.ErrNotFound) {
		return "", tenancy.ErrNotFound
	}
	return s.actorEmail(ctx, actorID)
}

func (s *Service) ListPendingWorkspaceInvitesForUser(ctx context.Context, actorID int64) ([]*tenancy.WorkspaceMemberInvite, error) {
	email, err := s.actorEmail(ctx, actorID)
	if err != nil {
		return nil, err
	}
	return s.Store.ListPendingWorkspaceInvitesForUser(ctx, actorID, email)
}

func (s *Service) ListPendingOrganizationInvitesForUser(ctx context.Context, actorID int64) ([]*tenancy.OrganizationMemberInvite, error) {
	email, err := s.actorEmail(ctx, actorID)
	if err != nil {
		return nil, err
	}
	return s.Store.ListPendingOrganizationInvitesForUser(ctx, actorID, email)
}
