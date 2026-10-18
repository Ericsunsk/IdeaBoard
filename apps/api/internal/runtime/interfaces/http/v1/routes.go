package v1

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/events"
	eventnats "ideaboard/api/internal/events/nats"
	eventoutbox "ideaboard/api/internal/events/outbox"
	eventwebhook "ideaboard/api/internal/events/webhook"
	"ideaboard/api/internal/generation/routing"
	"ideaboard/api/internal/identity/principal"
	identitydomain "ideaboard/api/internal/identity/users/domain"
	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	"ideaboard/api/internal/media/derivatives/infrastructure/preview"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/settings"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

// ContractV1Deps wires the Public Data Plane v1 handlers (issue #801).
//
// Store is the tenancy persistence seam: production uses PostgreSQL, and
// tests pass tenancy.NewInMemoryStoreForTests(). The in-memory fallback
// must never be wired in production.
type ContractV1Deps struct {
	Store       tenancy.Store
	Users       runtimehttp.V1UserDirectory
	Idempotency *contract.IdempotencyStore
	// Boards is the workspace-rooted board collaboration store (issue
	// #802). Nil keeps the boards surface fail-closed with 501 until the
	// postgres workspace-board mapping lands (#809); tests inject the
	// in-memory implementation. Role facts leave this store only through
	// the narrow boards.RoleReader port (#934: h.boardRoles()); Media /
	// Tasking / Events must not copy the role mapping or reach past it
	// into Boards storage.
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
	Tasks                 WorkspaceTaskReadStore
	TaskLifecycle         WorkspaceTaskLifecycleStore
	TaskBilling           WorkspaceTaskBillingStore
	TaskQuoter            WorkspaceTaskQuoter
	TaskHolder            WorkspaceTaskHolder
	TaskQueue             WorkspaceTaskEnqueuer
	TaskAssets            WorkspaceTaskAssetReader
	TaskCanonicalCompiler func(service, userPrompt string, params map[string]interface{}) (string, error)
	// TaskAcceptance is the submit gate for the settlement transaction.
	// Nil wires the production workspacemedia.TaskSubmissionAuthorizer;
	// in-memory HTTP fixtures inject their test double instead (no
	// database for the workspace lock the production gate takes).
	TaskAcceptance workspacetasks.SubmissionAuthorizer
	// TaskRouter is the shared RoutingResolver quote/submit consume (same
	// instance, never an independent lookup). Nil fails closed with the
	// canonical service_resolution_unavailable sentinel.
	TaskRouter *routing.RoutingResolver
	// TaskSnapshotSaver pins the Resolve snapshot. Production wiring must
	// provide it; atomic settlement fails closed when it is absent.
	TaskSnapshotSaver WorkspaceTaskSnapshotSaver
	// Media is the workspace-scoped upload-session + media-asset
	// surface (issue #803, restored in #820). Production wires the
	// workspace postgres store; tests inject the in-memory implementation.
	// MediaObjects reads stored bytes for the content endpoint (nil
	// fails content reads closed with 503 while metadata keeps
	// working).
	Media         WorkspaceMediaStore
	MediaObjects  WorkspaceMediaObjects
	MediaRenderer *imgproxy.Renderer
	// AvatarUID resolves a caller's storage UID for the user-scoped
	// avatar PUT (issue #809 media gap batch). Nil fails ONLY the
	// avatar endpoint closed with 501; every other v1 route is
	// unaffected. Production wires NewV1AvatarDirectory(userRepo).
	AvatarUID V1AvatarUIDDirectory
	// AvatarObjects persists avatar bytes through the storage
	// abstraction (active profile). Nil fails ONLY the avatar PUT
	// closed with 503. Production wires services.storageService;
	// tests inject fakes.
	AvatarObjects V1AvatarObjects
	// Events is the workspace event hot log + SSE replay window (issue
	// #807). Nil normalizes to a process-local log; tests inject a
	// fresh instance per engine for isolation.
	Events *events.WorkspaceEventLog
	// Webhooks owns signed webhook subscriptions + dispatch (issue
	// #807). Nil normalizes to a process-local store with the
	// production HTTPS sender; tests inject fakes.
	Webhooks *eventwebhook.WebhookStore
	// EventBus publishes v1 envelopes to NATS JetStream (event layer,
	// not a queue). Nil disables cross-instance fanout (single-process
	// tests); production wires the shared bus.
	EventBus *eventnats.NATSJetStreamEventBus
	// BoardAccessOutbox acknowledges immediate delivery of transaction-committed
	// access signals. Pending facts remain retryable by the River sweeper.
	// Nil is reserved for in-memory tests without a durable store.
	BoardAccessOutbox *eventoutbox.BoardAccessOutboxStore
	SupportChannels   settings.SupportChannelsSettingProvider
	// AvatarProfile persists uploads and projects the user record on session reads.
	// Nil omits the read projection and fails avatar uploads closed with 503.
	AvatarProfile runtimehttp.V1AvatarProfileDirectory
	MediaPreviews *preview.Service
	// MemberDisplay resolves workspace-member display facts (Auth-owned
	// email for invite search plus avatar public URL) for the member
	// list/write shapes (display auxiliaries; usr_ stays the write
	// identity). Nil fails closed to no facts (fields omitted) and never
	// fails the request. Production wires NewMemberDisplayService over
	// the users repository plus the Auth email reader; tests inject fakes.
	MemberDisplay V1MemberDisplayProvider
	// MemberLookup resolves one registered user's usr_ identity from an
	// Auth-owned email for the invite search (query-only; the email is
	// resolved per call and never stored). Nil fails the lookup closed
	// with 501. Production wires NewMemberLookupService over the users
	// repository plus the Auth lookup reader; tests inject fakes.
	MemberLookup V1MemberLookupProvider
}

// requireV1IdempotencyKey enforces the shared header-only idempotency contract
// for required v1 mutations. Missing, blank, malformed, and overlong values
// all converge to the stable 400 idempotency_key_required response.
func requireV1IdempotencyKey(c *gin.Context) (string, bool) {
	key, err := contract.IdempotencyKeyFromHeader(c)
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, runtimehttp.MsgIdempotencyKeyRequired2)
		c.Abort()
		return "", false
	}
	return key, true
}

// contractV1UserDirectoryNote: runtimehttp.V1UserDirectory, runtimehttp.V1AvatarProfileDirectory,
// usersRepositoryDirectory, runtimehttp.NewUsersRepositoryDirectory and
// runtimehttp.NewV1AvatarProfileDirectory live in the runtimehttp kernel (shared.go)
// so v1 and control share them without a control -> v1 edge.

func (d *ContractV1Deps) normalize() {
	if d.Tasks != nil {
		if d.TaskLifecycle == nil {
			if store, ok := d.Tasks.(workspacetasks.TaskLifecycleStore); ok {
				d.TaskLifecycle = store
			}
		}
		if d.TaskBilling == nil {
			if store, ok := d.Tasks.(workspacetasks.TaskBillingStore); ok {
				d.TaskBilling = store
			}
		}
	}
	if d.Idempotency == nil {
		d.Idempotency = contract.NewIdempotencyStore(contract.DefaultIdempotencyStoreOptions())
	}
	if d.Events == nil {
		d.Events = events.NewWorkspaceEventLog()
	}
	if d.Webhooks == nil {
		d.Webhooks = eventwebhook.NewWebhookStore(eventwebhook.NewHTTPSender(), nil)
	}
}

// contractV1AuthNote: session auth rides the kernel runtimehttp.V1AuthMiddleware
// (shared.go); runtimehttp.ResolvePrincipal is kernel runtimehttp.ResolvePrincipal.

// RegisterContractV1Routes mounts the real v1 handlers on group (exported
// for tests; production calls it from RegisterRoutes with the full auth
// deps). Unknown v1 paths still fail closed with 501.
func RegisterContractV1Routes(group *gin.RouterGroup, authDeps runtimehttp.AuthDeps, deps ContractV1Deps) {
	deps.normalize()
	handler := &contractV1Handler{deps: deps}
	group.GET("/support-channels", publicSupportChannelsV1Handler(deps.SupportChannels))
	// Public avatar delivery (no auth: avatars are public). Registered
	// before the session middleware so it never requires a principal;
	// per-IP rate limiting still rides the global engine middleware.
	group.GET("/public/users/:uid/avatar", handler.publicAvatar)
	// Public branding logo delivery (no auth: the logo is public). Same
	// placement discipline as the avatar route: before the session
	// middleware, single serving path for site-asset keys.
	group.GET("/public/branding/logo", handler.publicBrandingLogo)
	group.Use(runtimehttp.V1AuthMiddleware(authDeps), runtimehttp.ResolvePrincipal())
	group.GET("/user", handler.getUser)
	group.GET("/workspaces", handler.listWorkspaces)
	group.POST("/workspaces", handler.createWorkspace)
	group.GET("/workspaces/:workspace_id", handler.getWorkspace)
	group.PATCH("/workspaces/:workspace_id", handler.updateWorkspace)
	group.POST("/workspaces/:workspace_id/archive", handler.archiveWorkspace)
	group.GET("/workspaces/:workspace_id/members", handler.listWorkspaceMembers)
	group.POST("/workspaces/:workspace_id/members", handler.addWorkspaceMember)
	group.GET("/workspaces/:workspace_id/members/lookup", handler.lookupWorkspaceMemberByEmail)
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
		contract.RenderError(c, http.StatusUnauthorized, contract.CodeUnauthorized, runtimehttp.MsgUnauthorized)
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
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, runtimehttp.MsgListControlBillingAccounts)
		c.Abort()
		return nil, "", false
	}
	workspace, err := h.deps.Store.GetWorkspace(c.Request.Context(), raw)
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, runtimehttp.MsgListControlBillingAccounts)
		c.Abort()
		return nil, "", false
	}
	role, member, err := h.deps.Store.GetWorkspaceRole(c.Request.Context(), raw, identityPrincipal.UserID())
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, runtimehttp.MsgListControlBillingAccounts)
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
		contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationNotFound, runtimehttp.MsgOrganizationNotFound)
		c.Abort()
		return nil, "", false
	}
	organization, err := h.deps.Store.GetOrganization(c.Request.Context(), raw)
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationNotFound, runtimehttp.MsgOrganizationNotFound)
		c.Abort()
		return nil, "", false
	}
	role, member, err := h.deps.Store.GetOrganizationRole(c.Request.Context(), raw, identityPrincipal.UserID())
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationNotFound, runtimehttp.MsgOrganizationNotFound)
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
	Avatar              string  `json:"avatar,omitempty"`
	// StorageNamespace is users.storage_uid (Media namespace display).
	StorageNamespace string `json:"storage_namespace,omitempty"`
	CreatedAt        string `json:"created_at,omitempty"`
	UpdatedAt        string `json:"updated_at,omitempty"`
	// Status is the numeric user status (0 = banned). It is never
	// omitted: omitempty would drop the banned zero value.
	Status int16 `json:"status"`
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
		// The principal only exists for active sessions, so the
		// profile-less fallback (AvatarProfile nil in tests) reports
		// active rather than the banned zero value.
		Status: identitydomain.StatusActive,
	}
	if h.deps.AvatarProfile != nil {
		profile, err := h.deps.AvatarProfile.GetUserByID(c.Request.Context(), identityPrincipal.UserID())
		if err != nil || profile == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable52)
			c.Abort()
			return
		}
		dto.Avatar = runtimehttp.V1PublicAvatarURLForRow(profile.StorageUID, profile.Avatar)
		dto.StorageNamespace = strings.TrimSpace(profile.StorageUID)
		dto.CreatedAt = v1FormatTime(profile.CreatedAt)
		dto.UpdatedAt = v1FormatTime(profile.UpdatedAt)
		dto.Status = profile.Status
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
	pageSize, ok := v1PageSize(c)
	if !ok {
		return
	}
	offset := 0
	if raw := strings.TrimSpace(c.Query(contract.CursorParam)); raw != "" {
		cursor, decodeErr := contract.DecodeCursor[v1ListCursor](raw)
		if decodeErr != nil || cursor == nil {
			contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, runtimehttp.MsgPaginationCursorInvalid3)
			c.Abort()
			return
		}
		if cursor.Offset < 0 {
			contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, runtimehttp.MsgPaginationCursorInvalid3)
			c.Abort()
			return
		}
		offset = cursor.Offset
	}
	ctx := c.Request.Context()
	workspaces, err := h.deps.Store.ListWorkspacesForUser(ctx, identityPrincipal.UserID())
	if err != nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable38)
		c.Abort()
		return
	}
	// First-list auto-provision hook when the user has no workspace yet.
	if len(workspaces) == 0 {
		if personal, provisionErr := h.deps.Store.EnsurePersonalWorkspace(ctx, identityPrincipal.UserID()); provisionErr == nil && personal != nil {
			workspaces, err = h.deps.Store.ListWorkspacesForUser(ctx, identityPrincipal.UserID())
			if err != nil {
				contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable38)
				c.Abort()
				return
			}
		}
	}
	if offset > len(workspaces) {
		contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, runtimehttp.MsgPaginationCursorInvalid3)
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
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable38)
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
		contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, runtimehttp.MsgIdempotencyKeyRequired2)
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
			runtimehttp.MsgCreateWorkspace, contract.FieldErrors{Fields: map[string]string{"name": "required"}})
		c.Abort()
		return
	}
	var orgID *string
	if body.Data.OrganizationID != nil {
		candidate := strings.TrimSpace(*body.Data.OrganizationID)
		if !contract.IsOrganizationID(candidate) {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				runtimehttp.MsgCreateWorkspace2, contract.FieldErrors{Fields: map[string]string{"organization_id": "invalid"}})
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
			contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationNotFound, runtimehttp.MsgOrganizationNotFound)
			c.Abort()
			return
		}
		if role != tenancy.RoleOwner && role != tenancy.RoleAdmin {
			if !runtimehttp.RequireOrganizationCapability(c, true, false) {
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
			contract.RenderError(c, http.StatusConflict, contract.CodeWorkspaceIdempotencyConflict, runtimehttp.MsgWorkspaceIdempotencyConflict)
			c.Abort()
			return
		}
		if errors.Is(doErr, tenancy.ErrNotFound) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationNotFound, runtimehttp.MsgOrganizationNotFound)
			c.Abort()
			return
		}
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable38)
		c.Abort()
		return
	}
	created, ok := result.(*tenancy.Workspace)
	if !ok || created == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable38)
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
				runtimehttp.MsgCreateWorkspace, contract.FieldErrors{Fields: map[string]string{"name": "required"}})
			c.Abort()
			return
		}
		updated, err := h.deps.Store.UpdateWorkspace(c.Request.Context(), wctx.Workspace.PublicID, name)
		if err != nil {
			contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, runtimehttp.MsgListControlBillingAccounts)
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
			contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, runtimehttp.MsgListControlBillingAccounts)
			c.Abort()
			return
		}
		contract.RenderSuccess(c, toWorkspaceDTO(archived))
	})
}

// RegisterContractV1RoutesOnEngine mounts the additive v1 group on the
// engine: real handlers for the workspace/organization/member lifecycle
// (issue #801), boards + collaboration (#802), uploads + media (#803)
// with the shared kernel PublicContractChain. (Moved here from the
// runtimehttp kernel during the plane split so the kernel stays import-free
// of v1; composition calls it alongside the billing/control mounts.)
func RegisterContractV1RoutesOnEngine(r *gin.Engine, authDeps runtimehttp.AuthDeps, deps ContractV1Deps) {
	v1group := r.Group(runtimehttp.ContractV1GroupPath)
	v1group.Use(runtimehttp.PublicContractChain()...)
	RegisterContractV1Routes(v1group, authDeps, deps)
}
