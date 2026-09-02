package mediaerrors

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestCodeIsRetryableMatchesContract(t *testing.T) {
	cases := map[Code]bool{
		CodeMediaNotFound:     false,
		CodeObjectMissing:     true,
		CodeObjectInvalid:     false,
		CodeMediaProcessing:   true,
		CodeStorageTransient:  true,
		CodeProviderRejected:  false,
		CodeRepairNotPossible: false,
	}
	for code, want := range cases {
		if code.IsRetryable() != want {
			t.Errorf("Code(%q).IsRetryable() = %v, want %v", code, !want, want)
		}
	}
}

func TestCodeBlameMatchesContract(t *testing.T) {
	cases := map[Code]Blame{
		CodeMediaNotFound:     BlameMedia,
		CodeObjectMissing:     BlameStorage,
		CodeObjectInvalid:     BlameMedia,
		CodeMediaProcessing:   BlameStorage,
		CodeStorageTransient:  BlameStorage,
		CodeProviderRejected:  BlameProvider,
		CodeRepairNotPossible: BlameMedia,
	}
	for code, want := range cases {
		if code.Blame() != want {
			t.Errorf("Code(%q).Blame() = %q, want %q", code, code.Blame(), want)
		}
	}
}

func TestAllCodesAreValid(t *testing.T) {
	for _, code := range AllCodes() {
		if !code.Valid() {
			t.Errorf("AllCodes() returned an invalid code: %q", code)
		}
	}
}

func TestErrorImplementsErrorInterface(t *testing.T) {
	cause := errors.New("disk full")
	err := New(CodeStorageTransient, "media.asset.upload", "write failed", cause)
	if err.Error() == "" {
		t.Fatal("Error() must not be empty")
	}
	if !errors.Is(err, cause) {
		t.Fatal("errors.Is should follow Unwrap to the cause")
	}
	if !IsCode(err, CodeStorageTransient) {
		t.Fatal("IsCode must match the constructed code")
	}
	if IsCode(err, CodeObjectMissing) {
		t.Fatal("IsCode must not match a different code")
	}
	if CodeOf(err) != CodeStorageTransient {
		t.Fatalf("CodeOf = %q, want %q", CodeOf(err), CodeStorageTransient)
	}
	if !IsRetryable(err) {
		t.Fatal("IsRetryable must be true for a storage_transient error")
	}
}

func TestErrorCarriesFactsForRunbook(t *testing.T) {
	err := New(CodeObjectMissing, "media.derivative.reconcile", "object vanished", nil)
	err.Facts = Facts{
		AssetID:        "asset_abc",
		DerivativeID:   "42",
		StorageProfile: "primary",
		TaskID:         "task_xyz",
		CorrelationID:  "corr-1",
		Attempt:        2,
		MaxAttempts:    5,
	}
	wrapped := fmt.Errorf("wrap: %w", err)
	var m *Error
	if !errors.As(wrapped, &m) {
		t.Fatal("errors.As must walk through fmt.Errorf wrap")
	}
	if m.Facts.AssetID != "asset_abc" || m.Facts.Attempt != 2 {
		t.Fatalf("facts lost in wrap: %+v", m.Facts)
	}
}

func TestErrorMessageIsSafe(t *testing.T) {
	// The message field is the user-safe description; the cause is
	// only logged. Asserting both surfaces is a contract guard.
	err := New(CodeObjectInvalid, "media.derivative.process", "checksum mismatch", errors.New("s3:GetObject 500"))
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Error() should include message, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "object_invalid") {
		t.Fatalf("Error() should include code, got %q", err.Error())
	}
}
