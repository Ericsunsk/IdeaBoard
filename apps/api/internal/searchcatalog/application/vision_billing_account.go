// Vision workspace settlement identity (ADR-0038).
//
// A Vision Search call bills the consuming Workspace's stable Billing
// Account (fixed creation scope: the Organization pool for org workspaces,
// the owner's deterministic Personal account for personal workspaces; fail
// closed), never the operator's personal wallet. The operation row persists
// the settlement triple: submitter (user_id, the real initiator, also the
// Actor on every Command), consumption location (workspace_id), and payer
// (billing_account_id). Hold/Capture/Release and background reconciliation
// converge on the stored payer; they never re-resolve from the live Owner
// or membership, so an owner handover or member departure mid-flight
// cannot move the money.
//
// The member quota (#946) joins this same settlement transaction: the
// resolver runs inside the caller's tx before Quote/Hold, so the quota
// check fences the Hold atomically with the operation insert. This file
// implements no limit math, only the boundary.

package application

import (
	"context"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy/funds"

	"github.com/jackc/pgx/v5"
)

// VisionSettlementIdentity is the resolved settlement triple for one
// vision call inside the caller's transaction.
type VisionSettlementIdentity struct {
	// WorkspaceInternalID is the consumption location (workspaces.internal_id).
	WorkspaceInternalID int64
	// BillingAccountID is the stable payer (billing_accounts.internal_id).
	BillingAccountID int64
}

// VisionBillingAccountResolver resolves the settlement triple inside the
// caller's transaction and fails closed on ambiguity or conflict. The
// production implementation below owns the fixed-scope chain; tests inject a
// fake. Resolution never falls back to a personal wallet: scopeless
// callers do not exist on this path (every vision call names its
// consuming workspace).
type VisionBillingAccountResolver interface {
	ResolveVisionSettlementInTx(ctx context.Context, tx pgx.Tx, workspacePublicID string, actorUserID int64) (VisionSettlementIdentity, error)
	// ResolveWorkspaceInternalID maps a ws_ public id to its internal id
	// without any membership gate. Capture/Release use it only to verify
	// the stored resource scope; post-Hold membership changes never gate
	// settlement.
	ResolveWorkspaceInternalID(ctx context.Context, tx pgx.Tx, workspacePublicID string) (int64, error)
}

// visionBillingAccountResolver is the production fixed-scope chain over the
// caller's transaction: workspace visibility + membership gate, then the
// stable account resolution. Hidden workspaces and non-members converge
// to ErrVisionWorkspaceNotFound (never a distinct billing status), per
// anti-enumeration; billing ambiguity/conflict propagates fail-closed.
type visionBillingAccountResolver struct{}

func (visionBillingAccountResolver) ResolveVisionSettlementInTx(ctx context.Context, tx pgx.Tx, workspacePublicID string, actorUserID int64) (VisionSettlementIdentity, error) {
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	if tx == nil || actorUserID <= 0 || !contract.IsWorkspaceID(workspacePublicID) {
		return VisionSettlementIdentity{}, ErrVisionWorkspaceNotFound
	}
	queries := dbsqlc.New(tx)
	workspace, err := queries.GetWorkspaceByPublicID(ctx, workspacePublicID)
	if err != nil {
		return VisionSettlementIdentity{}, ErrVisionWorkspaceNotFound
	}
	// Closed Organizations suspend new spending (#955): the settlement
	// transaction fails closed even if the HTTP prelude was bypassed.
	// Terminal Capture/Release on row-pinned accounts never pass this
	// gate, so history survives closure.
	if closed, err := dbutil.IsOrganizationClosedForWorkspace(ctx, queries, workspacePublicID); err != nil {
		return VisionSettlementIdentity{}, err
	} else if closed {
		return VisionSettlementIdentity{}, ErrVisionWorkspaceNotFound
	}
	if _, err := queries.GetWorkspaceMembership(ctx, dbsqlc.GetWorkspaceMembershipParams{
		WorkspaceID: workspace.InternalID, UserID: actorUserID,
	}); err != nil {
		if workspace.OrganizationID.Valid {
			if orgMember, orgErr := queries.GetOrganizationMembership(ctx, dbsqlc.GetOrganizationMembershipParams{
				OrganizationID: workspace.OrganizationID.Int64, UserID: actorUserID,
			}); orgErr != nil || (orgMember.Role != "owner" && orgMember.Role != "admin") {
				return VisionSettlementIdentity{}, ErrVisionWorkspaceNotFound
			}
		} else {
			return VisionSettlementIdentity{}, ErrVisionWorkspaceNotFound
		}
	}
	account, err := funds.ResolveByPublicIDWithQueries(ctx, queries, workspacePublicID)
	if err != nil {
		return VisionSettlementIdentity{}, ErrVisionWorkspaceNotFound
	}
	if account.BillingAccountID <= 0 || account.WorkspaceInternalID <= 0 {
		return VisionSettlementIdentity{}, ErrVisionWorkspaceNotFound
	}
	return VisionSettlementIdentity{
		WorkspaceInternalID: account.WorkspaceInternalID,
		BillingAccountID:    account.BillingAccountID,
	}, nil
}

// ResolveWorkspaceInternalID maps a ws_ public id to its internal id
// without any membership gate (capture/release scope verification only).
// It delegates to the canonical dbutil seam so the vision path cannot
// drift from tasking/media/library resolution.
func (visionBillingAccountResolver) ResolveWorkspaceInternalID(ctx context.Context, tx pgx.Tx, workspacePublicID string) (int64, error) {
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	if tx == nil || !contract.IsWorkspaceID(workspacePublicID) {
		return 0, ErrVisionWorkspaceNotFound
	}
	internalID, err := dbutil.ResolveWorkspaceInternalID(ctx, dbsqlc.New(tx), workspacePublicID)
	if err != nil {
		return 0, ErrVisionWorkspaceNotFound
	}
	if internalID <= 0 {
		return 0, ErrVisionWorkspaceNotFound
	}
	return internalID, nil
}
