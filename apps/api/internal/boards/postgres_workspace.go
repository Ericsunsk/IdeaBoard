package boards

// Board writes lock the Workspace membership row before Board rows and re-read
// current membership in the accepting transaction. Document replacement accepts
// authorized Media references, the immutable revision, document and Preview Scene
// together. Access changes advance only access_version.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"ideaboard/api/internal/boards/previewscene"
	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	eventoutbox "ideaboard/api/internal/events/outbox"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/tenancy"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Local mint prefixes (not kernel prefixes yet). They match the
// board_revisions (rev_%) public_id CHECK, so minted ids pass through
// opaquely.
const (
	postgresRevisionPrefix  = "rev_"
	maxWorkspaceBoardRows   = int32(500)
	postgresRevisionIDBytes = 12
)

// DocumentMediaAccess exposes the Board facts used to accept Media references.
type DocumentMediaAccess interface {
	RoleReader
	BoardMediaRefReader
}

// DocumentAssetValidator validates references against the accepting transaction.
type DocumentAssetValidator func(ctx context.Context, q *dbsqlc.Queries, workspacePublicID string, actorUserID int64, access DocumentMediaAccess, assetIDs []string) error

// PostgresWorkspaceBoardStore is the postgres WorkspaceBoardStore. It must
// never be replaced by the test-only InMemoryWorkspaceBoardStore in
// production wiring.
type PostgresWorkspaceBoardStore struct {
	validateDocumentAssets DocumentAssetValidator
	db                     *pgxpool.Pool
	q                      *dbsqlc.Queries
}

// Compile-time convergence: the postgres store implements the seam.
var _ WorkspaceBoardStore = (*PostgresWorkspaceBoardStore)(nil)

type postgresBoardQueryer struct {
	ctx context.Context
	db  *pgxpool.Pool
	q   *dbsqlc.Queries
}

// NewPostgresWorkspaceBoardStore builds a postgres WorkspaceBoardStore over pool.
func NewPostgresWorkspaceBoardStore(db *pgxpool.Pool, validateDocumentAssets DocumentAssetValidator) *PostgresWorkspaceBoardStore {
	return &PostgresWorkspaceBoardStore{db: db, q: dbsqlc.New(db), validateDocumentAssets: validateDocumentAssets}
}

func (s *PostgresWorkspaceBoardStore) dbWithContext(ctx context.Context) postgresBoardQueryer {
	q := s.q
	if tx, ok := dbutil.AmbientTxFromContext(ctx); ok {
		q = dbsqlc.New(tx)
	}
	return postgresBoardQueryer{ctx: ctx, db: s.db, q: q}
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
	role := AccessRole(raw)
	switch role {
	case RoleOwner, RoleEditor, RoleViewer:
		return role, true
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
	board := &V1Board{
		WorkspacePublicID: row.WorkspacePublicID,
		PublicID:          row.PublicID,
		Name:              row.Title,
		Revision:          int64(row.DocumentVersion),
		SchemaVersion:     row.SchemaVersion,
		Document:          document,
		ArchivedAt:        dbutil.TimePtr(row.ArchivedAt),
		OwnerUserID:       row.OwnerUserID.Int64,
		CreatedBy:         row.OwnerUserID.Int64,
		WorkspaceAccess:   WorkspaceAccess(strings.TrimSpace(row.WorkspaceAccess)),
		AccessVersion:     row.AccessVersion,
		CreatedAt:         dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:         dbutil.TimeValue(row.UpdatedAt),
	}
	if row.ContentVersion.Valid {
		board.ContentVersion = row.ContentVersion.String
	}
	if len(row.PreviewScene) > 0 {
		scene := make(json.RawMessage, len(row.PreviewScene))
		copy(scene, row.PreviewScene)
		board.PreviewScene = scene
	}
	return board, nil
}

func mapPostgresBoardCatalogRow(row dbsqlc.ListWorkspaceBoardCatalogRow) (*V1BoardCatalogEntry, error) {
	if !row.OwnerUserID.Valid {
		// Owner-less rows predate the owner column: fail closed.
		return nil, ErrNotFound
	}
	role, ok := mapPostgresBoardRole(row.EffectiveRole.String)
	if !ok {
		return nil, ErrNotFound
	}
	hasDirectGrant, _ := row.HasDirectGrant.(bool)
	entry := &V1BoardCatalogEntry{
		WorkspacePublicID: row.WorkspacePublicID.String,
		PublicID:          row.PublicID.String,
		Name:              row.Title.String,
		Revision:          int64(row.DocumentVersion.Int32),
		SchemaVersion:     row.SchemaVersion.Int32,
		ArchivedAt:        dbutil.TimePtr(row.ArchivedAt),
		OwnerUserID:       row.OwnerUserID.Int64,
		CreatedBy:         row.OwnerUserID.Int64,
		WorkspaceAccess:   WorkspaceAccess(strings.TrimSpace(row.WorkspaceAccess.String)),
		AccessVersion:     row.AccessVersion.Int64,
		EffectiveRole:     role,
		HasDirectGrant:    hasDirectGrant,
		AccessState:       DeriveAccessState(WorkspaceAccess(strings.TrimSpace(row.WorkspaceAccess.String)), row.HasAnyGrants),
		CreatedAt:         dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:         dbutil.TimeValue(row.UpdatedAt),
	}
	if row.ContentVersion.Valid {
		entry.ContentVersion = row.ContentVersion.String
	}
	if len(row.PreviewScene) > 0 {
		scene := make(json.RawMessage, len(row.PreviewScene))
		copy(scene, row.PreviewScene)
		entry.PreviewScene = scene
	}
	return entry, nil
}

// previewDocumentColumns derives the persisted projection inputs for
// accepted document bytes: always the content version, plus the scene
// when the document schema is extractable (nil otherwise, without
// failing the write).
func previewDocumentColumns(document json.RawMessage) (pgtype.Text, []byte) {
	version, scene, _ := previewscene.DeriveCatalog(document)
	return pgtype.Text{String: version, Valid: version != ""}, []byte(scene)
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

// boardRoleTx resolves current membership and selected members through ResolveRole.
func boardRoleTx(ctx context.Context, q *dbsqlc.Queries, board *V1Board, userID int64) (AccessRole, bool, error) {
	workspace, err := q.GetWorkspaceByPublicID(ctx, board.WorkspacePublicID)
	if err != nil {
		return "", false, err
	}
	membership, err := q.GetWorkspaceMembership(ctx, dbsqlc.GetWorkspaceMembershipParams{WorkspaceID: workspace.InternalID, UserID: userID})
	if database.IsNotFound(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !IsWorkspaceMemberRole(membership.Role) {
		return "", false, nil
	}
	_, err = q.GetWorkspaceBoardMembership(ctx, dbsqlc.GetWorkspaceBoardMembershipParams{WorkspacePublicID: board.WorkspacePublicID, BoardPublicID: board.PublicID, UserID: userID})
	if err != nil && !database.IsNotFound(err) {
		return "", false, err
	}
	role, visible := ResolveRole(board.OwnerUserID, userID, err == nil, board.WorkspaceAccess, true)
	return role, visible, nil
}

func requireBoardWriteTx(ctx context.Context, q *dbsqlc.Queries, board *V1Board, actorUserID int64) error {
	role, visible, err := boardRoleTx(ctx, q, board, actorUserID)
	if err != nil {
		return err
	}
	return CheckCapability(role, visible, CapWrite)
}

func lockWorkspaceMembersTx(ctx context.Context, q *dbsqlc.Queries, workspacePublicID string) error {
	_, err := q.LockWorkspaceMemberships(ctx, workspacePublicID)
	if database.IsNotFound(err) {
		return ErrNotFound
	}
	return err
}

func lockWritableBoardTx(ctx context.Context, q *dbsqlc.Queries, workspacePublicID, boardPublicID string, actorUserID int64) (*V1Board, error) {
	if err := lockWorkspaceMembersTx(ctx, q, workspacePublicID); err != nil {
		return nil, err
	}
	if _, err := q.LockWorkspaceBoardAccess(ctx, dbsqlc.LockWorkspaceBoardAccessParams{
		WorkspacePublicID: workspacePublicID,
		BoardPublicID:     boardPublicID,
	}); err != nil {
		if database.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	board, err := getPostgresBoard(ctx, q, workspacePublicID, boardPublicID)
	if err != nil {
		return nil, err
	}
	if err := requireBoardWriteTx(ctx, q, board, actorUserID); err != nil {
		return nil, err
	}
	return board, nil
}

// CreateBoard mints a workspace-rooted board at revision 1 with the empty
// discriminated-union snapshot; the creator is the implicit owner. The
// sharing role is fixed to none by the database default: creation
// requests cannot widen it.
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
		if err := lockWorkspaceMembersTx(ctx, q, workspacePublicID); err != nil {
			return err
		}
		workspace, err := q.GetWorkspaceByPublicID(ctx, workspacePublicID)
		if err != nil {
			return err
		}
		member, err := q.GetWorkspaceMembership(ctx, dbsqlc.GetWorkspaceMembershipParams{WorkspaceID: workspace.InternalID, UserID: creatorUserID})
		if database.IsNotFound(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !IsWorkspaceMemberRole(member.Role) {
			return ErrNotFound
		}

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
		contentVersion, previewScene := previewDocumentColumns(document)
		if err := q.InsertBoardDocument(ctx, dbsqlc.InsertBoardDocumentParams{
			BoardID:         row.InternalID,
			DocumentVersion: 1,
			SnapshotData:    []byte(document),
			SchemaVersion:   v1InitialSchemaVerion,
			Etag:            boardDocumentETag(1, document),
			ContentVersion:  contentVersion,
			PreviewScene:    previewScene,
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

// ListBoardCatalog reads only the requested authorized cards and a bounded
// total count, without loading full documents or out-of-page scene bodies.
func (s *PostgresWorkspaceBoardStore) ListBoardCatalog(ctx context.Context, workspacePublicID string, includeArchived bool, userID int64, workspaceRole string, page V1BoardCatalogPageRequest) (V1BoardCatalogPage, error) {
	if err := ctx.Err(); err != nil {
		return V1BoardCatalogPage{}, err
	}
	if err := page.validate(); err != nil {
		return V1BoardCatalogPage{}, err
	}
	rows, err := s.dbWithContext(ctx).q.ListWorkspaceBoardCatalog(ctx, dbsqlc.ListWorkspaceBoardCatalogParams{
		WorkspacePublicID: strings.TrimSpace(workspacePublicID),
		IncludeArchived:   includeArchived,
		IsWorkspaceMember: IsWorkspaceMemberRole(workspaceRole),
		CatalogLimit:      maxBoardCatalogRows,
		RowOffset:         int32(page.Offset),
		RowLimit:          int32(page.Limit),
		UserID:            dbutil.NullInt64(&userID),
	})
	if err != nil {
		return V1BoardCatalogPage{}, err
	}
	out := V1BoardCatalogPage{Entries: make([]*V1BoardCatalogEntry, 0, len(rows))}
	for _, row := range rows {
		if int64(page.Offset) > row.TotalCount {
			return V1BoardCatalogPage{}, ErrV1CatalogOffset
		}
		out.HasMore = int64(page.Offset+page.Limit) < row.TotalCount
		if !row.InternalID.Valid {
			continue
		}
		entry, err := mapPostgresBoardCatalogRow(row)
		if err != nil {
			return V1BoardCatalogPage{}, err
		}
		out.Entries = append(out.Entries, entry)
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
func (s *PostgresWorkspaceBoardStore) RenameBoard(ctx context.Context, workspacePublicID, boardPublicID, name string, actorUserID int64) (*V1Board, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ValidateV1BoardName(name); err != nil {
		return nil, err
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	trimmedID := strings.TrimSpace(boardPublicID)
	var updated *V1Board
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		if _, err := lockWritableBoardTx(ctx, q, workspacePublicID, trimmedID, actorUserID); err != nil {
			return err
		}
		affected, err := q.RenameWorkspaceBoard(ctx, dbsqlc.RenameWorkspaceBoardParams{
			Title: strings.TrimSpace(name), WorkspacePublicID: workspacePublicID, BoardPublicID: trimmedID,
		})
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrV1Archived
		}
		updated, err = getPostgresBoard(ctx, q, workspacePublicID, trimmedID)
		return err
	})
	return updated, err
}

// ArchiveBoard archives a live board; already-archived rows read as not
// found so archive stays idempotent-safe at the handler layer.
func (s *PostgresWorkspaceBoardStore) ArchiveBoard(ctx context.Context, workspacePublicID, boardPublicID string, actorUserID int64) (*V1Board, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	trimmedID := strings.TrimSpace(boardPublicID)
	var updated *V1Board
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		if _, err := lockWritableBoardTx(ctx, q, workspacePublicID, trimmedID, actorUserID); err != nil {
			return err
		}
		affected, err := q.ArchiveWorkspaceBoard(ctx, dbsqlc.ArchiveWorkspaceBoardParams{
			WorkspacePublicID: workspacePublicID, BoardPublicID: trimmedID,
		})
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrNotFound
		}
		updated, err = getPostgresBoard(ctx, q, workspacePublicID, trimmedID)
		return err
	})
	return updated, err
}

// RestoreBoard clears the archive timestamp; live boards read as not found.
func (s *PostgresWorkspaceBoardStore) RestoreBoard(ctx context.Context, workspacePublicID, boardPublicID string, actorUserID int64) (*V1Board, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	trimmedID := strings.TrimSpace(boardPublicID)
	var updated *V1Board
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		if _, err := lockWritableBoardTx(ctx, q, workspacePublicID, trimmedID, actorUserID); err != nil {
			return err
		}
		affected, err := q.RestoreWorkspaceBoard(ctx, dbsqlc.RestoreWorkspaceBoardParams{
			WorkspacePublicID: workspacePublicID, BoardPublicID: trimmedID,
		})
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrNotFound
		}
		updated, err = getPostgresBoard(ctx, q, workspacePublicID, trimmedID)
		return err
	})
	return updated, err
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
		if _, err := lockWritableBoardTx(ctx, q, workspacePublicID, trimmedID, actorUserID); err != nil {
			return err
		}
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
		assetIDs := previewscene.ExtractMediaAssetIDs(snapshot)
		if len(assetIDs) > 0 {
			if s.validateDocumentAssets == nil {
				return ErrPermissionDenied
			}
			if err := s.validateDocumentAssets(ctx, q, workspacePublicID, actorUserID, &PostgresWorkspaceBoardStore{q: q}, assetIDs); err != nil {
				return err
			}
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
		// Document accept, content identity, and projection publish in
		// the SAME row update: 409/conflict returns above commit nothing.
		contentVersion, previewScene := previewDocumentColumns(document)
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
			SchemaVersion:   schemaVersion,
			Etag:            boardDocumentETag(nextRevision, document),
			ContentVersion:  contentVersion,
			PreviewScene:    previewScene,
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
		// #936: reconcile the accepted document -> media references in the
		// SAME accepting transaction. Removed ids stop authorizing here
		// while upload-target, owner, library, task, and other-board
		// sources stay untouched. Unknown asset shapes never match a row
		// (the upsert joins on the workspace-scoped asset), so forged or
		// foreign ids grant nothing.
		if err := q.DeleteBoardDocumentMediaRefs(ctx, dbsqlc.DeleteBoardDocumentMediaRefsParams{
			WorkspacePublicID: workspacePublicID,
			BoardPublicID:     trimmedID,
		}); err != nil {
			return err
		}
		for _, assetPublicID := range assetIDs {
			if err := q.UpsertBoardDocumentMediaRef(ctx, dbsqlc.UpsertBoardDocumentMediaRefParams{
				WorkspacePublicID: workspacePublicID,
				BoardPublicID:     trimmedID,
				AssetPublicID:     assetPublicID,
			}); err != nil {
				return err
			}
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

// GetBoardRole reports the effective board role through the unified policy
// (creator counts as owner; every non-creator grant needs valid workspace
// membership; selected members receive the common role; Workspace Owner / Admin hold no implicit access).
func (s *PostgresWorkspaceBoardStore) GetBoardRole(ctx context.Context, workspacePublicID, boardPublicID string, userID int64, workspaceRole string) (AccessRole, bool, error) {
	if !IsWorkspaceMemberRole(workspaceRole) {
		return "", false, nil
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	board, err := s.getBoard(ctx, workspacePublicID, boardPublicID)
	if err != nil {
		return "", false, err
	}
	return boardRoleTx(ctx, s.dbWithContext(ctx).q, board, userID)
}

// ListBoardsReferencingAsset serves the narrow BoardMediaRefReader port
// (#936): the workspace-scoped boards whose accepted document references
// one media asset. Public ids only; unknown or cross-workspace assets list
// empty (fail-closed, never a distinct status).
func (s *PostgresWorkspaceBoardStore) ListBoardsReferencingAsset(ctx context.Context, workspacePublicID, assetPublicID string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	trimmedAsset := strings.TrimSpace(assetPublicID)
	if !contract.IsAssetID(trimmedAsset) {
		return nil, nil
	}
	rows, err := s.dbWithContext(ctx).q.ListBoardsReferencingMediaAsset(ctx, dbsqlc.ListBoardsReferencingMediaAssetParams{
		WorkspacePublicID: strings.TrimSpace(workspacePublicID),
		PublicID:          trimmedAsset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if trimmed := strings.TrimSpace(row); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out, nil
}

// ListBoardMembers lists selected members ordered by user id.
// The Creator has implicit access and does not need a selection row.
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
		out = append(out, &V1BoardMember{
			BoardPublicID: trimmedID,
			UserID:        row.UserID,
			CreatedAt:     dbutil.TimeValue(row.CreatedAt),
		})
	}
	return out, nil
}

// readBoardAccessTx reads the common role and selected identities together.
func readBoardAccessTx(ctx context.Context, q *dbsqlc.Queries, workspacePublicID, boardPublicID string) (*V1BoardAccess, error) {
	row, err := q.GetWorkspaceBoardAccess(ctx, dbsqlc.GetWorkspaceBoardAccessParams{WorkspacePublicID: workspacePublicID, BoardPublicID: boardPublicID})
	if database.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !row.OwnerUserID.Valid {
		return nil, ErrNotFound
	}
	members, err := q.ListWorkspaceBoardMemberships(ctx, dbsqlc.ListWorkspaceBoardMembershipsParams{WorkspacePublicID: workspacePublicID, BoardPublicID: boardPublicID, RowLimit: maxWorkspaceBoardRows})
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.UserID)
	}
	return &V1BoardAccess{WorkspacePublicID: workspacePublicID, BoardPublicID: boardPublicID, WorkspaceAccess: WorkspaceAccess(row.WorkspaceAccess), AccessVersion: row.AccessVersion, AccessState: DeriveAccessState(WorkspaceAccess(row.WorkspaceAccess), len(ids) > 0), MemberUserIDs: ids, ArchivedAt: dbutil.TimePtr(row.ArchivedAt)}, nil
}

func (s *PostgresWorkspaceBoardStore) GetBoardAccess(ctx context.Context, workspacePublicID, boardPublicID string) (*V1BoardAccess, error) {
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	boardPublicID = strings.TrimSpace(boardPublicID)
	var out *V1BoardAccess
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		if err := lockWorkspaceMembersTx(ctx, q, workspacePublicID); err != nil {
			return err
		}
		var err error
		out, err = readBoardAccessTx(ctx, q, workspacePublicID, boardPublicID)
		return err
	})
	return out, err
}

// ReplaceBoardAccess accepts the complete selected-member policy in one transaction.
func (s *PostgresWorkspaceBoardStore) ReplaceBoardAccess(ctx context.Context, workspacePublicID, boardPublicID string, actorUserID, expectedVersion int64, access WorkspaceAccess, selectedUserIDs []int64) (*V1BoardAccess, error) {
	if !ValidateWorkspaceAccess(access) || actorUserID <= 0 || expectedVersion <= 0 {
		return nil, ErrInvalidInput
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	boardPublicID = strings.TrimSpace(boardPublicID)
	var out *V1BoardAccess
	err := dbutil.WithTxDBTX(ctx, s.db, func(q *dbsqlc.Queries, tx pgx.Tx) error {
		if err := lockWorkspaceMembersTx(ctx, q, workspacePublicID); err != nil {
			return err
		}
		locked, err := q.LockWorkspaceBoardAccess(ctx, dbsqlc.LockWorkspaceBoardAccessParams{WorkspacePublicID: workspacePublicID, BoardPublicID: boardPublicID})
		if database.IsNotFound(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !locked.OwnerUserID.Valid {
			return ErrNotFound
		}
		board := &V1Board{WorkspacePublicID: workspacePublicID, PublicID: boardPublicID, OwnerUserID: locked.OwnerUserID.Int64, WorkspaceAccess: WorkspaceAccess(locked.WorkspaceAccess), ArchivedAt: dbutil.TimePtr(locked.ArchivedAt)}
		role, visible, err := boardRoleTx(ctx, q, board, actorUserID)
		if err != nil {
			return err
		}
		if err := CheckCapability(role, visible, CapManage); err != nil {
			return err
		}
		if board.ArchivedAt != nil {
			return ErrV1Archived
		}
		if locked.AccessVersion != expectedVersion {
			return ErrAccessConflict
		}
		ids, err := selectedBoardMembers(access, selectedUserIDs, board.OwnerUserID)
		if err != nil {
			return err
		}
		workspace, err := q.GetWorkspaceByPublicID(ctx, workspacePublicID)
		if err != nil {
			return err
		}
		for _, id := range ids {
			member, err := q.GetWorkspaceMembership(ctx, dbsqlc.GetWorkspaceMembershipParams{WorkspaceID: workspace.InternalID, UserID: id})
			if database.IsNotFound(err) {
				return ErrMemberNotFound
			}
			if err != nil {
				return err
			}
			if !IsWorkspaceMemberRole(member.Role) {
				return ErrMemberNotFound
			}
		}
		before, err := readBoardAccessTx(ctx, q, workspacePublicID, boardPublicID)
		if err != nil {
			return err
		}
		if before.WorkspaceAccess == access && equalMemberIDs(before.MemberUserIDs, ids) {
			out = before
			return nil
		}
		if _, err := q.ClearBoardMemberships(ctx, dbsqlc.ClearBoardMembershipsParams{WorkspacePublicID: workspacePublicID, BoardPublicID: boardPublicID}); err != nil {
			return err
		}
		for _, id := range ids {
			if _, err := q.InsertWorkspaceBoardMembershipIgnore(ctx, dbsqlc.InsertWorkspaceBoardMembershipIgnoreParams{WorkspacePublicID: workspacePublicID, BoardPublicID: boardPublicID, UserID: id}); err != nil {
				return err
			}
		}
		affected, err := q.SetBoardWorkspaceAccess(ctx, dbsqlc.SetBoardWorkspaceAccessParams{WorkspacePublicID: workspacePublicID, BoardPublicID: boardPublicID, WorkspaceAccess: string(access), ExpectedVersion: expectedVersion})
		if err != nil {
			return err
		}
		if affected != 1 {
			return ErrAccessConflict
		}
		out, err = readBoardAccessTx(ctx, q, workspacePublicID, boardPublicID)
		if err != nil {
			return err
		}
		ordered := changedBoardMembers(before.MemberUserIDs, ids, board.OwnerUserID)
		occurredAt := time.Now().UTC()
		for _, id := range ordered {
			user, err := q.GetUserByID(ctx, id)
			if err != nil {
				return err
			}
			record := eventoutbox.NewBoardAccessOutboxRecord(workspacePublicID, boardPublicID, tenancy.FormatUserPublicID(uuid.UUID(user.PublicID.Bytes)), out.AccessVersion, occurredAt)
			if err := eventoutbox.NewBoardAccessOutboxStore(s.db).InsertTx(ctx, tx, record); err != nil {
				return err
			}
			out.AccessSignals = append(out.AccessSignals, record)
		}
		return nil
	})
	return out, err
}

// RemoveWorkspaceMemberGrants participates in the caller's membership transaction.
// Workspace membership is locked before Board rows, which lock in public_id order.
func (s *PostgresWorkspaceBoardStore) RemoveWorkspaceMemberGrants(ctx context.Context, workspacePublicID string, userID int64) ([]tenancy.MemberGrantCleanup, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if userID <= 0 {
		return nil, ErrInvalidInput
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	if workspacePublicID == "" {
		return nil, ErrInvalidInput
	}
	tx, ok := dbutil.AmbientTxFromContext(ctx)
	if !ok {
		return nil, ErrPermissionDenied
	}

	var out []tenancy.MemberGrantCleanup
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		grants, err := q.ListWorkspaceMemberBoardGrants(ctx, dbsqlc.ListWorkspaceMemberBoardGrantsParams{
			WorkspacePublicID: workspacePublicID,
			UserID:            userID,
		})
		if err != nil {
			return err
		}
		if len(grants) == 0 {
			return nil
		}
		ids := make([]string, 0, len(grants))
		for _, grant := range grants {
			ids = append(ids, grant.PublicID)
		}
		locked, err := q.LockWorkspaceMemberBoards(ctx, dbsqlc.LockWorkspaceMemberBoardsParams{
			WorkspacePublicID: workspacePublicID,
			BoardPublicIds:    ids,
		})
		if err != nil {
			return err
		}
		if _, err := q.DeleteWorkspaceMemberBoardGrants(ctx, dbsqlc.DeleteWorkspaceMemberBoardGrantsParams{
			WorkspacePublicID: workspacePublicID,
			UserID:            userID,
		}); err != nil {
			return err
		}
		bumped, err := q.BumpWorkspaceMemberBoardsAccess(ctx, dbsqlc.BumpWorkspaceMemberBoardsAccessParams{
			WorkspacePublicID: workspacePublicID,
			BoardPublicIds:    ids,
		})
		if err != nil {
			return err
		}
		bumpedByID := make(map[string]int64, len(bumped))
		for _, row := range bumped {
			bumpedByID[row.PublicID] = row.AccessVersion
		}
		out = make([]tenancy.MemberGrantCleanup, 0, len(locked))
		occurredAt := time.Now().UTC()
		for _, row := range locked {
			cleanup := tenancy.MemberGrantCleanup{BoardPublicID: row.PublicID, AccessVersion: bumpedByID[row.PublicID]}
			for _, recipient := range changedBoardMembers(nil, []int64{userID}, row.OwnerUserID.Int64) {
				user, err := q.GetUserByID(ctx, recipient)
				if err != nil {
					return err
				}
				publicID := tenancy.FormatUserPublicID(uuid.UUID(user.PublicID.Bytes))
				record := eventoutbox.NewBoardAccessOutboxRecord(workspacePublicID, row.PublicID, publicID, cleanup.AccessVersion, occurredAt)
				if err := eventoutbox.NewBoardAccessOutboxStore(s.db).InsertTx(ctx, tx, record); err != nil {
					return err
				}
				cleanup.AccessSignals = append(cleanup.AccessSignals, record)
			}
			out = append(out, cleanup)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
