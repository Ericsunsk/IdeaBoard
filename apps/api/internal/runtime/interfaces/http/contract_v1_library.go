package httpapi

// Workspace-scoped library + materialization handlers (issue #806).
//
// Additive only: all routes mount under
// /api/v1/workspaces/{workspace_id}/library via the existing
// ContractV1GroupPath + PublicContractChain pattern. Legacy
// /api/user library routes are untouched (removal is #809).
//
// Agreed surface:
//
//	GET/POST    /workspaces/{workspace_id}/library/folders
//	PATCH/DELETE /workspaces/{workspace_id}/library/folders/{folder_id}
//	GET/POST    /workspaces/{workspace_id}/library/assets
//	PATCH/DELETE /workspaces/{workspace_id}/library/assets/{asset_id}
//	POST        /workspaces/{workspace_id}/library/asset-resolutions
//	POST        /workspaces/{workspace_id}/library/materializations
//	GET         /workspaces/{workspace_id}/library/materializations/{materialization_id}
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
// Materialization is a separate durable job record (status-addressable
// pending -> running -> completed|failed; terminal states immutable,
// result links asset_... on success). Billing settles through the
// credits.Commands path (V1CreditSettler: Hold/Capture/Release with
// caller idempotency keys; no direct ledger writes here). Billing
// failures reuse the #804 codes (billing_quote_unavailable 503,
// billing_wallet_restricted 403).
//
// Production wiring leaves ContractV1LibraryDeps zero-valued until the
// postgres mapping + credits/billing adapters land (#809); nil store
// fails closed with 501, nil auxiliary seams with 503. Tests inject the
// in-memory store plus fakes.

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/library"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

// V1LibraryMaterializer turns an owned workspace library reference
// into a managed Media Asset. It owns remote fetch, storage, and Media
// registration (Media boundary); the handler only passes owned facts.
// The returned id must carry the asset_ shape; anything else fails the
// job closed. Production adapts the media librarymaterialization seam;
// tests inject fakes. Nil fails closed with 503.
type V1LibraryMaterializer interface {
	MaterializeWorkspaceAsset(ctx context.Context, workspacePublicID string, asset *library.V1Asset) (string, error)
}

// V1CreditSettler settles metered workspace operations through the
// credits.Commands path: Hold reserves, Capture consumes on success,
// Release refunds on failure. Every command carries a caller-owned
// idempotency key so retries converge. Only Credits owns ledger writes;
// this seam performs none. Production adapts *credits.Service;
// tests inject fakes. Nil fails closed with 503.
type V1CreditSettler interface {
	Hold(ctx context.Context, userID int64, amountCredits int64, idempotencyKey, reason string) (string, error)
	Capture(ctx context.Context, userID int64, holdID string, amountCredits int64, idempotencyKey string) error
	Release(ctx context.Context, userID int64, holdID string, amountCredits int64, idempotencyKey string) error
}

// V1BillingQuoter mints server-side price snapshots for metered
// workspace operations. No client price input is accepted on any route.
// Production adapts the billing catalog QuoteAtResolution seam;
// tests inject fakes. Nil fails closed with 503.
type V1BillingQuoter interface {
	Quote(ctx context.Context, workspacePublicID, sku string) (int64, error)
}

// ContractV1LibraryDeps wires the workspace library handlers. Zero
// values fail closed; production fills every field when the postgres
// mapping + adapters land (#809).
type ContractV1LibraryDeps struct {
	Store        library.WorkspaceLibraryStore
	Materializer V1LibraryMaterializer
	Billing      V1CreditSettler
	Quoter       V1BillingQuoter
	Idempotency  *contract.IdempotencyStore
}

func (d *ContractV1LibraryDeps) normalize() {
	if d.Idempotency == nil {
		d.Idempotency = contract.NewIdempotencyStore(contract.DefaultIdempotencyStoreOptions())
	}
}

// V1MaterializeSKU is the metered SKU quoted for library
// materialization. It has no registry entry yet: the production quoter
// reports billing_quote_unavailable until the SKU is registered
// (#805 follow-up); tests stub the quoter.
const V1MaterializeSKU = "library:materialize"

var (
	// errV1BillingQuoteUnavailable maps quoter failures to 503
	// billing_quote_unavailable. Seams return it (or wrap it) instead
	// of leaking provider errors.
	errV1BillingQuoteUnavailable = errors.New("v1 billing quote unavailable")
	// errV1BillingRestricted maps hold rejections (frozen/restricted/
	// insufficient wallets) to 403 billing_wallet_restricted.
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
	group.POST("/workspaces/:workspace_id/library/materializations", h.createLibraryMaterialization)
	group.GET("/workspaces/:workspace_id/library/materializations/:materialization_id", h.getLibraryMaterialization)
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
	case errors.Is(err, library.ErrV1MaterializationNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeLibraryMaterializationNotFound, "转存任务不存在")
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

func renderLibraryBillingError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errV1BillingRestricted):
		contract.RenderError(c, http.StatusForbidden, contract.CodeBillingWalletRestricted, "当前工作区钱包受限，暂不可用")
	case errors.Is(err, errV1BillingQuoteUnavailable):
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeBillingQuoteUnavailable, "价格服务暂不可用")
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
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) || !h.requireLibraryStore(c) {
		return
	}
	workspace, _ := h.libraryWorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return
	}
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
}

type v1CreateLibraryFolderBody struct {
	Data struct {
		Name string `json:"name"`
	} `json:"data"`
}

func (h *contractV1Handler) createLibraryFolder(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) || !h.requireLibraryStore(c) {
		return
	}
	workspace, _ := h.libraryWorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return
	}
	key, ok := requireLibraryIdempotencyKey(c)
	if !ok {
		return
	}
	var body v1CreateLibraryFolderBody
	if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
		contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
			"请求体非法", contract.FieldErrors{Fields: map[string]string{"data": "invalid"}})
		c.Abort()
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
}

type v1RenameLibraryFolderBody struct {
	Data struct {
		Name string `json:"name"`
	} `json:"data"`
}

func (h *contractV1Handler) libraryFolderOr404(c *gin.Context, workspace *tenancy.Workspace) (*library.V1Folder, bool) {
	raw := strings.TrimSpace(c.Param("folder_id"))
	if !library.IsV1FolderID(raw) {
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
		if folder.PublicID == raw {
			return folder, true
		}
	}
	contract.RenderError(c, http.StatusNotFound, contract.CodeLibraryFolderNotFound, "文件夹不存在")
	c.Abort()
	return nil, false
}

func (h *contractV1Handler) renameLibraryFolder(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) || !h.requireLibraryStore(c) {
		return
	}
	workspace, _ := h.libraryWorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return
	}
	folder, ok := h.libraryFolderOr404(c, workspace)
	if !ok {
		return
	}
	var body v1RenameLibraryFolderBody
	if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
		contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
			"请求体非法", contract.FieldErrors{Fields: map[string]string{"data": "invalid"}})
		c.Abort()
		return
	}
	renamed, err := h.deps.Library.Store.RenameFolder(c.Request.Context(), workspace.PublicID, folder.PublicID, body.Data.Name)
	if err != nil {
		renderLibraryError(c, err)
		return
	}
	contract.RenderSuccess(c, toLibraryFolderDTO(workspace.PublicID, renamed))
}

func (h *contractV1Handler) deleteLibraryFolder(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) || !h.requireLibraryStore(c) {
		return
	}
	workspace, _ := h.libraryWorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return
	}
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
}

func (h *contractV1Handler) listLibraryAssets(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) || !h.requireLibraryStore(c) {
		return
	}
	workspace, _ := h.libraryWorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return
	}
	var folderFilter *string
	if raw := strings.TrimSpace(c.Query("folder_id")); raw != "" {
		if !library.IsV1FolderID(raw) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeLibraryFolderNotFound, "文件夹不存在")
			c.Abort()
			return
		}
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
}

func (h *contractV1Handler) libraryFolderByID(c *gin.Context, workspace *tenancy.Workspace, folderID string) (*library.V1Folder, bool) {
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
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) || !h.requireLibraryStore(c) {
		return
	}
	workspace, _ := h.libraryWorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return
	}
	key, ok := requireLibraryIdempotencyKey(c)
	if !ok {
		return
	}
	var body v1SaveLibraryAssetBody
	if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
		contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
			"请求体非法", contract.FieldErrors{Fields: map[string]string{"data": "invalid"}})
		c.Abort()
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
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) || !h.requireLibraryStore(c) {
		return
	}
	workspace, _ := h.libraryWorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return
	}
	asset, ok := h.libraryAssetOr404(c, workspace)
	if !ok {
		return
	}
	var body v1MoveLibraryAssetBody
	if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
		contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
			"请求体非法", contract.FieldErrors{Fields: map[string]string{"data": "invalid"}})
		c.Abort()
		return
	}
	moved, err := h.deps.Library.Store.MoveAsset(c.Request.Context(), workspace.PublicID, asset.PublicID, body.Data.FolderID)
	if err != nil {
		renderLibraryError(c, err)
		return
	}
	contract.RenderSuccess(c, toLibraryAssetDTO(workspace.PublicID, moved))
}

func (h *contractV1Handler) deleteLibraryAsset(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) || !h.requireLibraryStore(c) {
		return
	}
	workspace, _ := h.libraryWorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return
	}
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
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) || !h.requireLibraryStore(c) {
		return
	}
	workspace, _ := h.libraryWorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return
	}
	var body v1ResolveLibraryAssetsBody
	if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
		contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
			"请求体非法", contract.FieldErrors{Fields: map[string]string{"data": "invalid"}})
		c.Abort()
		return
	}
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
}

type v1CreateLibraryMaterializationBody struct {
	Data struct {
		LibraryAssetID string `json:"library_asset_id"`
	} `json:"data"`
}

type v1LibraryMaterializationDTO struct {
	ID             string `json:"id"`
	WorkspaceID    string `json:"workspace_id"`
	LibraryAssetID string `json:"library_asset_id"`
	Status         string `json:"status"`
	MediaAssetID   string `json:"media_asset_id,omitempty"`
	ErrorCode      string `json:"error_code,omitempty"`
	ErrorMessage   string `json:"error_message,omitempty"`
	Replayed       bool   `json:"replayed,omitempty"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

func toLibraryMaterializationDTO(workspacePublicID string, record *library.V1Materialization, replayed bool) v1LibraryMaterializationDTO {
	return v1LibraryMaterializationDTO{
		ID: record.PublicID, WorkspaceID: workspacePublicID, LibraryAssetID: record.AssetPublicID,
		Status: string(record.Status), MediaAssetID: record.MediaAssetID,
		ErrorCode: record.ErrorCode, ErrorMessage: record.ErrorMessage, Replayed: replayed,
		CreatedAt: v1FormatTime(record.CreatedAt), UpdatedAt: v1FormatTime(record.UpdatedAt),
	}
}

func (h *contractV1Handler) createLibraryMaterialization(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) || !h.requireLibraryStore(c) {
		return
	}
	workspace, _ := h.libraryWorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return
	}
	if h.deps.Library.Quoter == nil || h.deps.Library.Billing == nil {
		renderLibraryBillingError(c, errV1BillingQuoteUnavailable)
		return
	}
	if h.deps.Library.Materializer == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "转存服务暂不可用")
		c.Abort()
		return
	}
	key, ok := requireLibraryIdempotencyKey(c)
	if !ok {
		return
	}
	var body v1CreateLibraryMaterializationBody
	if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
		contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
			"请求体非法", contract.FieldErrors{Fields: map[string]string{"data": "invalid"}})
		c.Abort()
		return
	}
	asset, ok := h.libraryAssetByID(c, workspace, body.Data.LibraryAssetID)
	if !ok {
		return
	}
	externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
	hash := contract.HashIdempotencyRequest("library:materialize", workspace.PublicID, asset.PublicID)
	result, replayed, fnErr, ok := h.libraryDo(c, externalUser, identityPrincipal.UserID(), "materialize", key, hash, func() (any, error) {
		return h.runLibraryMaterialization(c, workspace, identityPrincipal, asset)
	})
	if !ok {
		return
	}
	if fnErr != nil {
		// Billing failures surface as error responses (no job record is
		// created before the hold succeeds); materializer failures are
		// already folded into a failed job record below and never reach
		// here.
		if errors.Is(fnErr, errV1BillingRestricted) || errors.Is(fnErr, errV1BillingQuoteUnavailable) {
			renderLibraryBillingError(c, fnErr)
			return
		}
		renderLibraryError(c, fnErr)
		return
	}
	record, ok := result.(*library.V1Materialization)
	if !ok || record == nil {
		renderLibraryError(c, errors.New("library: materialize type mismatch"))
		return
	}
	logLibraryAudit("library_materialize", identityPrincipal.UserID(),
		workspace.PublicID+"/"+record.PublicID, "materialize:"+asset.PublicID, string(record.Status))
	// #807: materialization + billing.settled AFTER the terminal record
	// commit, fresh executions only (idempotent replays never re-emit).
	if !replayed {
		h.emitLibraryMaterialization(c.Request.Context(), identityPrincipal, workspace.PublicID, asset.PublicID, record)
	}
	contract.RenderAccepted(c, toLibraryMaterializationDTO(workspace.PublicID, record, replayed))
}

func (h *contractV1Handler) libraryAssetByID(c *gin.Context, workspace *tenancy.Workspace, raw string) (*library.V1Asset, bool) {
	trimmed := strings.TrimSpace(raw)
	if !library.IsV1AssetID(trimmed) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeLibraryAssetNotFound, "素材不存在")
		c.Abort()
		return nil, false
	}
	asset, err := h.deps.Library.Store.GetAsset(c.Request.Context(), workspace.PublicID, trimmed)
	if err != nil {
		renderLibraryError(c, err)
		return nil, false
	}
	return asset, true
}

// runLibraryMaterialization executes one materialization inside the
// library:materialize idempotency scope: server-side quote, credit
// hold through credits.Commands, job record (pending -> running),
// media settle, then capture (completed, asset_ link) or release
// (failed record). Materializer failures fold into the failed record
// so the job stays status-addressable; only billing failures abort
// with an error response.
func (h *contractV1Handler) runLibraryMaterialization(c *gin.Context, workspace *tenancy.Workspace, identityPrincipal principal.Principal, asset *library.V1Asset) (any, error) {
	ctx := c.Request.Context()
	amount, quoteErr := h.deps.Library.Quoter.Quote(ctx, workspace.PublicID, V1MaterializeSKU)
	if quoteErr != nil || amount < 0 {
		return nil, errV1BillingQuoteUnavailable
	}
	publicID, mintErr := library.NewV1MaterializationPublicID()
	if mintErr != nil {
		return nil, mintErr
	}
	holdKey := "library-materialize:" + publicID + ":hold"
	holdID, holdErr := h.deps.Library.Billing.Hold(ctx, identityPrincipal.UserID(), amount, holdKey, "library materialize")
	if holdErr != nil {
		if errors.Is(holdErr, errV1BillingRestricted) {
			return nil, errV1BillingRestricted
		}
		return nil, holdErr
	}
	if _, createErr := h.deps.Library.Store.CreateMaterialization(ctx, workspace.PublicID, asset.PublicID, publicID); createErr != nil {
		_ = h.deps.Library.Billing.Release(ctx, identityPrincipal.UserID(), holdID, amount, "library-materialize:"+publicID+":release")
		return nil, createErr
	}
	_, _ = h.deps.Library.Store.SetMaterializationResult(ctx, workspace.PublicID, publicID, library.V1MaterializationRunning, "", "", "")
	mediaAssetID, materializeErr := h.deps.Library.Materializer.MaterializeWorkspaceAsset(ctx, workspace.PublicID, asset)
	if materializeErr != nil {
		_ = h.deps.Library.Billing.Release(ctx, identityPrincipal.UserID(), holdID, amount, "library-materialize:"+publicID+":release")
		slog.Warn("library v1 materialize failed", "workspace", workspace.PublicID, "materialization", publicID, "error", materializeErr)
		failed, setErr := h.deps.Library.Store.SetMaterializationResult(ctx, workspace.PublicID, publicID,
			library.V1MaterializationFailed, "", "library_materialization_failed", "素材转存失败，请稍后重试")
		if setErr != nil {
			return nil, setErr
		}
		return failed, nil
	}
	if !contract.IsAssetID(strings.TrimSpace(mediaAssetID)) {
		_ = h.deps.Library.Billing.Release(ctx, identityPrincipal.UserID(), holdID, amount, "library-materialize:"+publicID+":release")
		slog.Warn("library v1 materialize returned non-asset id", "workspace", workspace.PublicID, "materialization", publicID)
		failed, setErr := h.deps.Library.Store.SetMaterializationResult(ctx, workspace.PublicID, publicID,
			library.V1MaterializationFailed, "", "library_materialization_failed", "素材转存失败，请稍后重试")
		if setErr != nil {
			return nil, setErr
		}
		return failed, nil
	}
	if captureErr := h.deps.Library.Billing.Capture(ctx, identityPrincipal.UserID(), holdID, amount, "library-materialize:"+publicID+":capture"); captureErr != nil {
		_ = h.deps.Library.Billing.Release(ctx, identityPrincipal.UserID(), holdID, amount, "library-materialize:"+publicID+":release")
		failed, setErr := h.deps.Library.Store.SetMaterializationResult(ctx, workspace.PublicID, publicID,
			library.V1MaterializationFailed, "", "billing_settle_failed", "结算失败，请稍后重试")
		if setErr != nil {
			return nil, setErr
		}
		return failed, nil
	}
	completed, setErr := h.deps.Library.Store.SetMaterializationResult(ctx, workspace.PublicID, publicID,
		library.V1MaterializationCompleted, strings.TrimSpace(mediaAssetID), "", "")
	if setErr != nil {
		return nil, setErr
	}
	return completed, nil
}

// emitLibraryMaterialization emits library.materialization_completed /
// _failed plus the billing.settled capture/release ref for a terminal
// record. Non-terminal records emit nothing.
func (h *contractV1Handler) emitLibraryMaterialization(ctx context.Context, identityPrincipal principal.Principal, workspacePublicID, libraryAssetID string, record *library.V1Materialization) {
	if record == nil {
		return
	}
	resource := events.V1Resource{Type: events.V1ResourceLibraryMaterialization, ID: record.PublicID}
	switch record.Status {
	case library.V1MaterializationCompleted:
		h.emitWorkspaceEvent(ctx, identityPrincipal, workspacePublicID,
			events.EventV1LibraryMaterializedComplete, resource,
			events.V1LibraryMaterializedCompletedData{
				MaterializationID: record.PublicID, LibraryAssetID: libraryAssetID,
				MediaAssetID: record.MediaAssetID,
			})
		h.emitWorkspaceEvent(ctx, identityPrincipal, workspacePublicID, events.EventV1BillingSettled,
			resource, events.V1BillingSettledData{
				ReferenceType: events.V1ResourceLibraryMaterialization,
				ReferenceID:   record.PublicID, SettlementState: "captured",
			})
	case library.V1MaterializationFailed:
		errorCode := strings.TrimSpace(record.ErrorCode)
		if errorCode == "" {
			errorCode = "library_materialization_failed"
		}
		h.emitWorkspaceEvent(ctx, identityPrincipal, workspacePublicID,
			events.EventV1LibraryMaterializedFailed, resource,
			events.V1LibraryMaterializedFailedData{
				MaterializationID: record.PublicID, LibraryAssetID: libraryAssetID,
				ErrorCode: errorCode,
			})
		h.emitWorkspaceEvent(ctx, identityPrincipal, workspacePublicID, events.EventV1BillingSettled,
			resource, events.V1BillingSettledData{
				ReferenceType: events.V1ResourceLibraryMaterialization,
				ReferenceID:   record.PublicID, SettlementState: "released",
			})
	}
}

func (h *contractV1Handler) getLibraryMaterialization(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) || !h.requireLibraryStore(c) {
		return
	}
	workspace, _ := h.libraryWorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return
	}
	raw := strings.TrimSpace(c.Param("materialization_id"))
	if !library.IsV1MaterializationID(raw) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeLibraryMaterializationNotFound, "转存任务不存在")
		c.Abort()
		return
	}
	record, err := h.deps.Library.Store.GetMaterialization(c.Request.Context(), workspace.PublicID, raw)
	if err != nil {
		renderLibraryError(c, err)
		return
	}
	contract.RenderSuccess(c, toLibraryMaterializationDTO(workspace.PublicID, record, false))
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
//	/workspaces/{workspace_id}/library/materializations:
//	  post: {operationId: createWorkspaceLibraryMaterialization, parameters: [WorkspaceId, IdempotencyKey]}
//	/workspaces/{workspace_id}/library/materializations/{materialization_id}:
//	  get: {operationId: getWorkspaceLibraryMaterialization, parameters: [WorkspaceId, {name: materialization_id, in: path, required: true, schema: {type: string, pattern: '^lmat_'}}]}
//
// Schemas (assets are provider URL + source metadata only, never bytes):
//
//	LibraryFolder: {id: {pattern: '^lfolder_'}, workspace_id, name: {minLength: 1, maxLength: 100}, created_at: date-time, updated_at: date-time}
//	LibraryAsset: {id: {pattern: '^lasset_'}, workspace_id, folder_id?: {pattern: '^lfolder_'}, provider: {enum: [unsplash, pexels]}, source_id, source_url: uri, preview_url: uri, original_url: uri, title?, author?, width: int32>=0, height: int32>=0, created_at, updated_at}
//	SaveLibraryAssetRequest: {data: {provider, source_id, source_url?: uri, preview_url?: uri, original_url?: uri, title?, author?, width?, height?, folder_id?}} (dedup on provider+source_id: 200 existing vs 201 created)
//	ResolveLibraryAssetsRequest: {data: {items: {type: array, maxItems: 20, items: {provider, source_id?, source_url?}}}}; ResolveLibraryAssetsResponse: {items: [{provider, source_id?, status: {enum: [resolved, unresolved]}, asset?: LibraryAsset}]}
//	LibraryMaterialization: {id: {pattern: '^lmat_'}, workspace_id, library_asset_id: {pattern: '^lasset_'}, status: {enum: [pending, running, completed, failed]}, media_asset_id?: {pattern: '^asset_'}, error_code?, error_message?, replayed?: boolean, created_at, updated_at}
//
// Error codes frozen: library_folder_not_found (404), library_asset_not_found (404),
// library_materialization_not_found (404), library_idempotency_conflict (409); reused kernel codes
// workspace_not_found/workspace_forbidden/pagination_cursor_invalid/bad_request/conflict/
// validation_failed/idempotency_key_required/service_unavailable plus #804
// billing_quote_unavailable (503)/billing_wallet_restricted (403).
//
// Integer-ID note for #809: v1 rejects legacy integer ids as malformed 404;
// integer-ID tolerance removal from the legacy handlers is NOT executed here.
