package workspacemedia

import (
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/tasking/workspacetasks"
)

// GateAsset is the single media readiness gate: every media-side check
// delegates here, so this matrix pins the deny reasons once.
func TestGateAssetMatrix(t *testing.T) {
	ready := &Asset{ID: "asset_ready", WorkspaceID: "ws_a", Status: AssetStatusReady}
	archivedAt := time.Now().UTC()
	archived := &Asset{
		ID: "asset_old", WorkspaceID: "ws_a", Status: AssetStatusReady,
		ArchivedAt: &archivedAt,
	}
	pending := &Asset{ID: "asset_pending", WorkspaceID: "ws_a", Status: "pending"}

	if err := GateAsset(ready); err != nil {
		t.Fatalf("ready err = %v, want nil", err)
	}
	if err := GateAsset(nil); !errors.Is(err, workspacetasks.ErrMediaNotFound) {
		t.Fatalf("nil err = %v, want media_not_found", err)
	}
	if err := GateAsset(archived); !errors.Is(err, workspacetasks.ErrMediaArchived) {
		t.Fatalf("archived err = %v, want media_archived", err)
	}
	if err := GateAsset(pending); !errors.Is(err, workspacetasks.ErrMediaNotReady) {
		t.Fatalf("pending err = %v, want media_not_ready", err)
	}
}
