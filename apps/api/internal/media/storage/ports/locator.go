package ports

import (
	"context"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/media/ingestion"
)

type StoredUploadLocator interface {
	LocateStoredUpload(context.Context, StoredUploadLookup) (StoredUploadLocation, bool, error)
}

type StoredDerivativeLocator interface {
	LocateStoredDerivative(context.Context, StoredDerivativeLookup) (StoredUploadLocation, bool, error)
}

type MediaIngestionRegistry interface {
	Begin(context.Context, ingestion.Request) (ingestion.Handle, error)
	Complete(context.Context, ingestion.Handle, ingestion.Completion) error
	Fail(context.Context, ingestion.Handle, error) error
	FindReadyOwnedMediaAssetByChecksum(context.Context, int64, string, string) (dbsqlc.MediaAsset, bool, error)
}
