// Package postgres implements invite-style studio handover (#948).
//
// Only a Billing Owner may initiate; the recipient accepts or rejects, and
// the initiator may cancel while pending. Acceptance hands management to
// the recipient (new Owner, previous Owner steps down to Admin) and grants
// the recipient Workspace Owner on every non-Default business Workspace
// associated with the account; the Default Workspace never transfers.
// Identity, balance, Ledger, and history stay stable; no funds move to the
// recipient's other accounts, quotas never reset, and no welcome grant is
// minted. Row-pinned settlement keeps existing Task/Search/payment/refund
// attribution on the original account.
//
// All repository methods take context.Context first and use dbWithContext.
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

func mapTransferInvite(row dbsqlc.BillingAccountTransferInvite) *tenancy.BillingTransferInvite {
	scope := append([]string(nil), row.ScopeWorkspacePublicIds...)
	if scope == nil {
		scope = []string{}
	}
	out := &tenancy.BillingTransferInvite{
		InternalID:              row.InternalID,
		PublicID:                row.PublicID,
		BillingAccountID:        row.BillingAccountID,
		BillingAccountPublicID:  row.BillingAccountPublicID,
		FromUserID:              row.FromUserID,
		ToUserID:                row.ToUserID,
		Status:                  row.Status,
		ScopeWorkspacePublicIDs: scope,
		CreatedAt:               dbutil.TimeValue(row.CreatedAt),
		ExpiresAt:               dbutil.TimeValue(row.ExpiresAt),
	}
	if row.DecidedAt.Valid {
		t := row.DecidedAt.Time
		out.DecidedAt = &t
	}
	if row.DecidedBy.Valid {
		v := row.DecidedBy.Int64
		out.DecidedBy = &v
	}
	return out
}

// InitiateBillingTransferInvite creates a pending handover invitation.
// Only a Billing Owner may initiate; the recipient need not already be a
// billing manager (acceptance grants management). At most one pending
// invite per account: concurrent initiates collapse to the winner, and the
// same (account, idempotency_key) replays the first invite instead of
// forking a second. The Default Workspace never enters the scope.
func (s *BillingAccountStore) InitiateBillingTransferInvite(ctx context.Context, billingAccountInternalID, fromUserID, toUserID int64, idempotencyKey string) (*tenancy.BillingTransferInvite, error) {
	if s == nil || s.db == nil {
		return nil, tenancy.ErrBillingStoreNotConfigured
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if billingAccountInternalID <= 0 || fromUserID <= 0 || toUserID <= 0 || fromUserID == toUserID || idempotencyKey == "" {
		return nil, tenancy.ErrBillingWorkspaceInvalid
	}
	var invited *tenancy.BillingTransferInvite
	err := dbutil.WithTx(ctx, s.db, func(q *dbsqlc.Queries) error {
		account, err := lockBillingAccountInTx(ctx, q, billingAccountInternalID)
		if err != nil {
			return err
		}
		from, err := q.GetBillingAccountMember(ctx, dbsqlc.GetBillingAccountMemberParams{
			BillingAccountID: billingAccountInternalID, UserID: fromUserID,
		})
		if err != nil {
			if database.IsNotFound(err) {
				return tenancy.ErrBillingForbidden
			}
			return err
		}
		if !tenancy.IsBillingOwner(from.Role) {
			return tenancy.ErrBillingForbidden
		}
		// Same-key replay: return the first invite instead of forking.
		// Unknown recipients fail closed via the to_user_id FK: map the
		// foreign-key violation to not-found instead of leaking a raw
		// constraint error (no extra pool connection inside the tx, so
		// concurrent initiates never deadlock on pool exhaustion). A replay
		// may return a lapsed invite when the caller reuses a key past its
		// expiry; re-issue (fresh key) refreshes the row below instead.
		if existing, err := q.GetBillingTransferInviteByAccountIdem(ctx, dbsqlc.GetBillingTransferInviteByAccountIdemParams{
			BillingAccountID: billingAccountInternalID, IdempotencyKey: idempotencyKey,
		}); err == nil {
			// Same key with a different recipient/initiator conflicts.
			if existing.FromUserID != fromUserID || existing.ToUserID != toUserID {
				return tenancy.ErrBillingConflict
			}
			invited = mapTransferInvite(existing)
			return nil
		}
		// At most one pending per account: a lapsed pending invite is
		// re-issued in place (same bti_ id, fresh parties/key/expiry) so a
		// new send after lapse never forks and never hits the pending
		// unique index. Zero rows means no lapsed invite: fall through to
		// the live-pending guard below.
		if reissued, err := q.ReissueBillingTransferInvite(ctx, dbsqlc.ReissueBillingTransferInviteParams{
			BillingAccountID: billingAccountInternalID, FromUserID: fromUserID, ToUserID: toUserID, IdempotencyKey: idempotencyKey,
		}); err == nil {
			invited = mapTransferInvite(reissued)
			return nil
		} else if !database.IsNotFound(err) {
			return err
		}
		// At most one pending per account: concurrent initiates collapse.
		if pending, err := q.GetPendingBillingTransferForAccount(ctx, billingAccountInternalID); err == nil {
			// Same recipient re-initiate replays the pending invite.
			if pending.FromUserID == fromUserID && pending.ToUserID == toUserID {
				invited = mapTransferInvite(pending)
				return nil
			}
			return tenancy.ErrBillingConflict
		} else if !database.IsNotFound(err) {
			return err
		}
		publicID, err := tenancy.NewBillingTransferInvitePublicID()
		if err != nil {
			return err
		}
		created, err := q.CreateBillingTransferInvite(ctx, dbsqlc.CreateBillingTransferInviteParams{
			PublicID:               publicID,
			BillingAccountID:       billingAccountInternalID,
			BillingAccountPublicID: account.PublicID,
			FromUserID:             fromUserID,
			ToUserID:               toUserID,
			IdempotencyKey:         idempotencyKey,
		})
		if err != nil {
			if isTransferRecipientNotFound(err) {
				return tenancy.ErrBillingNotFound
			}
			if database.IsDuplicateConstraintError(err) {
				// Lost race: read back the winner (same-key or pending).
				if existing, readErr := q.GetBillingTransferInviteByAccountIdem(ctx, dbsqlc.GetBillingTransferInviteByAccountIdemParams{
					BillingAccountID: billingAccountInternalID, IdempotencyKey: idempotencyKey,
				}); readErr == nil {
					if existing.FromUserID != fromUserID || existing.ToUserID != toUserID {
						return tenancy.ErrBillingConflict
					}
					invited = mapTransferInvite(existing)
					return nil
				}
				if pending, readErr := q.GetPendingBillingTransferForAccount(ctx, billingAccountInternalID); readErr == nil {
					if pending.FromUserID == fromUserID && pending.ToUserID == toUserID {
						invited = mapTransferInvite(pending)
						return nil
					}
					return tenancy.ErrBillingConflict
				}
			}
			return err
		}
		invited = mapTransferInvite(created)
		return nil
	})
	return invited, err
}

// GetBillingTransferInvite fetches one invite by its bti_ public id.
// Hidden ids converge to ErrBillingNotFound (never workspace existence).
func (s *BillingAccountStore) GetBillingTransferInvite(ctx context.Context, publicID string) (*tenancy.BillingTransferInvite, error) {
	if s == nil || s.q == nil {
		return nil, tenancy.ErrBillingStoreNotConfigured
	}
	trimmed := strings.TrimSpace(publicID)
	if trimmed == "" || !strings.HasPrefix(trimmed, "bti_") {
		return nil, tenancy.ErrBillingNotFound
	}
	row, err := s.dbWithContext(ctx).q.GetBillingTransferInviteByPublicID(ctx, trimmed)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, tenancy.ErrBillingNotFound
		}
		return nil, err
	}
	return mapTransferInvite(row), nil
}

// ListBillingTransferInvitesForAccount pages handover history newest-first
// for financial managers, or only the actor's addressed invites for a
// recipient without management. The HTTP prelude requires Workspace membership.
// beforeID is the numeric cursor (0 = first page); limit clamps to 1..100.
func (s *BillingAccountStore) ListBillingTransferInvitesForAccount(ctx context.Context, billingAccountInternalID, actorUserID, beforeID int64, limit int) ([]*tenancy.BillingTransferInvite, error) {
	if s == nil || s.q == nil {
		return nil, tenancy.ErrBillingStoreNotConfigured
	}
	if billingAccountInternalID <= 0 {
		return nil, tenancy.ErrBillingNotFound
	}
	role, ok, err := s.GetBillingAccountRole(ctx, billingAccountInternalID, actorUserID)
	if err != nil {
		return nil, err
	}
	// The HTTP Workspace prelude establishes content membership. A recipient
	// can discover their own invitations before gaining financial management.
	var recipientUserID int64
	if !ok || !tenancy.IsBillingManager(role) {
		recipientUserID = actorUserID
		if recipientUserID <= 0 {
			return nil, tenancy.ErrBillingForbidden
		}
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if beforeID < 0 {
		return nil, tenancy.ErrBillingWorkspaceInvalid
	}
	rows, err := s.dbWithContext(ctx).q.ListBillingTransferInvitesForAccount(ctx, dbsqlc.ListBillingTransferInvitesForAccountParams{
		BillingAccountID: billingAccountInternalID, RecipientUserID: recipientUserID, BeforeID: beforeID, RowLimit: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]*tenancy.BillingTransferInvite, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapTransferInvite(r))
	}
	return out, nil
}

// lockPendingBillingTransferInTx pins account scope and invitation state before
// a decision. Every decision takes locks in account -> invitation order.
func lockPendingBillingTransferInTx(ctx context.Context, q *dbsqlc.Queries, accountID int64, invitePublicID string) (dbsqlc.BillingAccountTransferInvite, error) {
	if _, err := lockBillingAccountInTx(ctx, q, accountID); err != nil {
		return dbsqlc.BillingAccountTransferInvite{}, err
	}
	invite, err := q.GetBillingTransferInviteForUpdate(ctx, invitePublicID)
	if database.IsNotFound(err) {
		return invite, tenancy.ErrBillingNotFound
	}
	if err != nil {
		return invite, err
	}
	if invite.BillingAccountID != accountID {
		return invite, tenancy.ErrBillingNotFound
	}
	if invite.Status != tenancy.BillingTransferPending {
		return invite, tenancy.ErrBillingConflict
	}
	// A lapsed invite never moves funds or membership: every decision
	// fails closed with conflict, and the row stays pending-lapsed so the
	// owner may re-issue it.
	if expiresAt := dbutil.TimeValue(invite.ExpiresAt); !expiresAt.IsZero() && !time.Now().Before(expiresAt) {
		return invite, tenancy.ErrBillingConflict
	}
	return invite, nil
}

// AcceptBillingTransferInvite completes the handover: only the recipient
// may accept a pending invite. The recipient becomes Billing Owner, the
// previous Owner steps down to Admin, and the recipient gains Workspace
// Owner on every non-Default business Workspace associated with the
// account. The Default Workspace never transfers. Balances, Ledger, and
// history stay stable; quotas never reset and no welcome grant is minted.
func (s *BillingAccountStore) AcceptBillingTransferInvite(ctx context.Context, billingAccountInternalID int64, invitePublicID string, recipientUserID int64) (*tenancy.BillingTransferInvite, error) {
	if s == nil || s.db == nil {
		return nil, tenancy.ErrBillingStoreNotConfigured
	}
	trimmed := strings.TrimSpace(invitePublicID)
	if billingAccountInternalID <= 0 || trimmed == "" || recipientUserID <= 0 {
		return nil, tenancy.ErrBillingWorkspaceInvalid
	}
	var decided *tenancy.BillingTransferInvite
	err := dbutil.WithTx(ctx, s.db, func(q *dbsqlc.Queries) error {
		invite, err := lockPendingBillingTransferInTx(ctx, q, billingAccountInternalID, trimmed)
		if err != nil {
			return err
		}
		if invite.ToUserID != recipientUserID {
			return tenancy.ErrBillingForbidden
		}
		// The initiator must still hold Billing Owner: concurrent
		// management changes fail the handover closed instead of
		// transferring from a demoted account.
		from, err := q.GetBillingAccountMember(ctx, dbsqlc.GetBillingAccountMemberParams{
			BillingAccountID: invite.BillingAccountID, UserID: invite.FromUserID,
		})
		if err != nil {
			if database.IsNotFound(err) {
				return tenancy.ErrBillingConflict
			}
			return err
		}
		if !tenancy.IsBillingOwner(from.Role) {
			return tenancy.ErrBillingConflict
		}
		if _, err := q.GetBillingAccountMember(ctx, dbsqlc.GetBillingAccountMemberParams{
			BillingAccountID: invite.BillingAccountID, UserID: invite.ToUserID,
		}); database.IsNotFound(err) {
			members, err := q.ListBillingAccountMembers(ctx, dbsqlc.ListBillingAccountMembersParams{
				BillingAccountID: invite.BillingAccountID, RowLimit: tenancy.MaxBillingAccountMembers,
			})
			if err != nil {
				return err
			}
			if len(members) >= tenancy.MaxBillingAccountMembers {
				return tenancy.ErrBillingConflict
			}
		} else if err != nil {
			return err
		}
		// Recipient gains management first so the account never loses its
		// last Owner mid-handover.
		if _, err := q.UpsertBillingAccountMember(ctx, dbsqlc.UpsertBillingAccountMemberParams{
			BillingAccountID: invite.BillingAccountID, UserID: invite.ToUserID, Role: tenancy.BillingRoleOwner,
		}); err != nil {
			return err
		}
		if _, err := q.UpsertBillingAccountMember(ctx, dbsqlc.UpsertBillingAccountMemberParams{
			BillingAccountID: invite.BillingAccountID, UserID: invite.FromUserID, Role: tenancy.BillingRoleAdmin,
		}); err != nil {
			return err
		}
		// Workspace permission linkage: grant the recipient Workspace
		// Owner on every non-Default business Workspace associated with
		// the account. Existing higher-or-equal grants are left intact
		// (owners stay owners); Default workspaces are never touched.
		scope, err := s.grantHandoverWorkspacesInTx(ctx, q, invite.BillingAccountID, invite.ToUserID)
		if err != nil {
			return err
		}
		row, err := q.DecideBillingTransferInvite(ctx, dbsqlc.DecideBillingTransferInviteParams{
			PublicID:                trimmed,
			Status:                  tenancy.BillingTransferAccepted,
			DecidedBy:               pgtype.Int8{Int64: recipientUserID, Valid: true},
			ScopeWorkspacePublicIds: scope,
		})
		if err != nil {
			if database.IsNotFound(err) {
				return tenancy.ErrBillingConflict
			}
			return err
		}
		decided = mapTransferInvite(row)
		return nil
	})
	return decided, err
}

// RejectBillingTransferInvite declines a pending invite: only the
// recipient may reject. The account membership is unchanged.
func (s *BillingAccountStore) RejectBillingTransferInvite(ctx context.Context, billingAccountInternalID int64, invitePublicID string, recipientUserID int64) (*tenancy.BillingTransferInvite, error) {
	if s == nil || s.db == nil {
		return nil, tenancy.ErrBillingStoreNotConfigured
	}
	trimmed := strings.TrimSpace(invitePublicID)
	if billingAccountInternalID <= 0 || trimmed == "" || recipientUserID <= 0 {
		return nil, tenancy.ErrBillingWorkspaceInvalid
	}
	var decided *tenancy.BillingTransferInvite
	err := dbutil.WithTx(ctx, s.db, func(q *dbsqlc.Queries) error {
		invite, err := lockPendingBillingTransferInTx(ctx, q, billingAccountInternalID, trimmed)
		if err != nil {
			return err
		}
		if invite.ToUserID != recipientUserID {
			return tenancy.ErrBillingForbidden
		}
		row, err := q.DecideBillingTransferInvite(ctx, dbsqlc.DecideBillingTransferInviteParams{
			PublicID:                trimmed,
			Status:                  tenancy.BillingTransferRejected,
			DecidedBy:               pgtype.Int8{Int64: recipientUserID, Valid: true},
			ScopeWorkspacePublicIds: []string{},
		})
		if err != nil {
			if database.IsNotFound(err) {
				return tenancy.ErrBillingConflict
			}
			return err
		}
		decided = mapTransferInvite(row)
		return nil
	})
	return decided, err
}

// CancelBillingTransferInvite withdraws a pending invite: only the
// initiator may cancel. The account membership is unchanged.
func (s *BillingAccountStore) CancelBillingTransferInvite(ctx context.Context, billingAccountInternalID int64, invitePublicID string, initiatorUserID int64) (*tenancy.BillingTransferInvite, error) {
	if s == nil || s.db == nil {
		return nil, tenancy.ErrBillingStoreNotConfigured
	}
	trimmed := strings.TrimSpace(invitePublicID)
	if billingAccountInternalID <= 0 || trimmed == "" || initiatorUserID <= 0 {
		return nil, tenancy.ErrBillingWorkspaceInvalid
	}
	var decided *tenancy.BillingTransferInvite
	err := dbutil.WithTx(ctx, s.db, func(q *dbsqlc.Queries) error {
		invite, err := lockPendingBillingTransferInTx(ctx, q, billingAccountInternalID, trimmed)
		if err != nil {
			return err
		}
		if invite.FromUserID != initiatorUserID {
			return tenancy.ErrBillingForbidden
		}
		row, err := q.DecideBillingTransferInvite(ctx, dbsqlc.DecideBillingTransferInviteParams{
			PublicID:                trimmed,
			Status:                  tenancy.BillingTransferCancelled,
			DecidedBy:               pgtype.Int8{Int64: initiatorUserID, Valid: true},
			ScopeWorkspacePublicIds: []string{},
		})
		if err != nil {
			if database.IsNotFound(err) {
				return tenancy.ErrBillingConflict
			}
			return err
		}
		decided = mapTransferInvite(row)
		return nil
	})
	return decided, err
}

// grantHandoverWorkspacesInTx grants the recipient Workspace Owner on every
// non-Default business Workspace associated with the account and returns
// the granted ws_ public ids for the audit scope. Organization workspaces
// grant workspace-level Owner (never org-level roles); Default workspaces
// are excluded by the ListHandoverWorkspacesForAccount query.
func (s *BillingAccountStore) grantHandoverWorkspacesInTx(ctx context.Context, q *dbsqlc.Queries, billingAccountID, recipientUserID int64) ([]string, error) {
	workspaces, err := q.ListHandoverWorkspacesForAccount(ctx, billingAccountID)
	if err != nil {
		return nil, err
	}
	scope := make([]string, 0, len(workspaces))
	for _, w := range workspaces {
		// Follow the same account -> Workspace lock order as scope deletion.
		// Membership mutations also lock this row before enforcing the cap.
		if _, err := q.LockWorkspaceMemberships(ctx, w.PublicID); err != nil {
			return nil, err
		}
		scope = append(scope, w.PublicID)
		existing, err := q.GetWorkspaceMembershipByIDs(ctx, dbsqlc.GetWorkspaceMembershipByIDsParams{
			WorkspaceID: w.InternalID, UserID: recipientUserID,
		})
		if err == nil {
			// Owners stay owners; members/admins rise to owner so the
			// recipient holds the accurate management scope. Never
			// demote an existing owner.
			if existing.Role == tenancy.RoleOwner {
				continue
			}
			if err := q.UpsertWorkspaceMembership(ctx, dbsqlc.UpsertWorkspaceMembershipParams{
				WorkspaceID: w.InternalID, UserID: recipientUserID, Role: tenancy.RoleOwner,
			}); err != nil {
				return nil, err
			}
			continue
		} else if !database.IsNotFound(err) {
			return nil, err
		}
		members, err := q.CountWorkspaceMembers(ctx, w.InternalID)
		if err != nil {
			return nil, err
		}
		if members >= tenancy.MaxWorkspaceMembers {
			return nil, tenancy.ErrWorkspaceMemberLimitReached
		}
		if err := q.UpsertWorkspaceMembership(ctx, dbsqlc.UpsertWorkspaceMembershipParams{
			WorkspaceID: w.InternalID, UserID: recipientUserID, Role: tenancy.RoleOwner,
		}); err != nil {
			return nil, err
		}
	}
	return scope, nil
}

// RemoveBillingAccountMember detaches one financial manager without
// deleting or freezing shared funds (#948 exit path). The Wallet,
// projection, Ledger, snapshots, and operation rows stay readable under
// their billing_account_id; only the management row leaves. The last Owner
// cannot be removed (ErrBillingLastOwner); leaving never mints a welcome
// grant, never resets quotas, and never rewrites existing attribution.
func (s *BillingAccountStore) RemoveBillingAccountMember(ctx context.Context, billingAccountInternalID, actorUserID, targetUserID int64) error {
	if s == nil || s.db == nil {
		return tenancy.ErrBillingStoreNotConfigured
	}
	if billingAccountInternalID <= 0 || targetUserID <= 0 {
		return tenancy.ErrBillingWorkspaceInvalid
	}
	return dbutil.WithTx(ctx, s.db, func(q *dbsqlc.Queries) error {
		if err := checkBillingMemberMutationInTx(ctx, q, billingAccountInternalID, actorUserID, targetUserID, "", true); err != nil {
			return err
		}
		rows, err := q.DeleteBillingAccountMember(ctx, dbsqlc.DeleteBillingAccountMemberParams{
			BillingAccountID: billingAccountInternalID, UserID: targetUserID,
		})
		if err != nil {
			return err
		}
		if rows == 0 {
			return tenancy.ErrBillingNotFound
		}
		return nil
	})
}

func isTransferRecipientNotFound(err error) bool {
	if err == nil {
		return false
	}
	// FK violations on to_user_id/from_user_id reference users(id).
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "to_user_id") && strings.Contains(msg, "fkey") ||
		strings.Contains(msg, "from_user_id") && strings.Contains(msg, "fkey") ||
		strings.Contains(msg, "billing_account_transfer_invites") && strings.Contains(msg, "foreign key")
}
