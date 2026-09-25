package boards

// Postgres WorkspaceBoardStore (issue #802 interface, #809 follow-up to
// #817).
//
// This is the production WorkspaceBoardStore over PostgreSQL/sqlc. It
// mirrors InMemoryWorkspaceBoardStore semantics:
//
//   - Board external ids mint board_<hex>; legacy non-prefixed ids are out
//     of scope and fail closed (ErrInvalidInput on create, ErrNotFound on
//     read). boards.workspace_id is the workspace link (no separate link
//     table); every board-scoped query joins workspaces on public_id so
//     foreign-workspace rows stay invisible.
//   - The board creator is the implicit owner via boards.owner_user_id and
//     never needs a board_memberships row. Rows predating the owner column
//     read as not found (fail-closed).
//   - Revisions are INSERT-only: ReplaceDocument appends a board_revisions
//     row and advances board_documents under SELECT ... FOR UPDATE plus a
//     version-guarded UPDATE, so stale writers get ErrRevisionConflict.
//     The revision counter doubles as the ETag value.
//   - Only the share-token hash is stored (token_secret); the bearer is
//     returned once at create and never persisted, so share secrets need
//     no reversible encryption at rest.
//   - Workspace membership does NOT grant board access: every board read
//     requires an explicit board role (creator counts as owner).
//   - The last owner cannot be removed or demoted (ErrV1LastOwner).
//
// Divergences from the in-memory store (documented residuals):
//   - V1ShareToken.UsageCount is persisted and advances on join, but
//     V1AuditEvent trails are not (durable audit is #808).
//   - Board-name uniqueness is not enforced (matches the in-memory store;
//     no UNIQUE(workspace_id, title) constraint exists).
//   - CreateBoard on an unknown workspace returns ErrNotFound (the
//     in-memory store materializes the workspace bucket implicitly);
//     handlers always resolve the workspace first.
//
// Repository methods take context.Context first and use dbWithContext(ctx).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Local mint prefixes (not kernel prefixes yet, like v1ShareTokenPrefix in
// v1_workspace.go). They match the board_revisions (rev_%) and
// board_share_tokens (bst_%) public_id CHECKs, so minted ids pass through
// opaquely.
const (
	postgresRevisionPrefix  = "rev_"
	postgresShareRowPrefix  = "bst_"
	maxWorkspaceBoardRows   = int32(500)
	postgresRevisionIDBytes = 12
	postgresShareRowIDBytes = 12
)

// PostgresWorkspaceBoardStore is the postgres WorkspaceBoardStore. It must
// never be replaced by the test-only InMemoryWorkspaceBoardStore in
// production wiring.
type PostgresWorkspaceBoardStore struct {
	db *pgxpool.Pool
	q  *dbsqlc.Queries
}

// Compile-time convergence: the postgres store implements the seam.
var _ WorkspaceBoardStore = (*PostgresWorkspaceBoardStore)(nil)

type postgresBoardQueryer struct {
	ctx context.Context
	db  *pgxpool.Pool
	q   *dbsqlc.Queries
}

// NewPostgresWorkspaceBoardStore builds a postgres WorkspaceBoardStore over pool.
func NewPostgresWorkspaceBoardStore(db *pgxpool.Pool) *PostgresWorkspaceBoardStore {
	return &PostgresWorkspaceBoardStore{db: db, q: dbsqlc.New(db)}
}

func (s *PostgresWorkspaceBoardStore) dbWithContext(ctx context.Context) postgresBoardQueryer {
	return postgresBoardQueryer{ctx: ctx, db: s.db, q: s.q}
}

func (db postgresBoardQueryer) withTx(run func(*dbsqlc.Queries) error) error {
	return dbutil.WithTx(db.ctx, db.db, run)
}

// boardDocumentETag renders the board_documents etag column: a hash of
// (document_version || snapshot_data). It satisfies the 8..128 length
// CHECK; the V1 ETag stays the revision counter (FormatV1ETag).
func boardDocumentETag(version int64, snapshot json.RawMessage) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("v%d:%s", version, string(snapshot))))
	return hex.EncodeToString(sum[:])
}

func mapPostgresBoardRole(raw string) (AccessRole, bool) {
	switch AccessRole(strings.TrimSpace(raw)) {
	case RoleOwner, RoleEditor, RoleViewer:
		return AccessRole(strings.TrimSpace(raw)), true
	default:
		return "", false
	}
}

func mapPostgresBoardRow(row dbsqlc.GetWorkspaceBoardRow) (*V1Board, error) {
	if !row.OwnerUserID.Valid {
		// Owner-less rows predate the owner column: fail closed.
		return nil, ErrNotFound
	}
	document := make(json.RawMessage, len(row.SnapshotData))
	copy(document, row.SnapshotData)
	return &V1Board{
		WorkspacePublicID: row.WorkspacePublicID,
		PublicID:          row.PublicID,
		Name:              row.Title,
		Revision:          int64(row.DocumentVersion),
		SchemaVersion:     row.SchemaVersion,
		Document:          document,
		ArchivedAt:        dbutil.TimePtr(row.ArchivedAt),
		OwnerUserID:       row.OwnerUserID.Int64,
		CreatedBy:         row.OwnerUserID.Int64,
		CreatedAt:         dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:         dbutil.TimeValue(row.UpdatedAt),
	}, nil
}

// getBoardTx reads one workspace-scoped board inside or outside a tx.
func getPostgresBoard(ctx context.Context, q *dbsqlc.Queries, workspacePublicID, boardPublicID string) (*V1Board, error) {
	row, err := q.GetWorkspaceBoard(ctx, dbsqlc.GetWorkspaceBoardParams{
		WorkspacePublicID: strings.TrimSpace(workspacePublicID),
		BoardPublicID:     strings.TrimSpace(boardPublicID),
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return mapPostgresBoardRow(row)
}

func (s *PostgresWorkspaceBoardStore) getBoard(ctx context.Context, workspacePublicID, boardPublicID string) (*V1Board, error) {
	return getPostgresBoard(ctx, s.dbWithContext(ctx).q, workspacePublicID, boardPublicID)
}

// boardRoleTx reports the explicit board role: the owner_user_id counts as
// owner; member rows grant editor/viewer/owner.
func boardRoleTx(ctx context.Context, q *dbsqlc.Queries, board *V1Board, userID int64) (AccessRole, bool, error) {
	if board.OwnerUserID == userID {
		return RoleOwner, true, nil
	}
	raw, err := q.GetWorkspaceBoardMembershipRole(ctx, dbsqlc.GetWorkspaceBoardMembershipRoleParams{
		WorkspacePublicID: board.WorkspacePublicID,
		BoardPublicID:     board.PublicID,
		UserID:            userID,
	})
	if err != nil {
		if database.IsNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}
	role, ok := mapPostgresBoardRole(raw)
	if !ok {
		return "", false, nil
	}
	return role, true, nil
}

// ownerCountTx counts owner-role member rows besides the implicit owner,
// then adds the implicit owner itself (mirrors ownerCountLocked).
func ownerCountTx(ctx context.Context, q *dbsqlc.Queries, board *V1Board) (int, error) {
	count, err := q.CountWorkspaceBoardOwners(ctx, dbsqlc.CountWorkspaceBoardOwnersParams{
		WorkspacePublicID: board.WorkspacePublicID,
		BoardPublicID:     board.PublicID,
	})
	if err != nil {
		return 0, err
	}
	return int(count) + 1, nil
}

// CreateBoard mints a workspace-rooted board at revision 1 with the empty
// discriminated-union snapshot; the creator is the implicit owner.
func (s *PostgresWorkspaceBoardStore) CreateBoard(ctx context.Context, workspacePublicID string, creatorUserID int64, name, publicID string) (*V1Board, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	trimmedID := strings.TrimSpace(publicID)
	if workspacePublicID == "" || creatorUserID <= 0 || trimmedID == "" {
		return nil, ErrInvalidInput
	}
	if !contract.IsBoardID(trimmedID) {
		return nil, ErrInvalidInput
	}
	if err := ValidateV1BoardName(name); err != nil {
		return nil, err
	}
	var created *V1Board
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		row, err := q.CreateWorkspaceBoard(ctx, dbsqlc.CreateWorkspaceBoardParams{
			PublicID:          trimmedID,
			WorkspacePublicID: workspacePublicID,
			OwnerUserID:       dbutil.NullInt64(&creatorUserID),
			Title:             strings.TrimSpace(name),
		})
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			if database.IsDuplicateConstraintError(err) {
				return ErrIDConflict
			}
			return err
		}
		document := V1InitialDocument()
		if err := q.InsertBoardDocument(ctx, dbsqlc.InsertBoardDocumentParams{
			BoardID:         row.InternalID,
			DocumentVersion: 1,
			SnapshotData:    []byte(document),
			Etag:            boardDocumentETag(1, document),
		}); err != nil {
			return err
		}
		revisionPublicID, err := contract.MintPublicID(postgresRevisionPrefix, postgresRevisionIDBytes)
		if err != nil {
			return err
		}
		if _, err := q.InsertBoardRevision(ctx, dbsqlc.InsertBoardRevisionParams{
			PublicID:         revisionPublicID,
			BoardID:          row.InternalID,
			RevisionNumber:   1,
			AuthorUserID:     creatorUserID,
			ParentRevisionID: dbutil.NullInt64(nil),
			RevisionData:     []byte(document),
		}); err != nil {
			if database.IsDuplicateConstraintError(err) {
				return ErrIDConflict
			}
			return err
		}
		board, err := getPostgresBoard(ctx, q, workspacePublicID, trimmedID)
		if err != nil {
			return err
		}
		created = board
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// ListBoards lists workspace boards ordered by updated_at desc (stable by
// public id); archived rows are excluded unless includeArchived.
// Owner-less legacy rows are skipped (fail-closed without failing the list).
func (s *PostgresWorkspaceBoardStore) ListBoards(ctx context.Context, workspacePublicID string, includeArchived bool) ([]*V1Board, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rows, err := s.dbWithContext(ctx).q.ListWorkspaceBoards(ctx, dbsqlc.ListWorkspaceBoardsParams{
		WorkspacePublicID: strings.TrimSpace(workspacePublicID),
		IncludeArchived:   includeArchived,
		RowLimit:          maxWorkspaceBoardRows,
	})
	if err != nil {
		return nil, err
	}
	out := make([]*V1Board, 0, len(rows))
	for _, row := range rows {
		board, err := mapPostgresBoardRow(dbsqlc.GetWorkspaceBoardRow(row))
		if err != nil {
			continue
		}
		out = append(out, board)
	}
	return out, nil
}

// GetBoard reads one board (archived rows stay visible here; callers decide
// whether the operation is allowed on them).
func (s *PostgresWorkspaceBoardStore) GetBoard(ctx context.Context, workspacePublicID, boardPublicID string) (*V1Board, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.getBoard(ctx, workspacePublicID, boardPublicID)
}

// RenameBoard renames a live board; the revision counter is untouched.
func (s *PostgresWorkspaceBoardStore) RenameBoard(ctx context.Context, workspacePublicID, boardPublicID, name string) (*V1Board, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ValidateV1BoardName(name); err != nil {
		return nil, err
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	trimmedID := strings.TrimSpace(boardPublicID)
	if _, err := s.getBoard(ctx, workspacePublicID, trimmedID); err != nil {
		return nil, err
	}
	affected, err := s.dbWithContext(ctx).q.RenameWorkspaceBoard(ctx, dbsqlc.RenameWorkspaceBoardParams{
		Title:             strings.TrimSpace(name),
		WorkspacePublicID: workspacePublicID,
		BoardPublicID:     trimmedID,
	})
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, ErrV1Archived
	}
	return s.getBoard(ctx, workspacePublicID, trimmedID)
}

// ArchiveBoard archives a live board; already-archived rows read as not
// found so archive stays idempotent-safe at the handler layer.
func (s *PostgresWorkspaceBoardStore) ArchiveBoard(ctx context.Context, workspacePublicID, boardPublicID string) (*V1Board, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	trimmedID := strings.TrimSpace(boardPublicID)
	if _, err := s.getBoard(ctx, workspacePublicID, trimmedID); err != nil {
		return nil, err
	}
	affected, err := s.dbWithContext(ctx).q.ArchiveWorkspaceBoard(ctx, dbsqlc.ArchiveWorkspaceBoardParams{
		WorkspacePublicID: workspacePublicID,
		BoardPublicID:     trimmedID,
	})
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, ErrNotFound
	}
	return s.getBoard(ctx, workspacePublicID, trimmedID)
}

// RestoreBoard clears the archive timestamp; live boards read as not found.
func (s *PostgresWorkspaceBoardStore) RestoreBoard(ctx context.Context, workspacePublicID, boardPublicID string) (*V1Board, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	trimmedID := strings.TrimSpace(boardPublicID)
	if _, err := s.getBoard(ctx, workspacePublicID, trimmedID); err != nil {
		return nil, err
	}
	affected, err := s.dbWithContext(ctx).q.RestoreWorkspaceBoard(ctx, dbsqlc.RestoreWorkspaceBoardParams{
		WorkspacePublicID: workspacePublicID,
		BoardPublicID:     trimmedID,
	})
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, ErrNotFound
	}
	return s.getBoard(ctx, workspacePublicID, trimmedID)
}

// ReplaceDocument appends an immutable revision when the expected revision
// matches the current counter; stale writers get ErrRevisionConflict.
func (s *PostgresWorkspaceBoardStore) ReplaceDocument(ctx context.Context, workspacePublicID, boardPublicID string, actorUserID, expectedRevision int64, schemaVersion int32, snapshot json.RawMessage) (*V1Board, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if actorUserID <= 0 || expectedRevision <= 0 || schemaVersion < 1 || len(snapshot) == 0 {
		return nil, ErrInvalidInput
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	trimmedID := strings.TrimSpace(boardPublicID)
	var updated *V1Board
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		locked, err := q.LockWorkspaceBoard(ctx, dbsqlc.LockWorkspaceBoardParams{
			WorkspacePublicID: workspacePublicID,
			BoardPublicID:     trimmedID,
		})
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return err
		}
		if locked.ArchivedAt.Valid {
			return ErrV1Archived
		}
		if int64(locked.DocumentVersion) != expectedRevision {
			return ErrRevisionConflict
		}
		parent, err := q.GetBoardRevisionByNumber(ctx, dbsqlc.GetBoardRevisionByNumberParams{
			WorkspacePublicID: workspacePublicID,
			BoardPublicID:     trimmedID,
			RevisionNumber:    locked.DocumentVersion,
		})
		if err != nil {
			// Version matched but the revision row is missing: treat as a
			// concurrent anomaly, not as a new board state.
			return ErrRevisionConflict
		}
		revisionPublicID, err := contract.MintPublicID(postgresRevisionPrefix, postgresRevisionIDBytes)
		if err != nil {
			return err
		}
		nextRevision := expectedRevision + 1
		document := make(json.RawMessage, len(snapshot))
		copy(document, snapshot)
		if _, err := q.InsertBoardRevision(ctx, dbsqlc.InsertBoardRevisionParams{
			PublicID:         revisionPublicID,
			BoardID:          locked.InternalID,
			RevisionNumber:   int32(nextRevision),
			AuthorUserID:     actorUserID,
			ParentRevisionID: dbutil.NullInt64(&parent.InternalID),
			RevisionData:     []byte(document),
		}); err != nil {
			if database.IsDuplicateConstraintError(err) {
				return ErrRevisionConflict
			}
			return err
		}
		affected, err := q.UpdateBoardDocument(ctx, dbsqlc.UpdateBoardDocumentParams{
			DocumentVersion: int32(nextRevision),
			SnapshotData:    []byte(document),
			Etag:            boardDocumentETag(nextRevision, document),
			BoardID:         locked.InternalID,
			ExpectedVersion: locked.DocumentVersion,
		})
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrRevisionConflict
		}
		if err := q.TouchWorkspaceBoard(ctx, locked.InternalID); err != nil {
			return err
		}
		board, err := getPostgresBoard(ctx, q, workspacePublicID, trimmedID)
		if err != nil {
			return err
		}
		updated = board
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// ListRevisions returns immutable snapshots oldest-first.
func (s *PostgresWorkspaceBoardStore) ListRevisions(ctx context.Context, workspacePublicID, boardPublicID string) ([]*V1Revision, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	trimmedID := strings.TrimSpace(boardPublicID)
	if _, err := s.getBoard(ctx, workspacePublicID, trimmedID); err != nil {
		return nil, err
	}
	rows, err := s.dbWithContext(ctx).q.ListBoardRevisions(ctx, dbsqlc.ListBoardRevisionsParams{
		WorkspacePublicID: workspacePublicID,
		BoardPublicID:     trimmedID,
		RowLimit:          maxWorkspaceBoardRows,
	})
	if err != nil {
		return nil, err
	}
	out := make([]*V1Revision, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapPostgresRevision(trimmedID, row.InternalID, row.RevisionNumber, row.SchemaVersion, row.RevisionData, row.SourceRevision, row.AuthorUserID, dbutil.TimeValue(row.CreatedAt)))
	}
	return out, nil
}

// GetRevision resolves an entry by revision number first, then by entry id.
func (s *PostgresWorkspaceBoardStore) GetRevision(ctx context.Context, workspacePublicID, boardPublicID string, revisionOrID int64) (*V1Revision, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	queries := s.dbWithContext(ctx).q
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	trimmedID := strings.TrimSpace(boardPublicID)
	if revisionOrID > 0 && revisionOrID <= math.MaxInt32 {
		if row, err := queries.GetBoardRevisionByNumber(ctx, dbsqlc.GetBoardRevisionByNumberParams{
			WorkspacePublicID: workspacePublicID,
			BoardPublicID:     trimmedID,
			RevisionNumber:    int32(revisionOrID),
		}); err == nil {
			return mapPostgresRevision(trimmedID, row.InternalID, row.RevisionNumber, row.SchemaVersion, row.RevisionData, row.SourceRevision, row.AuthorUserID, dbutil.TimeValue(row.CreatedAt)), nil
		} else if !database.IsNotFound(err) {
			return nil, err
		}
	}
	row, err := queries.GetBoardRevisionByID(ctx, dbsqlc.GetBoardRevisionByIDParams{
		WorkspacePublicID: workspacePublicID,
		BoardPublicID:     trimmedID,
		InternalID:        revisionOrID,
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return mapPostgresRevision(trimmedID, row.InternalID, row.RevisionNumber, row.SchemaVersion, row.RevisionData, row.SourceRevision, row.AuthorUserID, dbutil.TimeValue(row.CreatedAt)), nil
}

func mapPostgresRevision(boardPublicID string, id int64, revision, schemaVersion int32, snapshot []byte, sourceRevision int32, createdBy int64, createdAt time.Time) *V1Revision {
	document := make(json.RawMessage, len(snapshot))
	copy(document, snapshot)
	return &V1Revision{
		ID:             id,
		BoardPublicID:  boardPublicID,
		Revision:       int64(revision),
		SchemaVersion:  schemaVersion,
		Snapshot:       document,
		SourceRevision: int64(sourceRevision),
		CreatedBy:      createdBy,
		CreatedAt:      createdAt,
	}
}

// GetBoardRole reports the explicit board role (creator counts as owner).
func (s *PostgresWorkspaceBoardStore) GetBoardRole(ctx context.Context, workspacePublicID, boardPublicID string, userID int64) (AccessRole, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	board, err := s.getBoard(ctx, workspacePublicID, boardPublicID)
	if err != nil {
		return "", false, err
	}
	return boardRoleTx(ctx, s.dbWithContext(ctx).q, board, userID)
}

// ListBoardMembers lists explicit grants ordered by user id (the implicit
// owner row is not listed here; handlers prepend it from the board row).
func (s *PostgresWorkspaceBoardStore) ListBoardMembers(ctx context.Context, workspacePublicID, boardPublicID string) ([]*V1BoardMember, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	trimmedID := strings.TrimSpace(boardPublicID)
	if _, err := s.getBoard(ctx, workspacePublicID, trimmedID); err != nil {
		return nil, err
	}
	rows, err := s.dbWithContext(ctx).q.ListWorkspaceBoardMemberships(ctx, dbsqlc.ListWorkspaceBoardMembershipsParams{
		WorkspacePublicID: workspacePublicID,
		BoardPublicID:     trimmedID,
		RowLimit:          maxWorkspaceBoardRows,
	})
	if err != nil {
		return nil, err
	}
	out := make([]*V1BoardMember, 0, len(rows))
	for _, row := range rows {
		role, ok := mapPostgresBoardRole(row.Role)
		if !ok {
			continue
		}
		out = append(out, &V1BoardMember{
			BoardPublicID: trimmedID,
			UserID:        row.UserID,
			Role:          role,
			CreatedAt:     dbutil.TimeValue(row.CreatedAt),
		})
	}
	return out, nil
}

// AddBoardMember grants a role idempotently: re-adding keeps the existing
// role. Only board owners may grant.
func (s *PostgresWorkspaceBoardStore) AddBoardMember(ctx context.Context, workspacePublicID, boardPublicID string, actorUserID, targetUserID int64, role AccessRole) (*V1BoardMember, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ValidateV1BoardRole(role); err != nil {
		return nil, err
	}
	if actorUserID <= 0 || targetUserID <= 0 {
		return nil, ErrInvalidInput
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	trimmedID := strings.TrimSpace(boardPublicID)
	var added *V1BoardMember
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		board, err := getPostgresBoard(ctx, q, workspacePublicID, trimmedID)
		if err != nil {
			return err
		}
		actorRole, ok, err := boardRoleTx(ctx, q, board, actorUserID)
		if err != nil {
			return err
		}
		if !ok || actorRole != RoleOwner {
			return ErrPermissionDenied
		}
		if targetUserID == board.OwnerUserID {
			return ErrMemberInviteSelf
		}
		if _, err := q.InsertWorkspaceBoardMembershipIgnore(ctx, dbsqlc.InsertWorkspaceBoardMembershipIgnoreParams{
			WorkspacePublicID: workspacePublicID,
			BoardPublicID:     trimmedID,
			UserID:            targetUserID,
			Role:              string(role),
		}); err != nil {
			return err
		}
		raw, err := q.GetWorkspaceBoardMembershipRole(ctx, dbsqlc.GetWorkspaceBoardMembershipRoleParams{
			WorkspacePublicID: workspacePublicID,
			BoardPublicID:     trimmedID,
			UserID:            targetUserID,
		})
		if err != nil {
			if database.IsNotFound(err) {
				return ErrMemberNotFound
			}
			return err
		}
		current, ok := mapPostgresBoardRole(raw)
		if !ok {
			return ErrMemberNotFound
		}
		added = &V1BoardMember{BoardPublicID: board.PublicID, UserID: targetUserID, Role: current}
		// The re-add path keeps the stored role; surface the stored grant
		// timestamp for fidelity with the in-memory copy semantics.
		members, err := q.ListWorkspaceBoardMemberships(ctx, dbsqlc.ListWorkspaceBoardMembershipsParams{
			WorkspacePublicID: workspacePublicID,
			BoardPublicID:     trimmedID,
			RowLimit:          maxWorkspaceBoardRows,
		})
		if err != nil {
			return err
		}
		for _, member := range members {
			if member.UserID == targetUserID {
				added.CreatedAt = dbutil.TimeValue(member.CreatedAt)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return added, nil
}

// UpdateBoardMemberRole applies owner-only transitions with last-owner
// protection.
func (s *PostgresWorkspaceBoardStore) UpdateBoardMemberRole(ctx context.Context, workspacePublicID, boardPublicID string, actorUserID, targetUserID int64, newRole AccessRole) (*V1BoardMember, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ValidateV1BoardRole(newRole); err != nil {
		return nil, err
	}
	if actorUserID <= 0 || targetUserID <= 0 {
		return nil, ErrInvalidInput
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	trimmedID := strings.TrimSpace(boardPublicID)
	var updated *V1BoardMember
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		board, err := getPostgresBoard(ctx, q, workspacePublicID, trimmedID)
		if err != nil {
			return err
		}
		actorRole, ok, err := boardRoleTx(ctx, q, board, actorUserID)
		if err != nil {
			return err
		}
		if !ok || actorRole != RoleOwner {
			return ErrPermissionDenied
		}
		if targetUserID == board.OwnerUserID {
			if newRole != RoleOwner {
				count, err := ownerCountTx(ctx, q, board)
				if err != nil {
					return err
				}
				if count <= 1 {
					return ErrV1LastOwner
				}
				return ErrPermissionDenied
			}
			updated = &V1BoardMember{BoardPublicID: board.PublicID, UserID: targetUserID, Role: RoleOwner}
			return nil
		}
		raw, err := q.GetWorkspaceBoardMembershipRole(ctx, dbsqlc.GetWorkspaceBoardMembershipRoleParams{
			WorkspacePublicID: workspacePublicID,
			BoardPublicID:     trimmedID,
			UserID:            targetUserID,
		})
		if err != nil {
			if database.IsNotFound(err) {
				return ErrMemberNotFound
			}
			return err
		}
		current, ok := mapPostgresBoardRole(raw)
		if !ok {
			return ErrMemberNotFound
		}
		if current == RoleOwner && newRole != RoleOwner {
			count, err := ownerCountTx(ctx, q, board)
			if err != nil {
				return err
			}
			if count <= 1 {
				return ErrV1LastOwner
			}
		}
		affected, err := q.UpdateWorkspaceBoardMembershipRole(ctx, dbsqlc.UpdateWorkspaceBoardMembershipRoleParams{
			Role:              string(newRole),
			WorkspacePublicID: workspacePublicID,
			BoardPublicID:     trimmedID,
			UserID:            targetUserID,
		})
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrMemberNotFound
		}
		updated = &V1BoardMember{BoardPublicID: board.PublicID, UserID: targetUserID, Role: newRole}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// RemoveBoardMember removes a grant: owners may remove anyone, anyone may
// remove themselves. The last owner is protected.
func (s *PostgresWorkspaceBoardStore) RemoveBoardMember(ctx context.Context, workspacePublicID, boardPublicID string, actorUserID, targetUserID int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if actorUserID <= 0 || targetUserID <= 0 {
		return ErrInvalidInput
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	trimmedID := strings.TrimSpace(boardPublicID)
	return s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		board, err := getPostgresBoard(ctx, q, workspacePublicID, trimmedID)
		if err != nil {
			return err
		}
		if targetUserID == board.OwnerUserID {
			return ErrMemberInviteSelf
		}
		raw, err := q.GetWorkspaceBoardMembershipRole(ctx, dbsqlc.GetWorkspaceBoardMembershipRoleParams{
			WorkspacePublicID: workspacePublicID,
			BoardPublicID:     trimmedID,
			UserID:            targetUserID,
		})
		if err != nil {
			if database.IsNotFound(err) {
				return ErrMemberNotFound
			}
			return err
		}
		current, ok := mapPostgresBoardRole(raw)
		if !ok {
			return ErrMemberNotFound
		}
		if current == RoleOwner {
			count, err := ownerCountTx(ctx, q, board)
			if err != nil {
				return err
			}
			if count <= 1 {
				return ErrV1LastOwner
			}
		}
		if actorUserID != targetUserID {
			actorRole, ok, err := boardRoleTx(ctx, q, board, actorUserID)
			if err != nil {
				return err
			}
			if !ok || actorRole != RoleOwner {
				return ErrPermissionDenied
			}
		}
		affected, err := q.DeleteWorkspaceBoardMembership(ctx, dbsqlc.DeleteWorkspaceBoardMembershipParams{
			WorkspacePublicID: workspacePublicID,
			BoardPublicID:     trimmedID,
			UserID:            targetUserID,
		})
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrMemberNotFound
		}
		return nil
	})
}

func mapPostgresShareToken(boardPublicID, tokenID, tokenHash, tokenPrefix string, role AccessRole, createdBy int64, revoked bool, usageCount int64, createdAt time.Time, revokedAt *time.Time, revokedBy *int64) *V1ShareToken {
	return &V1ShareToken{
		TokenID:       tokenID,
		BoardPublicID: boardPublicID,
		TokenHash:     tokenHash,
		TokenPrefix:   tokenPrefix,
		Role:          role,
		CreatedBy:     createdBy,
		Revoked:       revoked,
		UsageCount:    usageCount,
		RevokedAt:     revokedAt,
		RevokedBy:     revokedBy,
		CreatedAt:     createdAt,
	}
}

// CreateShareToken stores one bearer hash (the bearer itself is never
// persisted). Hash uniqueness rides the token_secret unique index;
// collisions return ErrIDConflict so callers mint again.
func (s *PostgresWorkspaceBoardStore) CreateShareToken(ctx context.Context, workspacePublicID, boardPublicID string, creatorUserID int64, role AccessRole, tokenID, tokenHash, bearer string) (*V1ShareToken, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if role != RoleEditor && role != RoleViewer {
		return nil, ErrMemberInvalidRole
	}
	trimmedTokenID := strings.TrimSpace(tokenID)
	if !IsV1ShareTokenID(trimmedTokenID) || strings.TrimSpace(tokenHash) == "" || strings.TrimSpace(bearer) == "" {
		return nil, ErrInvalidInput
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	trimmedID := strings.TrimSpace(boardPublicID)
	var created *V1ShareToken
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		board, err := getPostgresBoard(ctx, q, workspacePublicID, trimmedID)
		if err != nil {
			return err
		}
		rowPublicID, err := contract.MintPublicID(postgresShareRowPrefix, postgresShareRowIDBytes)
		if err != nil {
			return err
		}
		row, err := q.InsertBoardShareToken(ctx, dbsqlc.InsertBoardShareTokenParams{
			PublicID:          rowPublicID,
			WorkspacePublicID: workspacePublicID,
			BoardPublicID:     trimmedID,
			TokenID:           dbutil.NullString(&trimmedTokenID),
			TokenSecret:       strings.TrimSpace(tokenHash),
			TokenPrefix:       V1ShareTokenPrefixOf(bearer),
			GrantedRole:       string(role),
			CreatedByUserID:   creatorUserID,
		})
		if err != nil {
			if database.IsDuplicateConstraintError(err) {
				return ErrIDConflict
			}
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return err
		}
		created = mapPostgresShareToken(board.PublicID, trimmedTokenID, strings.TrimSpace(tokenHash), V1ShareTokenPrefixOf(bearer), role, creatorUserID, false, 0, dbutil.TimeValue(row.CreatedAt), nil, nil)
		created.Bearer = strings.TrimSpace(bearer)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// ListShareTokens lists bearers newest-first without secrets.
func (s *PostgresWorkspaceBoardStore) ListShareTokens(ctx context.Context, workspacePublicID, boardPublicID string) ([]*V1ShareToken, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	trimmedID := strings.TrimSpace(boardPublicID)
	if _, err := s.getBoard(ctx, workspacePublicID, trimmedID); err != nil {
		return nil, err
	}
	rows, err := s.dbWithContext(ctx).q.ListBoardShareTokens(ctx, dbsqlc.ListBoardShareTokensParams{
		WorkspacePublicID: workspacePublicID,
		BoardPublicID:     trimmedID,
		RowLimit:          maxWorkspaceBoardRows,
	})
	if err != nil {
		return nil, err
	}
	out := make([]*V1ShareToken, 0, len(rows))
	for _, row := range rows {
		role, ok := mapPostgresBoardRole(row.GrantedRole)
		if !ok {
			continue
		}
		tokenID := ""
		if row.TokenID.Valid {
			tokenID = row.TokenID.String
		}
		out = append(out, mapPostgresShareToken(
			trimmedID,
			tokenID,
			row.TokenSecret,
			row.TokenPrefix,
			role,
			row.CreatedByUserID,
			row.RevokedAt.Valid,
			row.UsageCount,
			dbutil.TimeValue(row.CreatedAt),
			dbutil.TimePtr(row.RevokedAt),
			dbutil.Int64Ptr(row.RevokedByUserID),
		))
	}
	return out, nil
}

// RevokeShareToken revokes a bearer (idempotent: revoking twice succeeds).
func (s *PostgresWorkspaceBoardStore) RevokeShareToken(ctx context.Context, workspacePublicID, boardPublicID, tokenID string, revokedBy int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	trimmedID := strings.TrimSpace(boardPublicID)
	trimmedTokenID := strings.TrimSpace(tokenID)
	if _, err := s.getBoard(ctx, workspacePublicID, trimmedID); err != nil {
		return err
	}
	affected, err := s.dbWithContext(ctx).q.RevokeBoardShareToken(ctx, dbsqlc.RevokeBoardShareTokenParams{
		RevokedByUserID:   dbutil.NullInt64(&revokedBy),
		WorkspacePublicID: workspacePublicID,
		BoardPublicID:     trimmedID,
		TokenID:           dbutil.NullString(&trimmedTokenID),
	})
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrShareTokenNotFound
	}
	return nil
}

// JoinByToken consumes a bearer idempotently: repeat joins by the same user
// converge to the token role; the usage counter still advances. Unknown,
// revoked, and foreign-workspace bearers all read as ErrShareTokenNotFound
// so existence is never leaked.
func (s *PostgresWorkspaceBoardStore) JoinByToken(ctx context.Context, workspacePublicID, bearer string, userID int64) (*V1Board, AccessRole, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if userID <= 0 || strings.TrimSpace(bearer) == "" {
		return nil, "", ErrInvalidInput
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	var board *V1Board
	var role AccessRole
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		token, err := q.GetBoardShareTokenBySecret(ctx, dbsqlc.GetBoardShareTokenBySecretParams{
			WorkspacePublicID: workspacePublicID,
			TokenSecret:       V1ShareTokenHash(bearer),
		})
		if err != nil {
			if database.IsNotFound(err) {
				return ErrShareTokenNotFound
			}
			return err
		}
		if token.RevokedAt.Valid {
			return ErrShareTokenRevoked
		}
		granted, ok := mapPostgresBoardRole(token.GrantedRole)
		if !ok || (granted != RoleEditor && granted != RoleViewer) {
			return ErrShareTokenInvalid
		}
		if token.BoardArchivedAt.Valid {
			return ErrV1Archived
		}
		if !token.BoardOwnerUserID.Valid {
			return ErrNotFound
		}
		if err := q.IncrementBoardShareTokenUsage(ctx, token.InternalID); err != nil {
			return err
		}
		current, err := getPostgresBoard(ctx, q, workspacePublicID, token.BoardPublicID)
		if err != nil {
			return err
		}
		if current.OwnerUserID == userID {
			board, role = current, RoleOwner
			return nil
		}
		if err := q.UpsertJoinBoardMembership(ctx, dbsqlc.UpsertJoinBoardMembershipParams{
			BoardID: token.BoardInternalID,
			UserID:  userID,
			Role:    string(granted),
		}); err != nil {
			return err
		}
		board, role = current, granted
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return board, role, nil
}
