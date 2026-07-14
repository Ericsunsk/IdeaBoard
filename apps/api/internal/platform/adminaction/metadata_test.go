package adminaction

import (
	"errors"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		actorID    int64
		reason     string
		wantActor  int64
		wantReason string
		wantErr    error
	}{
		{name: "trims valid metadata", actorID: 42, reason: "  审核通过  ", wantActor: 42, wantReason: "审核通过"},
		{name: "accepts 1000 Unicode runes", actorID: 1, reason: strings.Repeat("点", MaxReasonRunes), wantActor: 1, wantReason: strings.Repeat("点", MaxReasonRunes)},
		{name: "rejects missing actor", reason: "valid", wantErr: ErrInvalidActor},
		{name: "rejects blank reason", actorID: 1, reason: " \t\n ", wantErr: ErrInvalidReason},
		{name: "rejects 1001 Unicode runes", actorID: 1, reason: strings.Repeat("点", MaxReasonRunes+1), wantErr: ErrInvalidReason},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			metadata, err := New(tt.actorID, tt.reason)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("New() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if metadata.IsValid() {
					t.Fatal("invalid metadata reported valid")
				}
				return
			}
			if !metadata.IsValid() || metadata.ActorID() != tt.wantActor || metadata.Reason() != tt.wantReason {
				t.Fatalf("New() = actor %d reason %q valid %v", metadata.ActorID(), metadata.Reason(), metadata.IsValid())
			}
		})
	}
}
