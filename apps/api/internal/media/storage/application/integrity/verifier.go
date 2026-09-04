package integrity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	mediaerrors "ideaboard/api/internal/media/errors"
	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/ports"
)

type ObjectReaderAtProfile interface {
	ReadObjectAtProfile(ctx context.Context, profileID, objectKey string, maxBytes int64) (*ports.StoredUploadObject, error)
}

type IntegrityVerifier struct {
	reader   ObjectReaderAtProfile
	timeout  time.Duration
	maxBytes int64
}

type IntegrityConfig struct {
	Timeout  time.Duration
	MaxBytes int64
}

const (
	defaultIntegrityTimeout  = 30 * time.Second
	defaultIntegrityMaxBytes = 50 * 1024 * 1024
)

func NewIntegrityVerifier(reader ObjectReaderAtProfile, cfg IntegrityConfig) *IntegrityVerifier {
	v := &IntegrityVerifier{
		reader:  reader,
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

func (v *IntegrityVerifier) Verify(ctx context.Context, profileID, objectKey, expectedSHA256, expectedMIMEType string, expectedSize int64) error {
	if v == nil || v.reader == nil {
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
	obj, err := v.reader.ReadObjectAtProfile(verifyCtx, profileID, objectKey, v.maxBytes)
	if err != nil {
		if errors.Is(err, domain.ErrStoredObjectNotFound) {
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
			fmt.Sprintf("mime mismatch: got %q, want %q", obj.ContentType, expectedMIMEType),
			nil,
		)
	}
	hasher := sha256.New()
	hasher.Write(obj.Data)
	actual := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return mediaerrors.New(
			mediaerrors.CodeObjectInvalid,
			"media.storage.integrity",
			fmt.Sprintf("checksum mismatch: got %q, want %q", actual, expected),
			nil,
		)
	}
	return nil
}
