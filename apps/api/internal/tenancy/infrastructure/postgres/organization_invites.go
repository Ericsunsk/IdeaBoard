// Package postgres implements tenancy.OrganizationInviteStore against PostgreSQL/sqlc (#957, ADR-0041).
//
// The write lifecycle (create/cancel/accept/decline) delegates to the
// unified core in member_invites.go; this file binds the organization scope:
// active-organization lock, inviter role gates (owner/admin, owner-only
// admin grants), the live single-organization limit at accept, and
// organization_memberships writes.
package postgres

import (
	"context"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tenancy"

	"github.com/jackc/pgx/v5/pgtype"
)

func mapOrganizationMemberInvite(row dbsqlc.OrganizationMemberInvite, organizationPublicID, organizationName string) *tenancy.OrganizationMemberInvite {
	out := &tenancy.OrganizationMemberInvite{
		InternalID:           row.InternalID,
		PublicID:             row.PublicID,
		OrganizationID:       row.OrganizationID,
		OrganizationPublicID: organizationPublicID,
		OrganizationName:     organizationName,
		InviterUserID:        row.InviterUserID,
		InviteeEmail:         row.InviteeEmail,
		Role:                 row.Role,
		Status:               row.Status,
		TokenHash:            row.TokenHash,
		ExpiresAt:            dbutil.TimeValue(row.ExpiresAt),
		CreatedAt:            dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:            dbutil.TimeValue(row.UpdatedAt),
	}
	if row.InviteeUserID.Valid {
		v := row.InviteeUserID.Int64
		out.InviteeUserID = &v
	}
	if row.RespondedAt.Valid {
		t := row.RespondedAt.Time
		out.RespondedAt = &t
	}
	if row.RespondedByUserID.Valid {
		v := row.RespondedByUserID.Int64
		out.RespondedByUserID = &v
	}
	return out
}

// organizationInviteTx binds the unified lifecycle to the organization scope.
// policy supplies shared rules; the four hooks below are the retained
// Tx/lock-order-sensitive parts (active-org locks + membership grant)
// documented in member_invites.go. Table dispatch lives in the core switch.
func organizationInviteTx(orgStore *OrganizationStore) postgresInviteTx {
	return postgresInviteTx{
		policy:   tenancy.OrganizationInvitePolicy(),
		orgStore: orgStore,
		lockScopeForCreate: func(ctx context.Context, q *dbsqlc.Queries, scopePublicID string) (int64, string, string, error) {
			organization, err := lockActiveOrganization(ctx, q, scopePublicID)
			if err != nil {
				return 0, "", "", err
			}
			return organization.InternalID, organization.PublicID, organization.Name, nil
		},
		lockScopeForCancel: func(ctx context.Context, q *dbsqlc.Queries, scopePublicID string) (int64, error) {
			organization, err := lockActiveOrganization(ctx, q, scopePublicID)
			if err != nil {
				return 0, err
			}
			return organization.InternalID, nil
		},
		checkAcceptScope: func(ctx context.Context, q *dbsqlc.Queries, inv *tenancy.MemberInvite) error {
			orgRow, err := q.GetOrganizationByInternalID(ctx, inv.ScopeInternalID)
			if err != nil {
				return notFound(err)
			}
			if _, err := lockActiveOrganization(ctx, q, orgRow.PublicID); err != nil {
				return err
			}
			return nil
		},
		grantMember: func(ctx context.Context, q *dbsqlc.Queries, inv *tenancy.MemberInvite, recipientUserID int64) (any, error) {
			organization, err := lockActiveOrganizationResult(ctx, q, inv.ScopeInternalID)
			if err != nil {
				return nil, err
			}
			// Lock user row and enforce single-organization limit.
			if _, err := q.LockUserRow(ctx, recipientUserID); err != nil {
				return nil, err
			}
			count, err := q.CountOrganizationsByUserID(ctx, recipientUserID)
			if err != nil {
				return nil, err
			}
			if count >= 1 {
				return nil, tenancy.ErrOrganizationLimitReached
			}
			if err := q.UpsertOrganizationMembership(ctx, dbsqlc.UpsertOrganizationMembershipParams{
				OrganizationID: organization.InternalID,
				UserID:         recipientUserID,
				Role:           inv.Role,
			}); err != nil {
				return nil, err
			}
			memRow, err := q.GetOrganizationMembership(ctx, dbsqlc.GetOrganizationMembershipParams{
				OrganizationID: organization.InternalID,
				UserID:         recipientUserID,
			})
			if err != nil {
				return nil, notFound(err)
			}
			externalIDs, err := userExternalIDs(ctx, q, []int64{recipientUserID})
			if err != nil {
				return nil, err
			}
			return &tenancy.OrganizationMember{
				OrganizationPublicID: organization.PublicID,
				UserID:               recipientUserID,
				UserExternalID:       externalIDs[recipientUserID],
				Role:                 memRow.Role,
				CreatedAt:            dbutil.TimeValue(memRow.CreatedAt),
			}, nil
		},
	}
}

// lockActiveOrganizationResult re-locks the live organization by internal id.
// The public lockActiveOrganization helper scopes by public id, so this
// resolves the public id first; both reads join the same transaction.
func lockActiveOrganizationResult(ctx context.Context, q *dbsqlc.Queries, organizationInternalID int64) (*tenancy.Organization, error) {
	orgRow, err := q.GetOrganizationByInternalID(ctx, organizationInternalID)
	if err != nil {
		return nil, notFound(err)
	}
	return lockActiveOrganization(ctx, q, orgRow.PublicID)
}

// CreateOrganizationMemberInvite creates a new member invite with token generation and permission checks.
// inviteeUserID carries the resolved registered user (>0) so the invitee_user_id
// column is persisted at creation; 0 leaves it NULL for unregistered invitees
// (email branch stays the lookup path).
func (s *OrganizationStore) CreateOrganizationMemberInvite(ctx context.Context, organizationPublicID string, inviterUserID int64, inviteeEmail, role string, inviteeUserID int64) (*tenancy.OrganizationMemberInvite, string, error) {
	var created *tenancy.MemberInvite
	var rawToken string
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		inv, token, err := createMemberInviteTx(ctx, q, organizationInviteTx(s), organizationPublicID, inviterUserID, inviteeEmail, role, inviteeUserID)
		if err != nil {
			return err
		}
		created, rawToken = inv, token
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return tenancy.OrganizationInviteFromMember(created), rawToken, nil
}

// ListOrganizationMemberInvites lists pending unexpired invites for an organization.
// Decided/expired rows are excluded: the owner list is the actionable set
// (cancel), matching ListWorkspaceMemberInvites; invite history stays in the
// invite row audit, not in this list (see ADR-0041).
// Read-tolerant by design: unlocked read, still listable when closed; mutations fail closed.
func (s *OrganizationStore) ListOrganizationMemberInvites(ctx context.Context, organizationPublicID string) ([]*tenancy.OrganizationMemberInvite, error) {
	organization, err := s.GetOrganization(ctx, organizationPublicID)
	if err != nil {
		return nil, err
	}
	rows, err := s.dbWithContext(ctx).q.ListOrganizationMemberInvitesByOrganizationID(ctx, dbsqlc.ListOrganizationMemberInvitesByOrganizationIDParams{
		OrganizationID: organization.InternalID,
		Status:         pgtype.Text{String: tenancy.MemberInviteStatusPending, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	unified := make([]*tenancy.MemberInvite, 0, len(rows))
	for _, r := range rows {
		unified = append(unified, tenancy.MemberInviteFromOrganization(mapOrganizationMemberInvite(r, organization.PublicID, organization.Name)))
	}
	filtered := filterPendingMemberInvites(unified, time.Now())
	out := make([]*tenancy.OrganizationMemberInvite, 0, len(filtered))
	for _, inv := range filtered {
		out = append(out, tenancy.OrganizationInviteFromMember(inv))
	}
	return out, nil
}

// CancelOrganizationMemberInvite cancels a pending invitation.
func (s *OrganizationStore) CancelOrganizationMemberInvite(ctx context.Context, organizationPublicID, invitePublicID string, callerUserID int64) error {
	organizationPublicID = strings.TrimSpace(organizationPublicID)
	invitePublicID = strings.TrimSpace(invitePublicID)
	if organizationPublicID == "" || invitePublicID == "" {
		return tenancy.ErrNotFound
	}
	return s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		return cancelMemberInviteTx(ctx, q, organizationInviteTx(s), organizationPublicID, invitePublicID, callerUserID)
	})
}

// ListPendingOrganizationInvitesForUser lists pending unexpired invites matching user's email or ID.
func (s *OrganizationStore) ListPendingOrganizationInvitesForUser(ctx context.Context, userID int64, userEmail string) ([]*tenancy.OrganizationMemberInvite, error) {
	userEmail = tenancy.NormalizeMemberInviteEmail(userEmail)
	var rows []dbsqlc.OrganizationMemberInvite
	seen := make(map[string]bool)
	q := s.dbWithContext(ctx).q
	if userEmail != "" {
		byEmail, err := q.ListPendingOrganizationMemberInvitesByEmail(ctx, userEmail)
		if err != nil {
			return nil, err
		}
		for _, r := range byEmail {
			if !seen[r.PublicID] {
				seen[r.PublicID] = true
				rows = append(rows, r)
			}
		}
	}
	if userID > 0 {
		byUser, err := q.ListPendingOrganizationMemberInvitesByInviteeUserID(ctx, pgtype.Int8{Int64: userID, Valid: true})
		if err != nil {
			return nil, err
		}
		for _, r := range byUser {
			if !seen[r.PublicID] {
				seen[r.PublicID] = true
				rows = append(rows, r)
			}
		}
	}
	out := make([]*tenancy.OrganizationMemberInvite, 0, len(rows))
	orgCache := make(map[int64]struct {
		publicID string
		name     string
	})
	for _, r := range rows {
		info, found := orgCache[r.OrganizationID]
		if !found {
			org, err := q.GetOrganizationByInternalID(ctx, r.OrganizationID)
			if err == nil {
				info.publicID = org.PublicID
				info.name = org.Name
			}
			orgCache[r.OrganizationID] = info
		}
		out = append(out, mapOrganizationMemberInvite(r, info.publicID, info.name))
	}
	return out, nil
}

// GetOrganizationMemberInvite returns an invitation by public ID for post-decision
// event rebuilds. Expiry is marked in memory only (same as GetOrganizationInviteByToken):
// the row is lazily expired on write paths (accept/decline/cancel), see ADR-0041.
func (s *OrganizationStore) GetOrganizationMemberInvite(ctx context.Context, invitePublicID string) (*tenancy.OrganizationMemberInvite, error) {
	invitePublicID = strings.TrimSpace(invitePublicID)
	if invitePublicID == "" {
		return nil, tenancy.ErrNotFound
	}
	q := s.dbWithContext(ctx).q
	row, err := q.GetOrganizationMemberInviteByPublicID(ctx, dbsqlc.GetOrganizationMemberInviteByPublicIDParams{
		PublicID:       invitePublicID,
		OrganizationID: pgtype.Int8{},
	})
	if err != nil {
		return nil, notFound(err)
	}
	org, err := q.GetOrganizationByInternalID(ctx, row.OrganizationID)
	orgPublicID, orgName := "", ""
	if err == nil {
		orgPublicID, orgName = org.PublicID, org.Name
	}
	invite := tenancy.MemberInviteFromOrganization(mapOrganizationMemberInvite(row, orgPublicID, orgName))
	if invite.Status == tenancy.MemberInviteStatusPending && tenancy.MemberInviteExpired(invite.ExpiresAt, time.Now()) {
		invite.Status = tenancy.MemberInviteStatusExpired
	}
	return tenancy.OrganizationInviteFromMember(invite), nil
}

// GetOrganizationInviteByToken returns an invitation by raw token.
// Expiry is marked in memory only (see GetOrganizationMemberInvite): write paths
// lazily persist the expired state. Callers must not treat the returned
// pending-via-token row as fresh without re-reading after a decision.
func (s *OrganizationStore) GetOrganizationInviteByToken(ctx context.Context, rawToken string) (*tenancy.OrganizationMemberInvite, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return nil, tenancy.ErrNotFound
	}
	q := s.dbWithContext(ctx).q
	row, err := q.GetOrganizationMemberInviteByTokenHash(ctx, dbsqlc.GetOrganizationMemberInviteByTokenHashParams{
		TokenHash:      tenancy.MemberInviteTokenHash(rawToken),
		OrganizationID: pgtype.Int8{},
	})
	if err != nil {
		return nil, notFound(err)
	}
	org, err := q.GetOrganizationByInternalID(ctx, row.OrganizationID)
	orgPublicID, orgName := "", ""
	if err == nil {
		orgPublicID, orgName = org.PublicID, org.Name
	}
	invite := tenancy.MemberInviteFromOrganization(mapOrganizationMemberInvite(row, orgPublicID, orgName))
	if invite.Status == tenancy.MemberInviteStatusPending && tenancy.MemberInviteExpired(invite.ExpiresAt, time.Now()) {
		invite.Status = tenancy.MemberInviteStatusExpired
	}
	return tenancy.OrganizationInviteFromMember(invite), nil
}

// AcceptOrganizationInvite runs the unified core (see postgres/member_invites.go and
// tenancy.OrganizationInviteStore); it returns the authoritative post-decision pair.
func (s *OrganizationStore) AcceptOrganizationInvite(ctx context.Context, rawTokenOrPublicID string, recipientUserID int64, recipientEmail string) (*tenancy.OrganizationMember, *tenancy.OrganizationMemberInvite, error) {
	var member *tenancy.OrganizationMember
	var decided *tenancy.MemberInvite
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		decidedInv, granted, err := acceptMemberInviteTx(ctx, q, organizationInviteTx(s), rawTokenOrPublicID, recipientUserID, recipientEmail)
		if err != nil {
			return err
		}
		m, ok := granted.(*tenancy.OrganizationMember)
		if !ok || m == nil || decidedInv == nil {
			return tenancy.ErrNotFound
		}
		member, decided = m, decidedInv
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return member, tenancy.OrganizationInviteFromMember(decided), nil
}

// DeclineOrganizationInvite runs the unified core (see postgres/member_invites.go and
// tenancy.OrganizationInviteStore); it returns the authoritative post-decision invite.
func (s *OrganizationStore) DeclineOrganizationInvite(ctx context.Context, rawTokenOrPublicID string, recipientUserID int64, recipientEmail string) (*tenancy.OrganizationMemberInvite, error) {
	var decided *tenancy.MemberInvite
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		decidedInv, err := declineMemberInviteTx(ctx, q, organizationInviteTx(s), rawTokenOrPublicID, recipientUserID, recipientEmail)
		if err != nil {
			return err
		}
		if decidedInv == nil {
			return tenancy.ErrNotFound
		}
		decided = decidedInv
		return nil
	})
	if err != nil {
		return nil, err
	}
	return tenancy.OrganizationInviteFromMember(decided), nil
}
