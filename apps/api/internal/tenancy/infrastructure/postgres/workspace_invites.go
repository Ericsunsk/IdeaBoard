// Package postgres implements tenancy.WorkspaceInviteStore against PostgreSQL/sqlc (#957).
//
// The write lifecycle (create/cancel/accept/decline) delegates to the
// unified core in member_invites.go; this file binds the workspace scope:
// scope lock + closed-org rejection + Default protection, the 5-member
// capacity invariant, workspace_memberships writes, and row mapping.
package postgres

import (
	"context"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/tenancy"

	"github.com/jackc/pgx/v5/pgtype"
)

func mapWorkspaceMemberInvite(row dbsqlc.WorkspaceMemberInvite, workspacePublicID, workspaceName string) *tenancy.WorkspaceMemberInvite {
	out := &tenancy.WorkspaceMemberInvite{
		InternalID:        row.InternalID,
		PublicID:          row.PublicID,
		WorkspaceID:       row.WorkspaceID,
		WorkspacePublicID: workspacePublicID,
		WorkspaceName:     workspaceName,
		InviterUserID:     row.InviterUserID,
		InviteeEmail:      row.InviteeEmail,
		Role:              row.Role,
		Status:            row.Status,
		TokenHash:         row.TokenHash,
		ExpiresAt:         dbutil.TimeValue(row.ExpiresAt),
		CreatedAt:         dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:         dbutil.TimeValue(row.UpdatedAt),
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

// workspaceInviteTx binds the unified lifecycle to the workspace scope.
// policy supplies shared rules; the four hooks below are the retained
// Tx/lock-order-sensitive parts (scope locks + membership grant) documented
// in member_invites.go. Table dispatch lives in the unified core switch.
func workspaceInviteTx() postgresInviteTx {
	return postgresInviteTx{
		policy:   tenancy.WorkspaceInvitePolicy(),
		orgStore: nil,
		lockScopeForCreate: func(ctx context.Context, q *dbsqlc.Queries, scopePublicID string) (int64, string, string, error) {
			workspaceRow, err := lockWorkspaceMemberScope(ctx, q, scopePublicID)
			if err != nil {
				return 0, "", "", err
			}
			workspaceID := workspaceRow.InternalID
			if err := rejectClosedOrgWorkspaceInTx(ctx, q, workspaceID); err != nil {
				return 0, "", "", err
			}
			// Personal Default is protected (#952).
			if !workspaceRow.OrganizationID.Valid && workspaceRow.IsDefault {
				return 0, "", "", tenancy.ErrDefaultWorkspaceProtected
			}
			return workspaceID, workspaceRow.PublicID, workspaceRow.Name, nil
		},
		lockScopeForCancel: func(ctx context.Context, q *dbsqlc.Queries, scopePublicID string) (int64, error) {
			workspace, err := lockWorkspaceMemberScope(ctx, q, scopePublicID)
			return workspace.InternalID, err
		},
		checkAcceptScope: func(ctx context.Context, q *dbsqlc.Queries, inv *tenancy.MemberInvite) error {
			workspaceRow, err := lockWorkspaceMemberScope(ctx, q, inv.ScopePublicID)
			if err != nil {
				return err
			}
			if err := rejectClosedOrgWorkspaceInTx(ctx, q, inv.ScopeInternalID); err != nil {
				return err
			}
			if !workspaceRow.OrganizationID.Valid && workspaceRow.IsDefault {
				return tenancy.ErrDefaultWorkspaceProtected
			}
			return nil
		},
		grantMember: func(ctx context.Context, q *dbsqlc.Queries, inv *tenancy.MemberInvite, recipientUserID int64) (any, error) {
			workspace, err := q.GetWorkspaceByInternalID(ctx, inv.ScopeInternalID)
			if err != nil {
				return nil, notFound(err)
			}
			if workspace.OrganizationID.Valid {
				if _, err := q.GetOrganizationMembership(ctx, dbsqlc.GetOrganizationMembershipParams{
					OrganizationID: workspace.OrganizationID.Int64, UserID: recipientUserID,
				}); err != nil {
					return nil, notFound(err)
				}
			}

			lookup := dbsqlc.GetWorkspaceMembershipParams{WorkspaceID: inv.ScopeInternalID, UserID: recipientUserID}
			row, err := q.GetWorkspaceMembership(ctx, lookup)
			if database.IsNotFound(err) {
				activeCount, countErr := q.CountWorkspaceMembers(ctx, inv.ScopeInternalID)
				if countErr != nil {
					return nil, countErr
				}
				if activeCount >= tenancy.MaxWorkspaceMembers {
					return nil, tenancy.ErrWorkspaceMemberLimitReached
				}
				if _, err := q.InsertWorkspaceMembershipIgnore(ctx, dbsqlc.InsertWorkspaceMembershipIgnoreParams{
					WorkspaceID: inv.ScopeInternalID,
					UserID:      recipientUserID,
					Role:        inv.Role,
				}); err != nil {
					return nil, err
				}
				row, err = q.GetWorkspaceMembership(ctx, lookup)
				if err != nil {
					return nil, notFound(err)
				}
			} else if err != nil {
				return nil, notFound(err)
			}
			externalIDs, err := userExternalIDs(ctx, q, []int64{recipientUserID})
			if err != nil {
				return nil, err
			}
			var displayPublicID string
			if ws, wsErr := q.GetWorkspaceByInternalID(ctx, inv.ScopeInternalID); wsErr == nil {
				displayPublicID = ws.PublicID
			} else {
				displayPublicID = inv.ScopePublicID
			}
			return mapWorkspaceMember(displayPublicID, row, externalIDs), nil
		},
	}
}

// resolveWorkspaceInviteScope loads the workspace display identity for reads.
func (s *Store) resolveWorkspaceInviteScope(ctx context.Context, workspacePublicID string) (*tenancy.Workspace, error) {
	return s.GetWorkspace(ctx, strings.TrimSpace(workspacePublicID))
}

// CreateWorkspaceMemberInvite creates a new member invite with token generation and 5-member limit check.
// inviteeUserID carries the resolved registered user (>0) so the invitee_user_id
// column is persisted at creation; 0 leaves it NULL for unregistered invitees
// (email branch stays the lookup path).
func (s *Store) CreateWorkspaceMemberInvite(ctx context.Context, workspacePublicID string, inviterUserID int64, inviteeEmail, role string, inviteeUserID int64) (*tenancy.WorkspaceMemberInvite, string, error) {
	var created *tenancy.MemberInvite
	var rawToken string
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		inv, token, err := createMemberInviteTx(ctx, q, workspaceInviteTx(), workspacePublicID, inviterUserID, inviteeEmail, role, inviteeUserID)
		if err != nil {
			return err
		}
		created, rawToken = inv, token
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return tenancy.WorkspaceInviteFromMember(created), rawToken, nil
}

// ListWorkspaceMemberInvites lists pending unexpired member invitations for a
// workspace. Decided/expired rows are excluded: the owner list is the
// actionable set (cancel), matching ListOrganizationMemberInvites; invite
// history stays in the invite row audit, not in this list (see ADR-0039).
// Read-tolerant by design: unlocked read, still listable when the owning Organization is closed; mutations fail closed.
func (s *Store) ListWorkspaceMemberInvites(ctx context.Context, workspacePublicID string) ([]*tenancy.WorkspaceMemberInvite, error) {
	workspace, err := s.resolveWorkspaceInviteScope(ctx, workspacePublicID)
	if err != nil {
		return nil, err
	}
	rows, err := s.dbWithContext(ctx).q.ListWorkspaceMemberInvitesByWorkspaceID(ctx, dbsqlc.ListWorkspaceMemberInvitesByWorkspaceIDParams{
		WorkspaceID: workspace.InternalID,
		Status:      pgtype.Text{String: tenancy.MemberInviteStatusPending, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	unified := make([]*tenancy.MemberInvite, 0, len(rows))
	for _, r := range rows {
		unified = append(unified, tenancy.MemberInviteFromWorkspace(mapWorkspaceMemberInvite(r, workspace.PublicID, workspace.Name)))
	}
	filtered := filterPendingMemberInvites(unified, time.Now())
	out := make([]*tenancy.WorkspaceMemberInvite, 0, len(filtered))
	for _, inv := range filtered {
		out = append(out, tenancy.WorkspaceInviteFromMember(inv))
	}
	return out, nil
}

// CancelWorkspaceMemberInvite cancels a pending workspace invitation.
func (s *Store) CancelWorkspaceMemberInvite(ctx context.Context, workspacePublicID, invitePublicID string, callerUserID int64) error {
	return s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		return cancelMemberInviteTx(ctx, q, workspaceInviteTx(), workspacePublicID, invitePublicID, callerUserID)
	})
}

// ListPendingWorkspaceInvitesForUser lists pending unexpired invites matching user's email or ID.
func (s *Store) ListPendingWorkspaceInvitesForUser(ctx context.Context, userID int64, userEmail string) ([]*tenancy.WorkspaceMemberInvite, error) {
	userEmail = tenancy.NormalizeMemberInviteEmail(userEmail)
	var rows []dbsqlc.WorkspaceMemberInvite
	seen := make(map[string]bool)
	if userEmail != "" {
		byEmail, err := s.q.ListPendingWorkspaceMemberInvitesByEmail(ctx, userEmail)
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
		byUser, err := s.q.ListPendingWorkspaceMemberInvitesByInviteeUserID(ctx, pgtype.Int8{Int64: userID, Valid: true})
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
	out := make([]*tenancy.WorkspaceMemberInvite, 0, len(rows))
	wsCache := make(map[int64]struct {
		publicID string
		name     string
	})
	for _, r := range rows {
		info, found := wsCache[r.WorkspaceID]
		if !found {
			ws, err := s.q.GetWorkspaceByInternalID(ctx, r.WorkspaceID)
			if err == nil {
				info.publicID = ws.PublicID
				info.name = ws.Name
			}
			wsCache[r.WorkspaceID] = info
		}
		out = append(out, mapWorkspaceMemberInvite(r, info.publicID, info.name))
	}
	return out, nil
}

// GetWorkspaceMemberInvite returns an invitation by public ID for post-decision
// event rebuilds. Expiry is marked in memory only (same as GetWorkspaceInviteByToken):
// the row is lazily expired on write paths (accept/decline/cancel), see ADR-0039.
// Read-tolerant by design: the lookup is unscoped (NULL workspace predicate)
// and never fails closed on closure; mutations enforce attribution and
// closure via locked writes. The SQL predicate exists for write-path scoping.
func (s *Store) GetWorkspaceMemberInvite(ctx context.Context, invitePublicID string) (*tenancy.WorkspaceMemberInvite, error) {
	invitePublicID = strings.TrimSpace(invitePublicID)
	if invitePublicID == "" {
		return nil, tenancy.ErrNotFound
	}
	row, err := s.q.GetWorkspaceMemberInviteByPublicID(ctx, dbsqlc.GetWorkspaceMemberInviteByPublicIDParams{
		PublicID:    invitePublicID,
		WorkspaceID: pgtype.Int8{},
	})
	if err != nil {
		return nil, notFound(err)
	}
	ws, err := s.q.GetWorkspaceByInternalID(ctx, row.WorkspaceID)
	wsPublicID, wsName := "", ""
	if err == nil {
		wsPublicID, wsName = ws.PublicID, ws.Name
	}
	invite := tenancy.MemberInviteFromWorkspace(mapWorkspaceMemberInvite(row, wsPublicID, wsName))
	if invite.Status == tenancy.MemberInviteStatusPending && tenancy.MemberInviteExpired(invite.ExpiresAt, time.Now()) {
		invite.Status = tenancy.MemberInviteStatusExpired
	}
	return tenancy.WorkspaceInviteFromMember(invite), nil
}

// GetWorkspaceInviteByToken returns an invitation by raw token.
// Expiry is marked in memory only (see GetWorkspaceMemberInvite): write paths
// lazily persist the expired state. Callers must not treat the returned
// pending-via-token row as fresh without re-reading after a decision.
// The token is an unguessable capability: the read stays unscoped (NULL
// workspace predicate) so public preview works without a workspace context.
// Write paths re-lock with the workspace predicate and keep Go secondary
// checks (workspace match, email/user match).
func (s *Store) GetWorkspaceInviteByToken(ctx context.Context, rawToken string) (*tenancy.WorkspaceMemberInvite, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return nil, tenancy.ErrNotFound
	}
	row, err := s.q.GetWorkspaceMemberInviteByTokenHash(ctx, dbsqlc.GetWorkspaceMemberInviteByTokenHashParams{
		TokenHash:   tenancy.MemberInviteTokenHash(rawToken),
		WorkspaceID: pgtype.Int8{},
	})
	if err != nil {
		return nil, notFound(err)
	}
	ws, err := s.q.GetWorkspaceByInternalID(ctx, row.WorkspaceID)
	wsPublicID, wsName := "", ""
	if err == nil {
		wsPublicID, wsName = ws.PublicID, ws.Name
	}
	invite := tenancy.MemberInviteFromWorkspace(mapWorkspaceMemberInvite(row, wsPublicID, wsName))
	if invite.Status == tenancy.MemberInviteStatusPending && tenancy.MemberInviteExpired(invite.ExpiresAt, time.Now()) {
		invite.Status = tenancy.MemberInviteStatusExpired
	}
	return tenancy.WorkspaceInviteFromMember(invite), nil
}

// AcceptWorkspaceInvite runs the unified core (see postgres/member_invites.go and
// tenancy.WorkspaceInviteStore); it returns the authoritative post-decision pair.
func (s *Store) AcceptWorkspaceInvite(ctx context.Context, rawTokenOrPublicID string, recipientUserID int64, recipientEmail string) (*tenancy.WorkspaceMember, *tenancy.WorkspaceMemberInvite, error) {
	var member *tenancy.WorkspaceMember
	var decided *tenancy.MemberInvite
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		decidedInv, granted, err := acceptMemberInviteTx(ctx, q, workspaceInviteTx(), rawTokenOrPublicID, recipientUserID, recipientEmail)
		if err != nil {
			return err
		}
		m, ok := granted.(*tenancy.WorkspaceMember)
		if !ok || m == nil || decidedInv == nil {
			return tenancy.ErrNotFound
		}
		member, decided = m, decidedInv
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return member, tenancy.WorkspaceInviteFromMember(decided), nil
}

// DeclineWorkspaceInvite runs the unified core (see postgres/member_invites.go and
// tenancy.WorkspaceInviteStore); it returns the authoritative post-decision invite.
func (s *Store) DeclineWorkspaceInvite(ctx context.Context, rawTokenOrPublicID string, recipientUserID int64, recipientEmail string) (*tenancy.WorkspaceMemberInvite, error) {
	var decided *tenancy.MemberInvite
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		decidedInv, err := declineMemberInviteTx(ctx, q, workspaceInviteTx(), rawTokenOrPublicID, recipientUserID, recipientEmail)
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
	return tenancy.WorkspaceInviteFromMember(decided), nil
}
