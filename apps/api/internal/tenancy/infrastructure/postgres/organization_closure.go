// Package postgres implements permanent Organization closure.
// Closure revokes access and spending permanently. Money facts remain for
// audit and legally required remedies; no automatic refund or Personal transfer occurs.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/tenancy"

	"github.com/jackc/pgx/v5/pgtype"
)

// maxClosureSnapshots bounds the credited-order refs pinned in one closure
// audit row. Older credited orders stay readable under their account.
const maxClosureSnapshots = int32(500)

// CloseOrganization permanently closes a live Organization. The actor must be the
// Owner; non-members read as not found (anti-enumeration), non-owners as
// forbidden. Already-closed rows read as ErrOrganizationClosed so a
// retried close never forks a second audit.
func (s *OrganizationStore) CloseOrganization(ctx context.Context, organizationPublicID string, actorUserID int64, reason string) (*tenancy.Organization, error) {
	organizationPublicID = strings.TrimSpace(organizationPublicID)
	reason = strings.TrimSpace(reason)
	if organizationPublicID == "" || actorUserID <= 0 {
		return nil, tenancy.ErrNotFound
	}
	var closed *tenancy.Organization
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		locked, err := q.LockOrganizationByPublicID(ctx, organizationPublicID)
		if err != nil {
			return notFound(err)
		}
		if locked.ClosedAt.Valid {
			return tenancy.ErrOrganizationClosed
		}
		membership, err := q.GetOrganizationMembership(ctx, dbsqlc.GetOrganizationMembershipParams{
			OrganizationID: locked.InternalID, UserID: actorUserID,
		})
		if err != nil {
			return notFound(err)
		}
		if membership.Role != tenancy.RoleOwner {
			return tenancy.ErrForbidden
		}
		// Resolve the shared pool without provisioning: closure must
		// never mint a funds identity as a side effect.
		account, err := q.GetBillingAccountByOrganizationID(ctx, pgtype.Int8{Int64: locked.InternalID, Valid: true})
		if err != nil {
			if database.IsNotFound(err) {
				return tenancy.ErrBillingConflict
			}
			return err
		}
		if _, err := lockBillingAccountInTx(ctx, q, account.InternalID); err != nil {
			return err
		}
		if err := guardOrganizationClosureInflight(ctx, q, locked.InternalID, account.InternalID); err != nil {
			return err
		}
		// Freeze and lock the Wallet before reading its closure snapshot. The
		// same transaction rolls the freeze back if any closure check fails.
		if err := q.FreezeOrganizationWallet(ctx, account.InternalID); err != nil {
			return err
		}
		snapshot, err := snapshotOrganizationClosure(ctx, q, locked.InternalID, account.InternalID)
		if err != nil {
			return err
		}
		if _, err := q.InsertOrganizationClosure(ctx, dbsqlc.InsertOrganizationClosureParams{
			OrganizationID:     locked.InternalID,
			BillingAccountID:   account.InternalID,
			ClosedBy:           pgtype.Int8{Int64: actorUserID, Valid: true},
			Reason:             reason,
			MembershipSnapshot: snapshot.membershipJSON,
			WorkspaceSnapshot:  snapshot.workspaceJSON,
			AvailableBalance:   snapshot.available,
			ReservedBalance:    snapshot.reserved,
			CreditedOrderRefs:  snapshot.ordersJSON,
			Disposition:        tenancy.OrganizationClosureExpired,
			DispositionDetail:  snapshot.dispositionJSON,
		}); err != nil {
			if database.IsDuplicateConstraintError(err) {
				return tenancy.ErrOrganizationClosed
			}
			return err
		}
		rows, err := q.SetOrganizationClosed(ctx, dbsqlc.SetOrganizationClosedParams{
			PublicID: organizationPublicID,
			ClosedBy: pgtype.Int8{Int64: actorUserID, Valid: true},
		})
		if err != nil {
			return err
		}
		if rows != 1 {
			return tenancy.ErrOrganizationClosed
		}
		read, err := q.GetOrganizationByPublicID(ctx, organizationPublicID)
		if err != nil {
			return notFound(err)
		}
		closed = mapOrganization(read)
		return nil
	})
	return closed, err
}

// guardOrganizationClosureInflight fails the closure closed while any
// obligation still settles on the shared pool. The account lock is already
// held, so concurrent submissions serialize here.
func guardOrganizationClosureInflight(ctx context.Context, q *dbsqlc.Queries, organizationInternalID, billingAccountID int64) error {
	orgKey := pgtype.Int8{Int64: organizationInternalID, Valid: true}
	accountKey := pgtype.Int8{Int64: billingAccountID, Valid: true}
	if count, err := q.CountActiveOrgTasks(ctx, orgKey); err != nil {
		return fmt.Errorf("tenancy: organization close: check active tasks: %w", err)
	} else if count > 0 {
		return tenancy.ErrOrganizationClosureBlocked
	}
	if count, err := q.CountActiveOrgSearchOperations(ctx, orgKey); err != nil {
		return fmt.Errorf("tenancy: organization close: check active search: %w", err)
	} else if count > 0 {
		return tenancy.ErrOrganizationClosureBlocked
	}
	if count, err := q.CountUnsettledOrgPaymentOrders(ctx, accountKey); err != nil {
		return fmt.Errorf("tenancy: organization close: check unsettled payments: %w", err)
	} else if count > 0 {
		return tenancy.ErrOrganizationClosureBlocked
	}
	if count, err := q.CountPendingOrgPaymentRefunds(ctx, accountKey); err != nil {
		return fmt.Errorf("tenancy: organization close: check pending refunds: %w", err)
	} else if count > 0 {
		return tenancy.ErrOrganizationClosureBlocked
	}
	if count, err := q.CountOpenOrgPaymentDisputes(ctx, accountKey); err != nil {
		return fmt.Errorf("tenancy: organization close: check open disputes: %w", err)
	} else if count > 0 {
		return tenancy.ErrOrganizationClosureBlocked
	}
	if count, err := q.CountPendingOrgTransferInvites(ctx, billingAccountID); err != nil {
		return fmt.Errorf("tenancy: organization close: check pending handover: %w", err)
	} else if count > 0 {
		return tenancy.ErrOrganizationClosureBlocked
	}
	return nil
}

// closureSnapshot pins immutable closure audit facts. Balances and Ledger rows
// remain unchanged; the permanently closed account cannot spend its balance.
type closureSnapshot struct {
	membershipJSON  []byte
	workspaceJSON   []byte
	ordersJSON      []byte
	dispositionJSON []byte
	available       int64
	reserved        int64
}

func snapshotOrganizationClosure(ctx context.Context, q *dbsqlc.Queries, organizationInternalID, billingAccountID int64) (*closureSnapshot, error) {
	members, err := q.ListOrganizationMembersForSnapshot(ctx, organizationInternalID)
	if err != nil {
		return nil, fmt.Errorf("tenancy: organization close: snapshot members: %w", err)
	}
	pinned := make([]tenancy.OrganizationClosureMemberSnapshot, 0, len(members))
	for _, m := range members {
		pinned = append(pinned, tenancy.OrganizationClosureMemberSnapshot{UserID: m.UserID, Role: m.Role})
	}
	membershipJSON, err := json.Marshal(pinned)
	if err != nil {
		return nil, fmt.Errorf("tenancy: organization close: encode members: %w", err)
	}
	workspaces, err := q.ListOrganizationWorkspacePublicIDs(ctx, pgtype.Int8{Int64: organizationInternalID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("tenancy: organization close: snapshot workspaces: %w", err)
	}
	if workspaces == nil {
		workspaces = []string{}
	}
	workspaceJSON, err := json.Marshal(workspaces)
	if err != nil {
		return nil, fmt.Errorf("tenancy: organization close: encode workspaces: %w", err)
	}
	// Wallet balances are read-only facts here: a missing Wallet is torn
	// state, so closure fails closed instead of recording blind zeros.
	balances, err := q.GetWalletBalancesForAccount(ctx, pgtype.Int8{Int64: billingAccountID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("tenancy: organization close: read balances: %w", err)
	}
	available, ok := balances.Max.(int64)
	if !ok {
		return nil, tenancy.ErrBillingConflict
	}
	reserved, ok := balances.Max_2.(int64)
	if !ok {
		return nil, tenancy.ErrBillingConflict
	}
	if reserved != 0 {
		return nil, tenancy.ErrOrganizationClosureBlocked
	}
	orders, err := q.ListCreditedOrgPaymentOrders(ctx, dbsqlc.ListCreditedOrgPaymentOrdersParams{
		BillingAccountID: pgtype.Int8{Int64: billingAccountID, Valid: true}, RowLimit: maxClosureSnapshots,
	})
	if err != nil {
		return nil, fmt.Errorf("tenancy: organization close: list credited orders: %w", err)
	}
	refs := make([]string, 0, len(orders))
	terms := make([]string, 0, len(orders))
	refundPolicies := make([]string, 0, len(orders))
	for _, o := range orders {
		refs = append(refs, o.PublicOrderNumber)
		terms = append(terms, o.TermsVersion)
		refundPolicies = append(refundPolicies, o.RefundPolicyVersion)
	}
	ordersJSON, err := json.Marshal(refs)
	if err != nil {
		return nil, fmt.Errorf("tenancy: organization close: encode orders: %w", err)
	}
	dispositionJSON, err := json.Marshal(map[string]any{
		"outcome":                "permanently_closed",
		"terms_versions":         terms,
		"refund_policy_versions": refundPolicies,
		"note":                   "permanently unavailable; no automatic refund or Personal transfer; legal and purchase commitments remain applicable",
	})
	if err != nil {
		return nil, fmt.Errorf("tenancy: organization close: encode disposition: %w", err)
	}
	return &closureSnapshot{
		membershipJSON:  membershipJSON,
		workspaceJSON:   workspaceJSON,
		ordersJSON:      ordersJSON,
		dispositionJSON: dispositionJSON,
		available:       available,
		reserved:        reserved,
	}, nil
}
