package v1

import (
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"net/http"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

// Shared workspace prelude for Public Data Plane v1 handlers.
//
// Every fitting workspace-scoped handler used to hand-roll the same
// 4-5-step prelude (principal -> tenancy/store guards -> workspace
// context -> capability). This file owns the ONE canonical
// implementation; handlers only declare how they differ via
// v1WorkspaceOpts. Auth semantics are unchanged: capability checks
// are relocated here, never weakened or reordered relative to the
// workspace lookup.
//
// Handlers with genuinely different preludes (binary plain-status
// paths, SSE streams, user-scoped avatar, org-scoped routes,
// bearer-capability joins, collection reads without a workspace)
// keep hand-rolled preludes and are listed as intentional exceptions
// at the call sites, never force-fit.
//
// Idempotency-Key acquisition and JSON binding order stay in the
// handler body: many routes render domain 404/403/503 checks between
// the capability gate and the key/bind steps, so hoisting either
// into the prelude would change error precedence. Bind-failure
// rendering is still centralized through writeV1InputError, which
// both the generic bind variant and the manual call sites use.

// v1WorkspaceCtx carries the resolved shared prelude for one request.
type v1WorkspaceCtx struct {
	Principal principal.Principal
	Workspace *tenancy.Workspace
	Role      string
}

// v1WorkspaceOpts tunes the shared prelude. Zero value is the common
// shape: tenancy store guard, explicit workspace context, member-only
// capability (hidden workspaces converge to 404 workspace_not_found).
type v1WorkspaceOpts struct {
	// Require guards the tenancy + domain stores (nil-seam fail-closed).
	// It must render and return false when the request is answered.
	// Nil defaults to the handler's tenancy guard (requireStore for the
	// data-plane handler, requireTenancy for the billing handler).
	Require func(c *gin.Context) bool
	// Resolve maps the principal to the explicit workspace context. It
	// must render and return nil on failure. Nil defaults to the
	// handler's standard workspace context (v1WorkspaceContext for the
	// data-plane handler, billingWorkspaceContext for billing).
	Resolve func(c *gin.Context, p principal.Principal) (*tenancy.Workspace, string, bool)
	// Allow maps (role, visible) to the capability `allowed` flag. Nil
	// defaults to member-only (allowed = visible).
	Allow func(role string, visible bool) bool
}

// v1AllowOwnerOrAdmin restricts the capability gate to workspace
// owners/admins (visible-but-forbidden members get 403).
func v1AllowOwnerOrAdmin(role string, visible bool) bool {
	return role == tenancy.RoleOwner || role == tenancy.RoleAdmin
}

// writeV1InputError renders the single frozen bind-failure shape for
// every v1 JSON body (400 bad_request, 请求体非法, details.data =
// invalid). All BindStrictJSON sites call this instead of rendering
// inline, so the shape cannot drift between surfaces.
func writeV1InputError(c *gin.Context) {
	contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
		runtimehttp.MsgRequireEmptyV1BillingActionBody, contract.FieldErrors{Fields: map[string]string{"data": "invalid"}})
	c.Abort()
}

// v1WorkspacePrelude is the ONE shared prelude implementation behind
// both handler types' withWorkspace methods: principal -> store
// guards -> workspace context -> capability -> fn. Every failure
// renders exactly what the hand-rolled preludes rendered, in the
// same order.
func v1WorkspacePrelude(
	c *gin.Context,
	require func(c *gin.Context) bool,
	resolve func(c *gin.Context, p principal.Principal) (*tenancy.Workspace, string, bool),
	allow func(role string, visible bool) bool,
	fn func(v1WorkspaceCtx),
) {
	p, ok := v1RequestPrincipal(c)
	if !ok {
		return
	}
	if !require(c) {
		return
	}
	workspace, role, visible := resolve(c, p)
	if workspace == nil {
		return
	}
	allowed := visible
	if allow != nil {
		allowed = allow(role, visible)
	}
	if !runtimehttp.RequireWorkspaceCapability(c, visible, allowed) {
		return
	}
	fn(v1WorkspaceCtx{Principal: p, Workspace: workspace, Role: role})
}

// withWorkspace runs the shared prelude for data-plane v1 handlers.
func (h *contractV1Handler) withWorkspace(c *gin.Context, opts v1WorkspaceOpts, fn func(v1WorkspaceCtx)) {
	require := opts.Require
	if require == nil {
		require = h.requireStore
	}
	resolve := opts.Resolve
	if resolve == nil {
		resolve = func(c *gin.Context, p principal.Principal) (*tenancy.Workspace, string, bool) {
			return h.v1WorkspaceContext(c, p)
		}
	}
	v1WorkspacePrelude(c, require, resolve, opts.Allow, fn)
}

// withV1WorkspaceBody runs the shared prelude plus JSON binding for
// the common shape where binding directly follows the capability
// gate. It is a free function (not a method) because Go methods
// cannot carry type parameters. Handlers with key/domain checks
// between the gate and the bind step must use withWorkspace plus a
// manual writeV1InputError call in the original position instead,
// so error precedence never changes.
func withV1WorkspaceBody[T any](h *contractV1Handler, c *gin.Context, opts v1WorkspaceOpts, fn func(v1WorkspaceCtx, T)) {
	h.withWorkspace(c, opts, func(wctx v1WorkspaceCtx) {
		var body T
		if err := contract.BindStrictJSON(c, &body); err != nil {
			writeV1InputError(c)
			return
		}
		fn(wctx, body)
	})
}

// withWorkspace runs the shared prelude for workspace billing v1
// handlers (same core, billing tenancy guard + workspace context).
func (h *contractV1BillingHandler) withWorkspace(c *gin.Context, opts v1WorkspaceOpts, fn func(v1WorkspaceCtx)) {
	require := opts.Require
	if require == nil {
		require = h.requireTenancy
	}
	resolve := opts.Resolve
	if resolve == nil {
		resolve = func(c *gin.Context, p principal.Principal) (*tenancy.Workspace, string, bool) {
			return h.billingWorkspaceContext(c, p)
		}
	}
	v1WorkspacePrelude(c, require, resolve, opts.Allow, fn)
}

// requireStoreAndBoards chains the tenancy + boards nil-seam guards
// with the original short-circuit order.
func (h *contractV1Handler) requireStoreAndBoards(c *gin.Context) bool {
	return h.requireStore(c) && h.requireBoardsStore(c)
}

// requireStoreAndLibrary chains the tenancy + library nil-seam guards.
func (h *contractV1Handler) requireStoreAndLibrary(c *gin.Context) bool {
	return h.requireStore(c) && h.requireLibraryStore(c)
}

// requireStoreAndTasks chains the tenancy + tasks nil-seam guards.
func (h *contractV1Handler) requireStoreAndTasks(c *gin.Context) bool {
	return h.requireStore(c) && h.requireTaskStores(c)
}

// requireStoreAndEvents chains the tenancy + event-sink guards.
func (h *contractV1Handler) requireStoreAndEvents(c *gin.Context) bool {
	return h.requireStore(c) && h.requireEventSinks(c)
}

// requireStoreAndMedia chains the tenancy + media nil-seam guards.
func (h *contractV1Handler) requireStoreAndMedia(c *gin.Context) bool {
	return h.requireStore(c) && h.requireMediaStore(c)
}

// resolveTaskWorkspace adapts the task surface context (which
// converges hidden workspaces to 404 task_not_found and needs no
// separate capability gate) to the shared resolve shape. The
// wrapper's capability check on the adapted (true, true) pair is a
// no-op, so behavior is identical.
func (h *contractV1Handler) resolveTaskWorkspace(c *gin.Context, p principal.Principal) (*tenancy.Workspace, string, bool) {
	workspace, ok := h.taskWorkspaceContext(c, p)
	if !ok {
		return nil, "", false
	}
	return workspace, "", true
}

// resolveLibraryWorkspace adapts the library surface context (member
// gate inside, 404 workspace_not_found outside) to the shared resolve
// shape. The wrapper's follow-up capability check on (true, true) is
// a no-op, so behavior is identical.
func (h *contractV1Handler) resolveLibraryWorkspace(c *gin.Context, p principal.Principal) (*tenancy.Workspace, string, bool) {
	workspace, ok := h.libraryWorkspaceContext(c, p)
	if !ok {
		return nil, "", false
	}
	return workspace, "", true
}

// resolveSearchWorkspace adapts the search surface context to the
// shared resolve shape (same no-op follow-up as the library adapter).
func (h *contractV1Handler) resolveSearchWorkspace(c *gin.Context, p principal.Principal) (*tenancy.Workspace, string, bool) {
	workspace, ok := h.searchWorkspaceContext(c, p)
	if !ok {
		return nil, "", false
	}
	return workspace, "", true
}
