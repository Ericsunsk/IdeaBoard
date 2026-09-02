package storage

import (
	"context"
	"testing"

	mediaerrors "ideaboard/api/internal/media/errors"
)

func TestIntegrityVerifierRejectsEmptyExpected(t *testing.T) {
	v := NewIntegrityVerifier(nil, IntegrityConfig{})
	if err := v.Verify(context.Background(), "p", "k", "", "", 0); !mediaerrors.IsCode(err, mediaerrors.CodeObjectInvalid) {
		t.Fatalf("expected object_invalid, got %v", err)
	}
}

func TestIntegrityVerifierIsRetryableContract(t *testing.T) {
	// The structured taxonomy classifies object_missing as
	// retryable because the bounded repair flow is the
	// canonical recovery path; the runbook pins the
	// distinction. The verifier only emits the structured
	// code; callers branch on it for retry policy.
	if !mediaerrors.CodeObjectMissing.IsRetryable() {
		t.Fatal("object_missing must be retryable for the bounded repair flow")
	}
	if !mediaerrors.CodeStorageTransient.IsRetryable() {
		t.Fatal("storage_transient must be retryable")
	}
	if !mediaerrors.CodeMediaProcessing.IsRetryable() {
		t.Fatal("media_processing must be retryable")
	}
	if mediaerrors.CodeMediaNotFound.IsRetryable() {
		t.Fatal("media_not_found must not be retryable")
	}
	if mediaerrors.CodeObjectInvalid.IsRetryable() {
		t.Fatal("object_invalid must not be retryable without explicit repair")
	}
	if mediaerrors.CodeProviderRejected.IsRetryable() {
		t.Fatal("provider_rejected must not be retryable")
	}
}
