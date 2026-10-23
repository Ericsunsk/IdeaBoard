package boards

// Postgres store-board writes: board lifecycle, document acceptance support,
// and shared row-mapping helpers. Access policy, revisions, and catalog
// cards live in postgres_access.go, postgres_revision.go, and
// postgres_catalog.go; this file owns the store struct and the board CRUD
// transaction bodies.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"ideaboard/api/internal/boards/previewscene"
	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/tenancy"

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
		open, err := organizationWorkspaceOpenTx(ctx, q, workspace)
		if err != nil {
			return err
		}
		if !open {
			return ErrNotFound
		}
		member, err := q.GetWorkspaceMembership(ctx, dbsqlc.GetWorkspaceMembershipParams{WorkspaceID: workspace.InternalID, UserID: creatorUserID})
		if err == nil {
			if !IsWorkspaceMemberRole(member.Role) {
				return ErrNotFound
			}
		} else if database.IsNotFound(err) {
			if !workspace.OrganizationID.Valid {
				return ErrNotFound
			}
			orgMember, orgErr := q.GetOrganizationMembership(ctx, dbsqlc.GetOrganizationMembershipParams{
				OrganizationID: workspace.OrganizationID.Int64, UserID: creatorUserID,
			})
			if database.IsNotFound(orgErr) {
				return ErrNotFound
			}
			if orgErr != nil {
				return orgErr
			}
			if orgMember.Role != tenancy.RoleOwner && orgMember.Role != tenancy.RoleAdmin {
				return ErrNotFound
			}
		} else if err != nil {
			return err
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

// GetBoard reads one board (archived rows stay visible here; callers decide
// whether the operation is allowed on them).
func (s *PostgresWorkspaceBoardStore) GetBoard(ctx context.Context, workspacePublicID, boardPublicID string) (*V1Board, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.getBoard(ctx, workspacePublicID, boardPublicID)
}

// GetBoardByPublicID resolves one board by its globally unique public id,
// returning the workspace-rooted fact (WorkspacePublicID filled) for the
// realtime admit path, which carries only the ticket bid. Unknown ids read
// as ErrNotFound; archived rows stay visible here (admit gates them).
func (s *PostgresWorkspaceBoardStore) GetBoardByPublicID(ctx context.Context, boardPublicID string) (*V1Board, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(boardPublicID)
	if !contract.IsBoardID(trimmed) {
		return nil, ErrNotFound
	}
	row, err := s.dbWithContext(ctx).q.GetBoardByPublicID(ctx, trimmed)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return mapPostgresBoardRow(dbsqlc.GetWorkspaceBoardRow(row))
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
