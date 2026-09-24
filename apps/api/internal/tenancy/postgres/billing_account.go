// Package postgres implements tenancy billing-account provisioning (#804).
//
// Additive only: the #801 workspace/organization lifecycle is untouched.
// This file promotes the billing_accounts owner stub to real Get-or-Create
// provisioning per workspace (owner = workspace, org-support later).
// The account is addressed via the workspace path; no public billing-account
// ID appears in URLs.
package postgres

import (
	"context"
	"errors"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/tenancy"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BillingAccount is the workspace-owned billing account projection.
// Internal numeric keys never cross the HTTP boundary; the API addresses
// the account via its workspace public id.
type BillingAccount struct {
	WorkspaceInternalID int64
	WorkspacePublicID   string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// BillingAccountStore provisions the implicit per-workspace billing account.
// Repository methods take context.Context first and use dbWithContext(ctx).
type BillingAccountStore struct {
	db *pgxpool.Pool
	q  *dbsqlc.Queries
}

// NewBillingAccountStore builds a provisioner over pool. Pass nil pool in
// tests that stub the interface instead.
func NewBillingAccountStore(db *pgxpool.Pool) *BillingAccountStore {
	if db == nil {
		return &BillingAccountStore{}
	}
	return &BillingAccountStore{db: db, q: dbsqlc.New(db)}
}

func (s *BillingAccountStore) dbWithContext(ctx context.Context) *dbsqlc.Queries {
	return s.q
}

// EnsureBillingAccountForWorkspace provisions the workspace billing account
// on first read (Get-or-Create). Owner CHECK (workspace_id NOT NULL) is
// preserved; organization-owned accounts are #804-future and stay NULL here.
//
// Concurrency note: ux_billing_accounts_workspace (partial unique index,
// baseline) collapses concurrent first reads to one row; the loser reads
// back the winner on the 23505 duplicate error below. The follow-up
// GetBillingAccountByWorkspaceID reads LIMIT 1.
func (s *BillingAccountStore) EnsureBillingAccountForWorkspace(ctx context.Context, workspaceInternalID int64, workspacePublicID string) (BillingAccount, error) {
	if s == nil || s.q == nil {
		return BillingAccount{}, errors.New("tenancy: billing account store not configured")
	}
	key := pgtype.Int8{Int64: workspaceInternalID, Valid: workspaceInternalID > 0}
	queries := s.dbWithContext(ctx)
	if row, err := queries.GetBillingAccountByWorkspaceID(ctx, key); err == nil {
		return mapBillingAccount(row, workspacePublicID), nil
	} else if !database.IsNotFound(err) {
		return BillingAccount{}, err
	}
	// Every billing_accounts row carries a ba_-prefixed public_id per the
	// contract kernel (chk_billing_accounts_public_id); mint one through
	// the tenancy seam like the workspace lifecycle does.
	publicID, err := tenancy.NewBillingAccountPublicID()
	if err != nil {
		return BillingAccount{}, err
	}
	created, err := queries.CreateBillingAccountStub(ctx, dbsqlc.CreateBillingAccountStubParams{
		PublicID:       publicID,
		WorkspaceID:    key,
		OrganizationID: pgtype.Int8{},
	})
	if err != nil {
		// Best-effort convergence on concurrent first read: the
		// ux_billing_accounts_workspace loser reads back the winner.
		// Non-duplicate errors (including a ba_ public_id collision,
		// which re-reads as not-found) surface unchanged.
		if database.IsDuplicateConstraintError(err) {
			if row, readErr := queries.GetBillingAccountByWorkspaceID(ctx, key); readErr == nil {
				return mapBillingAccount(row, workspacePublicID), nil
			}
		}
		return BillingAccount{}, err
	}
	return mapBillingAccount(created, workspacePublicID), nil
}

func mapBillingAccount(row dbsqlc.BillingAccount, workspacePublicID string) BillingAccount {
	var workspaceInternal int64
	if row.WorkspaceID.Valid {
		workspaceInternal = row.WorkspaceID.Int64
	}
	return BillingAccount{
		WorkspaceInternalID: workspaceInternal,
		WorkspacePublicID:   workspacePublicID,
		CreatedAt:           dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:           dbutil.TimeValue(row.UpdatedAt),
	}
}
