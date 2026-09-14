package assets

import (
	"context"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/media/ingestion"
	"ideaboard/api/internal/media/storage/ports"
)

// IngestionRegistry is the shared lifecycle capability used by workflows that
// create or materialize assets. The concrete registry remains the single
// persistence and transactionality owner.
type IngestionRegistry interface {
	Begin(context.Context, ingestion.Request) (ingestion.Handle, error)
	Complete(context.Context, ingestion.Handle, ingestion.Completion) error
	Fail(context.Context, ingestion.Handle, error) error
}

// PreparationRegistry is the subset required to materialize generation
// inputs. Keeping capability-specific seams makes test doubles small while
// the concrete implementation remains the single domain service.
type PreparationRegistry interface {
	IngestionRegistry
	FindPreparedGenerationMedia(context.Context, int64, string, string, int) (PreparedGenerationMedia, bool, error)
	FindIngestingPreparedGenerationMedia(context.Context, int64, string, string, int) (PreparedGenerationMedia, bool, error)
	FindReusableGenerationMedia(context.Context, int64, string) (int64, bool, error)
	FindReadyOwnedMediaAssetIDByChecksum(context.Context, int64, string, string) (int64, bool, error)
	Begin(context.Context, ingestion.Request) (ingestion.Handle, error)
	Complete(context.Context, ingestion.Handle, ingestion.Completion) error
	Fail(context.Context, ingestion.Handle, error) error
}

type LibraryMaterializationRegistry interface {
	IngestionRegistry
	FindReadyLibraryMaterializedMedia(context.Context, int64, int64) (dbsqlc.MediaAsset, bool, error)
	GetByID(context.Context, int64, int64) (dbsqlc.MediaAsset, error)
}

// ReadRegistry is the read-only capability used by callers that only project
// asset metadata. Derivative tiers resolve on demand through the image
// delivery layer, so there is nothing to enqueue here.
type ReadRegistry interface {
	GetByID(context.Context, int64, int64) (dbsqlc.MediaAsset, error)
	GetByAssetID(context.Context, int64, string) (dbsqlc.MediaAsset, error)
	ListDerivatives(context.Context, int64, int64) ([]dbsqlc.MediaDerivative, error)
}

type UploadRegistry interface {
	IngestionRegistry
	GetByID(context.Context, int64, int64) (dbsqlc.MediaAsset, error)
	FindReadyOwnedMediaAssetByChecksum(context.Context, int64, string, string) (dbsqlc.MediaAsset, bool, error)
	LocateStoredDerivative(context.Context, ports.StoredDerivativeLookup) (ports.StoredUploadLocation, bool, error)
	RetireMissingOwnedMediaAsset(context.Context, int64, int64) error
}

var _ IngestionRegistry = (*MediaAssetRegistry)(nil)
var _ PreparationRegistry = (*MediaAssetRegistry)(nil)
var _ LibraryMaterializationRegistry = (*MediaAssetRegistry)(nil)
var _ ReadRegistry = (*MediaAssetRegistry)(nil)
var _ UploadRegistry = (*MediaAssetRegistry)(nil)
