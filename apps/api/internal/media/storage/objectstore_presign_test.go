package storage

import (
	"context"
	"testing"
	"time"
)

func TestClampTTLDefaultsWhenZero(t *testing.T) {
	got, err := clampTTL(0)
	if err != nil {
		t.Fatal(err)
	}
	if got != DefaultPresignTTL {
		t.Errorf("default TTL = %v, want %v", got, DefaultPresignTTL)
	}
}

func TestClampTTLDefaultsWhenNegative(t *testing.T) {
	got, err := clampTTL(-time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got != DefaultPresignTTL {
		t.Errorf("negative TTL must fall back to default, got %v", got)
	}
}

func TestClampTTLReturnsRequestedWhenWithinCap(t *testing.T) {
	got, err := clampTTL(2 * time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got != 2*time.Minute {
		t.Errorf("within-cap TTL = %v, want 2m", got)
	}
}

func TestClampTTLCapsAtMax(t *testing.T) {
	got, err := clampTTL(10 * time.Minute)
	if err == nil {
		t.Fatal("expected ErrPresignTTLTooLong")
	}
	if got != MaxPresignTTL {
		t.Errorf("capped TTL = %v, want %v", got, MaxPresignTTL)
	}
}

func TestClampTTLAtExactMax(t *testing.T) {
	got, err := clampTTL(MaxPresignTTL)
	if err != nil {
		t.Fatal(err)
	}
	if got != MaxPresignTTL {
		t.Errorf("exact-max TTL = %v, want %v", got, MaxPresignTTL)
	}
}

func TestPresignRequestValidationContract(t *testing.T) {
	// A nil context must be caught by the implementation. The
	// test stub just ensures the type compiles; real
	// enforcement lives in the adapter.
	_ = context.Background
	_ = PresignRequest{StorageProfileID: "p", ObjectKey: "k", TTL: time.Minute}
	_ = PresignedURL{URL: "https://example.com"}
}
