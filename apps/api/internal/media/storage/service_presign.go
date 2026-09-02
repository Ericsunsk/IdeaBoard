package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Presign issues a short-lived signed URL for a single object on
// the requested storage profile. The implementation clamps the
// requested TTL to MaxPresignTTL, refuses unknown profiles, and
// stamps a fresh PresignID on every call. The signed URL is
// never returned in a slog line; callers MUST route the URL
// through worker.RedactSignedURL before logging it.
//
// This is the public port implementation for ObjectStore. The
// storage package owns the only legitimate signed-URL issuer;
// the worker and the runtime resolver MUST go through this
// method, never through the underlying SignedURLWithConfig.
func (s *StorageService) Presign(ctx context.Context, req PresignRequest) (PresignedURL, error) {
	if s == nil {
		return PresignedURL{}, fmt.Errorf("storage service is not configured")
	}
	if strings.TrimSpace(req.StorageProfileID) == "" {
		return PresignedURL{}, ErrPresignInvalidProfile
	}
	if strings.TrimSpace(req.ObjectKey) == "" {
		return PresignedURL{}, ErrPresignInvalidObjectKey
	}
	ttl, err := clampTTL(req.TTL)
	if err != nil {
		// Cap exceeded: fall back to the cap, never refuse.
		// The runbook pins this as the safe failure mode.
		ttl = MaxPresignTTL
	}
	profile, err := s.resolveProfileByID(ctx, req.StorageProfileID)
	if err != nil {
		return PresignedURL{}, fmt.Errorf("presign: %w", err)
	}
	if req.ExpectedSizeBytes <= 0 || strings.TrimSpace(req.ExpectedMIMEType) == "" || strings.TrimSpace(req.ExpectedChecksumSHA256) == "" || req.ExpectedObjectGeneration <= 0 {
		return PresignedURL{}, fmt.Errorf("presign: integrity facts are required")
	}
	if err := NewIntegrityVerifier(s, IntegrityConfig{}).Verify(ctx, req.StorageProfileID, req.ObjectKey, req.ExpectedChecksumSHA256, req.ExpectedMIMEType, req.ExpectedSizeBytes); err != nil {
		return PresignedURL{}, fmt.Errorf("presign: verify object: %w", err)
	}
	if profile.Provider == StorageProviderLocal {
		return PresignedURL{}, ErrPresignUnsupportedProvider
	}
	if err := s.s3CompatibleService.VerifyObjectGeneration(ctx, req.ObjectKey, req.ExpectedObjectGeneration, s.s3CompatibleConfigFromProfile(profile)); err != nil {
		return PresignedURL{}, fmt.Errorf("presign: verify object generation: %w", err)
	}
	signedURL, err := s.s3CompatibleService.SignedURLWithConfig(ctx, req.ObjectKey, ttl, s.s3CompatibleConfigFromProfile(profile))
	if err != nil {
		return PresignedURL{}, fmt.Errorf("presign: %w", err)
	}
	presignID, err := newPresignID()
	if err != nil {
		return PresignedURL{}, fmt.Errorf("presign: %w", err)
	}
	return PresignedURL{
		URL:              signedURL,
		ExpiresAt:        time.Now().UTC().Add(ttl),
		ObjectGeneration: req.ExpectedObjectGeneration,
		StorageProfileID: profile.ID,
		PresignID:        presignID,
	}, nil
}

func newPresignID() (string, error) {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate presign id: %w", err)
	}
	return "psg_" + hex.EncodeToString(buf[:]), nil
}

// Compile-time assertion: *StorageService implements ObjectStore.
var _ ObjectStore = (*StorageService)(nil)
