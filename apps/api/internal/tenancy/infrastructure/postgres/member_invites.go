// Package postgres implements tenancy invite persistence against PostgreSQL/sqlc.
//
// member_invites.go is the unified invitation lifecycle core: create, cancel,
// accept, and decline share one sequencing parameterized by per-scope
// bindings in workspace_invites.go / organization_invites.go. Both scopes
// persist to their own table (workspace_member_invites /
// organization_member_invites), each with the same row-level FOR UPDATE lock
// and partial-unique pending-email pattern. Scope differences ride the
// postgresInviteTx hooks:
//
//   - capacity: workspace active + pending <= MaxWorkspaceMembers;
//     organization enforces the live single-org limit inside grantMember.
//   - closure: workspace via rejectClosedOrgWorkspaceInTx; organization via
//     lockActiveOrganization.
//   - member write target: workspace_memberships vs organization_memberships.
//   - Default protection applies to workspace scope only.
//
// Read paths (list/get/token/pending-for-user) stay per-scope but share the
// pending-only filter and recipient helpers from the tenancy core.
package postgres

import (
	"context"
	"strings"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tenancy"
)

// postgresInviteTx parameterizes the unified write lifecycle per scope.
// All hooks run inside the caller-owned transaction via q.
type postgresInviteTx struct {
	policy tenancy.MemberInvitePolicy
	// lockScopeForCreate locks the scope and fails closed when the scope
	// cannot accept invites (closed org, Default workspace). Returns the
	// scope internal id plus display identity.
	lockScopeForCreate func(ctx context.Context, q *dbsqlc.Queries, scopePublicID string) (scopeInternalID int64, displayPublicID, displayName string, err error)
	// checkInviterRole enforces create-time inviter gates (nil = the
	// handler owns authorization and the store performs no extra check).
	checkInviterRole func(ctx context.Context, q *dbsqlc.Queries, scopeInternalID, inviterUserID int64, role string) error
	// checkCapacity enforces create-time capacity (nil = no cap).
	checkCapacity func(ctx context.Context, q *dbsqlc.Queries, scopeInternalID int64) error
	// findPendingByEmail reports a live pending row for the email.
	findPendingByEmail func(ctx context.Context, q *dbsqlc.Queries, scopeInternalID int64, email string) (publicID string, expiresAt time.Time, found bool, err error)
	// markExpired lazily expires a pending row.
	markExpired func(ctx context.Context, q *dbsqlc.Queries, scopeInternalID int64, publicID string) error
	// insertInvite persists the row and returns the unified invite.
	insertInvite func(ctx context.Context, q *dbsqlc.Queries, scopeInternalID int64, displayPublicID, displayName string, inviterUserID int64, email, role, tokenHash string, inviteeUserID int64, expiresAt time.Time, publicID string) (*tenancy.MemberInvite, error)
	// lockScopeForCancel locks the scope for cancellation. Workspace
	// cancel is read-tolerant on closure (membership lock only);
	// organization cancel fails closed (active lock).
	lockScopeForCancel func(ctx context.Context, q *dbsqlc.Queries, scopePublicID string) (scopeInternalID int64, err error)
	// getForUpdate fetches the invite row locked under the scope predicate.
	getForUpdate func(ctx context.Context, q *dbsqlc.Queries, publicID string, scopeInternalID int64) (*tenancy.MemberInvite, error)
	// cancelInvite persists cancellation.
	cancelInvite func(ctx context.Context, q *dbsqlc.Queries, scopeInternalID int64, publicID string, callerUserID int64) error
	// resolveForDecision resolves a public-id or raw-token ref to the
	// unified invite without locking; callers lock scope before the invite.
	resolveForDecision func(ctx context.Context, q *dbsqlc.Queries, ref string) (*tenancy.MemberInvite, error)
	// checkAcceptScope locks the scope and fails closed when the scope
	// cannot accept members (closed org, Default workspace).
	checkAcceptScope func(ctx context.Context, q *dbsqlc.Queries, inv *tenancy.MemberInvite) error
	// grantMember writes the membership row and returns the scope member
	// projection as any (the binding casts back to its member type).
	grantMember func(ctx context.Context, q *dbsqlc.Queries, inv *tenancy.MemberInvite, recipientUserID int64) (any, error)
	// markDecided persists the accepted/declined status.
	markDecided func(ctx context.Context, q *dbsqlc.Queries, inv *tenancy.MemberInvite, status string, recipientUserID int64) error
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
	if txScope.checkInviterRole != nil {
		if err := txScope.checkInviterRole(ctx, q, scopeInternalID, inviterUserID, role); err != nil {
			return nil, "", err
		}
	}
	if txScope.checkCapacity != nil {
		if err := txScope.checkCapacity(ctx, q, scopeInternalID); err != nil {
			return nil, "", err
		}
	}
	if pubID, expiresAt, found, err := txScope.findPendingByEmail(ctx, q, scopeInternalID, email); err != nil {
		return nil, "", err
	} else if found {
		if !tenancy.MemberInviteExpired(expiresAt, time.Now()) {
			return nil, "", policy.ConflictErr
		}
		// Lazily expire the stale pending row so the partial-unique
		// pending index permits insertion.
		_ = txScope.markExpired(ctx, q, scopeInternalID, pubID)
	}
	rawToken, tokenHash, err := tenancy.MintMemberInviteToken()
	if err != nil {
		return nil, "", err
	}
	publicID, err := policy.NewPublicID()
	if err != nil {
		return nil, "", err
	}
	created, err := txScope.insertInvite(ctx, q, scopeInternalID, displayPublicID, displayName, inviterUserID, email, role, tokenHash, inviteeUserID, time.Now().Add(tenancy.MemberInviteTTL), publicID)
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
	inv, err := txScope.getForUpdate(ctx, q, invitePublicID, scopeInternalID)
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
		_ = txScope.markExpired(ctx, q, scopeInternalID, inv.PublicID)
		return policy.ExpiredErr
	}
	return txScope.cancelInvite(ctx, q, scopeInternalID, invitePublicID, callerUserID)
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
	inv, err := txScope.resolveForDecision(ctx, q, ref)
	if err != nil {
		return nil, nil, err
	}
	if err := tenancy.CheckMemberInviteRecipient(inv.InviteeEmail, inv.InviteeUserID, recipientUserID, recipientEmail); err != nil {
		return nil, nil, err
	}
	if err := txScope.checkAcceptScope(ctx, q, inv); err != nil {
		return nil, nil, err
	}
	inv, err = txScope.getForUpdate(ctx, q, inv.PublicID, inv.ScopeInternalID)
	if err != nil {
		return nil, nil, err
	}
	if inv.Status != tenancy.MemberInviteStatusPending {
		return nil, nil, tenancy.ErrConflict
	}
	if tenancy.MemberInviteExpired(inv.ExpiresAt, time.Now()) {
		_ = txScope.markExpired(ctx, q, inv.ScopeInternalID, inv.PublicID)
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
	if err := txScope.markDecided(ctx, q, inv, tenancy.MemberInviteStatusAccepted, recipientUserID); err != nil {
		return nil, nil, err
	}
	// Authoritative post-decision row: re-lock and re-read in the same tx
	// so callers never stitch a stale pre-decision snapshot.
	decided, err := txScope.getForUpdate(ctx, q, inv.PublicID, inv.ScopeInternalID)
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
	inv, err := txScope.resolveForDecision(ctx, q, ref)
	if err != nil {
		return nil, err
	}
	if err := tenancy.CheckMemberInviteRecipient(inv.InviteeEmail, inv.InviteeUserID, recipientUserID, recipientEmail); err != nil {
		return nil, err
	}
	if _, err := txScope.lockScopeForCancel(ctx, q, inv.ScopePublicID); err != nil {
		return nil, err
	}
	inv, err = txScope.getForUpdate(ctx, q, inv.PublicID, inv.ScopeInternalID)
	if err != nil {
		return nil, err
	}
	if inv.Status != tenancy.MemberInviteStatusPending {
		return nil, tenancy.ErrConflict
	}
	if tenancy.MemberInviteExpired(inv.ExpiresAt, time.Now()) {
		_ = txScope.markExpired(ctx, q, inv.ScopeInternalID, inv.PublicID)
		return nil, policy.ExpiredErr
	}
	// Anti-enumeration: email/user mismatch converges to 404 (not 403).
	if err := tenancy.CheckMemberInviteRecipient(inv.InviteeEmail, inv.InviteeUserID, recipientUserID, recipientEmail); err != nil {
		return nil, err
	}
	if err := txScope.markDecided(ctx, q, inv, tenancy.MemberInviteStatusDeclined, recipientUserID); err != nil {
		return nil, err
	}
	decided, err := txScope.getForUpdate(ctx, q, inv.PublicID, inv.ScopeInternalID)
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
