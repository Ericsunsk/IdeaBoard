// Package postgres is the postgres WorkspaceLibraryStore (#818, Phase 4-01
// of #809). Repository methods take context.Context first and use the
// dbWithContext pattern. It must never be replaced by the test-only
// InMemoryWorkspaceLibraryStore in production wiring.
//
// Workspace scoping: every lookup resolves the workspace public id to
// its internal id first and keys all folder/asset statements by
// (workspace_id, public_id). Cross-workspace ids miss by construction
// and read as not found; unknown workspaces list empty and mutate as
// not found. Membership itself is enforced by the v1 handlers
// (RequireWorkspaceCapability) before the store is reached; the store
// guarantees no cross-workspace reference escapes.
//
// Quick Saves: the workspace tables require folder_id NOT NULL, so the
// seam's unfiled ("") state maps to the workspace Quick Saves system
// folder (is_system + is_default, name 'Quick Saves'), ensured on
// demand. Deleting a user folder moves its assets to Quick Saves first
// (same tx), mirroring the in-memory "become unfiled" semantics.
// System folders are immutable: rename/delete statements exclude them.
//
// Title/author are durable display metadata; task lineage and the
// server-side operator persist on media rows (never serialized).
//
// Materializations are durable jobs (pending -> running -> completed|
// failed; terminal states immutable). The durable status word for
// running is 'processing' (chk_library_materializations_status); the
// store maps it at the boundary and enforces the machine in SQL
// guards. Completion links a workspace-scoped Media Asset (the
// caller's asset_... id resolves to media_assets.internal_id; unknown
// links fail closed with the job left non-terminal). Failures persist
// error_message only (no durable error_code column exists; reads
// report an empty code) and rows carry no updated_at (reads report
// completed_at, else created_at, as updated_at).
package postgres

import (
	"context"
	"errors"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/library/ports"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	db *pgxpool.Pool
	q  *dbsqlc.Queries
}

// Compile-time convergence with the v1 persistence seam.
var _ ports.WorkspaceLibraryStore = (*Service)(nil)

// errWorkspaceNotFound stays unexported: callers map it to the
// folder/asset not-found error of their operation so hidden workspaces
// never leak a distinct status.
var errWorkspaceNotFound = errors.New("library workspace not found")

// ErrNotConfigured reports a Service built without a database pool.
var ErrNotConfigured = errors.New("library service is not configured")

// List caps: every list query carries a LIMIT so no caller can trigger
// an unbounded scan. Keyset pagination is a follow-up; the handler
// slices pages in memory like the in-memory store.
const (
	maxLibraryFolders          = int32(500)
	maxLibraryAssets           = int32(2000)
	maxLibraryMaterializations = int32(2000)
)

// mintAttempts bounds public_id regeneration after a random collision.
const mintAttempts = 3

// NewService builds a postgres library Service over pool.
func NewService(db *pgxpool.Pool) *Service {
	if db == nil {
		return &Service{}
	}
	return &Service{db: db, q: dbsqlc.New(db)}
}

type serviceQueryer struct {
	ctx context.Context
	db  *pgxpool.Pool
	q   *dbsqlc.Queries
}

func (s *Service) dbWithContext(ctx context.Context) serviceQueryer {
	return serviceQueryer{ctx: ctx, db: s.db, q: s.q}
}

func (db serviceQueryer) withTx(run func(*dbsqlc.Queries) error) error {
	return dbutil.WithTx(db.ctx, db.db, run)
}

func (db serviceQueryer) queries() (*dbsqlc.Queries, error) {
	if db.q == nil {
		return nil, ErrNotConfigured
	}
	return db.q, nil
}

// resolveWorkspaceInternal maps an opaque workspace public id to its
// internal id. Unknown workspaces report errWorkspaceNotFound so
// callers converge to their operation's not-found error.
func resolveWorkspaceInternal(ctx context.Context, q *dbsqlc.Queries, workspacePublicID string) (int64, error) {
	internalID, err := dbutil.ResolveWorkspaceInternalID(ctx, q, workspacePublicID)
	if database.IsNotFound(err) {
		return 0, errWorkspaceNotFound
	}
	if err != nil {
		return 0, err
	}
	return internalID, nil
}
