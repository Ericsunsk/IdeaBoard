package assets

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/platform/database"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	mediaerrors "ideaboard/api/internal/media/errors"
	"ideaboard/api/internal/media/ingestion"
	"ideaboard/api/internal/media/storage/ports"
	tasks "ideaboard/api/internal/tasking/domain"
	taskmedia "ideaboard/api/internal/tasking/ports/media"

	"github.com/jackc/pgx/v5/pgxpool"
)

// errLegacyMediaAssetsRemoved is the #817 (Phase 3b-08) fail-closed stand-in
// for the deleted legacy user-scoped media_assets postgres adapter.
//
// All user-scoped media_asset sqlc queries (MarkMediaAssetReady,
// MarkReadyMediaAssetFailed, MarkMediaAssetFailed,
// GetReadyLibraryMaterializedMediaAsset, BeginLibraryMaterializedMediaAsset,
// UpsertLibraryAssetMediaLink, CreateBoardUploadMediaAsset,
// GetMediaAssetByStorageLocator, BeginTaskResultMediaAsset,
// CreatePreparedMediaAsset, UpsertMediaRef, FindPreparedGenerationMedia,
// FindIngestingPreparedGenerationMedia, ListReadyOwnedMediaAssetIDsByStorageKey,
// FindReadyOwnedMediaAssetIDByURL, GetReadyMediaAssetStorageLocationByID,
// FindMediaAssetStorageLocationByURL, ListMediaAssetStorageLocationsByKey,
// GetMediaAssetByAssetIDForOwner, GetMediaAssetByIDForOwner,
// FindReadyOwnedMediaAssetByChecksum, SoftDeleteMediaAssetByID,
// CreateBoardUploadMediaAsset, Begin*MediaAsset, etc.) were removed with the
// media_assets table rename to the workspace-scoped model
// (media_assets with public_id asset_%), so every ingestion/read method below
// that depended on them fails closed. Derivative-tier methods that still map
// to the surviving media_derivatives queries (GetMediaDerivativeByAssetTier,
// GetProcessingInputManifestObject,
// GetReadyMediaDerivativeStorageLocationByAssetID, ListMediaDerivativesByAsset,
// UpsertMediaDerivative, UpsertMediaAssetSource) stay live. The
// workspace-scoped media asset store lands in Phase 4; until then legacy
// upload/ingestion paths (/api/user boards uploads, preparation,
// library materialization) fail closed through these stubs.

var errLegacyMediaAssetsRemoved = errors.New("legacy media asset store removed (#817 Phase 3b-08): workspace-scoped store lands in Phase 4")

func (r *MediaAssetRegistry) Begin(_ context.Context, _ ingestion.Request) (ingestion.Handle, error) {
	return ingestion.Handle{}, errLegacyMediaAssetsRemoved
}

func (r *MediaAssetRegistry) Complete(_ context.Context, _ ingestion.Handle, _ ingestion.Completion) error {
	return errLegacyMediaAssetsRemoved
}

func (r *MediaAssetRegistry) markReadyAssetFailed(_ context.Context, _ ingestion.Handle, _ error) error {
	return errLegacyMediaAssetsRemoved
}

func (r *MediaAssetRegistry) Fail(_ context.Context, _ ingestion.Handle, _ error) error {
	return errLegacyMediaAssetsRemoved
}

type MediaAssetRegistry struct {
	db                          *pgxpool.Pool
	q                           *dbsqlc.Queries
	now                         func() time.Time
	integrityVerifier           processingInputIntegrityVerifier
	processingInputMaterializer func(context.Context, int64, int64) error
}

type processingInputIntegrityVerifier interface {
	Verify(context.Context, string, string, string, string, int64) error
}

type ProcessingInputMaterializer func(ctx context.Context, assetID, ownerUserID int64) error

type PreparedGenerationMedia struct {
	AssetID        int64
	StorageProfile string
	StorageKey     string
}

func (r *MediaAssetRegistry) FindReadyLibraryMaterializedMedia(_ context.Context, _, _ int64) (dbsqlc.MediaAsset, bool, error) {
	return dbsqlc.MediaAsset{}, false, errLegacyMediaAssetsRemoved
}

func NewMediaAssetRegistry(db *pgxpool.Pool) *MediaAssetRegistry {
	return &MediaAssetRegistry{db: db, q: dbsqlc.New(db), now: time.Now}
}

func (r *MediaAssetRegistry) SetProcessingInputMaterializer(materializer ProcessingInputMaterializer) {
	if r != nil {
		r.processingInputMaterializer = materializer
	}
}

func (r *MediaAssetRegistry) SetIntegrityVerifier(verifier processingInputIntegrityVerifier) {
	if r != nil {
		r.integrityVerifier = verifier
	}
}

var _ taskmedia.TaskInputManifestBuilder = (*MediaAssetRegistry)(nil)

// BuildTaskInputManifest resolves each mutable Asset reference to the exact
// ready derivative object that will be used by the provider. The returned
// value is safe to persist with the Task and is never rebuilt from catalog
// state during retry.
func (r *MediaAssetRegistry) BuildTaskInputManifest(ctx context.Context, userID int64, media *tasks.MediaInputs) (*tasks.TaskInputManifest, error) {
	if r == nil || r.q == nil || userID <= 0 {
		return nil, fmt.Errorf("media derivative registry is unavailable")
	}
	if media == nil {
		return tasks.NewTaskInputManifest(nil, nil, nil, nil, nil)
	}
	inputs, err := r.resolveManifestRefs(ctx, userID, media.Inputs, "input")
	if err != nil {
		return nil, err
	}
	references, err := r.resolveManifestRefs(ctx, userID, media.References, "reference")
	if err != nil {
		return nil, err
	}
	var original, mask *tasks.TaskInputManifestRef
	if media.Original != nil {
		resolved, err := r.resolveManifestRef(ctx, userID, *media.Original)
		if err != nil {
			return nil, fmt.Errorf("resolve original media: %w", err)
		}
		original = &resolved
	}
	if media.Mask != nil {
		resolved, err := r.resolveManifestRef(ctx, userID, *media.Mask)
		if err != nil {
			return nil, fmt.Errorf("resolve mask media: %w", err)
		}
		mask = &resolved
	}
	return tasks.NewTaskInputManifest(inputs, references, original, mask, media.UseMask)
}

func (r *MediaAssetRegistry) resolveManifestRefs(ctx context.Context, userID int64, refs []tasks.MediaRef, kind string) ([]tasks.TaskInputManifestRef, error) {
	resolved := make([]tasks.TaskInputManifestRef, 0, len(refs))
	for index, ref := range refs {
		item, err := r.resolveManifestRef(ctx, userID, ref)
		if err != nil {
			return nil, fmt.Errorf("resolve %s media %d: %w", kind, index, err)
		}
		resolved = append(resolved, item)
	}
	return resolved, nil
}

func (r *MediaAssetRegistry) resolveManifestRef(ctx context.Context, userID int64, ref tasks.MediaRef) (tasks.TaskInputManifestRef, error) {
	if ref.AssetID <= 0 {
		return tasks.TaskInputManifestRef{}, mediaerrors.New(mediaerrors.CodeMediaNotFound, "media.manifest.resolve", "invalid asset id", nil)
	}
	row, err := r.q.GetProcessingInputManifestObject(ctx, dbsqlc.GetProcessingInputManifestObjectParams{MediaAssetID: ref.AssetID, OwnerUserID: userID})
	if database.IsNotFound(err) {
		// Assets whose processing-input was never materialized (e.g. task
		// outputs created before their first chained use) render it on
		// demand here instead of failing the submission.
		if r.processingInputMaterializer == nil {
			return tasks.TaskInputManifestRef{}, mediaerrors.New(mediaerrors.CodeMediaProcessing, "media.manifest.resolve", "processing-input derivative is not ready", err)
		}
		if materializeErr := r.processingInputMaterializer(ctx, ref.AssetID, userID); materializeErr != nil {
			return tasks.TaskInputManifestRef{}, mediaerrors.New(mediaerrors.CodeMediaProcessing, "media.manifest.resolve", "processing-input derivative is not ready", materializeErr)
		}
		row, err = r.q.GetProcessingInputManifestObject(ctx, dbsqlc.GetProcessingInputManifestObjectParams{MediaAssetID: ref.AssetID, OwnerUserID: userID})
		if database.IsNotFound(err) {
			return tasks.TaskInputManifestRef{}, mediaerrors.New(mediaerrors.CodeMediaProcessing, "media.manifest.resolve", "processing-input derivative is not ready", err)
		}
	}
	if err != nil {
		return tasks.TaskInputManifestRef{}, mediaerrors.New(mediaerrors.CodeStorageTransient, "media.manifest.resolve", "load processing-input derivative failed", err)
	}
	if r.integrityVerifier == nil {
		return tasks.TaskInputManifestRef{}, fmt.Errorf("media integrity verifier is unavailable")
	}
	if err := r.integrityVerifier.Verify(ctx, row.StorageProfileID, row.StorageKey, row.ChecksumSha256, row.MimeType, row.SizeBytes); err != nil {
		return tasks.TaskInputManifestRef{}, err
	}
	return tasks.TaskInputManifestRef{
		AssetID: row.AssetID, DerivativeID: row.DerivativeID, DerivativeTier: row.Tier,
		StorageProfileID: row.StorageProfileID, ObjectKey: row.StorageKey,
		ObjectGeneration: row.ObjectGeneration, ChecksumSHA256: row.ChecksumSha256,
		MIMEType: row.MimeType, SizeBytes: row.SizeBytes, Role: ref.Role, Weight: ref.Weight,
	}, nil
}

func (r *MediaAssetRegistry) GetByAssetID(_ context.Context, _ int64, _ string) (dbsqlc.MediaAsset, error) {
	return dbsqlc.MediaAsset{}, errLegacyMediaAssetsRemoved
}

func (r *MediaAssetRegistry) GetByID(_ context.Context, _, _ int64) (dbsqlc.MediaAsset, error) {
	return dbsqlc.MediaAsset{}, errLegacyMediaAssetsRemoved
}

// AuthorizeMediaAsset verifies that a ready, non-deleted asset belongs to the owner.
// The method intentionally exposes no storage details to task intake.
func (r *MediaAssetRegistry) AuthorizeMediaAsset(_ context.Context, _, _ int64) error {
	return errLegacyMediaAssetsRemoved
}

// FindReadyOwnedMediaAssetByChecksum returns an existing ready asset for the
// same owner, storage profile, and content digest. Restricting the profile
// keeps the returned URL valid when operators switch storage backends.
func (r *MediaAssetRegistry) FindReadyOwnedMediaAssetByChecksum(_ context.Context, _ int64, _, _ string) (dbsqlc.MediaAsset, bool, error) {
	return dbsqlc.MediaAsset{}, false, errLegacyMediaAssetsRemoved
}

func (r *MediaAssetRegistry) FindReadyOwnedMediaAssetIDByChecksum(_ context.Context, _ int64, _, _ string) (int64, bool, error) {
	return 0, false, errLegacyMediaAssetsRemoved
}

// RetireMissingOwnedMediaAsset soft-deletes a ready asset whose backing object
// is verifiably gone (e.g. wiped with a deleted board). Retiring releases the
// checksum dedupe slot so the next upload of identical bytes persists fresh
// instead of re-serving an asset that can never materialize derivatives.
// Missing rows are a no-op success so concurrent healers converge.
func (r *MediaAssetRegistry) RetireMissingOwnedMediaAsset(_ context.Context, _, _ int64) error {
	return errLegacyMediaAssetsRemoved
}

func (r *MediaAssetRegistry) ListDerivatives(ctx context.Context, ownerUserID, assetID int64) ([]dbsqlc.MediaDerivative, error) {
	if r == nil || r.q == nil || ownerUserID <= 0 || assetID <= 0 {
		return nil, fmt.Errorf("invalid media derivative lookup")
	}
	return r.q.ListMediaDerivativesByAsset(ctx, dbsqlc.ListMediaDerivativesByAssetParams{
		MediaAssetID: assetID, OwnerUserID: ownerUserID,
	})
}

func ingestionMetadata(origin ingestion.OriginFacts, completion ingestion.Completion) ([]byte, error) {
	switch facts := origin.(type) {
	case ingestion.BoardUploadFacts:
		return json.Marshal(map[string]any{
			"origin": string(ingestion.OriginBoardUpload), "board_id": strings.TrimSpace(facts.BoardID), "animated": completion.Animated,
		})
	case ingestion.GenerationPreparationFacts:
		return json.Marshal(map[string]any{
			"origin": string(ingestion.OriginGenerationPreparation), "ref_id": strings.TrimSpace(facts.RefID),
			"role": strings.TrimSpace(facts.Role), "input_index": facts.InputIndex, "unclaimed": true, "animated": completion.Animated,
		})
	case ingestion.TaskResultFacts:
		return json.Marshal(map[string]any{
			"origin": string(ingestion.OriginTaskResult), "task_id": strings.TrimSpace(facts.TaskID),
			"result_index": facts.ResultIndex, "animated": completion.Animated,
		})
	case ingestion.LibraryMaterializationFacts:
		return json.Marshal(map[string]any{
			"origin": string(ingestion.OriginLibraryMaterialization), "library_asset_id": facts.LibraryAssetID,
			"provider": strings.TrimSpace(facts.Provider), "source_id": strings.TrimSpace(facts.SourceID),
			"source_url": strings.TrimSpace(facts.CanonicalSourceURL), "download_url": strings.TrimSpace(completion.SourceURL),
			"author": strings.TrimSpace(facts.Author), "title": strings.TrimSpace(facts.Title), "animated": completion.Animated,
		})
	default:
		return nil, fmt.Errorf("unsupported media ingestion origin")
	}
}

func (r *MediaAssetRegistry) FindPreparedGenerationMedia(_ context.Context, _ int64, _, _ string, _ int) (PreparedGenerationMedia, bool, error) {
	return PreparedGenerationMedia{}, false, errLegacyMediaAssetsRemoved
}

func (r *MediaAssetRegistry) FindIngestingPreparedGenerationMedia(_ context.Context, _ int64, _, _ string, _ int) (PreparedGenerationMedia, bool, error) {
	return PreparedGenerationMedia{}, false, errLegacyMediaAssetsRemoved
}

func (r *MediaAssetRegistry) FindReusableGenerationMedia(_ context.Context, _ int64, _ string) (int64, bool, error) {
	return 0, false, errLegacyMediaAssetsRemoved
}

func (r *MediaAssetRegistry) LocateStoredUpload(_ context.Context, _ ports.StoredUploadLookup) (ports.StoredUploadLocation, bool, error) {
	return ports.StoredUploadLocation{}, false, errLegacyMediaAssetsRemoved
}

func (r *MediaAssetRegistry) LocateStoredDerivative(ctx context.Context, lookup ports.StoredDerivativeLookup) (ports.StoredUploadLocation, bool, error) {
	if r == nil || r.q == nil || lookup.AssetID <= 0 || strings.TrimSpace(lookup.Tier) == "" {
		return ports.StoredUploadLocation{}, false, fmt.Errorf("invalid media derivative lookup")
	}
	row, err := r.q.GetReadyMediaDerivativeStorageLocationByAssetID(ctx, dbsqlc.GetReadyMediaDerivativeStorageLocationByAssetIDParams{
		MediaAssetID: lookup.AssetID,
		Tier:         strings.TrimSpace(lookup.Tier),
	})
	if database.IsNotFound(err) {
		return ports.StoredUploadLocation{}, false, nil
	}
	if err != nil {
		return ports.StoredUploadLocation{}, false, fmt.Errorf("locate media derivative: %w", err)
	}
	return ports.StoredUploadLocation{StorageProfileID: row.StorageProfileID, StorageKey: row.StorageKey}, true, nil
}

func (r *MediaAssetRegistry) ValidateProcessingInputReadiness(ctx context.Context, userID int64, assetIDs []int64) error {
	if r == nil || r.q == nil || userID <= 0 {
		return fmt.Errorf("media derivative registry is unavailable")
	}
	seen := make(map[int64]struct{}, len(assetIDs))
	for _, assetID := range assetIDs {
		if assetID <= 0 {
			return mediaerrors.New(
				mediaerrors.CodeMediaNotFound,
				"media.derivative.validate_processing_input",
				fmt.Sprintf("invalid asset id %d", assetID),
				nil,
			)
		}
		if _, ok := seen[assetID]; ok {
			continue
		}
		seen[assetID] = struct{}{}
		derivative, err := r.q.GetMediaDerivativeByAssetTier(ctx, dbsqlc.GetMediaDerivativeByAssetTierParams{
			MediaAssetID: assetID, OwnerUserID: userID, Tier: "processing-input",
		})
		if database.IsNotFound(err) {
			// Task outputs never pass through ingestion completion, so they
			// carry no processing-input row until first use. Materialize on
			// demand here (same seam as the manifest builder) instead of
			// rejecting chained submissions.
			if r.processingInputMaterializer == nil {
				return mediaerrors.New(
					mediaerrors.CodeMediaProcessing,
					"media.derivative.validate_processing_input",
					fmt.Sprintf("media asset %d processing-input derivative is not ready", assetID),
					err,
				).WithFacts(fmt.Sprintf("%d", assetID), "", "", "", "", "")
			}
			if materializeErr := r.processingInputMaterializer(ctx, assetID, userID); materializeErr != nil {
				return mediaerrors.New(
					mediaerrors.CodeMediaProcessing,
					"media.derivative.validate_processing_input",
					fmt.Sprintf("media asset %d processing-input derivative is not ready", assetID),
					materializeErr,
				).WithFacts(fmt.Sprintf("%d", assetID), "", "", "", "", "")
			}
			derivative, err = r.q.GetMediaDerivativeByAssetTier(ctx, dbsqlc.GetMediaDerivativeByAssetTierParams{
				MediaAssetID: assetID, OwnerUserID: userID, Tier: "processing-input",
			})
			if database.IsNotFound(err) {
				return mediaerrors.New(
					mediaerrors.CodeMediaProcessing,
					"media.derivative.validate_processing_input",
					fmt.Sprintf("media asset %d processing-input derivative is not ready", assetID),
					err,
				).WithFacts(fmt.Sprintf("%d", assetID), "", "", "", "", "")
			}
		}
		if err != nil {
			return mediaerrors.New(
				mediaerrors.CodeStorageTransient,
				"media.derivative.validate_processing_input",
				fmt.Sprintf("lookup media asset %d processing-input derivative failed", assetID),
				err,
			).WithFacts(fmt.Sprintf("%d", assetID), "", "", "", "", "")
		}
		if derivative.Status != "ready" {
			return mediaerrors.New(
				mediaerrors.CodeMediaProcessing,
				"media.derivative.validate_processing_input",
				fmt.Sprintf("media asset %d processing-input derivative is in status %q", assetID, derivative.Status),
				nil,
			).WithFacts(fmt.Sprintf("%d", assetID), "", "", "", "", "")
		}
		if r.integrityVerifier == nil {
			return fmt.Errorf("media integrity verifier is unavailable")
		}
		if err := r.integrityVerifier.Verify(ctx, derivative.StorageProfileID, derivative.StorageKey, derivative.ChecksumSha256, derivative.MimeType, derivative.SizeBytes); err != nil {
			return mediaerrors.New(mediaerrors.CodeObjectMissing, "media.derivative.validate_processing_input", fmt.Sprintf("media asset %d processing-input object is not usable", assetID), err)
		}
	}
	return nil
}

func ingestionHandle(assetID int64, publicAssetID string, existingReady bool, request ingestion.Request) ingestion.Handle {
	return ingestion.Handle{
		AssetID:        assetID,
		PublicAssetID:  strings.TrimSpace(publicAssetID),
		ExistingReady:  existingReady,
		OwnerUserID:    request.OwnerUserID,
		StorageProfile: request.StorageProfile,
		StorageKey:     request.StorageKey,
		Origin:         request.Origin,
	}
}

func newMediaAssetID() (string, error) {
	var value [12]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate media asset id: %w", err)
	}
	return "asset_" + hex.EncodeToString(value[:]), nil
}
