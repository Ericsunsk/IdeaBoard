package workspacemedia

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/media/imageformat"
)

type IngestImageParams struct {
	WorkspaceID            string
	OwnerUserID            int64
	FilenameStem           string
	Data                   []byte
	ExpectedChecksumSHA256 string
}

// IngestImage registers validated bytes through the upload session protocol.
// A completed asset may outlive a later caller-side transaction failure.
func IngestImage(ctx context.Context, sessions SessionStore, objects MediaObjects, params IngestImageParams) (asset *Asset, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if sessions == nil || objects == nil {
		return nil, errors.New("workspacemedia: image ingestion is not configured")
	}
	if len(params.Data) == 0 || int64(len(params.Data)) > MaxUploadSizeBytes {
		return nil, NewFieldValidationError("size_bytes")
	}
	facts, err := imageformat.InspectDetectedBytes(params.Data)
	if err != nil {
		return nil, fmt.Errorf("workspacemedia: unsupported image: %w", err)
	}
	sum := sha256.Sum256(params.Data)
	checksum := hex.EncodeToString(sum[:])
	if expected := NormalizeChecksum(params.ExpectedChecksumSHA256); expected != "" && checksum != expected {
		return nil, errors.New("图片校验失败: checksum mismatch")
	}
	workspaceID := strings.TrimSpace(params.WorkspaceID)
	session, err := sessions.CreateSession(ctx, CreateSessionParams{
		WorkspaceID: workspaceID, OwnerUserID: params.OwnerUserID,
		Filename: strings.TrimSpace(params.FilenameStem) + facts.Extension,
		MIMEType: facts.MIMEType, SizeBytes: int64(len(params.Data)),
		ChecksumSHA256: checksum, MediaKind: MediaKindImage,
	})
	if err != nil {
		return nil, fmt.Errorf("workspacemedia: reserve image upload: %w", err)
	}
	defer func() {
		if err != nil {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_, _ = sessions.FailSession(cleanupCtx, workspaceID, session.ID, "image byte ingest failed")
		}
	}()
	if err := objects.PutObjectAtProfile(ctx, DefaultStorageProfile, session.StorageKey, params.Data, facts.MIMEType); err != nil {
		return nil, fmt.Errorf("workspacemedia: store image bytes: %w", err)
	}
	_, asset, err = sessions.CompleteSession(ctx, workspaceID, session.ID, CompleteFacts{
		ChecksumSHA256: checksum, SizeBytes: int64(len(params.Data)),
		MIMEType: facts.MIMEType, MediaKind: MediaKindImage,
		Width: int32(facts.Width), Height: int32(facts.Height),
	}, CreateAssetParams{})
	if err != nil {
		return nil, fmt.Errorf("workspacemedia: complete image upload: %w", err)
	}
	return asset, nil
}
