package workspacemedia

// Opaque-cursor channel for asset listing (P0-5 convergence).
//
// ListAssets keeps its (limit, offset) signature (frozen seam, see the
// audit migration note on the AssetStore interface); the offset it takes
// is the decoded position of an opaque cursor produced by
// EncodeAssetCursor. HTTP edges never accept a raw offset: v1
// listMediaAssets decodes its cursor via contract.DecodeCursor and passes
// Offset here.
//
// Migration plan (signature-preserving): a follow-up PR switches the
// seam to ListAssetsByCursor(ctx, workspaceID, limit, cursor string)
// returning (page, nextCursor), with AssetCursor as the payload. Until
// then, callers must round-trip through these helpers so no raw offset
// ever crosses the HTTP boundary.

import (
	"strings"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/pagination"
)

// AssetCursor is the opaque keyset position for asset listing: a
// creation-ordered offset bound to workspace + principal + filters by the
// encoding edge (Baseline 5.5).
type AssetCursor struct {
	Offset int `json:"offset"`
}

// EncodeAssetCursor serializes a listing position into an opaque cursor.
func EncodeAssetCursor(offset int) (string, error) {
	return contract.EncodeCursor(AssetCursor{Offset: offset})
}

// DecodeAssetCursor parses an opaque cursor from EncodeAssetCursor.
// Empty means first page (offset 0). Malformed or negative cursors
// surface pagination.ErrInvalidCursor for the edge to render as
// pagination_cursor_invalid.
func DecodeAssetCursor(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	cursor, err := contract.DecodeCursor[AssetCursor](raw)
	if err != nil {
		return 0, err
	}
	if cursor == nil || cursor.Offset < 0 {
		return 0, pagination.ErrInvalidCursor
	}
	return cursor.Offset, nil
}
