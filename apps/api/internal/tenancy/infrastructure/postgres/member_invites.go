// Package postgres implements tenancy invite persistence against PostgreSQL/sqlc.
//
// member_invites.go is the unified invitation lifecycle core: create, cancel,
// accept, and decline share one sequencing parameterized by per-scope
// bindings in workspace_invites.go / organization_invites.go. Both scopes
// persist to their own table (workspace_member_invites /
// organization_member_invites), each with the same row-level FOR UPDATE lock
// and partial-unique pending-email pattern.
//
// Shared inviter and capacity rules live in tenancy.MemberInvitePolicy.
// Scope hooks own the database-specific lock order, Default/closure guards,
// membership prerequisites and writes. Billing transfers share token/expiry
// value objects, but retain their separate fund-guarded transaction sequence.
//
// Read paths (list/get/token/pending-for-user) stay per-scope but share the
// pending-only filter and recipient helpers from the tenancy core.
package postgres

import (
	"context"
	"strings"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/tenancy"

	"github.com/jackc/pgx/v5/pgtype"
)

// postgresInviteTx parameterizes the unified write lifecycle per scope.
// policy supplies the shared rules. The hooks below own scope-specific
// transaction locks and membership writes.
type postgresInviteTx struct {
	policy tenancy.MemberInvitePolicy
	// orgStore backs the organization inviter gate (nil for workspace).
	orgStore *OrganizationStore
	// lockScopeForCreate locks the scope and fails closed when the scope
	// cannot accept invites (closed org, Default workspace). Returns the
	// scope internal id plus display identity.
	lockScopeForCreate func(ctx context.Context, q *dbsqlc.Queries, scopePublicID string) (scopeInternalID int64, displayPublicID, displayName string, err error)
	// lockScopeForCancel locks the scope for cancellation. Workspace
	// cancel is read-tolerant on closure (membership lock only);
	// organization cancel fails closed (active lock).
	lockScopeForCancel func(ctx context.Context, q *dbsqlc.Queries, scopePublicID string) (scopeInternalID int64, err error)
	// checkAcceptScope locks the scope and fails closed when the scope
	// cannot accept members (closed org, Default workspace).
	checkAcceptScope func(ctx context.Context, q *dbsqlc.Queries, inv *tenancy.MemberInvite) error
	// grantMember writes the membership row and returns the scope member
	// projection as any (the binding casts back to its member type).
	// Retained: workspace_memberships vs organization_memberships targets
	// with incompatible lock/grant orders.
	grantMember func(ctx context.Context, q *dbsqlc.Queries, inv *tenancy.MemberInvite, recipientUserID int64) (any, error)
}

// findPendingInviteByEmailTx reports a live pending row for the email.
// Table dispatch rides policy.Scope; per-scope error quirks are preserved:
// workspace treats a missing row as no pending and propagates other
// failures, organization propagates non-NotFound errors.
func findPendingInviteByEmailTx(ctx context.Context, q *dbsqlc.Queries, scope tenancy.InviteScope, scopeInternalID int64, email string) (publicID string, expiresAt time.Time, found bool, err error) {
	switch scope {
	case tenancy.InviteScopeWorkspace:
		existing, err := q.GetPendingWorkspaceMemberInviteByEmail(ctx, dbsqlc.GetPendingWorkspaceMemberInviteByEmailParams{
			WorkspaceID:  scopeInternalID,
			InviteeEmail: email,
		})
		if err != nil {
			if database.IsNotFound(err) {
				return "", time.Time{}, false, nil
			}
			return "", time.Time{}, false, err
		}
		return existing.PublicID, existing.ExpiresAt.Time, true, nil
	case tenancy.InviteScopeOrganization:
		existing, err := q.GetPendingOrganizationMemberInviteByEmail(ctx, dbsqlc.GetPendingOrganizationMemberInviteByEmailParams{
			OrganizationID: scopeInternalID,
			InviteeEmail:   email,
		})
		if err != nil {
			if database.IsNotFound(err) {
				return "", time.Time{}, false, nil
			}
			return "", time.Time{}, false, err
		}
		return existing.PublicID, existing.ExpiresAt.Time, true, nil
	default:
		return "", time.Time{}, false, nil
	}
}

// markInviteExpiredTx lazily expires a pending row.
func markInviteExpiredTx(ctx context.Context, q *dbsqlc.Queries, scope tenancy.InviteScope, scopeInternalID int64, publicID string) error {
	switch scope {
	case tenancy.InviteScopeWorkspace:
		_, err := q.UpdateWorkspaceMemberInviteStatus(ctx, dbsqlc.UpdateWorkspaceMemberInviteStatusParams{
			PublicID:    publicID,
			WorkspaceID: scopeInternalID,
			Status:      tenancy.MemberInviteStatusExpired,
		})
		return err
	case tenancy.InviteScopeOrganization:
		_, err := q.UpdateOrganizationMemberInviteStatus(ctx, dbsqlc.UpdateOrganizationMemberInviteStatusParams{
			PublicID:       publicID,
			OrganizationID: scopeInternalID,
			Status:         tenancy.MemberInviteStatusExpired,
		})
		return err
	default:
		return tenancy.ErrNotFound
	}
}

// insertInviteTx persists the row and returns the unified invite.
// Duplicate races map to ErrConflict (not the scope ConflictErr, which is
// reserved for the pending-email conflict); preserved from both bindings.
func insertInviteTx(ctx context.Context, q *dbsqlc.Queries, scope tenancy.InviteScope, scopeInternalID int64, displayPublicID, displayName string, inviterUserID int64, email, role, tokenHash string, inviteeUserID int64, expiresAt time.Time, publicID string) (*tenancy.MemberInvite, error) {
	switch scope {
	case tenancy.InviteScopeWorkspace:
		row, err := q.CreateWorkspaceMemberInvite(ctx, dbsqlc.CreateWorkspaceMemberInviteParams{
			PublicID:      publicID,
			WorkspaceID:   scopeInternalID,
			InviterUserID: inviterUserID,
			InviteeEmail:  email,
			InviteeUserID: pgtype.Int8{Int64: inviteeUserID, Valid: inviteeUserID > 0},
			Role:          role,
			TokenHash:     tokenHash,
			ExpiresAt:     pgtype.Timestamptz{Time: expiresAt, Valid: true},
		})
		if err != nil {
			if database.IsDuplicateConstraintError(err) {
				return nil, tenancy.ErrConflict
			}
			return nil, err
		}
		return tenancy.MemberInviteFromWorkspace(mapWorkspaceMemberInvite(row, displayPublicID, displayName)), nil
	case tenancy.InviteScopeOrganization:
		row, err := q.CreateOrganizationMemberInvite(ctx, dbsqlc.CreateOrganizationMemberInviteParams{
			PublicID:       publicID,
			OrganizationID: scopeInternalID,
			InviterUserID:  inviterUserID,
			InviteeEmail:   email,
			InviteeUserID:  pgtype.Int8{Int64: inviteeUserID, Valid: inviteeUserID > 0},
			Role:           role,
			TokenHash:      tokenHash,
			ExpiresAt:      pgtype.Timestamptz{Time: expiresAt, Valid: true},
		})
		if err != nil {
			if database.IsDuplicateConstraintError(err) {
				return nil, tenancy.ErrConflict
			}
			return nil, err
		}
		return tenancy.MemberInviteFromOrganization(mapOrganizationMemberInvite(row, displayPublicID, displayName)), nil
	default:
		return nil, tenancy.ErrNotFound
	}
}

// getInviteForUpdateTx fetches the invite row locked under the scope predicate.
func getInviteForUpdateTx(ctx context.Context, q *dbsqlc.Queries, scope tenancy.InviteScope, publicID string, scopeInternalID int64) (*tenancy.MemberInvite, error) {
	switch scope {
	case tenancy.InviteScopeWorkspace:
		invite, err := q.GetWorkspaceMemberInviteByPublicIDForUpdate(ctx, dbsqlc.GetWorkspaceMemberInviteByPublicIDForUpdateParams{
			PublicID:    publicID,
			WorkspaceID: pgtype.Int8{Int64: scopeInternalID, Valid: true},
		})
		if err != nil {
			return nil, notFound(err)
		}
		ws, wsErr := q.GetWorkspaceByInternalID(ctx, invite.WorkspaceID)
		publicIDDisplay, name := "", ""
		if wsErr == nil {
			publicIDDisplay, name = ws.PublicID, ws.Name
		}
		return tenancy.MemberInviteFromWorkspace(mapWorkspaceMemberInvite(invite, publicIDDisplay, name)), nil
	case tenancy.InviteScopeOrganization:
		invite, err := q.GetOrganizationMemberInviteByPublicIDForUpdate(ctx, dbsqlc.GetOrganizationMemberInviteByPublicIDForUpdateParams{
			PublicID:       publicID,
			OrganizationID: pgtype.Int8{Int64: scopeInternalID, Valid: true},
		})
		if err != nil {
			return nil, notFound(err)
		}
		org, orgErr := q.GetOrganizationByInternalID(ctx, invite.OrganizationID)
		orgPublicID, orgName := "", ""
		if orgErr == nil {
			orgPublicID, orgName = org.PublicID, org.Name
		}
		return tenancy.MemberInviteFromOrganization(mapOrganizationMemberInvite(invite, orgPublicID, orgName)), nil
	default:
		return nil, tenancy.ErrNotFound
	}
}

// persistCancelInviteTx persists cancellation.
func persistCancelInviteTx(ctx context.Context, q *dbsqlc.Queries, scope tenancy.InviteScope, scopeInternalID int64, publicID string, callerUserID int64) error {
	switch scope {
	case tenancy.InviteScopeWorkspace:
		_, err := q.CancelWorkspaceMemberInvite(ctx, dbsqlc.CancelWorkspaceMemberInviteParams{
			CancelledByUserID: pgtype.Int8{Int64: callerUserID, Valid: callerUserID > 0},
			PublicID:          publicID,
			WorkspaceID:       scopeInternalID,
		})
		return err
	case tenancy.InviteScopeOrganization:
		_, err := q.CancelOrganizationMemberInvite(ctx, dbsqlc.CancelOrganizationMemberInviteParams{
			CallerUserID:   pgtype.Int8{Int64: callerUserID, Valid: callerUserID > 0},
			PublicID:       publicID,
			OrganizationID: scopeInternalID,
		})
		return err
	default:
		return tenancy.ErrNotFound
	}
}

// resolveInviteForDecisionTx resolves a public-id or raw-token ref to the
// unified invite without locking; callers lock scope before the invite.
func resolveInviteForDecisionTx(ctx context.Context, q *dbsqlc.Queries, scope tenancy.InviteScope, ref string) (*tenancy.MemberInvite, error) {
	switch scope {
	case tenancy.InviteScopeWorkspace:
		var invite dbsqlc.WorkspaceMemberInvite
		var err error
		if tenancy.IsMemberInviteRef(ref, tenancy.WorkspaceInvitePrefix) {
			invite, err = q.GetWorkspaceMemberInviteByPublicID(ctx, dbsqlc.GetWorkspaceMemberInviteByPublicIDParams{
				PublicID:    ref,
				WorkspaceID: pgtype.Int8{},
			})
		} else {
			invite, err = q.GetWorkspaceMemberInviteByTokenHash(ctx, dbsqlc.GetWorkspaceMemberInviteByTokenHashParams{
				TokenHash:   tenancy.MemberInviteTokenHash(ref),
				WorkspaceID: pgtype.Int8{},
			})
		}
		if err != nil {
			return nil, notFound(err)
		}
		ws, wsErr := q.GetWorkspaceByInternalID(ctx, invite.WorkspaceID)
		publicIDDisplay, name := "", ""
		if wsErr == nil {
			publicIDDisplay, name = ws.PublicID, ws.Name
		}
		return tenancy.MemberInviteFromWorkspace(mapWorkspaceMemberInvite(invite, publicIDDisplay, name)), nil
	case tenancy.InviteScopeOrganization:
		var invite dbsqlc.OrganizationMemberInvite
		var err error
		if tenancy.IsMemberInviteRef(ref, tenancy.OrganizationInvitePrefix) {
			invite, err = q.GetOrganizationMemberInviteByPublicID(ctx, dbsqlc.GetOrganizationMemberInviteByPublicIDParams{
				PublicID:       ref,
				OrganizationID: pgtype.Int8{},
			})
		} else {
			invite, err = q.GetOrganizationMemberInviteByTokenHash(ctx, dbsqlc.GetOrganizationMemberInviteByTokenHashParams{
				TokenHash:      tenancy.MemberInviteTokenHash(ref),
				OrganizationID: pgtype.Int8{},
			})
		}
		if err != nil {
			return nil, notFound(err)
		}
		org, orgErr := q.GetOrganizationByInternalID(ctx, invite.OrganizationID)
		orgPublicID, orgName := "", ""
		if orgErr == nil {
			orgPublicID, orgName = org.PublicID, org.Name
		}
		return tenancy.MemberInviteFromOrganization(mapOrganizationMemberInvite(invite, orgPublicID, orgName)), nil
	default:
		return nil, tenancy.ErrNotFound
	}
}

// markInviteDecidedTx persists the accepted/declined status.
func markInviteDecidedTx(ctx context.Context, q *dbsqlc.Queries, scope tenancy.InviteScope, inv *tenancy.MemberInvite, status string, recipientUserID int64) error {
	switch scope {
	case tenancy.InviteScopeWorkspace:
		_, err := q.UpdateWorkspaceMemberInviteStatus(ctx, dbsqlc.UpdateWorkspaceMemberInviteStatusParams{
			PublicID:          inv.PublicID,
			WorkspaceID:       inv.ScopeInternalID,
			Status:            status,
			RespondedByUserID: pgtype.Int8{Int64: recipientUserID, Valid: true},
		})
		return err
	case tenancy.InviteScopeOrganization:
		_, err := q.UpdateOrganizationMemberInviteStatus(ctx, dbsqlc.UpdateOrganizationMemberInviteStatusParams{
			PublicID:          inv.PublicID,
			OrganizationID:    inv.ScopeInternalID,
			Status:            status,
			RespondedByUserID: pgtype.Int8{Int64: recipientUserID, Valid: true},
		})
		return err
	default:
		return tenancy.ErrNotFound
	}
}

// checkMemberInviterRoleTx reads the actor role for the shared inviter gate.
// Workspace has no gate (handler owns authorization); organization requires
// owner/admin with owner-only admin grants.
func checkMemberInviterRoleTx(ctx context.Context, q *dbsqlc.Queries, txScope postgresInviteTx, scopeInternalID, inviterUserID int64, role string) error {
	if txScope.policy.Scope != tenancy.InviteScopeOrganization {
		return nil
	}
	if txScope.orgStore == nil {
		return tenancy.ErrForbidden
	}
	actorRole, _, err := txScope.orgStore.orgMutationContext(ctx, q, scopeInternalID, inviterUserID)
	if err != nil {
		return err
	}
	return txScope.policy.CheckInviterRole(actorRole, role)
}

// checkMemberCreateCapacityTx reads member/invite counts for the shared capacity rule.
func checkMemberCreateCapacityTx(ctx context.Context, q *dbsqlc.Queries, txScope postgresInviteTx, scopeInternalID int64) error {
	if txScope.policy.Scope != tenancy.InviteScopeWorkspace {
		return nil
	}
	activeCount, err := q.CountWorkspaceMembers(ctx, scopeInternalID)
	if err != nil {
		return err
	}
	pendingCount, err := q.CountPendingWorkspaceMemberInvites(ctx, scopeInternalID)
	if err != nil {
		return err
	}
	return txScope.policy.CheckCreateCapacity(activeCount, pendingCount)
}

// createMemberInviteTx runs the shared creation sequencing: normalize,
// scope lock, inviter gate, capacity, pending-email conflict (lazily
// expiring the stale row so the partial-unique index permits insertion),
// then insert. inviteeUserID > 0 persists invitee_user_id, 0 leaves NULL.
func createMemberInviteTx(ctx context.Context, q *dbsqlc.Queries, txScope postgresInviteTx, scopePublicID string, inviterUserID int64, email, role string, inviteeUserID int64) (*tenancy.MemberInvite, string, error) {
	policy := txScope.policy
	email, role, err := tenancy.ValidateMemberInviteCreateInput(email, role)
	if err != nil {
		return nil, "", err
	}
	scopeInternalID, displayPublicID, displayName, err := txScope.lockScopeForCreate(ctx, q, strings.TrimSpace(scopePublicID))
	if err != nil {
		return nil, "", err
	}
	if err := checkMemberInviterRoleTx(ctx, q, txScope, scopeInternalID, inviterUserID, role); err != nil {
		return nil, "", err
	}
	if err := checkMemberCreateCapacityTx(ctx, q, txScope, scopeInternalID); err != nil {
		return nil, "", err
	}
	if pubID, expiresAt, found, err := findPendingInviteByEmailTx(ctx, q, policy.Scope, scopeInternalID, email); err != nil {
		return nil, "", err
	} else if found {
		if !tenancy.MemberInviteExpired(expiresAt, time.Now()) {
			return nil, "", policy.ConflictErr
		}
		// Lazily expire the stale pending row so the partial-unique
		// pending index permits insertion.
		_ = markInviteExpiredTx(ctx, q, policy.Scope, scopeInternalID, pubID)
	}
	rawToken, tokenHash, err := tenancy.MintMemberInviteToken()
	if err != nil {
		return nil, "", err
	}
	publicID, err := policy.NewPublicID()
	if err != nil {
		return nil, "", err
	}
	created, err := insertInviteTx(ctx, q, policy.Scope, scopeInternalID, displayPublicID, displayName, inviterUserID, email, role, tokenHash, inviteeUserID, time.Now().Add(tenancy.MemberInviteTTL), publicID)
	if err != nil {
		return nil, "", err
	}
	return created, rawToken, nil
}

// cancelMemberInviteTx runs the shared cancellation sequencing.
func cancelMemberInviteTx(ctx context.Context, q *dbsqlc.Queries, txScope postgresInviteTx, scopePublicID, invitePublicID string, callerUserID int64) error {
	policy := txScope.policy
	scopeInternalID, err := txScope.lockScopeForCancel(ctx, q, strings.TrimSpace(scopePublicID))
	if err != nil {
		return err
	}
	invitePublicID = strings.TrimSpace(invitePublicID)
	inv, err := getInviteForUpdateTx(ctx, q, policy.Scope, invitePublicID, scopeInternalID)
	if err != nil {
		return err
	}
	if inv.ScopeInternalID != scopeInternalID {
		return tenancy.ErrNotFound
	}
	if inv.Status != tenancy.MemberInviteStatusPending {
		return tenancy.ErrConflict
	}
	if tenancy.MemberInviteExpired(inv.ExpiresAt, time.Now()) {
		_ = markInviteExpiredTx(ctx, q, policy.Scope, scopeInternalID, inv.PublicID)
		return policy.ExpiredErr
	}
	return persistCancelInviteTx(ctx, q, policy.Scope, scopeInternalID, invitePublicID, callerUserID)
}

// acceptMemberInviteTx runs the shared accept sequencing and returns the
// decided authoritative invite plus the scope member projection as any
// (the binding casts to its member type).
func acceptMemberInviteTx(ctx context.Context, q *dbsqlc.Queries, txScope postgresInviteTx, ref string, recipientUserID int64, recipientEmail string) (*tenancy.MemberInvite, any, error) {
	policy := txScope.policy
	ref = strings.TrimSpace(ref)
	if err := tenancy.ValidateMemberInviteDecisionInput(ref, recipientUserID); err != nil {
		return nil, nil, err
	}
	inv, err := resolveInviteForDecisionTx(ctx, q, policy.Scope, ref)
	if err != nil {
		return nil, nil, err
	}
	if err := tenancy.CheckMemberInviteRecipient(inv.InviteeEmail, inv.InviteeUserID, recipientUserID, recipientEmail); err != nil {
		return nil, nil, err
	}
	if err := txScope.checkAcceptScope(ctx, q, inv); err != nil {
		return nil, nil, err
	}
	inv, err = getInviteForUpdateTx(ctx, q, policy.Scope, inv.PublicID, inv.ScopeInternalID)
	if err != nil {
		return nil, nil, err
	}
	if inv.Status != tenancy.MemberInviteStatusPending {
		return nil, nil, tenancy.ErrConflict
	}
	if tenancy.MemberInviteExpired(inv.ExpiresAt, time.Now()) {
		_ = markInviteExpiredTx(ctx, q, policy.Scope, inv.ScopeInternalID, inv.PublicID)
		return nil, nil, policy.ExpiredErr
	}
	// Anti-enumeration: email/user mismatch converges to 404 (not 403).
	if err := tenancy.CheckMemberInviteRecipient(inv.InviteeEmail, inv.InviteeUserID, recipientUserID, recipientEmail); err != nil {
		return nil, nil, err
	}
	member, err := txScope.grantMember(ctx, q, inv, recipientUserID)
	if err != nil {
		return nil, nil, err
	}
	if err := markInviteDecidedTx(ctx, q, policy.Scope, inv, tenancy.MemberInviteStatusAccepted, recipientUserID); err != nil {
		return nil, nil, err
	}
	// Authoritative post-decision row: re-lock and re-read in the same tx
	// so callers never stitch a stale pre-decision snapshot.
	decided, err := getInviteForUpdateTx(ctx, q, policy.Scope, inv.PublicID, inv.ScopeInternalID)
	if err != nil {
		return nil, nil, err
	}
	return decided, member, nil
}

// declineMemberInviteTx runs the shared decline sequencing and returns the
// decided authoritative invite.
func declineMemberInviteTx(ctx context.Context, q *dbsqlc.Queries, txScope postgresInviteTx, ref string, recipientUserID int64, recipientEmail string) (*tenancy.MemberInvite, error) {
	policy := txScope.policy
	ref = strings.TrimSpace(ref)
	if err := tenancy.ValidateMemberInviteDecisionInput(ref, recipientUserID); err != nil {
		return nil, err
	}
	inv, err := resolveInviteForDecisionTx(ctx, q, policy.Scope, ref)
	if err != nil {
		return nil, err
	}
	if err := tenancy.CheckMemberInviteRecipient(inv.InviteeEmail, inv.InviteeUserID, recipientUserID, recipientEmail); err != nil {
		return nil, err
	}
	if _, err := txScope.lockScopeForCancel(ctx, q, inv.ScopePublicID); err != nil {
		return nil, err
	}
	inv, err = getInviteForUpdateTx(ctx, q, policy.Scope, inv.PublicID, inv.ScopeInternalID)
	if err != nil {
		return nil, err
	}
	if inv.Status != tenancy.MemberInviteStatusPending {
		return nil, tenancy.ErrConflict
	}
	if tenancy.MemberInviteExpired(inv.ExpiresAt, time.Now()) {
		_ = markInviteExpiredTx(ctx, q, policy.Scope, inv.ScopeInternalID, inv.PublicID)
		return nil, policy.ExpiredErr
	}
	// Anti-enumeration: email/user mismatch converges to 404 (not 403).
	if err := tenancy.CheckMemberInviteRecipient(inv.InviteeEmail, inv.InviteeUserID, recipientUserID, recipientEmail); err != nil {
		return nil, err
	}
	if err := markInviteDecidedTx(ctx, q, policy.Scope, inv, tenancy.MemberInviteStatusDeclined, recipientUserID); err != nil {
		return nil, err
	}
	decided, err := getInviteForUpdateTx(ctx, q, policy.Scope, inv.PublicID, inv.ScopeInternalID)
	if err != nil {
		return nil, err
	}
	return decided, nil
}

// filterPendingMemberInvites drops decided rows and marks expired rows on
// the in-memory copy (write paths lazily persist expiry). Both scopes use
// the pending-only actionable set for owner lists.
func filterPendingMemberInvites(invites []*tenancy.MemberInvite, now time.Time) []*tenancy.MemberInvite {
	out := make([]*tenancy.MemberInvite, 0, len(invites))
	for _, inv := range invites {
		if inv == nil || inv.Status != tenancy.MemberInviteStatusPending {
			continue
		}
		if tenancy.MemberInviteExpired(inv.ExpiresAt, now) {
			inv.Status = tenancy.MemberInviteStatusExpired
			continue
		}
		out = append(out, inv)
	}
	return out
}
