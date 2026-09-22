package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/media/storage/application/integrity"
	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/ports"
)

var _ ports.ObjectStore = (*Service)(nil)

func (s *Service) Presign(ctx context.Context, req ports.PresignRequest) (ports.PresignedURL, error) {
	if s == nil {
		return ports.PresignedURL{}, fmt.Errorf("storage service is not configured")
	}
	if strings.TrimSpace(req.StorageProfileID) == "" {
		return ports.PresignedURL{}, domain.ErrPresignInvalidProfile
	}
	if strings.TrimSpace(req.ObjectKey) == "" {
		return ports.PresignedURL{}, domain.ErrPresignInvalidObjectKey
	}
	ttl, err := ports.ClampTTL(req.TTL)
	if err != nil {
		ttl = ports.MaxPresignTTL
	}
	profile, err := s.profiles.ResolveProfileByID(ctx, req.StorageProfileID)
	if err != nil {
		return ports.PresignedURL{}, fmt.Errorf("presign: resolve storage profile %q: %w", req.StorageProfileID, err)
	}
	if profile.Provider == domain.StorageProviderLocal {
		return ports.PresignedURL{}, domain.ErrPresignUnsupportedProvider
	}
	if req.ExpectedSizeBytes <= 0 || strings.TrimSpace(req.ExpectedMIMEType) == "" || strings.TrimSpace(req.ExpectedChecksumSHA256) == "" || req.ExpectedObjectGeneration <= 0 {
		return ports.PresignedURL{}, fmt.Errorf("presign: integrity facts are required")
	}
	verifier := integrity.NewIntegrityVerifier(s, integrity.IntegrityConfig{})
	if err := verifier.Verify(ctx, req.StorageProfileID, req.ObjectKey, req.ExpectedChecksumSHA256, req.ExpectedMIMEType, req.ExpectedSizeBytes); err != nil {
		return ports.PresignedURL{}, fmt.Errorf("presign: verify object: %w", err)
	}
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return ports.PresignedURL{}, fmt.Errorf("presign: %w", err)
	}
	if err := driver.VerifyObjectGeneration(ctx, profile, req.ObjectKey, req.ExpectedObjectGeneration); err != nil {
		return ports.PresignedURL{}, fmt.Errorf("presign: verify object generation: %w", err)
	}
	signedURL, err := driver.SignedURL(ctx, profile, req.ObjectKey, ttl)
	if err != nil {
		return ports.PresignedURL{}, fmt.Errorf("presign: %w", err)
	}
	presignID, err := newPresignID()
	if err != nil {
		return ports.PresignedURL{}, fmt.Errorf("presign: %w", err)
	}
	return ports.PresignedURL{
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
