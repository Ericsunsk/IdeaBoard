package workspacemedia

import (
	"context"
	"strings"

	"ideaboard/api/internal/boards"
)

// Media asset authorization (#936).
//
// Decision (single home: authorizeLoadedAsset below, mirrored by the
// in-memory twin in access_memory.go): a workspace member may read one
// asset's metadata and bytes when ANY independent source authorizes:
//
//  1. The caller owns the asset (media_assets.owner_user_id). The owner is
//     recorded at session create (pre-complete) and survives every later
//     lifecycle event.
//  2. The asset backs a published Workspace Library row (library_assets
//     media link). Library rows are member-visible; existence is consumed
//     as a fact, never as a copied policy.
//  3. The asset has a durable Task origin in the same workspace. The caller
//     must be its submitter or currently read its original Board; Boardless
//     origins are submitter-only. Task deletion does not erase this fact.
//  4. The asset's creating session named a restricted upload target board
//     and the caller can read that board today (live RoleReader check).
//     The target is recorded pre-complete and verified against the
//     creator's board write access at session create — never trusted from
//     the client. A NULL target (board not autosaved yet) authorizes
//     nobody: there is never a full-workspace readable window.
//  5. The asset is referenced by the accepted document of a board the
//     caller can read today (board_document_media_refs + live RoleReader
//     check per board).
//
// Denied, malformed, cross-workspace, and non-member reads all report
// ErrNotFound (Baseline 4 anti-enumeration): ids and URLs are not
// credentials, and failures never distinguish "no such asset" from "no
// access". Archived assets stay authorization-neutral here; the binary
// layer gates archived bytes to 410 after authorization passes.
//
// Cross-context rule: board access facts enter ONLY through the narrow
// BoardAccess port below (the RoleReader-shaped role fact plus the
// reference-listing fact). Media never copies the board role mapping,
// never imports Boards storage internals, and never depends on HTTP
// handlers. Library/origin facts are read from their own durable rows
// (postgres) or test-seeded sets (in-memory); both twins
// agree on authorization outcomes.

// BoardAccess is the narrow port through which Media consumes Board
// access facts (#936, #934 convergence). It is satisfied structurally by
// both boards store implementations; Media depends only on
// this shape, never on the full store.
type BoardAccess interface {
	GetBoardRole(ctx context.Context, workspacePublicID, boardPublicID string, userID int64, workspaceRole string) (boards.AccessRole, bool, error)
	ListBoardsReferencingAsset(ctx context.Context, workspacePublicID, assetPublicID string) ([]string, error)
}

// Compile-time note: both boards store implementations satisfy BoardAccess
// structurally (GetBoardRole + ListBoardsReferencingAsset);
// the HTTP layer passes its boards store directly, so any drift fails there.

// AssetAccessStore is the authorization half of the workspace media seam.
// Repository methods take context.Context first.
type AssetAccessStore interface {
	// AuthorizeAsset resolves one workspace-scoped asset for a caller and
	// reports ErrNotFound unless an independent source (owner, library,
	// task, upload-target board, or readable referencing board)
	// authorizes the read. The returned asset is metadata-readable;
	// binary endpoints apply their own archived/readiness gates after.
	AuthorizeAsset(ctx context.Context, workspaceID, assetID string, callerUserID int64, workspaceRole string, access BoardAccess) (*Asset, error)
	// ListAuthorizedAssets pages the caller's readable assets newest-first
	// (limit clamped to [1,100] like the contract kernel). Offset counts
	// AUTHORIZED items: filtering happens before pagination so page
	// contents and has_more never leak the existence of unauthorized rows.
	ListAuthorizedAssets(ctx context.Context, workspaceID string, callerUserID int64, workspaceRole string, access BoardAccess, limit, offset int) ([]*Asset, error)
}

// authorizeBoardRead reports whether the caller holds a live read
// capability on one board. Any lookup failure denies that source (fail
// closed) without failing the whole decision: other independent sources
// are still evaluated.
func authorizeBoardRead(ctx context.Context, access BoardAccess, workspacePublicID, boardPublicID string, callerUserID int64, workspaceRole string) bool {
	if access == nil {
		return false
	}
	boardID := strings.TrimSpace(boardPublicID)
	if boardID == "" {
		return false
	}
	role, ok, err := access.GetBoardRole(ctx, strings.TrimSpace(workspacePublicID), boardID, callerUserID, workspaceRole)
	if err != nil {
		return false
	}
	return boards.CheckCapability(role, ok, boards.CapRead) == nil
}

// checkMemberPrecondition denies callers without visible workspace
// membership before any asset fact is touched.
func checkMemberPrecondition(workspaceRole string) error {
	if !boards.IsWorkspaceMemberRole(workspaceRole) {
		return ErrNotFound
	}
	return nil
}
