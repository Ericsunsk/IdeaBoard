package workspacemedia

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5/pgtype"
)

// sessionExpiryDeadline computes the lazy-expiry deadline from the
// creation timestamp and current status.
func sessionExpiryDeadline(createdAt time.Time, status SessionStatus) time.Time {
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
		Status:         SessionStatus(row.Status),
		StorageKey:     slot,
		CreatedAt:      createdAt,
		CompletedAt:    completedAt,
		ExpiredAt:      expiredAt,
		ExpiresAt:      sessionExpiryDeadline(createdAt, SessionStatus(row.Status)),
		UpdatedAt:      updatedAt,
	}
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
	// Resolve the upload target in the same Workspace; HTTP checks Board write access.
	targetBoardID, err := resolveTargetBoardInternal(ctx, q, params.WorkspaceID, params.TargetBoardPublicID)
	if err != nil {
		return nil, err
	}
	canonicalMIME := CanonicalMIME(mimeType)
	minted, err := contract.MintPublicID(contract.PrefixUpload, 12)
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
			TargetBoardID:     targetBoardID,
		})
		if isSessionPublicIDConflict(err) {
			minted, err = contract.MintPublicID(contract.PrefixUpload, 12)
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
	if !contract.IsUploadID(strings.TrimSpace(uploadID)) {
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
	switch SessionStatus(row.Status) {
	case StatusPending, StatusTransferring:
		createdAt := dbutil.TimeValue(row.CreatedAt)
		if !createdAt.IsZero() && !now.Before(sessionExpiryDeadline(createdAt, SessionStatus(row.Status))) {
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
		if SessionStatus(row.Status) == StatusPending {
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
	if !contract.IsUploadID(strings.TrimSpace(uploadID)) {
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
		if SessionStatus(row.Status) != StatusPending && SessionStatus(row.Status) != StatusTransferring {
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
		if !contract.IsAssetID(minted) {
			minted, err = contract.MintPublicID(contract.PrefixAsset, 12)
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
				Width:           dbutil.Int4FromPresent(facts.Width, facts.Width > 0),
				Height:          dbutil.Int4FromPresent(facts.Height, facts.Height > 0),
				DurationMs:      pgtype.Int4{},
				Metadata:        []byte(`{}`),
			})
			if isAssetPublicIDConflict(err) {
				minted, err = contract.MintPublicID(contract.PrefixAsset, 12)
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
		if SessionStatus(current.Status) == StatusFailed {
			return sessionFromRow(trimmedWorkspace, current), nil
		}
		return nil, ErrStateConflict
	}
	if err != nil {
		return nil, err
	}
	return sessionFromRow(trimmedWorkspace, failed), nil
}
