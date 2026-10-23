package boards

// Postgres board revisions: immutable document snapshots and the
// document-accept transaction shared with compaction.

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"

	"ideaboard/api/internal/boards/previewscene"
	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/database"
)

// ReplaceDocument appends an immutable revision when the expected revision
// matches the current counter; stale writers get ErrRevisionConflict.
func (s *PostgresWorkspaceBoardStore) ReplaceDocument(ctx context.Context, workspacePublicID, boardPublicID string, actorUserID, expectedRevision int64, schemaVersion int32, snapshot json.RawMessage) (*V1Board, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if actorUserID <= 0 || expectedRevision <= 0 || schemaVersion < 1 || len(snapshot) == 0 {
		return nil, ErrInvalidInput
	}
	return s.acceptDocument(ctx, workspacePublicID, boardPublicID, actorUserID, &expectedRevision, schemaVersion, snapshot)
}

// acceptDocument owns the document, revision, catalog, and media-ref transaction.
// A nil expectedRevision accepts a sanitized Y compaction and trims its update log.
func (s *PostgresWorkspaceBoardStore) acceptDocument(ctx context.Context, workspacePublicID, boardPublicID string, actorUserID int64, expectedRevision *int64, schemaVersion int32, snapshot json.RawMessage) (*V1Board, error) {
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
		if expectedRevision != nil && int64(locked.DocumentVersion) != *expectedRevision {
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
		nextRevision := int64(locked.DocumentVersion) + 1
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
		if expectedRevision == nil {
			// Commit the compaction and log trim atomically.
			if err := q.DeleteBoardYUpdates(ctx, dbsqlc.DeleteBoardYUpdatesParams{
				WorkspacePublicID: workspacePublicID,
				BoardPublicID:     trimmedID,
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
