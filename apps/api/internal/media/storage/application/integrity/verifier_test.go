package integrity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"ideaboard/api/internal/media/storage/ports"
)

type stubObjectReader struct {
	obj *ports.StoredUploadObject
	err error
}

func (s *stubObjectReader) ReadObjectAtProfile(ctx context.Context, profileID, objectKey string, maxBytes int64) (*ports.StoredUploadObject, error) {
	return s.obj, s.err
}

func TestIntegrityVerifierSuccess(t *testing.T) {
	payload := []byte("hello integrity verification")
	hash := sha256.Sum256(payload)
	checksum := hex.EncodeToString(hash[:])

	reader := &stubObjectReader{
		obj: &ports.StoredUploadObject{
			ObjectKey:   "uploads/test.png",
			ContentType: "image/png",
			Data:        payload,
		},
	}

	verifier := NewIntegrityVerifier(reader, IntegrityConfig{Timeout: time.Second})
	err := verifier.Verify(context.Background(), "p1", "uploads/test.png", checksum, "image/png", int64(len(payload)))
	if err != nil {
		t.Fatalf("expected verification success, got: %v", err)
	}
}

func TestIntegrityVerifierMismatch(t *testing.T) {
	payload := []byte("hello integrity verification")
	reader := &stubObjectReader{
		obj: &ports.StoredUploadObject{
			ObjectKey:   "uploads/test.png",
			ContentType: "image/png",
			Data:        payload,
		},
	}

	verifier := NewIntegrityVerifier(reader, IntegrityConfig{})
	err := verifier.Verify(context.Background(), "p1", "uploads/test.png", "wrong-checksum", "image/png", int64(len(payload)))
	if err == nil {
		t.Fatal("expected error on checksum mismatch")
	}
}
