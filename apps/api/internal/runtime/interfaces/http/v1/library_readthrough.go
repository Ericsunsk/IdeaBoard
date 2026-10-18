package v1

// Library materialization read-through (owner-approved cut, internal-only).
//
// The public library:materialize job API is deleted (no POST/GET
// lmat_ routes, no billing, no events, no frontend polling), so the
// Canvas import path (frontend materializeLibraryAssetForCanvas: one
// poll-free GET media-assets/:asset_id with the lasset_ id, retrying
// through the Canvas pending-fact loop) relies on this read-through:
//
//   - lasset_ id on GET media-assets/:asset_id resolves the owned
//     library asset's direct Media link or newest completed external
//     materialization. A hit serves the linked managed asset (200)
//     with its asset_-based content/derivative URLs.
//   - A miss fires ONE best-effort async fetch and answers 404
//     media_not_found now; the Canvas retry loop re-reads until the
//     fetch lands (200).
//
// Ownership (issue #908):
//   - Media (media/assets.LibraryMaterializer) owns remote download
//     and managed-media writes.
//   - Library application (library/application.ReadThroughService)
//     owns materialization lifecycle + replay orchestration
//     (resolve/claim/dedup/fetch/settle over typed
//     store/materializer/executor seams injected by composition).
//   - This adapter only translates the authenticated request/response:
//     resolve the owned asset, render the managed DTO on a hit, else
//     request one best-effort fetch and render 404. It constructs no
//     materializer, manages no detached execution, and advances no
//     job state.
//
// Failure posture: trigger errors NEVER fail the read (404 either
// way) and fold into a failed job record inside the application
// service with its fixed message. Terminal job records are
// immutable, so concurrent settlers converge. Managed references
// resolve their existing Media identity without creating a job.

import (
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/contract"

	"github.com/gin-gonic/gin"
)

// getLibraryMaterializedMediaAsset serves GET media-assets/:asset_id
// for lasset_ ids (the Canvas import read path). A managed link
// renders the managed asset DTO (200); an external miss requests one
// best-effort fetch and renders 404 media_not_found so the Canvas
// pending-fact retry loop owns the retry. Unknown assets and
// unwired stores 404 without requesting.
func (h *contractV1Handler) getLibraryMaterializedMediaAsset(c *gin.Context, workspacePublicID, assetPublicID string, userID int64, workspaceRole string) {
	svc := h.deps.Library.ReadThrough
	if svc == nil {
		renderMediaStoreError(c, workspacemedia.ErrNotFound)
		return
	}
	ctx := c.Request.Context()
	stored, mediaAssetID, found := svc.Resolve(ctx, workspacePublicID, assetPublicID)
	if stored == nil {
		renderMediaStoreError(c, workspacemedia.ErrNotFound)
		return
	}
	if found {
		// #936: the managed asset behind a completed library link enforces
		// the same authorization (members pass via the library source).
		// Denials stop here with 404 and never trigger a fetch.
		if asset, ok := h.authorizeMediaAsset(c, workspacePublicID, mediaAssetID, userID, workspaceRole); ok {
			contract.RenderSuccess(c, toMediaAssetDTO(workspacePublicID, asset, v1FormatTime(asset.CreatedAt)))
		}
		return
	}
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok {
		return
	}
	svc.Request(ctx, workspacePublicID, stored, identityPrincipal.UserID())
	renderMediaStoreError(c, workspacemedia.ErrNotFound)
}
