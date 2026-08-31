package assets

import (
	"context"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/media/ingestion"
)

// Registry is the stable Media Asset domain seam consumed by media workflows.
// Implementations own persistence and ingestion transactionality; callers only
// depend on asset facts and lifecycle operations.
//
// Keep this interface intentionally narrow. Storage, derivative scheduling,
// and SQL details remain behind MediaAssetRegistry so those boundaries can
// evolve without changing workflow packages.
type Registry interface {
	FindReadyLibraryMaterializedMedia(context.Context, int64, int64) (dbsqlc.MediaAsset, bool, error)
	Begin(context.Context, ingestion.Request) (ingestion.Handle, error)
	Complete(context.Context, ingestion.Handle, ingestion.Completion) error
	Fail(context.Context, ingestion.Handle, error) error
	GetByID(context.Context, int64, int64) (dbsqlc.MediaAsset, error)
}

var _ Registry = (*MediaAssetRegistry)(nil)
