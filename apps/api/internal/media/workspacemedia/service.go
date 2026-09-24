package workspacemedia

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Service is the postgres WorkspaceMediaStore (#820, Phase 4-03 of
// #809). Repository methods take context.Context first and use the
// dbWithContext pattern. It must never be replaced by the test-only
// InMemoryWorkspaceMediaStore in production wiring.
//
// Workspace scoping: every lookup resolves the workspace public id to
// its internal id first and keys all session/asset statements by
// workspace. Cross-workspace ids miss by construction and read as not
// found; unknown workspaces list empty and mutate as not found.
// Membership itself is enforced by the v1 handlers
// (RequireWorkspaceCapability) before the store is reached; the store
// guarantees no cross-workspace reference escapes.
//
// Expiry is lazy: GetSession flips pending/transferring rows past
// their TTL to expired on read (UploadPendingTTL/UploadTransferTTL
// from created_at). No background sweeper exists by design.
type Service struct {
	db *pgxpool.Pool
	q  *dbsqlc.Queries
}

// Compile-time convergence with the v1 persistence seam.
var _ WorkspaceMediaStore = (*Service)(nil)

// errWorkspaceNotFound stays unexported: callers map it to the
// session/asset not-found error of their operation so hidden
// workspaces never leak a distinct status.
var errWorkspaceNotFound = errors.New("workspacemedia workspace not found")

// errNotConfigured reports a Service built without a database pool.
var errNotConfigured = errors.New("workspacemedia service is not configured")

// List caps: every list query carries a LIMIT so no caller can trigger
// an unbounded scan.
const maxMediaAssetRows = int32(100)

// mintAttempts bounds public_id regeneration after a random collision.
const mintAttempts = 3

// NewService builds a postgres media Service over pool.
func NewService(db *pgxpool.Pool) *Service {
	if db == nil {
		return &Service{}
	}
	return &Service{db: db, q: dbsqlc.New(db)}
}

type serviceQueryer struct {
	ctx context.Context
	db  *pgxpool.Pool
	q   *dbsqlc.Queries
}

func (s *Service) dbWithContext(ctx context.Context) serviceQueryer {
	return serviceQueryer{ctx: ctx, db: s.db, q: s.q}
}

func (db serviceQueryer) withTx(run func(*dbsqlc.Queries) error) error {
	return dbutil.WithTx(db.ctx, db.db, run)
}

func (db serviceQueryer) queries() (*dbsqlc.Queries, error) {
	if db.q == nil {
		return nil, errNotConfigured
	}
	return db.q, nil
}

// resolveWorkspaceInternal maps an opaque workspace public id to its
// internal id. Unknown workspaces report errWorkspaceNotFound so
// callers converge to their operation's not-found error.
func resolveWorkspaceInternal(ctx context.Context, q *dbsqlc.Queries, workspacePublicID string) (int64, error) {
	row, err := q.GetWorkspaceByPublicID(ctx, strings.TrimSpace(workspacePublicID))
	if database.IsNotFound(err) {
		return 0, errWorkspaceNotFound
	}
	if err != nil {
		return 0, err
	}
	return row.InternalID, nil
}

func nullText(value string) pgtype.Text {
	if strings.TrimSpace(value) == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func nullInt4(value int32, present bool) pgtype.Int4 {
	if !present || value < 0 {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: value, Valid: true}
}

func int4Value(value pgtype.Int4) int32 {
	if !value.Valid || value.Int32 < 0 {
		return 0
	}
	return value.Int32
}

// sessionExpiryDeadline computes the lazy-expiry deadline from the
// creation timestamp and current status.
func sessionExpiryDeadline(createdAt time.Time, status string) time.Time {
	if status == StatusTransferring {
		return createdAt.Add(UploadTransferTTL)
	}
	return createdAt.Add(UploadPendingTTL)
}

func sessionFromRow(workspacePublicID string, row dbsqlc.UploadSession) *Session {
	createdAt := dbutil.TimeValue(row.CreatedAt)
	completedAt := dbutil.TimePtr(row.CompletedAt)
	expiredAt := dbutil.TimePtr(row.ExpiredAt)
	slot := ""
	if row.StorageKey.Valid {
		slot = row.StorageKey.String
	}
	checksum := ""
	if row.DeclaredChecksum.Valid {
		checksum = row.DeclaredChecksum.String
	}
	updatedAt := createdAt
	if completedAt != nil && completedAt.After(updatedAt) {
		updatedAt = *completedAt
	}
	if expiredAt != nil && expiredAt.After(updatedAt) {
		updatedAt = *expiredAt
	}
	return &Session{
		ID:             row.PublicID,
		WorkspaceID:    strings.TrimSpace(workspacePublicID),
		OwnerUserID:    row.UploaderUserID,
		Filename:       SlotFilename(slot),
		MIMEType:       row.DeclaredMimeType,
		SizeBytes:      row.DeclaredSizeBytes,
		ChecksumSHA256: checksum,
		MediaKind:      MediaKindForMIME(row.DeclaredMimeType),
		Status:         row.Status,
		StorageKey:     slot,
		CreatedAt:      createdAt,
		CompletedAt:    completedAt,
		ExpiredAt:      expiredAt,
		ExpiresAt:      sessionExpiryDeadline(createdAt, row.Status),
		UpdatedAt:      updatedAt,
	}
}

func assetFromRow(workspacePublicID string, row dbsqlc.MediaAsset) *Asset {
	return &Asset{
		InternalID:     row.InternalID,
		ID:             row.PublicID,
		WorkspaceID:    strings.TrimSpace(workspacePublicID),
		OwnerUserID:    row.OwnerUserID,
		Status:         AssetStatusReady,
		MediaKind:      row.MediaKind,
		MIMEType:       row.MimeType,
		Width:          int4Value(row.Width),
		Height:         int4Value(row.Height),
		SizeBytes:      row.SizeBytes,
		ChecksumSHA256: row.Checksum,
		StorageKey:     row.StorageKey,
		ArchivedAt:     dbutil.TimePtr(row.ArchivedAt),
		CreatedAt:      dbutil.TimeValue(row.CreatedAt),
	}
}

func derivativeFromRow(row dbsqlc.MediaAssetDerivative) *Derivative {
	return &Derivative{
		VariantName: row.VariantName,
		MIMEType:    row.MimeType,
		Width:       int4Value(row.Width),
		Height:      int4Value(row.Height),
		SizeBytes:   row.SizeBytes,
		StorageKey:  row.StorageKey,
		CreatedAt:   dbutil.TimeValue(row.CreatedAt),
	}
}

func isAssetPublicIDConflict(err error) bool {
	if !database.IsDuplicateConstraintError(err) {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "uq_media_assets_public_id")
}

func isSessionPublicIDConflict(err error) bool {
	if !database.IsDuplicateConstraintError(err) {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "uq_upload_sessions_public_id")
}

// CreateSession validates the declaration, reserves the server-side
// slot eagerly (storage_key), and inserts the pending row. The slot
// reservation fixes the transfer target at create time; bytes
// themselves land out of band.
func (s *Service) CreateSession(ctx context.Context, params CreateSessionParams) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	filename := strings.TrimSpace(params.Filename)
	mimeType := strings.TrimSpace(params.MIMEType)
	checksum := NormalizeChecksum(params.ChecksumSHA256)
	kind := strings.TrimSpace(params.MediaKind)
	if kind == "" {
		kind = MediaKindImage
	}
	if validateErr := ValidateCreateInput(filename, mimeType, params.SizeBytes, checksum, kind); validateErr != nil {
		return nil, validateErr
	}
	if params.OwnerUserID <= 0 {
		return nil, NewFieldValidationError("owner")
	}
	workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, params.WorkspaceID)
	if err != nil {
		if errors.Is(err, errWorkspaceNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	canonicalMIME := CanonicalMIME(mimeType)
	minted, err := NewUploadPublicID()
	if err != nil {
		return nil, err
	}
	slot := SlotKey(strings.TrimSpace(params.WorkspaceID), minted, filename)
	for attempt := 0; attempt < mintAttempts; attempt++ {
		row, err := q.CreateUploadSession(ctx, dbsqlc.CreateUploadSessionParams{
			PublicID:          minted,
			WorkspaceID:       workspaceInternalID,
			UploaderUserID:    params.OwnerUserID,
			DeclaredMimeType:  canonicalMIME,
			DeclaredSizeBytes: params.SizeBytes,
			DeclaredChecksum:  nullText(checksum),
			StorageKey:        nullText(slot),
		})
		if isSessionPublicIDConflict(err) {
			minted, err = NewUploadPublicID()
			if err != nil {
				return nil, err
			}
			slot = SlotKey(strings.TrimSpace(params.WorkspaceID), minted, filename)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("workspacemedia: create upload session: %w", err)
		}
		return sessionFromRow(strings.TrimSpace(params.WorkspaceID), row), nil
	}
	return nil, errors.New("workspacemedia: upload public id mint collision, retry the request")
}

// getSessionRow loads the scoped session row without lifecycle effects.
func getSessionRow(ctx context.Context, q *dbsqlc.Queries, workspacePublicID, uploadID string) (dbsqlc.UploadSession, error) {
	if !IsUploadID(strings.TrimSpace(uploadID)) {
		return dbsqlc.UploadSession{}, ErrNotFound
	}
	row, err := q.GetUploadSessionByPublicID(ctx, dbsqlc.GetUploadSessionByPublicIDParams{
		PublicID:          strings.TrimSpace(uploadID),
		WorkspacePublicID: strings.TrimSpace(workspacePublicID),
	})
	if database.IsNotFound(err) {
		return dbsqlc.UploadSession{}, ErrNotFound
	}
	if err != nil {
		return dbsqlc.UploadSession{}, err
	}
	return row, nil
}

// applyLazySessionEffects flips pending -> transferring on first read
// and expires rows past their TTL. It returns the current row plus a
// terminal marker: expired rows report ErrExpired.
func applyLazySessionEffects(ctx context.Context, q *dbsqlc.Queries, workspacePublicID string, row dbsqlc.UploadSession, now time.Time) (dbsqlc.UploadSession, error) {
	switch row.Status {
	case StatusPending, StatusTransferring:
		createdAt := dbutil.TimeValue(row.CreatedAt)
		if !createdAt.IsZero() && !now.Before(sessionExpiryDeadline(createdAt, row.Status)) {
			expired, err := q.ExpireUploadSession(ctx, dbsqlc.ExpireUploadSessionParams{
				PublicID:          row.PublicID,
				WorkspacePublicID: strings.TrimSpace(workspacePublicID),
			})
			if database.IsNotFound(err) {
				return getSessionRow(ctx, q, workspacePublicID, row.PublicID)
			}
			if err != nil {
				return dbsqlc.UploadSession{}, err
			}
			return expired, ErrExpired
		}
		if row.Status == StatusPending {
			moved, err := q.MarkUploadSessionTransferring(ctx, dbsqlc.MarkUploadSessionTransferringParams{
				PublicID:          row.PublicID,
				WorkspacePublicID: strings.TrimSpace(workspacePublicID),
			})
			if database.IsNotFound(err) {
				return getSessionRow(ctx, q, workspacePublicID, row.PublicID)
			}
			if err != nil {
				return dbsqlc.UploadSession{}, err
			}
			return moved, nil
		}
		return row, nil
	case StatusExpired:
		return row, ErrExpired
	default:
		return row, nil
	}
}

// GetSession resolves a workspace-scoped session with lazy lifecycle
// effects. Cross-workspace and malformed ids read as ErrNotFound;
// expired rows read as ErrExpired (410 at the handler).
func (s *Service) GetSession(ctx context.Context, workspaceID, uploadID string) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	row, err := getSessionRow(ctx, q, workspaceID, uploadID)
	if err != nil {
		return nil, err
	}
	row, effectErr := applyLazySessionEffects(ctx, q, workspaceID, row, time.Now().UTC())
	if effectErr != nil && !errors.Is(effectErr, ErrExpired) {
		return nil, effectErr
	}
	session := sessionFromRow(strings.TrimSpace(workspaceID), row)
	if errors.Is(effectErr, ErrExpired) {
		return session, ErrExpired
	}
	return session, nil
}

// matchCompletionFacts enforces the checksum/size/kind contract: the
// completion echo must equal the session declarations (after
// canonicalization). MIME/kind echoes must match when present.
func matchCompletionFacts(session *Session, facts CompleteFacts) error {
	if NormalizeChecksum(facts.ChecksumSHA256) != session.ChecksumSHA256 {
		return NewFieldValidationError("checksum_sha256")
	}
	if facts.SizeBytes != session.SizeBytes {
		return NewFieldValidationError("size_bytes")
	}
	if strings.TrimSpace(facts.MIMEType) != "" && CanonicalMIME(facts.MIMEType) != session.MIMEType {
		return NewFieldValidationError("mime_type")
	}
	kind := strings.TrimSpace(facts.MediaKind)
	if kind != "" && kind != session.MediaKind {
		return NewFieldValidationError("media_kind")
	}
	return nil
}

// CompleteSession atomically inserts the durable asset row and flips
// the session to completed in one transaction (completion-only
// durability). Facts must echo the declarations; mismatches fail the
// session and report a ValidationError. Terminal sessions report
// ErrStateConflict.
func (s *Service) CompleteSession(ctx context.Context, workspaceID, uploadID string, facts CompleteFacts, asset CreateAssetParams) (*Session, *Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	db := s.dbWithContext(ctx)
	if _, err := db.queries(); err != nil {
		return nil, nil, err
	}
	trimmedWorkspace := strings.TrimSpace(workspaceID)
	if !IsUploadID(strings.TrimSpace(uploadID)) {
		return nil, nil, ErrNotFound
	}
	if facts.Width < 0 || facts.Height < 0 || facts.Width > 32768 || facts.Height > 32768 {
		return nil, nil, NewFieldValidationError("dimensions")
	}
	var completed *Session
	var created *Asset
	err := db.withTx(func(q *dbsqlc.Queries) error {
		row, err := getSessionRow(ctx, q, trimmedWorkspace, uploadID)
		if err != nil {
			return err
		}
		live, effectErr := applyLazySessionEffects(ctx, q, trimmedWorkspace, row, time.Now().UTC())
		if errors.Is(effectErr, ErrExpired) {
			return ErrExpired
		}
		if effectErr != nil {
			return effectErr
		}
		row = live
		if row.Status != StatusPending && row.Status != StatusTransferring {
			return ErrStateConflict
		}
		session := sessionFromRow(trimmedWorkspace, row)
		if mismatch := matchCompletionFacts(session, facts); mismatch != nil {
			if _, failErr := q.FailUploadSession(ctx, dbsqlc.FailUploadSessionParams{
				PublicID:          row.PublicID,
				WorkspacePublicID: trimmedWorkspace,
			}); failErr != nil && !database.IsNotFound(failErr) {
				return failErr
			}
			return mismatch
		}
		minted := strings.TrimSpace(asset.PublicID)
		if !IsAssetID(minted) {
			minted, err = NewAssetPublicID()
			if err != nil {
				return err
			}
		}
		slot := session.StorageKey
		if strings.TrimSpace(slot) == "" {
			slot = SlotKey(trimmedWorkspace, session.ID, session.Filename)
		}
		workspaceInternalID, err := resolveWorkspaceInternal(ctx, q, trimmedWorkspace)
		if err != nil {
			if errors.Is(err, errWorkspaceNotFound) {
				return ErrNotFound
			}
			return err
		}
		for attempt := 0; attempt < mintAttempts; attempt++ {
			inserted, err := q.CreateMediaAsset(ctx, dbsqlc.CreateMediaAssetParams{
				PublicID:        minted,
				WorkspaceID:     workspaceInternalID,
				UploadSessionID: row.InternalID,
				OwnerUserID:     session.OwnerUserID,
				StorageKey:      slot,
				MimeType:        session.MIMEType,
				SizeBytes:       session.SizeBytes,
				Checksum:        session.ChecksumSHA256,
				MediaKind:       session.MediaKind,
				Width:           nullInt4(facts.Width, facts.Width > 0),
				Height:          nullInt4(facts.Height, facts.Height > 0),
				DurationMs:      pgtype.Int4{},
				Metadata:        []byte(`{}`),
			})
			if isAssetPublicIDConflict(err) {
				minted, err = NewAssetPublicID()
				if err != nil {
					return err
				}
				continue
			}
			if err != nil {
				if _, failErr := q.FailUploadSession(ctx, dbsqlc.FailUploadSessionParams{
					PublicID:          row.PublicID,
					WorkspacePublicID: trimmedWorkspace,
				}); failErr != nil && !database.IsNotFound(failErr) {
					return failErr
				}
				return fmt.Errorf("workspacemedia: create media asset: %w", err)
			}
			created = assetFromRow(trimmedWorkspace, inserted)
			break
		}
		if created == nil {
			return errors.New("workspacemedia: asset public id mint collision, retry the request")
		}
		done, err := q.CompleteUploadSession(ctx, dbsqlc.CompleteUploadSessionParams{
			StorageKey:        pgtype.Text{String: slot, Valid: true},
			PublicID:          row.PublicID,
			WorkspacePublicID: trimmedWorkspace,
		})
		if err != nil {
			if database.IsNotFound(err) {
				return ErrStateConflict
			}
			return err
		}
		completed = sessionFromRow(trimmedWorkspace, done)
		completed.AssetID = created.ID
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return completed, created, nil
}

// FailSession marks a live session failed. Terminal sessions report
// ErrStateConflict; invisible ids report ErrNotFound.
func (s *Service) FailSession(ctx context.Context, workspaceID, uploadID, reason string) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	_ = strings.TrimSpace(reason)
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	trimmedWorkspace := strings.TrimSpace(workspaceID)
	row, err := getSessionRow(ctx, q, trimmedWorkspace, uploadID)
	if err != nil {
		return nil, err
	}
	live, effectErr := applyLazySessionEffects(ctx, q, trimmedWorkspace, row, time.Now().UTC())
	if errors.Is(effectErr, ErrExpired) {
		return sessionFromRow(trimmedWorkspace, live), ErrExpired
	}
	if effectErr != nil {
		return nil, effectErr
	}
	failed, err := q.FailUploadSession(ctx, dbsqlc.FailUploadSessionParams{
		PublicID:          live.PublicID,
		WorkspacePublicID: trimmedWorkspace,
	})
	if database.IsNotFound(err) {
		current, getErr := getSessionRow(ctx, q, trimmedWorkspace, uploadID)
		if getErr != nil {
			return nil, getErr
		}
		if current.Status == StatusFailed {
			return sessionFromRow(trimmedWorkspace, current), nil
		}
		return nil, ErrStateConflict
	}
	if err != nil {
		return nil, err
	}
	return sessionFromRow(trimmedWorkspace, failed), nil
}

// GetAsset resolves a workspace-scoped asset. Cross-workspace and
// malformed ids read as ErrNotFound; archived assets stay readable.
func (s *Service) GetAsset(ctx context.Context, workspaceID, assetID string) (*Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	if !IsAssetID(strings.TrimSpace(assetID)) {
		return nil, ErrNotFound
	}
	row, err := q.GetMediaAssetByPublicID(ctx, dbsqlc.GetMediaAssetByPublicIDParams{
		PublicID:          strings.TrimSpace(assetID),
		WorkspacePublicID: strings.TrimSpace(workspaceID),
	})
	if database.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return assetFromRow(strings.TrimSpace(workspaceID), row), nil
}

// ListAssets pages live workspace assets newest-first. Unknown
// workspaces list empty, matching the in-memory store.
func (s *Service) ListAssets(ctx context.Context, workspaceID string, limit, offset int) ([]*Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	if limit < 1 {
		limit = 1
	}
	if limit > int(maxMediaAssetRows) {
		limit = int(maxMediaAssetRows)
	}
	if offset < 0 {
		offset = 0
	}
	trimmedWorkspace := strings.TrimSpace(workspaceID)
	if _, err := resolveWorkspaceInternal(ctx, q, trimmedWorkspace); err != nil {
		if errors.Is(err, errWorkspaceNotFound) {
			return []*Asset{}, nil
		}
		return nil, err
	}
	rows, err := q.ListMediaAssets(ctx, dbsqlc.ListMediaAssetsParams{
		WorkspacePublicID: trimmedWorkspace,
		RowLimit:          int32(limit),
		RowOffset:         int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("workspacemedia: list media assets: %w", err)
	}
	out := make([]*Asset, 0, len(rows))
	for _, row := range rows {
		out = append(out, assetFromRow(trimmedWorkspace, row))
	}
	return out, nil
}

// ArchiveAsset stamps the archive lifecycle marker. Repeat archives
// converge on the stored row so idempotent replays never 404.
func (s *Service) ArchiveAsset(ctx context.Context, workspaceID, assetID string) (*Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	trimmedWorkspace := strings.TrimSpace(workspaceID)
	if !IsAssetID(strings.TrimSpace(assetID)) {
		return nil, ErrNotFound
	}
	row, err := q.ArchiveMediaAsset(ctx, dbsqlc.ArchiveMediaAssetParams{
		PublicID:          strings.TrimSpace(assetID),
		WorkspacePublicID: trimmedWorkspace,
	})
	if database.IsNotFound(err) {
		current, getErr := getSessionRowAsset(ctx, q, trimmedWorkspace, assetID)
		if getErr != nil {
			return nil, getErr
		}
		return assetFromRow(trimmedWorkspace, current), nil
	}
	if err != nil {
		return nil, err
	}
	return assetFromRow(trimmedWorkspace, row), nil
}

func getSessionRowAsset(ctx context.Context, q *dbsqlc.Queries, workspacePublicID, assetID string) (dbsqlc.MediaAsset, error) {
	row, err := q.GetMediaAssetByPublicID(ctx, dbsqlc.GetMediaAssetByPublicIDParams{
		PublicID:          strings.TrimSpace(assetID),
		WorkspacePublicID: strings.TrimSpace(workspacePublicID),
	})
	if database.IsNotFound(err) {
		return dbsqlc.MediaAsset{}, ErrNotFound
	}
	if err != nil {
		return dbsqlc.MediaAsset{}, err
	}
	return row, nil
}

// UpdateAssetMetadata refreshes dimensions and merges metadata
// (unknown keys from other writers survive the merge).
func (s *Service) UpdateAssetMetadata(ctx context.Context, workspaceID, assetID string, params UpdateMetadataParams) (*Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	trimmedWorkspace := strings.TrimSpace(workspaceID)
	if !IsAssetID(strings.TrimSpace(assetID)) {
		return nil, ErrNotFound
	}
	merge := params.Metadata
	if merge == nil {
		merge = map[string]any{}
	}
	raw, err := json.Marshal(merge)
	if err != nil {
		return nil, fmt.Errorf("workspacemedia: encode asset metadata: %w", err)
	}
	row, err := q.UpdateMediaAssetMetadata(ctx, dbsqlc.UpdateMediaAssetMetadataParams{
		// Zero/negative dimensions normalize to NULL (unknown):
		// the image CHECK requires width/height > 0 when present.
		Width:             nullInt4(params.Width, params.HasDims && params.Width > 0),
		Height:            nullInt4(params.Height, params.HasDims && params.Height > 0),
		DurationMs:        nullInt4(params.DurationMs, params.HasDur),
		Metadata:          raw,
		PublicID:          strings.TrimSpace(assetID),
		WorkspacePublicID: trimmedWorkspace,
	})
	if database.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return assetFromRow(trimmedWorkspace, row), nil
}

// RecordDerivative upserts one materialized derivative variant for a
// workspace-scoped asset. Regeneration converges on
// (asset_id, variant_name).
func (s *Service) RecordDerivative(ctx context.Context, workspaceID, assetID string, derivative Derivative) (*Derivative, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	trimmedWorkspace := strings.TrimSpace(workspaceID)
	asset, err := getSessionRowAsset(ctx, q, trimmedWorkspace, assetID)
	if err != nil {
		return nil, err
	}
	variant := strings.TrimSpace(derivative.VariantName)
	if variant == "" {
		return nil, NewFieldValidationError("variant")
	}
	slot := strings.TrimSpace(derivative.StorageKey)
	if slot == "" {
		return nil, NewFieldValidationError("storage_key")
	}
	row, err := q.UpsertMediaAssetDerivative(ctx, dbsqlc.UpsertMediaAssetDerivativeParams{
		AssetID:     asset.InternalID,
		VariantName: variant,
		StorageKey:  slot,
		MimeType:    strings.TrimSpace(derivative.MIMEType),
		SizeBytes:   derivative.SizeBytes,
		Width:       nullInt4(derivative.Width, derivative.Width > 0),
		Height:      nullInt4(derivative.Height, derivative.Height > 0),
	})
	if err != nil {
		return nil, fmt.Errorf("workspacemedia: record derivative: %w", err)
	}
	return derivativeFromRow(row), nil
}

// ListDerivatives returns stored variants for a workspace-scoped
// asset. Unknown assets read as ErrNotFound.
func (s *Service) ListDerivatives(ctx context.Context, workspaceID, assetID string) ([]*Derivative, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	asset, err := getSessionRowAsset(ctx, q, strings.TrimSpace(workspaceID), assetID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListMediaAssetDerivatives(ctx, asset.InternalID)
	if err != nil {
		return nil, fmt.Errorf("workspacemedia: list derivatives: %w", err)
	}
	out := make([]*Derivative, 0, len(rows))
	for _, row := range rows {
		out = append(out, derivativeFromRow(row))
	}
	return out, nil
}

// outputRole extracts the #819 output role from task_outputs.metadata.
func outputRole(raw []byte) string {
	var decoded struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return ""
	}
	return strings.TrimSpace(decoded.Role)
}

// ListTaskAssets queries media by task via task_outputs (media_asset
// outputs only, ordered by output_index). It reuses #819's output
// mapping (asset public-id in metadata, role projection) without
// duplicating it: the asset rows come from the media tables, the role
// from the output metadata envelope.
func (s *Service) ListTaskAssets(ctx context.Context, workspaceID, taskID string) ([]*TaskAsset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	trimmedWorkspace := strings.TrimSpace(workspaceID)
	if !contract.IsTaskID(strings.TrimSpace(taskID)) {
		return nil, ErrNotFound
	}
	rows, err := q.ListTaskMediaAssets(ctx, dbsqlc.ListTaskMediaAssetsParams{
		PublicID:          strings.TrimSpace(taskID),
		WorkspacePublicID: trimmedWorkspace,
	})
	if err != nil {
		return nil, fmt.Errorf("workspacemedia: list task media: %w", err)
	}
	out := make([]*TaskAsset, 0, len(rows))
	for _, row := range rows {
		out = append(out, &TaskAsset{
			Asset: &Asset{
				InternalID:     row.InternalID,
				ID:             row.PublicID,
				WorkspaceID:    trimmedWorkspace,
				OwnerUserID:    row.OwnerUserID,
				Status:         AssetStatusReady,
				MediaKind:      row.MediaKind,
				MIMEType:       row.MimeType,
				Width:          int4Value(row.Width),
				Height:         int4Value(row.Height),
				SizeBytes:      row.SizeBytes,
				ChecksumSHA256: row.Checksum,
				StorageKey:     row.StorageKey,
				ArchivedAt:     dbutil.TimePtr(row.ArchivedAt),
				CreatedAt:      dbutil.TimeValue(row.CreatedAt),
			},
			OutputIndex: row.OutputIndex,
			Role:        outputRole(row.OutputMetadata),
		})
	}
	return out, nil
}
