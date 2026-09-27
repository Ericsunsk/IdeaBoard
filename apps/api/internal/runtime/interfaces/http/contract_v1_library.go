package httpapi

// Workspace-scoped library handlers (issue #806).
//
// Additive only: all routes mount under
// /api/v1/workspaces/{workspace_id}/library via the existing
// ContractV1GroupPath + PublicContractChain pattern. Legacy
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
// and the library.materialization_* emitters were deleted (owner hard
// cut); materialization runs internal-only.
//
// Auth model: ResolvePrincipal + RequireWorkspaceCapability for the
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
// Save is additionally idempotent on (workspace, provider, source_id):
// re-saving returns the existing asset with 200 instead of 201.
//
// Assets NEVER store bytes: provider URL + source metadata only (the
// store reuses the legacy provider-allowlist URL gate). Asset
// resolution is a typed batch lookup (max 20) against saved workspace
// assets only — NOT a general proxy; unknown references report
// unresolved per item.
//
// Store-level lmat_ job records, the library.ErrV1MaterializationNotFound
// sentinel, and the library_materialization_not_found code stay frozen
// for internal tooling; this surface renders no materialization 404.
//
// Production wiring supplies the postgres mapping. Tests inject the
// in-memory store plus fakes.

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/library"
	"ideaboard/api/internal/platform/contract"
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
// fail closed; production supplies the store. The V1BillingQuoter/
// V1CreditSettler interfaces below stay defined here because the search
// surface still settles through them.
type ContractV1LibraryDeps struct {
	Store       library.WorkspaceLibraryStore
	Idempotency *contract.IdempotencyStore
}

func (d *ContractV1LibraryDeps) normalize() {
	if d.Idempotency == nil {
		d.Idempotency = contract.NewIdempotencyStore(contract.DefaultIdempotencyStoreOptions())
	}
}

var (
	// errV1BillingQuoteUnavailable marks transient quoter failures for
	// the 503 billing_quote_unavailable branch. Defined here, consumed
	// by the search surface (contract_v1_search.go); the library
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
	if !RequireWorkspaceCapability(c, visible, visible) {
		return nil, false
	}
	return workspace, true
}

func renderLibraryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, library.ErrV1FolderNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeLibraryFolderNotFound, "文件夹不存在")
	case errors.Is(err, library.ErrV1AssetNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeLibraryAssetNotFound, "素材不存在")
	case errors.Is(err, library.ErrV1ResolveTooMany):
		contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
			"单次最多解析 20 条", contract.FieldErrors{Fields: map[string]string{"items": "too_many"}})
	case errors.Is(err, library.ErrV1FolderNameConflict):
		contract.RenderError(c, http.StatusConflict, contract.CodeConflict, "同名文件夹已存在")
	case errors.Is(err, library.ErrSystemFolderImmutable):
		contract.RenderError(c, http.StatusConflict, contract.CodeConflict, "系统文件夹不可操作")
	case errors.Is(err, library.ErrInvalidFolderName):
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
			"文件夹名称非法", contract.FieldErrors{Fields: map[string]string{"name": "invalid"}})
	case errors.Is(err, library.ErrInvalidProvider):
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
			"素材来源非法", contract.FieldErrors{Fields: map[string]string{"provider": "invalid"}})
	case errors.Is(err, library.ErrInvalidSource):
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
			"素材引用非法", contract.FieldErrors{Fields: map[string]string{"asset": "invalid"}})
	default:
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "素材库服务暂不可用")
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
			contract.RenderError(c, http.StatusConflict, contract.CodeLibraryIdempotencyConflict, "幂等请求与原请求不一致")
			c.Abort()
			return nil, false, nil, false
		}
		return nil, false, err, true
	}
	return result, replayed, nil, true
}

func requireLibraryIdempotencyKey(c *gin.Context) (string, bool) {
	key, err := contract.IdempotencyKeyFromHeader(c)
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, "缺少幂等键，请重试此次操作")
		c.Abort()
		return "", false
	}
	return key, true
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

func toLibraryFolderDTO(workspacePublicID string, folder *library.V1Folder) v1LibraryFolderDTO {
	return v1LibraryFolderDTO{
		ID: folder.PublicID, WorkspaceID: workspacePublicID, Name: folder.Name,
		CreatedAt: v1FormatTime(folder.CreatedAt), UpdatedAt: v1FormatTime(folder.UpdatedAt),
	}
}

type v1LibraryAssetDTO struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	FolderID    string `json:"folder_id,omitempty"`
	Provider    string `json:"provider"`
	SourceID    string `json:"source_id"`
	SourceURL   string `json:"source_url"`
	PreviewURL  string `json:"preview_url"`
	OriginalURL string `json:"original_url"`
	Title       string `json:"title,omitempty"`
	Author      string `json:"author,omitempty"`
	Width       int32  `json:"width"`
	Height      int32  `json:"height"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func toLibraryAssetDTO(workspacePublicID string, asset *library.V1Asset) v1LibraryAssetDTO {
	return v1LibraryAssetDTO{
		ID: asset.PublicID, WorkspaceID: workspacePublicID, FolderID: asset.FolderPublicID,
		Provider: asset.Provider, SourceID: asset.SourceID,
		SourceURL: asset.SourceURL, PreviewURL: asset.PreviewURL, OriginalURL: asset.OriginalURL,
		Title: asset.Title, Author: asset.Author, Width: asset.Width, Height: asset.Height,
		CreatedAt: v1FormatTime(asset.CreatedAt), UpdatedAt: v1FormatTime(asset.UpdatedAt),
	}
}

func libraryPageSlice[T any](items []T, c *gin.Context) ([]T, contract.PageMeta, bool) {
	pageSize, err := contract.ParsePageSize(c.Query(contract.PageSizeParam))
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "page_size 非法")
		c.Abort()
		return nil, contract.PageMeta{}, false
	}
	offset := 0
	if raw := strings.TrimSpace(c.Query(contract.CursorParam)); raw != "" {
		cursor, decodeErr := contract.DecodeCursor[v1ListCursor](raw)
		if decodeErr != nil || cursor == nil || cursor.Offset < 0 {
			contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, "分页游标非法")
			c.Abort()
			return nil, contract.PageMeta{}, false
		}
		offset = cursor.Offset
	}
	if offset > len(items) {
		contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, "分页游标非法")
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
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "素材库服务暂不可用")
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
		key, ok := requireLibraryIdempotencyKey(c)
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
			publicID, mintErr := library.NewV1FolderPublicID()
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
		created, ok := result.(*library.V1Folder)
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

func (h *contractV1Handler) libraryFolderByID(c *gin.Context, workspace *tenancy.Workspace, folderID string) (*library.V1Folder, bool) {
	folderID = strings.TrimSpace(folderID)
	if !library.IsV1FolderID(folderID) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeLibraryFolderNotFound, "文件夹不存在")
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
	contract.RenderError(c, http.StatusNotFound, contract.CodeLibraryFolderNotFound, "文件夹不存在")
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
		if !library.IsV1FolderID(folderID) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeLibraryFolderNotFound, "文件夹不存在")
			c.Abort()
			return
		}
		key, ok := requireLibraryIdempotencyKey(c)
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
		page := make([]v1LibraryAssetDTO, 0, len(all))
		for _, asset := range all {
			if folderFilter != nil && asset.FolderPublicID != *folderFilter {
				continue
			}
			page = append(page, toLibraryAssetDTO(workspace.PublicID, asset))
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
		Provider    string `json:"provider"`
		SourceID    string `json:"source_id"`
		SourceURL   string `json:"source_url"`
		PreviewURL  string `json:"preview_url"`
		OriginalURL string `json:"original_url"`
		Title       string `json:"title"`
		Author      string `json:"author"`
		Width       int32  `json:"width"`
		Height      int32  `json:"height"`
		FolderID    string `json:"folder_id"`
	} `json:"data"`
}

func (h *contractV1Handler) saveLibraryAsset(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndLibrary, Resolve: h.resolveLibraryWorkspace}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		key, ok := requireLibraryIdempotencyKey(c)
		if !ok {
			return
		}
		var body v1SaveLibraryAssetBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
			return
		}
		input := library.V1SaveAssetInput{
			FolderPublicID: strings.TrimSpace(body.Data.FolderID),
			Asset: library.AssetInput{
				Provider: body.Data.Provider, SourceID: body.Data.SourceID,
				SourceURL: body.Data.SourceURL, PreviewURL: body.Data.PreviewURL, OriginalURL: body.Data.OriginalURL,
				Title: body.Data.Title, Author: body.Data.Author,
				Width: body.Data.Width, Height: body.Data.Height,
			},
		}
		// Folder binding is validated up front so unknown folders 404 before
		// any idempotency state is recorded.
		if input.FolderPublicID != "" {
			if _, ok := h.libraryFolderByID(c, workspace, input.FolderPublicID); !ok {
				return
			}
		}
		externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
		hash := contract.HashIdempotencyRequest("library:asset-save", workspace.PublicID,
			strings.ToLower(strings.TrimSpace(body.Data.Provider)), strings.TrimSpace(body.Data.SourceID), input.FolderPublicID)
		result, _, fnErr, ok := h.libraryDo(c, externalUser, identityPrincipal.UserID(), "asset-save", key, hash, func() (any, error) {
			publicID, mintErr := library.NewV1AssetPublicID()
			if mintErr != nil {
				return nil, mintErr
			}
			saved, created, saveErr := h.deps.Library.Store.SaveAsset(c.Request.Context(), workspace.PublicID, input, publicID)
			if saveErr != nil {
				return nil, saveErr
			}
			return &v1SavedAsset{Asset: saved, Created: created}, nil
		})
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
		if saved.Created {
			contract.RenderCreated(c, toLibraryAssetDTO(workspace.PublicID, saved.Asset))
			return
		}
		contract.RenderSuccess(c, toLibraryAssetDTO(workspace.PublicID, saved.Asset))
	})
}

type v1SavedAsset struct {
	Asset   *library.V1Asset
	Created bool
}

func (h *contractV1Handler) libraryAssetOr404(c *gin.Context, workspace *tenancy.Workspace) (*library.V1Asset, bool) {
	raw := strings.TrimSpace(c.Param("asset_id"))
	if !library.IsV1AssetID(raw) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeLibraryAssetNotFound, "素材不存在")
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
		contract.RenderSuccess(c, toLibraryAssetDTO(workspace.PublicID, moved))
	})
}

func (h *contractV1Handler) deleteLibraryAsset(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndLibrary, Resolve: h.resolveLibraryWorkspace}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		assetID := strings.TrimSpace(c.Param("asset_id"))
		if !library.IsV1AssetID(assetID) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeLibraryAssetNotFound, "素材不存在")
			c.Abort()
			return
		}
		key, ok := requireLibraryIdempotencyKey(c)
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
		workspace := wctx.Workspace
		items := make([]library.V1ResolveItem, 0, len(body.Data.Items))
		for _, item := range body.Data.Items {
			items = append(items, library.V1ResolveItem{
				Provider: item.Provider, SourceID: item.SourceID, SourceURL: item.SourceURL,
			})
		}
		results, err := h.deps.Library.Store.ResolveAssets(c.Request.Context(), workspace.PublicID, items)
		if err != nil {
			renderLibraryError(c, err)
			return
		}
		dto := v1ResolveLibraryResponseDTO{Items: make([]v1ResolveLibraryResultDTO, 0, len(results))}
		for _, result := range results {
			entry := v1ResolveLibraryResultDTO{Provider: result.Provider, SourceID: result.SourceID, Status: result.Status}
			if result.Asset != nil {
				asset := toLibraryAssetDTO(workspace.PublicID, result.Asset)
				entry.Asset = &asset
			}
			dto.Items = append(dto.Items, entry)
		}
		contract.RenderSuccess(c, dto)
	})
}

// OpenAPI deltas for the merger (DO NOT edit apps/api/openapi*.yaml here).
// Add under paths (tags: WorkspaceLibrary), reusing components parameters
// Cursor/PageSize/WorkspaceId/IdempotencyKey/RequestId and responses
// BadRequest/Unauthorized/Forbidden/NotFound/Conflict/UnprocessableEntity:
//
//	/workspaces/{workspace_id}/library/folders:
//	  get: {operationId: listWorkspaceLibraryFolders, parameters: [WorkspaceId, Cursor, PageSize]}
//	  post: {operationId: createWorkspaceLibraryFolder, parameters: [WorkspaceId, IdempotencyKey]}
//	/workspaces/{workspace_id}/library/folders/{folder_id}:
//	  patch: {operationId: renameWorkspaceLibraryFolder, parameters: [WorkspaceId, {name: folder_id, in: path, required: true, schema: {type: string, pattern: '^lfolder_'}}]}
//	  delete: {operationId: deleteWorkspaceLibraryFolder, parameters: [WorkspaceId, {folder_id path}, IdempotencyKey]}
//	/workspaces/{workspace_id}/library/assets:
//	  get: {operationId: listWorkspaceLibraryAssets, parameters: [WorkspaceId, Cursor, PageSize, {name: folder_id, in: query, schema: {type: string}}]}
//	  post: {operationId: saveWorkspaceLibraryAsset, parameters: [WorkspaceId, IdempotencyKey]}
//	/workspaces/{workspace_id}/library/assets/{asset_id}:
//	  patch: {operationId: moveWorkspaceLibraryAsset, parameters: [WorkspaceId, {name: asset_id, in: path, required: true, schema: {type: string, pattern: '^lasset_'}}]}
//	  delete: {operationId: deleteWorkspaceLibraryAsset, parameters: [WorkspaceId, {asset_id path}, IdempotencyKey]}
//	/workspaces/{workspace_id}/library/asset-resolutions:
//	  post: {operationId: resolveWorkspaceLibraryAssets, parameters: [WorkspaceId]}
//
// Materialization paths REMOVED (owner hard cut, internal-only now):
// former POST /workspaces/{workspace_id}/library/materializations
// (createWorkspaceLibraryMaterialization) and GET
// /workspaces/{workspace_id}/library/materializations/{materialization_id}
// (getWorkspaceLibraryMaterialization) are deleted; the merger must
// drop them (and the LibraryMaterialization schema) from openapi YAMLs.
//
// Schemas (assets are provider URL + source metadata only, never bytes):
//
//	LibraryFolder: {id: {pattern: '^lfolder_'}, workspace_id, name: {minLength: 1, maxLength: 100}, created_at: date-time, updated_at: date-time}
//	LibraryAsset: {id: {pattern: '^lasset_'}, workspace_id, folder_id?: {pattern: '^lfolder_'}, provider: {enum: [unsplash, pexels]}, source_id, source_url: uri, preview_url: uri, original_url: uri, title?, author?, width: int32>=0, height: int32>=0, created_at, updated_at}
//	SaveLibraryAssetRequest: {data: {provider, source_id, source_url?: uri, preview_url?: uri, original_url?: uri, title?, author?, width?, height?, folder_id?}} (dedup on provider+source_id: 200 existing vs 201 created)
//	ResolveLibraryAssetsRequest: {data: {items: {type: array, maxItems: 20, items: {provider, source_id?, source_url?}}}}; ResolveLibraryAssetsResponse: {items: [{provider, source_id?, status: {enum: [resolved, unresolved]}, asset?: LibraryAsset}]}
//	LibraryMaterialization schema REMOVED with the HTTP materialization
//	surface (internal-only now; lmat_ records stay store-level only).
//
// Error codes frozen: library_folder_not_found (404), library_asset_not_found (404),
// library_idempotency_conflict (409); reused kernel codes
// workspace_not_found/workspace_forbidden/pagination_cursor_invalid/bad_request/conflict/
// validation_failed/idempotency_key_required/service_unavailable.
// (library_materialization_not_found stays frozen in the kernel error-code
// registry for internal tooling; this surface no longer renders it.)
//
// Integer-ID note for #809: v1 rejects legacy integer ids as malformed 404;
// integer-ID tolerance removal from the legacy handlers is NOT executed here.
