package assets

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/media/derivatives"
	mediaerrors "ideaboard/api/internal/media/errors"
	"ideaboard/api/internal/media/ingestion"
	"ideaboard/api/internal/media/reclamation"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/tasking/taskmedia"
	"ideaboard/api/internal/tasking/tasks"
)

func (r *MediaAssetRegistry) Begin(ctx context.Context, request ingestion.Request) (ingestion.Handle, error) {
	if r == nil || r.db == nil || r.q == nil || request.OwnerUserID <= 0 || request.Origin == nil {
		return ingestion.Handle{}, fmt.Errorf("invalid media ingestion request")
	}
	switch facts := request.Origin.(type) {
	case ingestion.BoardUploadFacts:
		return r.beginBoardUpload(ctx, request, facts)
	case ingestion.GenerationPreparationFacts:
		return r.beginGenerationPreparation(ctx, request, facts)
	case ingestion.TaskResultFacts:
		return r.beginTaskResult(ctx, request, facts)
	case ingestion.LibraryMaterializationFacts:
		return r.beginLibraryMaterialization(ctx, request, facts)
	default:
		return ingestion.Handle{}, fmt.Errorf("unsupported media ingestion origin %q", request.Origin.IngestionOrigin())
	}
}

func (r *MediaAssetRegistry) Complete(ctx context.Context, handle ingestion.Handle, completion ingestion.Completion) error {
	if r == nil || r.db == nil || handle.AssetID <= 0 || handle.OwnerUserID <= 0 || handle.Origin == nil {
		return fmt.Errorf("invalid media asset completion")
	}
	if strings.TrimSpace(completion.StorageKey) == "" || strings.TrimSpace(completion.StorageKey) != strings.TrimSpace(handle.StorageKey) {
		return fmt.Errorf("media asset completion storage key does not match ingestion handle")
	}
	if r.integrityVerifier == nil {
		return fmt.Errorf("media asset integrity verifier is not configured")
	}
	if err := r.integrityVerifier.Verify(ctx, handle.StorageProfile, completion.StorageKey, completion.ChecksumSHA256, completion.MIMEType, completion.SizeBytes); err != nil {
		return fmt.Errorf("verify completed media asset: %w", err)
	}
	metadata, err := ingestionMetadata(handle.Origin, completion)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin media asset completion: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	q := dbsqlc.New(tx)
	if _, err := q.MarkMediaAssetReady(ctx, dbsqlc.MarkMediaAssetReadyParams{
		ID: handle.AssetID, OwnerUserID: handle.OwnerUserID, StorageKey: completion.StorageKey,
		Url: completion.URL, SourceUrl: completion.SourceURL,
		MimeType: completion.MIMEType, Width: int32(completion.Width), Height: int32(completion.Height), SizeBytes: completion.SizeBytes,
		ChecksumSha256: completion.ChecksumSHA256, Metadata: metadata,
	}); err != nil {
		return fmt.Errorf("complete media asset: %w", err)
	}
	derivativeIDs, err := derivatives.ScheduleDefault(ctx, q, handle.AssetID, handle.OwnerUserID)
	if err != nil {
		return fmt.Errorf("queue media asset derivatives: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit media asset completion: %w", err)
	}
	committed = true
	r.enqueueDerivatives(ctx, derivativeIDs)
	return nil
}

func (r *MediaAssetRegistry) Fail(ctx context.Context, handle ingestion.Handle, cause error) error {
	if r == nil || r.q == nil || handle.OwnerUserID <= 0 || handle.AssetID <= 0 {
		return fmt.Errorf("invalid media asset failure")
	}
	message := "media asset ingestion failed"
	if cause != nil && strings.TrimSpace(cause.Error()) != "" {
		message = cause.Error()
	}
	_, err := r.q.MarkMediaAssetFailed(ctx, dbsqlc.MarkMediaAssetFailedParams{ID: handle.AssetID, OwnerUserID: handle.OwnerUserID, ErrorMessage: message})
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}

type MediaAssetRegistry struct {
	db                 *sql.DB
	q                  *dbsqlc.Queries
	now                func() time.Time
	derivativeEnqueuer func(context.Context, int64) error
	integrityVerifier  processingInputIntegrityVerifier
}

type processingInputIntegrityVerifier interface {
	Verify(context.Context, string, string, string, string, int64) error
}

type DerivativeEnqueuer func(context.Context, int64) error

type PreparedGenerationMedia struct {
	AssetID        int64
	StorageProfile string
	StorageKey     string
}

func (r *MediaAssetRegistry) FindReadyLibraryMaterializedMedia(ctx context.Context, ownerUserID, libraryAssetID int64) (dbsqlc.MediaAsset, bool, error) {
	if r == nil || r.q == nil || ownerUserID <= 0 || libraryAssetID <= 0 {
		return dbsqlc.MediaAsset{}, false, fmt.Errorf("invalid library materialized media lookup")
	}
	asset, err := r.q.GetReadyLibraryMaterializedMediaAsset(ctx, dbsqlc.GetReadyLibraryMaterializedMediaAssetParams{
		OwnerUserID: ownerUserID, LibraryAssetID: libraryAssetID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return dbsqlc.MediaAsset{}, false, nil
	}
	if err != nil {
		return dbsqlc.MediaAsset{}, false, fmt.Errorf("lookup materialized library media: %w", err)
	}
	return asset, true, nil
}

func (r *MediaAssetRegistry) beginLibraryMaterialization(ctx context.Context, request ingestion.Request, facts ingestion.LibraryMaterializationFacts) (ingestion.Handle, error) {
	if facts.LibraryAssetID <= 0 || strings.TrimSpace(request.StorageProfile) == "" || strings.TrimSpace(request.StorageKey) == "" {
		return ingestion.Handle{}, fmt.Errorf("invalid library materialized media")
	}
	assetID, err := newMediaAssetID()
	if err != nil {
		return ingestion.Handle{}, err
	}
	metadata, err := ingestionMetadata(request.Origin, ingestion.Completion{})
	if err != nil {
		return ingestion.Handle{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return ingestion.Handle{}, fmt.Errorf("begin library materialized media transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	q := dbsqlc.New(tx)
	assetIDValue, err := q.BeginLibraryMaterializedMediaAsset(ctx, dbsqlc.BeginLibraryMaterializedMediaAssetParams{
		AssetID: assetID, OwnerUserID: request.OwnerUserID, StorageProfileID: request.StorageProfile, StorageKey: request.StorageKey,
		Url: request.URL, SourceUrl: request.SourceURL, Metadata: metadata,
		ReclaimAfter: sql.NullTime{Time: r.now().Add(reclamation.GracePeriod), Valid: true},
	})
	if err != nil {
		return ingestion.Handle{}, fmt.Errorf("begin library materialized media: %w", err)
	}
	if err := q.UpsertLibraryAssetMediaLink(ctx, dbsqlc.UpsertLibraryAssetMediaLinkParams{
		LibraryAssetID: facts.LibraryAssetID, OwnerUserID: request.OwnerUserID, MediaAssetID: assetIDValue,
	}); err != nil {
		return ingestion.Handle{}, fmt.Errorf("link library materialized media: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ingestion.Handle{}, fmt.Errorf("commit library materialized media transaction: %w", err)
	}
	committed = true
	return ingestionHandle(assetIDValue, assetID, false, request), nil
}

func NewMediaAssetRegistry(db *sql.DB) *MediaAssetRegistry {
	return &MediaAssetRegistry{db: db, q: dbsqlc.New(db), now: time.Now}
}

func (r *MediaAssetRegistry) SetDerivativeEnqueuer(enqueuer DerivativeEnqueuer) {
	if r != nil {
		r.derivativeEnqueuer = enqueuer
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
	resolve := func(ref tasks.MediaRef) (tasks.TaskInputManifestRef, error) {
		if ref.AssetID <= 0 {
			return tasks.TaskInputManifestRef{}, mediaerrors.New(mediaerrors.CodeMediaNotFound, "media.manifest.resolve", "invalid asset id", nil)
		}
		row, err := r.q.GetProcessingInputManifestObject(ctx, dbsqlc.GetProcessingInputManifestObjectParams{MediaAssetID: ref.AssetID, OwnerUserID: userID})
		if errors.Is(err, sql.ErrNoRows) {
			return tasks.TaskInputManifestRef{}, mediaerrors.New(mediaerrors.CodeMediaProcessing, "media.manifest.resolve", "processing-input derivative is not ready", err)
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
	inputs := make([]tasks.TaskInputManifestRef, 0, len(media.Inputs))
	for index, ref := range media.Inputs {
		resolved, err := resolve(ref)
		if err != nil {
			return nil, fmt.Errorf("resolve input media %d: %w", index, err)
		}
		inputs = append(inputs, resolved)
	}
	references := make([]tasks.TaskInputManifestRef, 0, len(media.References))
	for index, ref := range media.References {
		resolved, err := resolve(ref)
		if err != nil {
			return nil, fmt.Errorf("resolve reference media %d: %w", index, err)
		}
		references = append(references, resolved)
	}
	var original, mask *tasks.TaskInputManifestRef
	if media.Original != nil {
		resolved, err := resolve(*media.Original)
		if err != nil {
			return nil, fmt.Errorf("resolve original media: %w", err)
		}
		original = &resolved
	}
	if media.Mask != nil {
		resolved, err := resolve(*media.Mask)
		if err != nil {
			return nil, fmt.Errorf("resolve mask media: %w", err)
		}
		mask = &resolved
	}
	return tasks.NewTaskInputManifest(inputs, references, original, mask, media.UseMask)
}

func (r *MediaAssetRegistry) GetByAssetID(ctx context.Context, ownerUserID int64, assetID string) (dbsqlc.MediaAsset, error) {
	if r == nil || r.q == nil || ownerUserID <= 0 || strings.TrimSpace(assetID) == "" {
		return dbsqlc.MediaAsset{}, fmt.Errorf("invalid media asset lookup")
	}
	return r.q.GetMediaAssetByAssetIDForOwner(ctx, dbsqlc.GetMediaAssetByAssetIDForOwnerParams{
		AssetID: assetID, OwnerUserID: ownerUserID,
	})
}

func (r *MediaAssetRegistry) GetByID(ctx context.Context, ownerUserID, assetID int64) (dbsqlc.MediaAsset, error) {
	if r == nil || r.q == nil || ownerUserID <= 0 || assetID <= 0 {
		return dbsqlc.MediaAsset{}, fmt.Errorf("invalid media asset lookup")
	}
	return r.q.GetMediaAssetByIDForOwner(ctx, dbsqlc.GetMediaAssetByIDForOwnerParams{ID: assetID, OwnerUserID: ownerUserID})
}

// AuthorizeMediaAsset verifies that a ready, non-deleted asset belongs to the owner.
// The method intentionally exposes no storage details to task intake.
func (r *MediaAssetRegistry) AuthorizeMediaAsset(ctx context.Context, ownerUserID, assetID int64) error {
	asset, err := r.GetByID(ctx, ownerUserID, assetID)
	if err != nil {
		return err
	}
	if strings.ToLower(strings.TrimSpace(asset.Status)) != "ready" {
		return fmt.Errorf("媒体资产尚未就绪")
	}
	return nil
}

// FindReadyOwnedMediaAssetByChecksum returns an existing ready asset for the
// same owner, storage profile, and content digest. Restricting the profile
// keeps the returned URL valid when operators switch storage backends.
func (r *MediaAssetRegistry) FindReadyOwnedMediaAssetByChecksum(ctx context.Context, ownerUserID int64, storageProfileID, checksum string) (dbsqlc.MediaAsset, bool, error) {
	if r == nil || r.q == nil || ownerUserID <= 0 || strings.TrimSpace(storageProfileID) == "" || strings.TrimSpace(checksum) == "" {
		return dbsqlc.MediaAsset{}, false, fmt.Errorf("invalid media checksum lookup")
	}
	asset, err := r.q.FindReadyOwnedMediaAssetByChecksum(ctx, dbsqlc.FindReadyOwnedMediaAssetByChecksumParams{
		OwnerUserID: ownerUserID, StorageProfileID: strings.TrimSpace(storageProfileID), ChecksumSha256: strings.TrimSpace(checksum),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return dbsqlc.MediaAsset{}, false, nil
	}
	if err != nil {
		return dbsqlc.MediaAsset{}, false, fmt.Errorf("find media asset by checksum: %w", err)
	}
	return asset, true, nil
}

func (r *MediaAssetRegistry) FindReadyOwnedMediaAssetIDByChecksum(ctx context.Context, ownerUserID int64, storageProfileID, checksum string) (int64, bool, error) {
	asset, found, err := r.FindReadyOwnedMediaAssetByChecksum(ctx, ownerUserID, storageProfileID, checksum)
	if err != nil || !found {
		return 0, found, err
	}
	return asset.ID, true, nil
}

func (r *MediaAssetRegistry) ListDerivatives(ctx context.Context, ownerUserID, assetID int64) ([]dbsqlc.MediaDerivative, error) {
	if r == nil || r.q == nil || ownerUserID <= 0 || assetID <= 0 {
		return nil, fmt.Errorf("invalid media derivative lookup")
	}
	return r.q.ListMediaDerivativesByAsset(ctx, dbsqlc.ListMediaDerivativesByAssetParams{
		MediaAssetID: assetID, OwnerUserID: ownerUserID,
	})
}

func (r *MediaAssetRegistry) EnsureDerivative(ctx context.Context, ownerUserID, assetID int64, tier string) error {
	if r == nil || r.q == nil || ownerUserID <= 0 || assetID <= 0 {
		return fmt.Errorf("invalid media derivative request")
	}
	derivativeID, needsEnqueue, err := derivatives.EnsureOnDemand(ctx, r.q, assetID, ownerUserID, tier)
	if err != nil {
		return err
	}
	// The outbox remains the durable source of truth, but enqueue immediately
	// so an interactive Apply does not wait for the periodic dispatcher.
	if needsEnqueue && r.derivativeEnqueuer != nil {
		if err := r.derivativeEnqueuer(ctx, derivativeID); err != nil {
			// The outbox is durable and will enqueue this derivative on its next
			// reconciliation pass. Direct River insertion only reduces latency.
			slog.Warn("immediate media derivative enqueue failed", "derivative_id", derivativeID, "err", err)
		}
	}
	return nil
}

func (r *MediaAssetRegistry) beginBoardUpload(ctx context.Context, request ingestion.Request, facts ingestion.BoardUploadFacts) (ingestion.Handle, error) {
	boardID := strings.TrimSpace(facts.BoardID)
	storageProfileID := strings.TrimSpace(request.StorageProfile)
	storageKey := strings.TrimSpace(request.StorageKey)
	url := strings.TrimSpace(request.URL)
	if boardID == "" || storageProfileID == "" || storageKey == "" || url == "" {
		return ingestion.Handle{}, fmt.Errorf("incomplete board upload media asset")
	}
	assetID, err := newMediaAssetID()
	if err != nil {
		return ingestion.Handle{}, err
	}
	metadata, err := ingestionMetadata(request.Origin, ingestion.Completion{})
	if err != nil {
		return ingestion.Handle{}, fmt.Errorf("encode board upload media metadata: %w", err)
	}
	registeredAt := r.now()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return ingestion.Handle{}, fmt.Errorf("begin board upload media registration: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	q := dbsqlc.New(tx)
	id, err := q.CreateBoardUploadMediaAsset(ctx, dbsqlc.CreateBoardUploadMediaAssetParams{
		AssetID:          assetID,
		OwnerUserID:      request.OwnerUserID,
		StorageProfileID: storageProfileID,
		StorageKey:       storageKey,
		Url:              url,
		MimeType:         "",
		Width:            0,
		Height:           0,
		SizeBytes:        0,
		ChecksumSha256:   "",
		Status:           "uploading",
		Metadata:         metadata,
		ReclaimAfter:     sql.NullTime{Time: registeredAt.Add(reclamation.GracePeriod), Valid: true},
	})
	if err != nil {
		return ingestion.Handle{}, fmt.Errorf("register board upload media asset: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ingestion.Handle{}, fmt.Errorf("commit board upload media registration: %w", err)
	}
	committed = true
	return ingestionHandle(id, assetID, false, request), nil
}

func (r *MediaAssetRegistry) beginTaskResult(ctx context.Context, request ingestion.Request, facts ingestion.TaskResultFacts) (ingestion.Handle, error) {
	if facts.ResultIndex < 0 {
		return ingestion.Handle{}, fmt.Errorf("invalid task result ingestion")
	}
	taskID := strings.TrimSpace(facts.TaskID)
	profileID := strings.TrimSpace(request.StorageProfile)
	storageKey := strings.TrimSpace(request.StorageKey)
	if taskID == "" || profileID == "" || storageKey == "" {
		return ingestion.Handle{}, fmt.Errorf("incomplete task result ingestion")
	}
	assetID, err := newMediaAssetID()
	if err != nil {
		return ingestion.Handle{}, err
	}
	metadata, err := ingestionMetadata(request.Origin, ingestion.Completion{})
	if err != nil {
		return ingestion.Handle{}, fmt.Errorf("encode task result ingestion metadata: %w", err)
	}
	existingReady := false
	if existing, lookupErr := r.q.GetMediaAssetByStorageLocator(ctx, dbsqlc.GetMediaAssetByStorageLocatorParams{StorageProfileID: profileID, StorageKey: storageKey}); lookupErr == nil {
		existingReady = existing.Status == "ready"
	} else if !errors.Is(lookupErr, sql.ErrNoRows) {
		return ingestion.Handle{}, fmt.Errorf("check task result ingestion state: %w", lookupErr)
	}
	id, err := r.q.BeginTaskResultMediaAsset(ctx, dbsqlc.BeginTaskResultMediaAssetParams{
		AssetID: assetID, OwnerUserID: request.OwnerUserID, StorageProfileID: profileID,
		StorageKey: storageKey, Url: strings.TrimSpace(request.URL), SourceUrl: strings.TrimSpace(request.SourceURL),
		Metadata: metadata, ReclaimAfter: sql.NullTime{Time: r.now().Add(reclamation.GracePeriod), Valid: true},
	})
	if err != nil {
		return ingestion.Handle{}, fmt.Errorf("begin task result ingestion: %w", err)
	}
	return ingestionHandle(id, assetID, existingReady, request), nil
}

func (r *MediaAssetRegistry) beginGenerationPreparation(ctx context.Context, request ingestion.Request, facts ingestion.GenerationPreparationFacts) (ingestion.Handle, error) {
	if facts.InputIndex < 0 {
		return ingestion.Handle{}, fmt.Errorf("invalid prepared generation media")
	}
	if strings.TrimSpace(request.StorageProfile) == "" || strings.TrimSpace(request.StorageKey) == "" || strings.TrimSpace(request.URL) == "" || strings.TrimSpace(facts.RefID) == "" || strings.TrimSpace(facts.Role) == "" {
		return ingestion.Handle{}, fmt.Errorf("incomplete prepared generation media")
	}
	assetID, err := newMediaAssetID()
	if err != nil {
		return ingestion.Handle{}, err
	}
	metadata, err := ingestionMetadata(request.Origin, ingestion.Completion{})
	if err != nil {
		return ingestion.Handle{}, fmt.Errorf("encode prepared media metadata: %w", err)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return ingestion.Handle{}, fmt.Errorf("begin prepared media registration: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	q := dbsqlc.New(tx)
	asset, err := q.CreatePreparedMediaAsset(ctx, dbsqlc.CreatePreparedMediaAssetParams{
		AssetID: assetID, OwnerUserID: request.OwnerUserID, StorageProfileID: request.StorageProfile,
		StorageKey: request.StorageKey, Url: request.URL, SourceUrl: request.SourceURL, MimeType: "",
		Width: 0, Height: 0, SizeBytes: 0, Metadata: metadata,
		ChecksumSha256: "", Status: "uploading",
		ReclaimAfter: sql.NullTime{Time: r.now().Add(reclamation.GracePeriod), Valid: true},
	})
	if err != nil {
		return ingestion.Handle{}, fmt.Errorf("register prepared media asset: %w", err)
	}
	if _, err := q.UpsertMediaRef(ctx, dbsqlc.UpsertMediaRefParams{MediaAssetID: asset, OwnerUserID: request.OwnerUserID, RefType: "prompt_reference", RefID: facts.RefID, TaskID: "", Role: facts.Role, InputIndex: int32(facts.InputIndex), Metadata: metadata}); err != nil {
		return ingestion.Handle{}, fmt.Errorf("register prepared media ref: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ingestion.Handle{}, fmt.Errorf("commit prepared media registration: %w", err)
	}
	committed = true
	return ingestionHandle(asset, assetID, false, request), nil
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

func (r *MediaAssetRegistry) enqueueDerivatives(ctx context.Context, ids []int64) {
	if r == nil || r.derivativeEnqueuer == nil {
		return
	}
	for _, id := range ids {
		if err := r.derivativeEnqueuer(ctx, id); err != nil {
			continue
		}
	}
}

func (r *MediaAssetRegistry) FindPreparedGenerationMedia(ctx context.Context, ownerUserID int64, refID, role string, inputIndex int) (PreparedGenerationMedia, bool, error) {
	if r == nil || r.q == nil {
		return PreparedGenerationMedia{}, false, fmt.Errorf("media asset registry is unavailable")
	}
	row, err := r.q.FindPreparedGenerationMedia(ctx, dbsqlc.FindPreparedGenerationMediaParams{OwnerUserID: ownerUserID, RefID: refID, Role: role, InputIndex: int32(inputIndex)})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PreparedGenerationMedia{}, false, nil
		}
		return PreparedGenerationMedia{}, false, err
	}
	return PreparedGenerationMedia{AssetID: row.ID, StorageProfile: row.StorageProfileID, StorageKey: row.StorageKey}, true, nil
}

func (r *MediaAssetRegistry) FindIngestingPreparedGenerationMedia(ctx context.Context, ownerUserID int64, refID, role string, inputIndex int) (PreparedGenerationMedia, bool, error) {
	if r == nil || r.q == nil {
		return PreparedGenerationMedia{}, false, fmt.Errorf("media asset registry is unavailable")
	}
	row, err := r.q.FindIngestingPreparedGenerationMedia(ctx, dbsqlc.FindIngestingPreparedGenerationMediaParams{OwnerUserID: ownerUserID, RefID: refID, Role: role, InputIndex: int32(inputIndex)})
	if errors.Is(err, sql.ErrNoRows) {
		return PreparedGenerationMedia{}, false, nil
	}
	if err != nil {
		return PreparedGenerationMedia{}, false, err
	}
	return PreparedGenerationMedia{AssetID: row.ID, StorageProfile: row.StorageProfileID, StorageKey: row.StorageKey}, true, nil
}

func (r *MediaAssetRegistry) FindReusableGenerationMedia(ctx context.Context, ownerUserID int64, source string) (int64, bool, error) {
	if r == nil || r.q == nil || ownerUserID <= 0 {
		return 0, false, fmt.Errorf("media asset registry is unavailable")
	}
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return 0, false, nil
	}
	if strings.HasPrefix(trimmed, "/uploads/") {
		resource, err := storage.ParseUploadResource(strings.TrimPrefix(trimmed, "/"))
		if err != nil {
			return 0, false, nil
		}
		ids, err := r.q.ListReadyOwnedMediaAssetIDsByStorageKey(ctx, dbsqlc.ListReadyOwnedMediaAssetIDsByStorageKeyParams{
			OwnerUserID: ownerUserID,
			StorageKey:  resource.ObjectKey,
		})
		if err != nil {
			return 0, false, fmt.Errorf("find reusable media asset by storage key: %w", err)
		}
		switch len(ids) {
		case 0:
			return 0, false, nil
		case 1:
			return ids[0], true, nil
		default:
			return 0, false, fmt.Errorf("reusable media asset storage key is ambiguous")
		}
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || !strings.HasPrefix(parsed.Path, "/uploads/") {
		return 0, false, nil
	}
	assetID, err := r.q.FindReadyOwnedMediaAssetIDByURL(ctx, dbsqlc.FindReadyOwnedMediaAssetIDByURLParams{
		OwnerUserID: ownerUserID,
		Url:         trimmed,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("find reusable media asset by URL: %w", err)
	}
	return assetID, true, nil
}

func (r *MediaAssetRegistry) LocateStoredUpload(ctx context.Context, lookup storage.StoredUploadLookup) (storage.StoredUploadLocation, bool, error) {
	if r == nil || r.q == nil {
		return storage.StoredUploadLocation{}, false, fmt.Errorf("media asset registry is unavailable")
	}
	if lookup.AssetID > 0 {
		row, err := r.q.GetReadyMediaAssetStorageLocationByID(ctx, lookup.AssetID)
		if err == nil {
			return storage.StoredUploadLocation{StorageProfileID: row.StorageProfileID, StorageKey: row.StorageKey}, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return storage.StoredUploadLocation{}, false, fmt.Errorf("locate media asset by ID: %w", err)
		}
		return storage.StoredUploadLocation{}, false, nil
	}
	if rawURL := strings.TrimSpace(lookup.URL); rawURL != "" {
		row, err := r.q.FindMediaAssetStorageLocationByURL(ctx, rawURL)
		if err == nil {
			return storage.StoredUploadLocation{
				StorageProfileID: row.StorageProfileID,
				StorageKey:       row.StorageKey,
			}, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return storage.StoredUploadLocation{}, false, fmt.Errorf("locate media asset by URL: %w", err)
		}
	}

	storageKey := strings.TrimSpace(lookup.ObjectKey)
	if storageKey == "" {
		return storage.StoredUploadLocation{}, false, nil
	}
	rows, err := r.q.ListMediaAssetStorageLocationsByKey(ctx, storageKey)
	if err != nil {
		return storage.StoredUploadLocation{}, false, fmt.Errorf("locate media asset by storage key: %w", err)
	}
	if len(rows) == 0 {
		return storage.StoredUploadLocation{}, false, nil
	}
	if len(rows) > 1 {
		return storage.StoredUploadLocation{}, false, fmt.Errorf("media asset storage key is ambiguous")
	}
	return storage.StoredUploadLocation{
		StorageProfileID: rows[0].StorageProfileID,
		StorageKey:       rows[0].StorageKey,
	}, true, nil
}

func (r *MediaAssetRegistry) LocateStoredDerivative(ctx context.Context, lookup storage.StoredDerivativeLookup) (storage.StoredUploadLocation, bool, error) {
	if r == nil || r.q == nil || lookup.AssetID <= 0 || strings.TrimSpace(lookup.Tier) == "" {
		return storage.StoredUploadLocation{}, false, fmt.Errorf("invalid media derivative lookup")
	}
	row, err := r.q.GetReadyMediaDerivativeStorageLocationByAssetID(ctx, dbsqlc.GetReadyMediaDerivativeStorageLocationByAssetIDParams{
		MediaAssetID: lookup.AssetID,
		Tier:         strings.TrimSpace(lookup.Tier),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return storage.StoredUploadLocation{}, false, nil
	}
	if err != nil {
		return storage.StoredUploadLocation{}, false, fmt.Errorf("locate media derivative: %w", err)
	}
	return storage.StoredUploadLocation{StorageProfileID: row.StorageProfileID, StorageKey: row.StorageKey}, true, nil
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
		if errors.Is(err, sql.ErrNoRows) {
			return mediaerrors.New(
				mediaerrors.CodeMediaProcessing,
				"media.derivative.validate_processing_input",
				fmt.Sprintf("media asset %d processing-input derivative is not yet scheduled", assetID),
				err,
			).WithFacts(fmt.Sprintf("%d", assetID), "", "", "", "", "")
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
