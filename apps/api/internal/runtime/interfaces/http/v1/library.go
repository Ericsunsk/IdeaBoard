package v1

// Workspace-scoped library handlers (issue #806).
//
// Additive only: all routes mount under
// /api/v1/workspaces/{workspace_id}/library via the existing
// runtimehttp.ContractV1GroupPath + runtimehttp.PublicContractChain pattern. Legacy
// /api/user library routes were removed in #809.
//
// Agreed surface:
//
//	GET/POST    /workspaces/{workspace_id}/library/folders
//	PATCH/DELETE /workspaces/{workspace_id}/library/folders/{folder_id}
//	GET/POST    /workspaces/{workspace_id}/library/assets
//	PATCH/DELETE /workspaces/{workspace_id}/library/assets/{asset_id}
//	POST        /workspaces/{workspace_id}/library/asset-resolutions
//
// Materialization has no HTTP surface: the former
// POST/GET .../library/materializations routes, the lmat_ job DTO,
// and the librarydomain.materialization_* emitters were deleted (owner hard
// cut); materialization runs internal-only.
//
// Auth model: runtimehttp.ResolvePrincipal + runtimehttp.RequireWorkspaceCapability for the
// explicit workspace context. Any workspace member reads and writes;
// outsiders (no membership) converge to 404 workspace_not_found.
// Resource ids are lfolder_/lasset_/lmat_ opaque segments: malformed shapes
// (including legacy integer ids, which v1 never accepts), unknown ids,
// and cross-workspace ids converge to the matching *_not_found 404
// (Baseline 4 anti-enumeration). Integer-ID tolerance removal from the
// legacy handlers is documented for #809 and NOT executed here.
//
// Envelope: contract.RenderSuccess/RenderCreated/RenderPaginated/
// RenderError; cursor pagination via cursor/page_size; POST/DELETE
// require the Idempotency-Key header (scope library:<op>; reuse with a
// different request hash renders 409 library_idempotency_conflict).
// Save is additionally idempotent per source: external re-saving on
// (workspace, provider, source_id) and media re-saving on
// (workspace, media_asset_id) return the existing asset with 200
// instead of 201, without touching its folder or metadata.
//
// Assets NEVER store bytes. External rows keep provider URL + source
// metadata (the store reuses the provider-allowlist URL gate); media
// rows keep the managed Media identity with no provider/URL keys and
// project dimensions from Media at read time. Asset
// resolution is a typed batch lookup (max 20) against saved workspace
// assets only — NOT a general proxy; unknown references report
// unresolved per item.
//
// Store-level lmat_ job records, the domain.ErrV1MaterializationNotFound
// sentinel, and the library_materialization_not_found code stay frozen
// for internal tooling; this surface renders no materialization 404.
//
// Production wiring supplies the postgres mapping. Tests inject the
// in-memory store plus fakes.

import (
	"context"
	"errors"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"log/slog"
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/principal"
	libraryapplication "ideaboard/api/internal/library/application"
	librarydomain "ideaboard/api/internal/library/domain"
	libraryports "ideaboard/api/internal/library/ports"
	mediadomain "ideaboard/api/internal/media/derivatives/domain"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

// V1CreditSettler settles metered workspace operations through the
// credits.Commands path: Hold reserves, Capture consumes on success,
// Release refunds on failure. Every command carries a caller-owned
// idempotency key so retries converge. Only Credits owns ledger writes;
// this seam performs none. Production adapts *credits.Service;
// tests inject fakes. The library surface no longer settles (permanent
// free tier); the search surface still does.
type V1CreditSettler interface {
	Hold(ctx context.Context, userID int64, amountCredits int64, idempotencyKey, reason string) (string, error)
	Capture(ctx context.Context, userID int64, holdID string, amountCredits int64, idempotencyKey string) error
	Release(ctx context.Context, userID int64, holdID string, amountCredits int64, idempotencyKey string) error
}

// V1BillingQuoter mints server-side price snapshots for metered
// workspace operations. No client price input is accepted on any route.
// Production adapts the billing catalog QuoteAtResolution seam;
// tests inject fakes. The library surface no longer quotes (permanent
// free tier); the search surface still does.
type V1BillingQuoter interface {
	Quote(ctx context.Context, workspacePublicID, sku string) (int64, error)
}

// ContractV1LibraryDeps wires the workspace library handlers. Zero values
// fail closed; production supplies the store plus the read-through
// orchestrator. The V1BillingQuoter/
// V1CreditSettler interfaces below stay defined here because the search
// surface still settles through them.
type ContractV1LibraryDeps struct {
	Store       libraryports.V1Store
	Idempotency *contract.IdempotencyStore
	// ReadThrough owns library materialization lifecycle + replay
	// orchestration (issue #908). Nil keeps the Canvas import read
	// path fail-closed (404 without spawning); composition injects
	// the typed service built over the store/materializer/executor.
	ReadThrough *libraryapplication.ReadThroughService
	Sources     *libraryapplication.SourceResolver
}

func (d *ContractV1LibraryDeps) normalize() {
	if d.Idempotency == nil {
		d.Idempotency = contract.NewIdempotencyStore(contract.DefaultIdempotencyStoreOptions())
	}
}

var (
	// errV1BillingQuoteUnavailable marks transient quoter failures for
	// the 503 billing_quote_unavailable branch. Defined here, consumed
	// by the search surface (v1/search.go); the library
	// surface no longer quotes.
	errV1BillingQuoteUnavailable = errors.New("v1 billing quote unavailable")
	// errV1BillingRestricted maps hold rejections (frozen/restricted/
	// insufficient wallets) to 403 billing_wallet_restricted. Defined
	// here, consumed by the search surface; the library surface no
	// longer holds.
	errV1BillingRestricted = errors.New("v1 billing wallet restricted")
)

func (h *contractV1Handler) registerLibraryRoutes(group *gin.RouterGroup) {
	group.GET("/workspaces/:workspace_id/library/folders", h.listLibraryFolders)
	group.POST("/workspaces/:workspace_id/library/folders", h.createLibraryFolder)
	group.PATCH("/workspaces/:workspace_id/library/folders/:folder_id", h.renameLibraryFolder)
	group.DELETE("/workspaces/:workspace_id/library/folders/:folder_id", h.deleteLibraryFolder)
	group.GET("/workspaces/:workspace_id/library/assets", h.listLibraryAssets)
	group.GET("/workspaces/:workspace_id/library/assets/:asset_id/derivatives/:tier", h.derivativeLibraryAsset)
	group.POST("/workspaces/:workspace_id/library/assets", h.saveLibraryAsset)
	group.PATCH("/workspaces/:workspace_id/library/assets/:asset_id", h.moveLibraryAsset)
	group.DELETE("/workspaces/:workspace_id/library/assets/:asset_id", h.deleteLibraryAsset)
	group.POST("/workspaces/:workspace_id/library/asset-resolutions", h.resolveLibraryAssets)
}

func (h *contractV1Handler) requireLibraryStore(c *gin.Context) bool {
	if h.deps.Library.Store == nil {
		contract.RenderError(c, http.StatusNotImplemented,
			contract.CodeContractNotImplemented, "The v1 library surface is not implemented yet")
		c.Abort()
		return false
	}
	return true
}

// libraryWorkspaceContext resolves the explicit workspace context and
// enforces membership: hidden workspaces converge to 404
// workspace_not_found, non-members to the same 404.
func (h *contractV1Handler) libraryWorkspaceContext(c *gin.Context, identityPrincipal principal.Principal) (*tenancy.Workspace, bool) {
	workspace, _, visible := h.v1WorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return nil, false
	}
	if !runtimehttp.RequireWorkspaceCapability(c, visible, visible) {
		return nil, false
	}
	return workspace, true
}

func renderLibraryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, workspacetasks.ErrForbidden):
		contract.RenderError(c, http.StatusForbidden, contract.CodeBoardForbidden, "Access denied")
	case errors.Is(err, librarydomain.ErrV1FolderNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeLibraryFolderNotFound, runtimehttp.MsgLibraryFolderNotFound)
	case errors.Is(err, librarydomain.ErrV1AssetNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeLibraryAssetNotFound, runtimehttp.MsgLibraryAssetNotFound)
	case errors.Is(err, librarydomain.ErrV1ResolveTooMany):
		contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
			runtimehttp.MsgRenderLibraryError, contract.FieldErrors{Fields: map[string]string{"items": "too_many"}})
	case errors.Is(err, librarydomain.ErrV1FolderNameConflict):
		contract.RenderError(c, http.StatusConflict, contract.CodeConflict, runtimehttp.MsgConflict5)
	case errors.Is(err, librarydomain.ErrSystemFolderImmutable):
		contract.RenderError(c, http.StatusConflict, contract.CodeConflict, runtimehttp.MsgConflict6)
	case errors.Is(err, librarydomain.ErrInvalidFolderName):
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
			runtimehttp.MsgRenderLibraryError2, contract.FieldErrors{Fields: map[string]string{"name": "invalid"}})
	case errors.Is(err, librarydomain.ErrInvalidProvider):
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
			runtimehttp.MsgRenderLibraryError3, contract.FieldErrors{Fields: map[string]string{"provider": "invalid"}})
	case errors.Is(err, librarydomain.ErrInvalidSource):
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
			runtimehttp.MsgRenderLibraryError4, contract.FieldErrors{Fields: map[string]string{"asset": "invalid"}})
	default:
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable49)
	}
	c.Abort()
}

// libraryDo runs fn under the header-only Idempotency-Key scope
// library:<op>: missing keys render 400 idempotency_key_required, key
// reuse with a different request hash renders 409
// library_idempotency_conflict (aborted), same key+hash replays the
// first result (replayed=true).
func (h *contractV1Handler) libraryDo(c *gin.Context, userExternalID string, userID int64, op, key, hash string, fn func() (any, error)) (any, bool, error, bool) {
	h.deps.Library.normalize()
	scope := contract.IdempotencyScope(userExternalID, "library:"+op)
	result, replayed, err := h.deps.Library.Idempotency.Do(c.Request.Context(), scope, userID, key, hash, fn)
	if err != nil {
		if contract.IsIdempotencyConflict(err) {
			contract.RenderError(c, http.StatusConflict, contract.CodeLibraryIdempotencyConflict, runtimehttp.MsgWorkspaceIdempotencyConflict)
			c.Abort()
			return nil, false, nil, false
		}
		return nil, false, err, true
	}
	return result, replayed, nil, true
}

func logLibraryAudit(op string, operatorUserID int64, target, reason, outcome string) {
	slog.Info("library v1 audit",
		"op", op,
		"operator_user_id", operatorUserID,
		"target", target,
		"reason", reason,
		"outcome", outcome,
	)
}

type v1LibraryFolderDTO struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func toLibraryFolderDTO(workspacePublicID string, folder *librarydomain.V1Folder) v1LibraryFolderDTO {
	return v1LibraryFolderDTO{
		ID: folder.PublicID, WorkspaceID: workspacePublicID, Name: folder.Name,
		CreatedAt: v1FormatTime(folder.CreatedAt), UpdatedAt: v1FormatTime(folder.UpdatedAt),
	}
}

type v1LibraryAssetDTO struct {
	ID              string               `json:"id"`
	WorkspaceID     string               `json:"workspace_id"`
	FolderID        string               `json:"folder_id,omitempty"`
	Source          string               `json:"source"`
	Provider        string               `json:"provider,omitempty"`
	SourceID        string               `json:"source_id,omitempty"`
	SourceURL       string               `json:"source_url,omitempty"`
	PreviewURL      string               `json:"preview_url,omitempty"`
	OriginalURL     string               `json:"original_url,omitempty"`
	MediaAssetID    string               `json:"media_asset_id,omitempty"`
	Title           string               `json:"title,omitempty"`
	Author          string               `json:"author,omitempty"`
	Width           *int32               `json:"width"`
	Height          *int32               `json:"height"`
	TaskID          string               `json:"task_id,omitempty"`
	TaskResultIndex *int32               `json:"task_result_index,omitempty"`
	CreatedAt       string               `json:"created_at"`
	UpdatedAt       string               `json:"updated_at"`
	Display         *mediadomain.Display `json:"display,omitempty"`
}

func toLibraryAssetDTO(workspacePublicID string, asset *librarydomain.V1Asset) v1LibraryAssetDTO {
	dto := v1LibraryAssetDTO{
		ID: asset.PublicID, WorkspaceID: workspacePublicID, FolderID: asset.FolderPublicID,
		Source: string(asset.Source),
		Title:  asset.Title, Author: asset.Author, Width: asset.Width, Height: asset.Height,
		CreatedAt: v1FormatTime(asset.CreatedAt), UpdatedAt: v1FormatTime(asset.UpdatedAt),
		Display: libraryDisplay(workspacePublicID, asset),
	}
	if asset.Source == librarydomain.AssetSourceMedia {
		dto.MediaAssetID = asset.MediaAssetID
		if asset.Lineage != nil {
			dto.TaskID = asset.Lineage.TaskID
			index := asset.Lineage.ResultIndex
			dto.TaskResultIndex = &index
		}
		return dto
	}
	dto.Provider = asset.Provider
	dto.SourceID = asset.SourceID
	dto.SourceURL = asset.SourceURL
	dto.PreviewURL = asset.PreviewURL
	dto.OriginalURL = asset.OriginalURL
	return dto
}

func libraryPageSlice[T any](items []T, c *gin.Context) ([]T, contract.PageMeta, bool) {
	pageSize, ok := v1PageSize(c)
	if !ok {
		return nil, contract.PageMeta{}, false
	}
	offset := 0
	if raw := strings.TrimSpace(c.Query(contract.CursorParam)); raw != "" {
		cursor, decodeErr := contract.DecodeCursor[v1ListCursor](raw)
		if decodeErr != nil || cursor == nil || cursor.Offset < 0 {
			contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, runtimehttp.MsgPaginationCursorInvalid3)
			c.Abort()
			return nil, contract.PageMeta{}, false
		}
		offset = cursor.Offset
	}
	if offset > len(items) {
		contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, runtimehttp.MsgPaginationCursorInvalid3)
		c.Abort()
		return nil, contract.PageMeta{}, false
	}
	end := offset + pageSize
	if end > len(items) {
		end = len(items)
	}
	meta := contract.NewPageMeta("", false)
	if end < len(items) {
		next, encodeErr := contract.EncodeCursor(v1ListCursor{Offset: end})
		if encodeErr != nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable49)
			c.Abort()
			return nil, contract.PageMeta{}, false
		}
		meta = contract.NewPageMeta(next, true)
	}
	return items[offset:end], meta, true
}

func (h *contractV1Handler) listLibraryFolders(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndLibrary, Resolve: h.resolveLibraryWorkspace}, func(wctx v1WorkspaceCtx) {
		workspace := wctx.Workspace
		all, err := h.deps.Library.Store.ListFolders(c.Request.Context(), workspace.PublicID)
		if err != nil {
			renderLibraryError(c, err)
			return
		}
		page := make([]v1LibraryFolderDTO, 0, len(all))
		for _, folder := range all {
			page = append(page, toLibraryFolderDTO(workspace.PublicID, folder))
		}
		slice, meta, ok := libraryPageSlice(page, c)
		if !ok {
			return
		}
		contract.RenderPaginated(c, slice, meta)
	})
}

type v1CreateLibraryFolderBody struct {
	Data struct {
		Name string `json:"name"`
	} `json:"data"`
}

func (h *contractV1Handler) createLibraryFolder(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndLibrary, Resolve: h.resolveLibraryWorkspace}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		key, ok := requireV1IdempotencyKey(c)
		if !ok {
			return
		}
		var body v1CreateLibraryFolderBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
			return
		}
		name := strings.TrimSpace(body.Data.Name)
		externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
		hash := contract.HashIdempotencyRequest("library:folder-create", workspace.PublicID, name)
		result, _, fnErr, ok := h.libraryDo(c, externalUser, identityPrincipal.UserID(), "folder-create", key, hash, func() (any, error) {
			publicID, mintErr := librarydomain.NewV1FolderPublicID()
			if mintErr != nil {
				return nil, mintErr
			}
			return h.deps.Library.Store.CreateFolder(c.Request.Context(), workspace.PublicID, name, publicID)
		})
		if !ok {
			return
		}
		if fnErr != nil {
			renderLibraryError(c, fnErr)
			return
		}
		created, ok := result.(*librarydomain.V1Folder)
		if !ok || created == nil {
			renderLibraryError(c, errors.New("library: folder-create type mismatch"))
			return
		}
		contract.RenderCreated(c, toLibraryFolderDTO(workspace.PublicID, created))
	})
}

type v1RenameLibraryFolderBody struct {
	Data struct {
		Name string `json:"name"`
	} `json:"data"`
}

func (h *contractV1Handler) libraryFolderByID(c *gin.Context, workspace *tenancy.Workspace, folderID string) (*librarydomain.V1Folder, bool) {
	folderID = strings.TrimSpace(folderID)
	if !librarydomain.IsV1FolderID(folderID) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeLibraryFolderNotFound, runtimehttp.MsgLibraryFolderNotFound)
		c.Abort()
		return nil, false
	}
	folders, err := h.deps.Library.Store.ListFolders(c.Request.Context(), workspace.PublicID)
	if err != nil {
		renderLibraryError(c, err)
		return nil, false
	}
	for _, folder := range folders {
		if folder.PublicID == folderID {
			return folder, true
		}
	}
	contract.RenderError(c, http.StatusNotFound, contract.CodeLibraryFolderNotFound, runtimehttp.MsgLibraryFolderNotFound)
	c.Abort()
	return nil, false
}

func (h *contractV1Handler) renameLibraryFolder(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndLibrary, Resolve: h.resolveLibraryWorkspace}, func(wctx v1WorkspaceCtx) {
		workspace := wctx.Workspace
		folder, ok := h.libraryFolderByID(c, workspace, c.Param("folder_id"))
		if !ok {
			return
		}
		var body v1RenameLibraryFolderBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
			return
		}
		renamed, err := h.deps.Library.Store.RenameFolder(c.Request.Context(), workspace.PublicID, folder.PublicID, body.Data.Name)
		if err != nil {
			renderLibraryError(c, err)
			return
		}
		contract.RenderSuccess(c, toLibraryFolderDTO(workspace.PublicID, renamed))
	})
}

func (h *contractV1Handler) deleteLibraryFolder(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndLibrary, Resolve: h.resolveLibraryWorkspace}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		// Shape-check only here: the existence lookup runs inside the
		// idempotent fn so replays converge instead of 404ing on the
		// already-deleted folder.
		folderID := strings.TrimSpace(c.Param("folder_id"))
		if !librarydomain.IsV1FolderID(folderID) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeLibraryFolderNotFound, runtimehttp.MsgLibraryFolderNotFound)
			c.Abort()
			return
		}
		key, ok := requireV1IdempotencyKey(c)
		if !ok {
			return
		}
		externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
		hash := contract.HashIdempotencyRequest("library:folder-delete", workspace.PublicID, folderID)
		result, _, fnErr, ok := h.libraryDo(c, externalUser, identityPrincipal.UserID(), "folder-delete", key, hash, func() (any, error) {
			return &v1DeletedDTO{Deleted: true}, h.deps.Library.Store.DeleteFolder(c.Request.Context(), workspace.PublicID, folderID)
		})
		if !ok {
			return
		}
		if fnErr != nil {
			renderLibraryError(c, fnErr)
			return
		}
		deleted, ok := result.(*v1DeletedDTO)
		if !ok || deleted == nil {
			renderLibraryError(c, errors.New("library: folder-delete type mismatch"))
			return
		}
		logLibraryAudit("library_folder_delete", identityPrincipal.UserID(), workspace.PublicID+"/"+folderID, "folder_delete", "deleted")
		contract.RenderSuccess(c, *deleted)
	})
}

func (h *contractV1Handler) listLibraryAssets(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndLibrary, Resolve: h.resolveLibraryWorkspace}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		var folderFilter *string
		if raw := strings.TrimSpace(c.Query("folder_id")); raw != "" {
			if _, ok := h.libraryFolderByID(c, workspace, raw); !ok {
				return
			}
			folderFilter = &raw
		}
		all, err := h.deps.Library.Store.ListAssets(c.Request.Context(), workspace.PublicID)
		if err != nil {
			renderLibraryError(c, err)
			return
		}
		// #937: list responses redact restricted task linkage per asset.
		workspaceRole, _ := h.libraryCallerRole(c.Request.Context(), workspace, identityPrincipal.UserID())
		page := make([]v1LibraryAssetDTO, 0, len(all))
		for _, asset := range all {
			if folderFilter != nil && asset.FolderPublicID != *folderFilter {
				continue
			}
			visible := h.redactLibraryLineageForCopy(c.Request.Context(), workspace, workspaceRole, identityPrincipal.UserID(), asset)
			page = append(page, toLibraryAssetDTO(workspace.PublicID, visible))
		}
		slice, meta, ok := libraryPageSlice(page, c)
		if !ok {
			return
		}
		contract.RenderPaginated(c, slice, meta)
	})
}

type v1SaveLibraryAssetBody struct {
	Data struct {
		Source       string `json:"source"`
		Provider     string `json:"provider"`
		SourceID     string `json:"source_id"`
		SourceURL    string `json:"source_url"`
		PreviewURL   string `json:"preview_url"`
		OriginalURL  string `json:"original_url"`
		MediaAssetID string `json:"media_asset_id"`
		Title        string `json:"title"`
		Author       string `json:"author"`
		Width        *int32 `json:"width"`
		Height       *int32 `json:"height"`
		FolderID     string `json:"folder_id"`
	} `json:"data"`
}

func (h *contractV1Handler) saveLibraryAsset(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndLibrary, Resolve: h.resolveLibraryWorkspace}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		key, ok := requireV1IdempotencyKey(c)
		if !ok {
			return
		}
		var body v1SaveLibraryAssetBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
			return
		}
		source := librarydomain.AssetSource(strings.TrimSpace(body.Data.Source))
		// Folder binding is validated up front so unknown folders 404 before
		// any idempotency state is recorded.
		folderPublicID := strings.TrimSpace(body.Data.FolderID)
		if folderPublicID != "" {
			if _, ok := h.libraryFolderByID(c, workspace, folderPublicID); !ok {
				return
			}
		}
		externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
		switch source {
		case librarydomain.AssetSourceExternal:
			h.saveExternalLibraryAsset(c, wctx, body, folderPublicID, externalUser, identityPrincipal.UserID(), key)
		case librarydomain.AssetSourceMedia:
			h.saveManagedLibraryAsset(c, wctx, body, folderPublicID, externalUser, identityPrincipal.UserID(), key)
		default:
			renderLibraryError(c, librarydomain.ErrInvalidSource)
		}
	})
}

func (h *contractV1Handler) saveExternalLibraryAsset(c *gin.Context, wctx v1WorkspaceCtx, body v1SaveLibraryAssetBody, folderPublicID, externalUser string, operatorUserID int64, key string) {
	workspace := wctx.Workspace
	if strings.TrimSpace(body.Data.MediaAssetID) != "" {
		renderLibraryError(c, librarydomain.ErrInvalidSource)
		return
	}
	var width, height int32
	if body.Data.Width != nil {
		width = *body.Data.Width
	}
	if body.Data.Height != nil {
		height = *body.Data.Height
	}
	input := librarydomain.V1SaveAssetInput{
		FolderPublicID: folderPublicID,
		Asset: librarydomain.AssetInput{
			Provider: body.Data.Provider, SourceID: body.Data.SourceID,
			SourceURL: body.Data.SourceURL, PreviewURL: body.Data.PreviewURL, OriginalURL: body.Data.OriginalURL,
			Title: body.Data.Title, Author: body.Data.Author,
			Width: width, Height: height,
		},
		OperatorUserID: operatorUserID,
	}
	hash := contract.HashIdempotencyRequest("library:asset-save", workspace.PublicID, "external",
		strings.ToLower(strings.TrimSpace(body.Data.Provider)), strings.TrimSpace(body.Data.SourceID), folderPublicID)
	result, _, fnErr, ok := h.libraryDo(c, externalUser, operatorUserID, "asset-save", key, hash, func() (any, error) {
		publicID, mintErr := librarydomain.NewV1AssetPublicID()
		if mintErr != nil {
			return nil, mintErr
		}
		saved, created, saveErr := h.deps.Library.Store.SaveAsset(c.Request.Context(), workspace.PublicID, input, publicID)
		if saveErr != nil {
			return nil, saveErr
		}
		return &v1SavedAsset{Asset: saved, Created: created}, nil
	})
	h.renderSavedLibraryAsset(c, wctx, result, fnErr, ok)
}

func (h *contractV1Handler) saveManagedLibraryAsset(c *gin.Context, wctx v1WorkspaceCtx, body v1SaveLibraryAssetBody, folderPublicID, externalUser string, operatorUserID int64, key string) {
	workspace := wctx.Workspace
	if strings.TrimSpace(body.Data.Provider) != "" || strings.TrimSpace(body.Data.SourceID) != "" ||
		strings.TrimSpace(body.Data.SourceURL) != "" || strings.TrimSpace(body.Data.PreviewURL) != "" ||
		strings.TrimSpace(body.Data.OriginalURL) != "" || body.Data.Width != nil || body.Data.Height != nil {
		renderLibraryError(c, librarydomain.ErrInvalidSource)
		return
	}
	input := librarydomain.V1SaveManagedAssetInput{
		FolderPublicID: folderPublicID,
		MediaAssetID:   strings.TrimSpace(body.Data.MediaAssetID),
		Title:          body.Data.Title,
		Author:         body.Data.Author,
		OperatorUserID: operatorUserID,
	}
	hash := contract.HashIdempotencyRequest("library:asset-save", workspace.PublicID, "media",
		strings.TrimSpace(body.Data.MediaAssetID), folderPublicID)
	result, _, fnErr, ok := h.libraryDo(c, externalUser, operatorUserID, "asset-save", key, hash, func() (any, error) {
		publicID, mintErr := librarydomain.NewV1AssetPublicID()
		if mintErr != nil {
			return nil, mintErr
		}
		saved, created, saveErr := h.deps.Library.Store.SaveManagedMediaAsset(c.Request.Context(), workspace.PublicID, input, publicID)
		if saveErr != nil {
			return nil, saveErr
		}
		return &v1SavedAsset{Asset: saved, Created: created}, nil
	})
	h.renderSavedLibraryAsset(c, wctx, result, fnErr, ok)
}

func (h *contractV1Handler) renderSavedLibraryAsset(c *gin.Context, wctx v1WorkspaceCtx, result any, fnErr error, ok bool) {
	if !ok {
		return
	}
	if fnErr != nil {
		renderLibraryError(c, fnErr)
		return
	}
	saved, ok := result.(*v1SavedAsset)
	if !ok || saved == nil || saved.Asset == nil {
		renderLibraryError(c, errors.New("library: asset-save type mismatch"))
		return
	}
	// #937: the save response never reverse-leaks restricted task
	// linkage the saver may not read.
	workspaceRole, _ := h.libraryCallerRole(c.Request.Context(), wctx.Workspace, wctx.Principal.UserID())
	asset := h.redactLibraryLineageForCopy(c.Request.Context(), wctx.Workspace, workspaceRole, wctx.Principal.UserID(), saved.Asset)
	if saved.Created {
		contract.RenderCreated(c, toLibraryAssetDTO(wctx.Workspace.PublicID, asset))
		return
	}
	contract.RenderSuccess(c, toLibraryAssetDTO(wctx.Workspace.PublicID, asset))
}

type v1SavedAsset struct {
	Asset   *librarydomain.V1Asset
	Created bool
}

func (h *contractV1Handler) libraryAssetOr404(c *gin.Context, workspace *tenancy.Workspace) (*librarydomain.V1Asset, bool) {
	raw := strings.TrimSpace(c.Param("asset_id"))
	if !librarydomain.IsV1AssetID(raw) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeLibraryAssetNotFound, runtimehttp.MsgLibraryAssetNotFound)
		c.Abort()
		return nil, false
	}
	asset, err := h.deps.Library.Store.GetAsset(c.Request.Context(), workspace.PublicID, raw)
	if err != nil {
		renderLibraryError(c, err)
		return nil, false
	}
	return asset, true
}

type v1MoveLibraryAssetBody struct {
	Data struct {
		FolderID string `json:"folder_id"`
	} `json:"data"`
}

func (h *contractV1Handler) moveLibraryAsset(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndLibrary, Resolve: h.resolveLibraryWorkspace}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		asset, ok := h.libraryAssetOr404(c, workspace)
		if !ok {
			return
		}
		var body v1MoveLibraryAssetBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
			return
		}
		moved, err := h.deps.Library.Store.MoveAsset(c.Request.Context(), workspace.PublicID, asset.PublicID, body.Data.FolderID)
		if err != nil {
			renderLibraryError(c, err)
			return
		}
		// #937: move responses redact restricted task linkage.
		workspaceRole, _ := h.libraryCallerRole(c.Request.Context(), workspace, identityPrincipal.UserID())
		visible := h.redactLibraryLineageForCopy(c.Request.Context(), workspace, workspaceRole, identityPrincipal.UserID(), moved)
		contract.RenderSuccess(c, toLibraryAssetDTO(workspace.PublicID, visible))
	})
}

func (h *contractV1Handler) deleteLibraryAsset(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndLibrary, Resolve: h.resolveLibraryWorkspace}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		assetID := strings.TrimSpace(c.Param("asset_id"))
		if !librarydomain.IsV1AssetID(assetID) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeLibraryAssetNotFound, runtimehttp.MsgLibraryAssetNotFound)
			c.Abort()
			return
		}
		key, ok := requireV1IdempotencyKey(c)
		if !ok {
			return
		}
		externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
		hash := contract.HashIdempotencyRequest("library:asset-delete", workspace.PublicID, assetID)
		result, _, fnErr, ok := h.libraryDo(c, externalUser, identityPrincipal.UserID(), "asset-delete", key, hash, func() (any, error) {
			return &v1DeletedDTO{Deleted: true}, h.deps.Library.Store.DeleteAsset(c.Request.Context(), workspace.PublicID, assetID)
		})
		if !ok {
			return
		}
		if fnErr != nil {
			renderLibraryError(c, fnErr)
			return
		}
		deleted, ok := result.(*v1DeletedDTO)
		if !ok || deleted == nil {
			renderLibraryError(c, errors.New("library: asset-delete type mismatch"))
			return
		}
		logLibraryAudit("library_asset_delete", identityPrincipal.UserID(), workspace.PublicID+"/"+assetID, "asset_delete", "deleted")
		contract.RenderSuccess(c, *deleted)
	})
}

type v1ResolveLibraryAssetsBody struct {
	Data struct {
		Items []v1ResolveLibraryItemBody `json:"items"`
	} `json:"data"`
}

type v1ResolveLibraryItemBody struct {
	Provider  string `json:"provider"`
	SourceID  string `json:"source_id"`
	SourceURL string `json:"source_url"`
}

type v1ResolveLibraryResultDTO struct {
	Provider string             `json:"provider"`
	SourceID string             `json:"source_id,omitempty"`
	Status   string             `json:"status"`
	Asset    *v1LibraryAssetDTO `json:"asset,omitempty"`
}

type v1ResolveLibraryResponseDTO struct {
	Items []v1ResolveLibraryResultDTO `json:"items"`
}

func (h *contractV1Handler) resolveLibraryAssets(c *gin.Context) {
	withV1WorkspaceBody(h, c, v1WorkspaceOpts{Require: h.requireStoreAndLibrary, Resolve: h.resolveLibraryWorkspace}, func(wctx v1WorkspaceCtx, body v1ResolveLibraryAssetsBody) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		items := make([]librarydomain.V1ResolveItem, 0, len(body.Data.Items))
		for _, item := range body.Data.Items {
			items = append(items, librarydomain.V1ResolveItem{
				Provider: item.Provider, SourceID: item.SourceID, SourceURL: item.SourceURL,
			})
		}
		results, err := h.deps.Library.Store.ResolveAssets(c.Request.Context(), workspace.PublicID, items)
		if err != nil {
			renderLibraryError(c, err)
			return
		}
		// #937: resolution responses redact restricted task linkage.
		workspaceRole, _ := h.libraryCallerRole(c.Request.Context(), workspace, identityPrincipal.UserID())
		dto := v1ResolveLibraryResponseDTO{Items: make([]v1ResolveLibraryResultDTO, 0, len(results))}
		for _, result := range results {
			entry := v1ResolveLibraryResultDTO{Provider: result.Provider, SourceID: result.SourceID, Status: string(result.Status)}
			if result.Asset != nil {
				visible := h.redactLibraryLineageForCopy(c.Request.Context(), workspace, workspaceRole, identityPrincipal.UserID(), result.Asset)
				asset := toLibraryAssetDTO(workspace.PublicID, visible)
				entry.Asset = &asset
			}
			dto.Items = append(dto.Items, entry)
		}
		contract.RenderSuccess(c, dto)
	})
}
