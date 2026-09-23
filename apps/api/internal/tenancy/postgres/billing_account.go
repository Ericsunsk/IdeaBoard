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
// Concurrency note: billing_accounts has no workspace unique constraint
// (migration avoided per #804 minimal-diff). Concurrent first reads may
// insert duplicate rows; the follow-up GetBillingAccountByWorkspaceID reads
// LIMIT 1. A unique partial index on workspace_id is the #805 follow-up if
// duplicates are observed.
func (s *BillingAccountStore) EnsureBillingAccountForWorkspace(ctx context.Context, workspaceInternalID int64, workspacePublicID string) (BillingAccount, error) {
	if s == nil || s.q == nil {
		return BillingAccount{}, errors.New("tenancy: billing account store not configured")
	}
	key := pgtype.Int8{Int64: workspaceInternalID, Valid: workspaceInternalID > 0}
	if row, err := s.q.GetBillingAccountByWorkspaceID(ctx, key); err == nil {
		return mapBillingAccount(row, workspacePublicID), nil
	} else if !database.IsNotFound(err) {
		return BillingAccount{}, err
	}
	created, err := s.q.CreateBillingAccountStub(ctx, dbsqlc.CreateBillingAccountStubParams{
		WorkspaceID: key,
	})
	if err != nil {
		// Best-effort convergence on concurrent insert: re-read.
		if row, readErr := s.q.GetBillingAccountByWorkspaceID(ctx, key); readErr == nil {
			return mapBillingAccount(row, workspacePublicID), nil
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
