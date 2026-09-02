package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	mediaerrors "ideaboard/api/internal/media/errors"
)

// IntegrityVerifier is the post-write re-hash seam the
// materialization path uses to confirm the bytes that landed
// in storage match the bytes the worker sent. It is the
// durable guard against partial writes, network corruption,
// and any future adapter that returns success before flushing
// to disk.
//
// The verifier is intentionally read-only against the storage
// adapter: it reads the object back, hashes the result, and
// compares. A mismatch is a structured object_invalid error.
type IntegrityVerifier struct {
	storage  *StorageService
	timeout  time.Duration
	maxBytes int64
}

// IntegrityConfig is the bounded input to NewIntegrityVerifier.
type IntegrityConfig struct {
	Timeout  time.Duration
	MaxBytes int64
}

const (
	defaultIntegrityTimeout  = 30 * time.Second
	defaultIntegrityMaxBytes = 50 * 1024 * 1024
)

// NewIntegrityVerifier wires a verifier to the shared storage
// service. The storage service is the only legitimate reader
// of the object after write; the verifier MUST NOT bypass it.
func NewIntegrityVerifier(storage *StorageService, cfg IntegrityConfig) *IntegrityVerifier {
	v := &IntegrityVerifier{
		storage: storage,
		timeout: cfg.Timeout,
	}
	if v.timeout <= 0 {
		v.timeout = defaultIntegrityTimeout
	}
	if cfg.MaxBytes > 0 {
		v.maxBytes = cfg.MaxBytes
	} else {
		v.maxBytes = defaultIntegrityMaxBytes
	}
	return v
}

// Verify reads the object back and compares its SHA-256
// checksum against the supplied one. The verifier returns a
// structured media error on mismatch; callers can route the
// retry policy through the structured taxonomy. The verifier
// returns nil for an exact match and a typed error otherwise.
func (v *IntegrityVerifier) Verify(ctx context.Context, profileID, objectKey, expectedSHA256, expectedMIMEType string, expectedSize int64) error {
	if v == nil || v.storage == nil {
		return mediaerrors.New(
			mediaerrors.CodeObjectInvalid,
			"media.storage.integrity",
			"integrity verifier is not configured",
			nil,
		)
	}
	expected := strings.TrimSpace(expectedSHA256)
	if expected == "" {
		return mediaerrors.New(
			mediaerrors.CodeObjectInvalid,
			"media.storage.integrity",
			"expected checksum is empty",
			nil,
		)
	}
	verifyCtx, cancel := context.WithTimeout(ctx, v.timeout)
	defer cancel()
	obj, err := v.storage.ReadObjectAtProfile(verifyCtx, profileID, objectKey, v.maxBytes)
	if err != nil {
		if errors.Is(err, ErrStoredObjectNotFound) {
			return mediaerrors.New(
				mediaerrors.CodeObjectMissing,
				"media.storage.integrity",
				"object missing on integrity re-read",
				err,
			)
		}
		return mediaerrors.New(
			mediaerrors.CodeStorageTransient,
			"media.storage.integrity",
			"integrity re-read failed",
			err,
		)
	}
	if obj == nil {
		return mediaerrors.New(
			mediaerrors.CodeObjectMissing,
			"media.storage.integrity",
			"object is nil after re-read",
			nil,
		)
	}
	if expectedSize > 0 && int64(len(obj.Data)) != expectedSize {
		return mediaerrors.New(
			mediaerrors.CodeObjectInvalid,
			"media.storage.integrity",
			fmt.Sprintf("size mismatch: got %d, want %d", len(obj.Data), expectedSize),
			nil,
		)
	}
	if expectedMIMEType != "" && !strings.EqualFold(strings.TrimSpace(obj.ContentType), strings.TrimSpace(expectedMIMEType)) {
		return mediaerrors.New(
			mediaerrors.CodeObjectInvalid,
			"media.storage.integrity",
			fmt.Sprintf("MIME mismatch: got %q, want %q", obj.ContentType, expectedMIMEType),
			nil,
		)
	}
	hash := sha256.Sum256(obj.Data)
	got := hex.EncodeToString(hash[:])
	if got != expected {
		return mediaerrors.New(
			mediaerrors.CodeObjectInvalid,
			"media.storage.integrity",
			fmt.Sprintf("checksum mismatch: got %s, want %s", got, expected),
			nil,
		)
	}
	return nil
}
