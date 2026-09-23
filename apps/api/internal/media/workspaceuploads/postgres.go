package workspaceuploads

import (
	"context"
	"strings"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Bounded list cap: list queries always carry a LIMIT.
const maxWorkspaceAssetRows = int32(100)

// PostgresAssetStore implements AssetStore against PostgreSQL/sqlc with NO
// schema change: the workspace binding travels in media_assets.metadata
// (workspace_id), the archive marker in metadata.archived_at. It must
// never be replaced by the test-only InMemoryAssetStore in production
// wiring.
type PostgresAssetStore struct {
	db *pgxpool.Pool
	q  *dbsqlc.Queries
}

// Compile-time convergence with the agreed interface.
var _ AssetStore = (*PostgresAssetStore)(nil)

// NewPostgresAssetStore builds a postgres AssetStore over pool.
func NewPostgresAssetStore(db *pgxpool.Pool) *PostgresAssetStore {
	return &PostgresAssetStore{db: db, q: dbsqlc.New(db)}
}

// dbWithContext returns the query set bound to the request context.
// Repository methods take context.Context first and use this pattern.
func (s *PostgresAssetStore) dbWithContext(ctx context.Context) *dbsqlc.Queries {
	return s.q
}

func mapPostgresAsset(row dbsqlc.MediaAsset) *Asset {
	return &Asset{
		InternalID:       row.ID,
		ID:               row.AssetID,
		WorkspaceID:      DecodeAssetWorkspace(row.Metadata),
		OwnerUserID:      row.OwnerUserID,
		Status:           row.Status,
		MediaKind:        row.MediaKind,
		MIMEType:         row.MimeType,
		Width:            row.Width,
		Height:           row.Height,
		SizeBytes:        row.SizeBytes,
		ChecksumSHA256:   row.ChecksumSha256,
		ObjectGeneration: row.ObjectGeneration,
		StorageProfileID: row.StorageProfileID,
		StorageKey:       row.StorageKey,
		ArchivedAt:       DecodeAssetArchivedAt(row.Metadata),
		CreatedAt:        timeValue(row.CreatedAt),
		UpdatedAt:        timeValue(row.UpdatedAt),
	}
}

func timeValue(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}

// CreateAsset inserts the durable asset row. Status starts at pending:
// complete accepted declarations only, and byte verification (storage
// integrity gate) lands with #805, which flips ready and cancels the
// abandonment reclaim_after.
func (s *PostgresAssetStore) CreateAsset(ctx context.Context, params CreateAssetParams) (*Asset, error) {
	if err := ValidateCreateInput("asset", params.MIMEType, params.SizeBytes, params.ChecksumSHA256, params.MediaKind); err != nil {
		return nil, err
	}
	id, err := NewAssetPublicID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	row, err := s.dbWithContext(ctx).CreateWorkspaceMediaAsset(ctx, dbsqlc.CreateWorkspaceMediaAssetParams{
		AssetID:          id,
		OwnerUserID:      params.OwnerUserID,
		StorageProfileID: params.StorageProfileID,
		StorageKey:       params.StorageKey,
		Url:              "",
		MediaKind:        MediaKindImage,
		MimeType:         CanonicalMIME(params.MIMEType),
		Width:            params.Width,
		Height:           params.Height,
		SizeBytes:        params.SizeBytes,
		ChecksumSha256:   NormalizeChecksum(params.ChecksumSHA256),
		Status:           StatusPending,
		Metadata:         EncodeAssetMetadata(params.WorkspaceID),
		ReclaimAfter:     pgtype.Timestamptz{Time: now.Add(WorkspaceAssetAbandonedTTL), Valid: true},
	})
	if err != nil {
		if database.IsDuplicateConstraintError(err) {
			return nil, ErrStateConflict
		}
		return nil, err
	}
	return mapPostgresAsset(row), nil
}

// GetAsset resolves an asset by opaque id within its workspace. Legacy
// rows without a workspace binding and cross-workspace ids are invisible
// (ErrNotFound -> 404), never 403.
func (s *PostgresAssetStore) GetAsset(ctx context.Context, workspaceID, assetID string) (*Asset, error) {
	row, err := s.dbWithContext(ctx).GetWorkspaceMediaAsset(ctx, dbsqlc.GetWorkspaceMediaAssetParams{
		AssetID:     strings.TrimSpace(assetID),
		WorkspaceID: strings.TrimSpace(workspaceID),
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return mapPostgresAsset(row), nil
}

// ListAssets returns newest-first non-archived assets with a bounded page.
func (s *PostgresAssetStore) ListAssets(ctx context.Context, workspaceID string, limit, offset int) ([]*Asset, error) {
	limit, offset = clampAssetPage(limit, offset)
	rows, err := s.dbWithContext(ctx).ListWorkspaceMediaAssets(ctx, dbsqlc.ListWorkspaceMediaAssetsParams{
		WorkspaceID: strings.TrimSpace(workspaceID),
		RowOffset:   int32(offset),
		RowLimit:    int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]*Asset, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapPostgresAsset(row))
	}
	return out, nil
}

// ArchiveAsset stamps the archive lifecycle marker plus the retention
// reclaim_after. It is idempotent: an already-archived asset reads back
// unchanged (parity with the in-memory sibling).
func (s *PostgresAssetStore) ArchiveAsset(ctx context.Context, workspaceID, assetID string) (*Asset, error) {
	asset, err := s.GetAsset(ctx, workspaceID, assetID)
	if err != nil {
		return nil, err
	}
	if asset.IsArchived() {
		return asset, nil
	}
	now := time.Now().UTC()
	row, err := s.dbWithContext(ctx).ArchiveWorkspaceMediaAsset(ctx, dbsqlc.ArchiveWorkspaceMediaAssetParams{
		ArchivedAt:   FormatArchivedAt(now),
		ReclaimAfter: pgtype.Timestamptz{Time: now.Add(WorkspaceArchivedAssetRetentionTTL), Valid: true},
		AssetID:      strings.TrimSpace(assetID),
		WorkspaceID:  strings.TrimSpace(workspaceID),
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return mapPostgresAsset(row), nil
}

// ListDerivatives reuses the existing media_derivatives queries for the
// v1 derivative-bytes endpoint.
func (s *PostgresAssetStore) ListDerivatives(ctx context.Context, ownerUserID, assetInternalID int64) ([]Derivative, error) {
	rows, err := s.dbWithContext(ctx).ListMediaDerivativesByAsset(ctx, dbsqlc.ListMediaDerivativesByAssetParams{
		MediaAssetID: assetInternalID,
		OwnerUserID:  ownerUserID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Derivative, 0, len(rows))
	for _, row := range rows {
		out = append(out, Derivative{
			Tier:                   row.Tier,
			Format:                 row.Format,
			Status:                 row.Status,
			MIMEType:               row.MimeType,
			Width:                  row.Width,
			Height:                 row.Height,
			SizeBytes:              row.SizeBytes,
			StorageProfileID:       row.StorageProfileID,
			StorageKey:             row.StorageKey,
			SourceObjectGeneration: row.SourceObjectGeneration,
		})
	}
	return out, nil
}
