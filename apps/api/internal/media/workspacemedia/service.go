package workspacemedia

import (
	"context"
	"errors"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Service is the postgres WorkspaceMediaStore (#820, Phase 4-03 of
// #809). Repository methods take context.Context first and use the
// dbWithContext pattern. It must never be replaced by the test-only
// InMemoryWorkspaceMediaStore in production wiring.
//
// Workspace scoping: every lookup resolves the workspace public id to
// its internal id first and keys all session/asset statements by
// workspace. Cross-workspace ids miss by construction and read as not
// found; unknown workspaces list empty and mutate as not found.
// Membership itself is enforced by the v1 handlers
// (RequireWorkspaceCapability) before the store is reached; the store
// guarantees no cross-workspace reference escapes.
//
// Expiry is lazy: GetSession flips pending/transferring rows past
// their TTL to expired on read (UploadPendingTTL/UploadTransferTTL
// from created_at). No background sweeper exists by design.
type Service struct {
	db *pgxpool.Pool
	q  *dbsqlc.Queries
}

// Compile-time convergence with the v1 persistence seam.
var _ WorkspaceMediaStore = (*Service)(nil)

// errWorkspaceNotFound stays unexported: callers map it to the
// session/asset not-found error of their operation so hidden
// workspaces never leak a distinct status.
var errWorkspaceNotFound = errors.New("workspacemedia workspace not found")

// errNotConfigured reports a Service built without a database pool.
var errNotConfigured = errors.New("workspacemedia service is not configured")

// List caps: every list query carries a LIMIT so no caller can trigger
// an unbounded scan.
const maxMediaAssetRows = int32(100)

// mintAttempts bounds public_id regeneration after a random collision.
const mintAttempts = 3

// NewService builds a postgres media Service over pool.
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
		return nil, errNotConfigured
	}
	return db.q, nil
}

// resolveWorkspaceInternal maps an opaque workspace public id to its
// internal id. Unknown workspaces report errWorkspaceNotFound so
// callers converge to their operation's not-found error.
func resolveWorkspaceInternal(ctx context.Context, q *dbsqlc.Queries, workspacePublicID string) (int64, error) {
	row, err := q.GetWorkspaceByPublicID(ctx, strings.TrimSpace(workspacePublicID))
	if database.IsNotFound(err) {
		return 0, errWorkspaceNotFound
	}
	if err != nil {
		return 0, err
	}
	return row.InternalID, nil
}

func nullText(value string) pgtype.Text {
	if strings.TrimSpace(value) == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func nullInt4(value int32, present bool) pgtype.Int4 {
	if !present || value < 0 {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: value, Valid: true}
}

func int4Value(value pgtype.Int4) int32 {
	if !value.Valid || value.Int32 < 0 {
		return 0
	}
	return value.Int32
}
