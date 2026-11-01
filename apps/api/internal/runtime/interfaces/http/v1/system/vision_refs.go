package system

// Vision reference resolution for the v1 search surface.
//
// This file owns the server-side translation of vision-query inputs
// (owned library assets, staged uploads, external image URLs) into the
// resolved V1VisionReference values handed to the extractor seam.
// Shapes are checked here; existence and workspace binding are checked
// against the owning stores at the handler; hidden refs converge to 404.

import (
	"errors"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"net/http"

	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/contract"
	v1media "ideaboard/api/internal/runtime/interfaces/http/v1/media"
	"ideaboard/api/internal/searchcatalog"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

func (h *Handler) resolveVisionReferences(c *gin.Context, workspace *tenancy.Workspace, assetIDs []string) ([]searchcatalog.V1VisionReference, bool) {
	references := make([]searchcatalog.V1VisionReference, 0, len(assetIDs))
	for _, id := range assetIDs {
		asset, err := h.deps.Search.Library.GetAsset(c.Request.Context(), workspace.PublicID, id)
		if err != nil {
			contract.RenderError(c, http.StatusNotFound, contract.CodeLibraryAssetNotFound, runtimehttp.MsgLibraryAssetNotFound)
			c.Abort()
			return nil, false
		}
		references = append(references, searchcatalog.V1VisionReference{
			Provider: asset.Provider, SourceID: asset.SourceID,
			SourceURL: asset.SourceURL, PreviewURL: asset.PreviewURL, Title: asset.Title,
			Kind: searchcatalog.V1VisionReferenceLibrary,
		})
	}
	return references, true
}

// errV1VisionUploadUnstaged reports a staged-upload vision ref whose
// bytes were never PUT (422: the caller must stage first, then retry
// the same vision call).
var errV1VisionUploadUnstaged = errors.New("v1 vision upload has no staged bytes")

// resolveVisionUploads resolves staged-upload vision refs (the
// stage-then-reference half of the vision file-input gap). Unknown,
// malformed, or cross-workspace ids converge to 404 upload_not_found;
// expired sessions to 410 upload_expired; failed sessions to 409
// upload_state_conflict. Completed sessions stay usable (their bytes
// were verified at complete time). Live sessions must have staged
// bytes readable through the byte seam, verified against the session
// declaration (unstaged reads 422); the session itself is never
// failed here — vision is a reader, not the transfer owner.
func (h *Handler) resolveVisionUploads(c *gin.Context, workspace *tenancy.Workspace, uploadIDs []string) ([]searchcatalog.V1VisionReference, bool) {
	if len(uploadIDs) == 0 {
		return nil, true
	}
	if h.deps.Media.Media == nil {
		contract.RenderError(c, http.StatusNotImplemented,
			contract.CodeContractNotImplemented, "The v1 vision-upload surface is not implemented yet")
		c.Abort()
		return nil, false
	}
	if h.deps.Media.MediaObjects == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable54)
		c.Abort()
		return nil, false
	}
	ctx := c.Request.Context()
	references := make([]searchcatalog.V1VisionReference, 0, len(uploadIDs))
	for _, id := range uploadIDs {
		session, err := h.deps.Media.Media.GetSession(ctx, workspace.PublicID, id)
		if err != nil {
			switch {
			case errors.Is(err, workspacemedia.ErrExpired):
				contract.RenderError(c, contract.StatusOf(contract.CodeUploadExpired),
					contract.CodeUploadExpired, runtimehttp.MsgUploadExpired)
			case errors.Is(err, workspacemedia.ErrStateConflict):
				contract.RenderError(c, contract.StatusOf(contract.CodeUploadStateConflict),
					contract.CodeUploadStateConflict, runtimehttp.MsgUploadStateConflict)
			default:
				contract.RenderError(c, http.StatusNotFound, contract.CodeUploadNotFound, runtimehttp.MsgUploadNotFound)
			}
			c.Abort()
			return nil, false
		}
		if session.Status == workspacemedia.StatusFailed {
			contract.RenderError(c, contract.StatusOf(contract.CodeUploadStateConflict),
				contract.CodeUploadStateConflict, runtimehttp.MsgUploadStateConflict)
			c.Abort()
			return nil, false
		}
		object, err := h.deps.Media.MediaObjects.ReadObjectAtProfile(ctx, workspacemedia.DefaultStorageProfile, session.StorageKey, workspacemedia.MaxUploadSizeBytes)
		if err != nil || object == nil || len(object.Data) == 0 || int64(len(object.Data)) != session.SizeBytes ||
			v1media.SniffedImageMIME(object.Data) != session.MIMEType || v1media.StagedChecksum(object.Data) != session.ChecksumSHA256 {
			renderSearchError(c, errV1VisionUploadUnstaged)
			return nil, false
		}
		references = append(references, searchcatalog.V1VisionReference{
			Kind: searchcatalog.V1VisionReferenceUpload, UploadID: session.ID,
			ObjectKey: session.StorageKey, MIMEType: session.MIMEType,
		})
	}
	return references, true
}

// runVisionQuery executes one vision query inside the search:vision
// idempotency scope: server-side quote for the workspace Billing
// Account, credit hold through credits.Commands, provider extraction,
// then capture (201 with the normalized query) or release (provider
// failure maps to 502/503 with the hold refunded).
