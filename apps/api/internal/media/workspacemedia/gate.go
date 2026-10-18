package workspacemedia

import (
	"ideaboard/api/internal/tasking/workspacetasks"
)

// GateAsset is the single media readiness gate (P0-4). Every media-side
// readiness check (submit-time input authorization, the task asset adapter,
// and library publication) delegates here instead of repeating the
// archived/ready comparison: archived assets deny as ErrMediaArchived,
// non-ready assets deny as ErrMediaNotReady. A nil asset denies as
// ErrMediaNotFound (invisible, never a distinct status).
//
// The ready comparison is exact (no case folding, no trimming):
// durable statuses are written exactly as AssetStatusReady, so anything
// else fails closed to not-ready. Tasking never inspects asset status
// itself: it consumes only these sentinels through the AssetReader seam.
func GateAsset(asset *Asset) error {
	if asset == nil {
		return workspacetasks.ErrMediaNotFound
	}
	if asset.IsArchived() {
		return workspacetasks.ErrMediaArchived
	}
	if asset.Status != AssetStatusReady {
		return workspacetasks.ErrMediaNotReady
	}
	return nil
}
