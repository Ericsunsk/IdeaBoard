package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/identity/auth"
	"ideaboard/api/internal/identity/principal"
	identityports "ideaboard/api/internal/identity/users/ports"
	materializerassets "ideaboard/api/internal/media/assets"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ContractV1Deps wires the Public Data Plane v1 handlers (issue #801).
//
// Store is the tenancy.Store interface: the DB sibling implements it
// against sqlc once organizations/workspaces tables land; until then it
// stays nil in production wiring (handlers fail closed with 501) and
// tests pass tenancy.NewInMemoryStoreForTests(). The in-memory fallback
// must never be wired in production.
type ContractV1Deps struct {
	Store       tenancy.Store
	Users       V1UserDirectory
	Idempotency *contract.IdempotencyStore
	// Boards is the workspace-rooted board collaboration store (issue
	// #802). Nil keeps the boards surface fail-closed with 501 until the
	// postgres workspace-board mapping lands (#809); tests inject the
	// in-memory implementation.
	Boards boards.WorkspaceBoardStore
	// Library is the workspace-scoped library + materialization surface
	// (issue #806). Zero value keeps the surface fail-closed (501/503);
	// tests inject the in-memory store plus fakes.
	Library ContractV1LibraryDeps
	// Search is the workspace-scoped search + vision-query surface
	// (issue #806). Zero value keeps the surface fail-closed (501/503);
	// tests inject fakes.
	Search ContractV1SearchDeps
	// Tasks is the workspace-scoped async job surface (issue #805).
	// Production wires the postgres store and settlement adapters; tests
	// inject the in-memory implementation plus quoter/holder/queue/asset fakes.
	Tasks                 WorkspaceTaskStore
	TaskQuoter            WorkspaceTaskQuoter
	TaskHolder            WorkspaceTaskHolder
	TaskQueue             WorkspaceTaskEnqueuer
	TaskAssets            WorkspaceTaskAssetReader
	TaskCanonicalCompiler func(feature, userPrompt string, params map[string]interface{}) (string, error)
	// TaskModelResolver mints the submit-time resolved model into the
	// durable plan (ai-gateway model mapping in production). Nil makes
	// task creation fail closed.
	TaskModelResolver workspacetasks.ModelResolver
	// Media is the workspace-scoped upload-session + media-asset
	// surface (issue #803, restored in #820). Production wires the
	// workspace postgres store; tests inject the in-memory implementation.
	// MediaObjects reads stored bytes for the content/derivative
	// endpoints (nil fails binary reads closed with 503 while
	// metadata keeps working); MediaRenderer is the optional
	// derivative renderer (nil skips generation, tiers stay pending).
	Media         WorkspaceMediaStore
	MediaObjects  WorkspaceMediaObjects
	MediaRenderer v1MediaRenderer
	// AvatarUID resolves a caller's storage UID for the user-scoped
	// avatar PUT (issue #809 media gap batch). Nil fails ONLY the
	// avatar endpoint closed with 501; every other v1 route is
	// unaffected. Production wires NewV1AvatarDirectory(userRepo).
	AvatarUID V1AvatarUIDDirectory
	// Events is the workspace event hot log + SSE replay window (issue
	// #807). Nil normalizes to a process-local log; tests inject a
	// fresh instance per engine for isolation.
	Events *events.WorkspaceEventLog
	// Webhooks owns signed webhook subscriptions + dispatch (issue
	// #807). Nil normalizes to a process-local store with the
	// production HTTPS sender; tests inject fakes.
	Webhooks *events.WebhookStore
	// EventBus publishes v1 envelopes to NATS JetStream (event layer,
	// not a queue). Nil disables cross-instance fanout (single-process
	// tests); production wires the shared bus.
	EventBus *events.NATSJetStreamEventBus
	// libraryMaterializer is the internal remote-fetch seam for the
	// library read-through (media/assets.LibraryMaterializer fetch
	// contract). Nil normalizes to the production remote fetcher
	// (shared outbound downloader: SSRF allowlist, timeout, size cap);
	// tests inject stubs. Unexported: production never overrides it.
	libraryMaterializer materializerassets.LibrarySourceFetcher
	// libraryMaterializeSpawn runs the read-through fetch off the
	// request path. Nil normalizes to a fire-and-forget goroutine;
	// tests inject an inline runner for determinism. Unexported:
	// production never overrides it.
	libraryMaterializeSpawn func(func())
}

// V1UserDirectory translates between internal numeric user ids and the
// external usr_<hex> identifiers without leaking database ids to the API.
// Production adapts the identity UserRepository; tests use a static map.
type V1UserDirectory interface {
	ResolveUserID(ctx context.Context, publicID uuid.UUID) (int64, error)
	GetUserPublicID(ctx context.Context, userID int64) (uuid.UUID, error)
}

// usersRepositoryDirectory adapts identityports.UserRepository to
// V1UserDirectory (production wiring).
type usersRepositoryDirectory struct {
	repo identityports.UserRepository
}

// NewUsersRepositoryDirectory exposes the production V1UserDirectory
// adapter for composition wiring outside this package.
func NewUsersRepositoryDirectory(repo identityports.UserRepository) V1UserDirectory {
	return usersRepositoryDirectory{repo: repo}
}

func (d usersRepositoryDirectory) ResolveUserID(ctx context.Context, publicID uuid.UUID) (int64, error) {
	if d.repo == nil || publicID == uuid.Nil {
		return 0, tenancy.ErrNotFound
	}
	user, err := d.repo.GetByPublicID(ctx, publicID.String())
	if err != nil || user == nil {
		return 0, tenancy.ErrNotFound
	}
	return user.ID, nil
}

func (d usersRepositoryDirectory) GetUserPublicID(ctx context.Context, userID int64) (uuid.UUID, error) {
	if d.repo == nil || userID <= 0 {
		return uuid.Nil, tenancy.ErrNotFound
	}
	user, err := d.repo.GetByID(ctx, userID)
	if err != nil || user == nil {
		return uuid.Nil, tenancy.ErrNotFound
	}
	return user.PublicID, nil
}

func (d *ContractV1Deps) normalize() {
	if d.Idempotency == nil {
		d.Idempotency = contract.NewIdempotencyStore(contract.DefaultIdempotencyStoreOptions())
	}
	if d.Events == nil {
		d.Events = events.NewWorkspaceEventLog()
	}
	if d.Webhooks == nil {
		d.Webhooks = events.NewWebhookStore(events.NewHTTPSender(), nil)
	}
	if d.libraryMaterializer == nil {
		d.libraryMaterializer = materializerassets.RemoteLibrarySourceFetcher{}
	}
	if d.libraryMaterializeSpawn == nil {
		d.libraryMaterializeSpawn = func(fn func()) { go fn() }
	}
}

// v1AuthMiddleware keeps the cookie-session path working in production by
// delegating to the Better Auth session middleware when it is configured;
// in tests (no auth service) it accepts an injected principal instead.
// Either way ResolvePrincipal() runs next to resolve the TenantScope.
func v1AuthMiddleware(authDeps AuthDeps) gin.HandlerFunc {
	if authDeps.AuthServiceClient != nil && authDeps.UserRepo != nil {
		return auth.AuthMiddleware(authDeps.AuthServiceClient, authDeps.UserRepo, authDeps.UserLifecycle)
	}
	return func(c *gin.Context) {
		if _, ok := principal.From(c); !ok {
			contract.RenderError(c, http.StatusUnauthorized, contract.CodeUnauthorized, "未登录")
			c.Abort()
			return
		}
		c.Next()
	}
}

// RegisterContractV1Routes mounts the real v1 handlers on group (exported
// for tests; production calls it from RegisterRoutes with the full auth
// deps). Unknown v1 paths still fail closed with 501.
func RegisterContractV1Routes(group *gin.RouterGroup, authDeps AuthDeps, deps ContractV1Deps) {
	deps.normalize()
	handler := &contractV1Handler{deps: deps}
	group.Use(v1AuthMiddleware(authDeps), ResolvePrincipal())
	group.GET("/user", handler.getUser)
	group.GET("/workspaces", handler.listWorkspaces)
	group.POST("/workspaces", handler.createWorkspace)
	group.GET("/workspaces/:workspace_id", handler.getWorkspace)
	group.PATCH("/workspaces/:workspace_id", handler.updateWorkspace)
	group.POST("/workspaces/:workspace_id/archive", handler.archiveWorkspace)
	group.GET("/workspaces/:workspace_id/members", handler.listWorkspaceMembers)
	group.POST("/workspaces/:workspace_id/members", handler.addWorkspaceMember)
	group.PATCH("/workspaces/:workspace_id/members/:member_id", handler.updateWorkspaceMember)
	group.DELETE("/workspaces/:workspace_id/members/:member_id", handler.removeWorkspaceMember)
	group.GET("/organizations", handler.listOrganizations)
	group.POST("/organizations", handler.createOrganization)
	group.GET("/organizations/:organization_id", handler.getOrganization)
	group.PATCH("/organizations/:organization_id", handler.updateOrganization)
	group.GET("/organizations/:organization_id/workspaces", handler.listOrganizationWorkspaces)
	group.POST("/organizations/:organization_id/workspaces", handler.createOrganizationWorkspace)
	// Workspace upload sessions + media assets (issue #803, restored
	// in #820): the routes below serve the workspace-scoped
	// replacement; media.asset_ready and media.asset_archived emit
	// after their commits.
	handler.registerMediaRoutes(group)
	handler.registerBoardRoutes(group)
	handler.registerLibraryRoutes(group)
	handler.registerSearchRoutes(group)
	handler.registerTaskRoutes(group)
	handler.registerEventRoutes(group)
}

type contractV1Handler struct {
	deps ContractV1Deps
}

// v1ListCursor is the opaque keyset position for v1 list endpoints. It is
// bound to principal + operation at encode sites (Baseline 5.5); decode
// failures render pagination_cursor_invalid.
type v1ListCursor struct {
	Offset int `json:"offset"`
}

func (h *contractV1Handler) requireStore(c *gin.Context) bool {
	if h.deps.Store == nil {
		contract.RenderError(c, http.StatusNotImplemented,
			contract.CodeContractNotImplemented, "The v1 contract surface is not implemented yet")
		c.Abort()
		return false
	}
	return true
}

func v1RequestPrincipal(c *gin.Context) (principal.Principal, bool) {
	identityPrincipal, ok := principal.From(c)
	if !ok {
		contract.RenderError(c, http.StatusUnauthorized, contract.CodeUnauthorized, "未登录")
		c.Abort()
		return principal.Principal{}, false
	}
	return identityPrincipal, true
}

// v1WorkspaceContext resolves the explicit workspace context: malformed
// ids and unknown workspaces are invisible (ok=false, 404 already
// rendered); visible workspaces return the caller's role (member=false
// when the caller holds no membership).
func (h *contractV1Handler) v1WorkspaceContext(c *gin.Context, identityPrincipal principal.Principal) (*tenancy.Workspace, string, bool) {
	raw := strings.TrimSpace(c.Param("workspace_id"))
	if !contract.IsWorkspaceID(raw) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, "工作区不存在")
		c.Abort()
		return nil, "", false
	}
	workspace, err := h.deps.Store.GetWorkspace(c.Request.Context(), raw)
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, "工作区不存在")
		c.Abort()
		return nil, "", false
	}
	role, member, err := h.deps.Store.GetWorkspaceRole(c.Request.Context(), raw, identityPrincipal.UserID())
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, "工作区不存在")
		c.Abort()
		return nil, "", false
	}
	if !member {
		return workspace, "", false
	}
	return workspace, role, true
}

// v1OrganizationContext is the organization-scoped counterpart: hidden
// orgs converge to 404 organization_not_found.
func (h *contractV1Handler) v1OrganizationContext(c *gin.Context, identityPrincipal principal.Principal) (*tenancy.Organization, string, bool) {
	raw := strings.TrimSpace(c.Param("organization_id"))
	if !contract.IsOrganizationID(raw) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationNotFound, "组织不存在")
		c.Abort()
		return nil, "", false
	}
	organization, err := h.deps.Store.GetOrganization(c.Request.Context(), raw)
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationNotFound, "组织不存在")
		c.Abort()
		return nil, "", false
	}
	role, member, err := h.deps.Store.GetOrganizationRole(c.Request.Context(), raw, identityPrincipal.UserID())
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationNotFound, "组织不存在")
		c.Abort()
		return nil, "", false
	}
	if !member {
		return organization, "", false
	}
	return organization, role, true
}

func v1FormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

type v1UserDTO struct {
	ID                  string  `json:"id"`
	DisplayName         string  `json:"display_name"`
	Role                string  `json:"role"`
	PersonalWorkspaceID *string `json:"personal_workspace_id,omitempty"`
}

func (h *contractV1Handler) getUser(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok {
		return
	}
	dto := v1UserDTO{
		ID:          tenancy.FormatUserPublicID(identityPrincipal.PublicID()),
		DisplayName: identityPrincipal.DisplayName(),
		Role:        identityPrincipal.Role().String(),
	}
	// Personal workspace auto-provision hook: idempotent and
	// concurrent-safe; a provision failure never fails /user itself.
	if h.deps.Store != nil {
		if personal, err := h.deps.Store.EnsurePersonalWorkspace(c.Request.Context(), identityPrincipal.UserID()); err == nil && personal != nil {
			id := personal.PublicID
			dto.PersonalWorkspaceID = &id
		}
	}
	contract.RenderSuccess(c, dto)
}

type v1WorkspaceDTO struct {
	ID             string  `json:"id"`
	OrganizationID *string `json:"organization_id"`
	Name           string  `json:"name"`
	CreatedAt      string  `json:"created_at"`
	ArchivedAt     *string `json:"archived_at,omitempty"`
}

func toWorkspaceDTO(w *tenancy.Workspace) v1WorkspaceDTO {
	dto := v1WorkspaceDTO{
		ID:        w.PublicID,
		Name:      w.Name,
		CreatedAt: v1FormatTime(w.CreatedAt),
	}
	if w.OrganizationID != nil {
		org := *w.OrganizationID
		dto.OrganizationID = &org
	}
	if w.ArchivedAt != nil {
		archived := v1FormatTime(*w.ArchivedAt)
		dto.ArchivedAt = &archived
	}
	return dto
}

func (h *contractV1Handler) listWorkspaces(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) {
		return
	}
	pageSize, err := contract.ParsePageSize(c.Query(contract.PageSizeParam))
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "page_size 非法")
		c.Abort()
		return
	}
	offset := 0
	if raw := strings.TrimSpace(c.Query(contract.CursorParam)); raw != "" {
		cursor, decodeErr := contract.DecodeCursor[v1ListCursor](raw)
		if decodeErr != nil || cursor == nil {
			contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, "分页游标非法")
			c.Abort()
			return
		}
		if cursor.Offset < 0 {
			contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, "分页游标非法")
			c.Abort()
			return
		}
		offset = cursor.Offset
	}
	ctx := c.Request.Context()
	workspaces, err := h.deps.Store.ListWorkspacesForUser(ctx, identityPrincipal.UserID())
	if err != nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "工作区服务暂不可用")
		c.Abort()
		return
	}
	// First-list auto-provision hook when the user has no workspace yet.
	if len(workspaces) == 0 {
		if personal, provisionErr := h.deps.Store.EnsurePersonalWorkspace(ctx, identityPrincipal.UserID()); provisionErr == nil && personal != nil {
			workspaces, err = h.deps.Store.ListWorkspacesForUser(ctx, identityPrincipal.UserID())
			if err != nil {
				contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "工作区服务暂不可用")
				c.Abort()
				return
			}
		}
	}
	if offset > len(workspaces) {
		contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, "分页游标非法")
		c.Abort()
		return
	}
	end := offset + pageSize
	if end > len(workspaces) {
		end = len(workspaces)
	}
	page := make([]v1WorkspaceDTO, 0, end-offset)
	for _, w := range workspaces[offset:end] {
		page = append(page, toWorkspaceDTO(w))
	}
	meta := contract.NewPageMeta("", false)
	if end < len(workspaces) {
		next, encodeErr := contract.EncodeCursor(v1ListCursor{Offset: end})
		if encodeErr != nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "工作区服务暂不可用")
			c.Abort()
			return
		}
		meta = contract.NewPageMeta(next, true)
	}
	contract.RenderPaginated(c, page, meta)
}

type v1CreateWorkspaceBody struct {
	Data struct {
		Name           string  `json:"name"`
		OrganizationID *string `json:"organization_id"`
	} `json:"data"`
}

func (h *contractV1Handler) createWorkspace(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) {
		return
	}
	key, err := contract.IdempotencyKeyFromHeader(c)
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, "缺少幂等键，请重试此次操作")
		c.Abort()
		return
	}
	var body v1CreateWorkspaceBody
	if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
		writeV1InputError(c)
		return
	}
	name := strings.TrimSpace(body.Data.Name)
	if validateErr := tenancy.ValidateName(name); validateErr != nil {
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
			"工作区名称非法", contract.FieldErrors{Fields: map[string]string{"name": "required"}})
		c.Abort()
		return
	}
	var orgID *string
	if body.Data.OrganizationID != nil {
		candidate := strings.TrimSpace(*body.Data.OrganizationID)
		if !contract.IsOrganizationID(candidate) {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				"组织标识非法", contract.FieldErrors{Fields: map[string]string{"organization_id": "invalid"}})
			c.Abort()
			return
		}
		orgID = &candidate
	}
	ctx := c.Request.Context()
	// Organization-scoped creation requires org visibility: hidden orgs
	// (unknown id or caller holds no membership) converge to 404
	// organization_not_found; org members without owner/admin get 403.
	if orgID != nil {
		_, role, orgErr := h.orgRoleForCreate(ctx, *orgID, identityPrincipal.UserID())
		if orgErr != nil {
			contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationNotFound, "组织不存在")
			c.Abort()
			return
		}
		if role != tenancy.RoleOwner && role != tenancy.RoleAdmin {
			if !RequireOrganizationCapability(c, true, false) {
				return
			}
		}
	}
	externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
	orgPart := ""
	if orgID != nil {
		orgPart = *orgID
	}
	scope := contract.IdempotencyScope(externalUser, "workspace:create")
	hash := contract.HashIdempotencyRequest("workspace:create", name, orgPart)
	result, _, doErr := h.deps.Idempotency.Do(ctx, scope, identityPrincipal.UserID(), key, hash, func() (any, error) {
		params := tenancy.CreateWorkspaceParams{Name: name, OrganizationID: orgID, OwnerUserID: identityPrincipal.UserID()}
		if orgID != nil {
			return h.deps.Store.CreateOrganizationWorkspace(ctx, *orgID, identityPrincipal.UserID(), name)
		}
		return h.deps.Store.CreateWorkspace(ctx, params)
	})
	if doErr != nil {
		if contract.IsIdempotencyConflict(doErr) {
			contract.RenderError(c, http.StatusConflict, contract.CodeWorkspaceIdempotencyConflict, "幂等请求与原请求不一致")
			c.Abort()
			return
		}
		if errors.Is(doErr, tenancy.ErrNotFound) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationNotFound, "组织不存在")
			c.Abort()
			return
		}
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "工作区服务暂不可用")
		c.Abort()
		return
	}
	created, ok := result.(*tenancy.Workspace)
	if !ok || created == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "工作区服务暂不可用")
		c.Abort()
		return
	}
	contract.RenderCreated(c, toWorkspaceDTO(created))
}

func (h *contractV1Handler) orgRoleForCreate(ctx context.Context, orgID string, userID int64) (*tenancy.Organization, string, error) {
	organization, err := h.deps.Store.GetOrganization(ctx, orgID)
	if err != nil {
		return nil, "", err
	}
	role, member, err := h.deps.Store.GetOrganizationRole(ctx, orgID, userID)
	if err != nil {
		return nil, "", err
	}
	if !member {
		return organization, "", tenancy.ErrForbidden
	}
	return organization, role, nil
}

func (h *contractV1Handler) getWorkspace(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx) {
		contract.RenderSuccess(c, toWorkspaceDTO(wctx.Workspace))
	})
}

type v1UpdateWorkspaceBody struct {
	Data struct {
		Name string `json:"name"`
	} `json:"data"`
}

func (h *contractV1Handler) updateWorkspace(c *gin.Context) {
	withV1WorkspaceBody(h, c, v1WorkspaceOpts{Allow: v1AllowOwnerOrAdmin}, func(wctx v1WorkspaceCtx, body v1UpdateWorkspaceBody) {
		name := strings.TrimSpace(body.Data.Name)
		if validateErr := tenancy.ValidateName(name); validateErr != nil {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				"工作区名称非法", contract.FieldErrors{Fields: map[string]string{"name": "required"}})
			c.Abort()
			return
		}
		updated, err := h.deps.Store.UpdateWorkspace(c.Request.Context(), wctx.Workspace.PublicID, name)
		if err != nil {
			contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, "工作区不存在")
			c.Abort()
			return
		}
		contract.RenderSuccess(c, toWorkspaceDTO(updated))
	})
}

func (h *contractV1Handler) archiveWorkspace(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Allow: v1AllowOwnerOrAdmin}, func(wctx v1WorkspaceCtx) {
		archived, err := h.deps.Store.ArchiveWorkspace(c.Request.Context(), wctx.Workspace.PublicID)
		if err != nil {
			contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, "工作区不存在")
			c.Abort()
			return
		}
		contract.RenderSuccess(c, toWorkspaceDTO(archived))
	})
}
