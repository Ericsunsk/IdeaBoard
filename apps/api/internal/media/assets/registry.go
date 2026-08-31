package assets

import (
	"context"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/media/ingestion"
	"ideaboard/api/internal/media/storage"
)

// Registry is the stable Media Asset domain seam consumed by media workflows.
// Implementations own persistence and ingestion transactionality; callers only
// depend on asset facts and lifecycle operations.
//
// Registry is the single Media domain boundary. It owns asset lifecycle,
// deduplication, derivative readiness and storage-location facts. Callers do
// not depend on SQL or the concrete registry implementation.
type Registry interface {
	FindReadyLibraryMaterializedMedia(context.Context, int64, int64) (dbsqlc.MediaAsset, bool, error)
	Begin(context.Context, ingestion.Request) (ingestion.Handle, error)
	Complete(context.Context, ingestion.Handle, ingestion.Completion) error
	Fail(context.Context, ingestion.Handle, error) error
	GetByID(context.Context, int64, int64) (dbsqlc.MediaAsset, error)
	GetByAssetID(context.Context, int64, string) (dbsqlc.MediaAsset, error)
	AuthorizeMediaAsset(context.Context, int64, int64) error
	FindReadyOwnedMediaAssetByChecksum(context.Context, int64, string, string) (dbsqlc.MediaAsset, bool, error)
	FindReadyOwnedMediaAssetIDByChecksum(context.Context, int64, string, string) (int64, bool, error)
	FindPreparedGenerationMedia(context.Context, int64, string, string, int) (PreparedGenerationMedia, bool, error)
	FindIngestingPreparedGenerationMedia(context.Context, int64, string, string, int) (PreparedGenerationMedia, bool, error)
	FindReusableGenerationMedia(context.Context, int64, string) (int64, bool, error)
	ListDerivatives(context.Context, int64, int64) ([]dbsqlc.MediaDerivative, error)
	EnsureDerivative(context.Context, int64, int64, string) error
	LocateStoredUpload(context.Context, storage.StoredUploadLookup) (storage.StoredUploadLocation, bool, error)
	LocateStoredDerivative(context.Context, storage.StoredDerivativeLookup) (storage.StoredUploadLocation, bool, error)
	ValidateProcessingInputReadiness(context.Context, int64, []int64) error
}

// PreparationRegistry is the subset required to materialize generation
// inputs. Keeping capability-specific seams makes test doubles small while
// the concrete implementation remains the single domain service.
type PreparationRegistry interface {
	FindPreparedGenerationMedia(context.Context, int64, string, string, int) (PreparedGenerationMedia, bool, error)
	FindIngestingPreparedGenerationMedia(context.Context, int64, string, string, int) (PreparedGenerationMedia, bool, error)
	FindReusableGenerationMedia(context.Context, int64, string) (int64, bool, error)
	FindReadyOwnedMediaAssetIDByChecksum(context.Context, int64, string, string) (int64, bool, error)
	Begin(context.Context, ingestion.Request) (ingestion.Handle, error)
	Complete(context.Context, ingestion.Handle, ingestion.Completion) error
	Fail(context.Context, ingestion.Handle, error) error
}

type LibraryMaterializationRegistry interface {
	FindReadyLibraryMaterializedMedia(context.Context, int64, int64) (dbsqlc.MediaAsset, bool, error)
	Begin(context.Context, ingestion.Request) (ingestion.Handle, error)
	Complete(context.Context, ingestion.Handle, ingestion.Completion) error
	Fail(context.Context, ingestion.Handle, error) error
	GetByID(context.Context, int64, int64) (dbsqlc.MediaAsset, error)
}

type ReadRegistry interface {
	GetByID(context.Context, int64, int64) (dbsqlc.MediaAsset, error)
	GetByAssetID(context.Context, int64, string) (dbsqlc.MediaAsset, error)
	ListDerivatives(context.Context, int64, int64) ([]dbsqlc.MediaDerivative, error)
	EnsureDerivative(context.Context, int64, int64, string) error
}

type UploadRegistry interface {
	Begin(context.Context, ingestion.Request) (ingestion.Handle, error)
	Complete(context.Context, ingestion.Handle, ingestion.Completion) error
	Fail(context.Context, ingestion.Handle, error) error
	FindReadyOwnedMediaAssetByChecksum(context.Context, int64, string, string) (dbsqlc.MediaAsset, bool, error)
}

var _ Registry = (*MediaAssetRegistry)(nil)
var _ PreparationRegistry = (*MediaAssetRegistry)(nil)
var _ LibraryMaterializationRegistry = (*MediaAssetRegistry)(nil)
var _ ReadRegistry = (*MediaAssetRegistry)(nil)
var _ UploadRegistry = (*MediaAssetRegistry)(nil)
