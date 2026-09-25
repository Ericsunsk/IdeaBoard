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

func (s *BillingAccountStore) dbWithContext(ctx context.Context) repositoryQueryer {
	return repositoryQueryer{ctx: ctx, db: s.db, q: s.q}
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
	created, err := ensureBillingAccountForWorkspace(ctx, s.dbWithContext(ctx).q, workspaceInternalID)
	if err != nil {
		return BillingAccount{}, err
	}
	return mapBillingAccount(created, workspacePublicID), nil
}

// EnsureBillingAccountForWorkspaceWithQueries applies the same provisioning
// rules to an existing query set, including a caller-owned transaction.
// Task settlement uses this form so billing-account creation remains in the
// task insert transaction without duplicating the get-or-create logic.
func EnsureBillingAccountForWorkspaceWithQueries(ctx context.Context, queries *dbsqlc.Queries, workspaceInternalID int64) (dbsqlc.BillingAccount, error) {
	return ensureBillingAccountForWorkspace(ctx, queries, workspaceInternalID)
}

func ensureBillingAccountForWorkspace(ctx context.Context, queries *dbsqlc.Queries, workspaceInternalID int64) (dbsqlc.BillingAccount, error) {
	if queries == nil || workspaceInternalID <= 0 {
		return dbsqlc.BillingAccount{}, errors.New("tenancy: billing account workspace is invalid")
	}
	key := pgtype.Int8{Int64: workspaceInternalID, Valid: true}
	if row, err := queries.GetBillingAccountByWorkspaceID(ctx, key); err == nil {
		return row, nil
	} else if !database.IsNotFound(err) {
		return dbsqlc.BillingAccount{}, err
	}
	publicID, err := tenancy.NewBillingAccountPublicID()
	if err != nil {
		return dbsqlc.BillingAccount{}, err
	}
	created, err := queries.CreateBillingAccountStub(ctx, dbsqlc.CreateBillingAccountStubParams{
		PublicID: publicID, WorkspaceID: key, OrganizationID: pgtype.Int8{},
	})
	if err != nil {
		if database.IsDuplicateConstraintError(err) {
			if row, readErr := queries.GetBillingAccountByWorkspaceID(ctx, key); readErr == nil {
				return row, nil
			}
		}
		return dbsqlc.BillingAccount{}, err
	}
	return created, nil
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
