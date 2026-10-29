package v1

import (
	"errors"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"net/http"
	"strings"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

type v1OrganizationDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	ClosedAt  string `json:"closed_at,omitempty"`
	CreatedAt string `json:"created_at"`
	// Role is the caller's direct organization role, present only with
	// ?expand=role (resolved once by the shared org prelude, never
	// re-fetched per expansion).
	Role string `json:"role,omitempty"`
	// WorkspaceCount is the visible workspace count, present only with
	// ?expand=workspace_counts on the list endpoint. Closed
	// organizations report zero, matching workspace listing visibility.
	WorkspaceCount *int64 `json:"workspace_count,omitempty"`
	// Members is the organization member list with email/avatar display,
	// present only with ?expand=members on the detail endpoint (and only
	// when non-empty; absent reads as empty).
	Members []v1OrganizationMemberDTO `json:"members,omitempty"`
	// Workspaces is the organization workspace list, present only with
	// ?expand=workspaces on the detail endpoint (and only when
	// non-empty; absent reads as empty).
	Workspaces []v1WorkspaceDTO `json:"workspaces,omitempty"`
}

func toOrganizationDTO(o *tenancy.Organization) v1OrganizationDTO {
	dto := v1OrganizationDTO{ID: o.PublicID, Name: o.Name, Status: o.Status(), CreatedAt: v1FormatTime(o.CreatedAt)}
	if o.ClosedAt != nil {
		dto.ClosedAt = v1FormatTime(*o.ClosedAt)
	}
	return dto
}

func (h *contractV1Handler) listOrganizations(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) {
		return
	}
	// ?expand=role,workspace_counts is optional and additive: the default
	// list shape is unchanged. Unknown values render 400 bad_request.
	expand, ok := parseExpandQuery(c, "role", "workspace_counts")
	if !ok {
		return
	}
	organizations, err := h.deps.Store.ListOrganizationsForUser(c.Request.Context(), identityPrincipal.UserID())
	if err != nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable53)
		c.Abort()
		return
	}
	slice, meta, ok := v1OffsetPage(c, organizations, runtimehttp.MsgServiceUnavailable53)
	if !ok {
		return
	}
	// Batch expansions: one role lookup and one count lookup for the whole
	// page, replacing per-organization GetOrganizationRole and
	// ListWorkspacesByOrganizationID round trips.
	roles := map[string]string{}
	counts := map[string]int64{}
	if expand["role"] || expand["workspace_counts"] {
		orgIDs := make([]string, 0, len(slice))
		for _, o := range slice {
			orgIDs = append(orgIDs, o.PublicID)
		}
		if expand["role"] {
			fetched, err := h.deps.Store.ListOrganizationRolesForUser(c.Request.Context(), orgIDs, identityPrincipal.UserID())
			if err != nil {
				contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable53)
				c.Abort()
				return
			}
			roles = fetched
		}
		if expand["workspace_counts"] {
			fetched, err := h.deps.Store.ListOrganizationWorkspaceCounts(c.Request.Context(), orgIDs)
			if err != nil {
				contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable53)
				c.Abort()
				return
			}
			counts = fetched
		}
	}
	page := make([]v1OrganizationDTO, 0, len(slice))
	for _, o := range slice {
		dto := toOrganizationDTO(o)
		if expand["role"] {
			// Membership-backed rows always carry a role; a raced
			// removal reads as empty without failing the list.
			dto.Role = roles[o.PublicID]
		}
		if expand["workspace_counts"] {
			count := counts[o.PublicID]
			dto.WorkspaceCount = &count
		}
		page = append(page, dto)
	}
	contract.RenderPaginated(c, page, meta)
}

type v1CreateOrganizationBody struct {
	Data struct {
		Name string `json:"name"`
	} `json:"data"`
}

func (h *contractV1Handler) createOrganization(c *gin.Context) {
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
	var body v1CreateOrganizationBody
	if bindErr := contract.BindLenientJSON(c, &body); bindErr != nil {
		writeV1InputError(c)
		return
	}
	name := strings.TrimSpace(body.Data.Name)
	if validateErr := tenancy.ValidateName(name); validateErr != nil {
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
			runtimehttp.MsgCreateOrganization, contract.FieldErrors{Fields: map[string]string{"name": "required"}})
		c.Abort()
		return
	}
	ctx := c.Request.Context()
	externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
	scope := contract.IdempotencyScope(externalUser, "organization:create")
	hash := contract.HashIdempotencyRequest("organization:create", name)
	result, _, doErr := h.deps.Idempotency.Do(ctx, scope, identityPrincipal.UserID(), key, hash, func() (any, error) {
		return h.deps.Store.CreateOrganization(ctx, identityPrincipal.UserID(), name)
	})
	if doErr != nil {
		if contract.IsIdempotencyConflict(doErr) {
			contract.RenderError(c, http.StatusConflict, contract.CodeOrganizationIdempotencyConflict, runtimehttp.MsgWorkspaceIdempotencyConflict)
			c.Abort()
			return
		}
		if errors.Is(doErr, tenancy.ErrOrganizationLimitReached) {
			contract.RenderError(c, http.StatusConflict, contract.CodeOrganizationLimitReached, runtimehttp.MsgOrganizationLimitReached)
			c.Abort()
			return
		}
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable53)
		c.Abort()
		return
	}
	created, ok := result.(*tenancy.Organization)
	if !ok || created == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable53)
		c.Abort()
		return
	}
	contract.RenderCreated(c, toOrganizationDTO(created))
}

func (h *contractV1Handler) getOrganization(c *gin.Context) {
	h.withOrganization(c, v1OrganizationOpts{}, func(octx v1OrganizationCtx) {
		// ?expand=members,workspaces,role is optional and additive: the
		// default detail shape is unchanged (one organization read for
		// the whole panel instead of three). Unknown values render 400
		// bad_request. The caller's role reuses the prelude resolution
		// (GetOrganizationRole runs once per request, never per
		// expansion).
		expand, ok := parseExpandQuery(c, "members", "workspaces", "role")
		if !ok {
			return
		}
		dto := toOrganizationDTO(octx.Organization)
		if expand["role"] {
			dto.Role = octx.Role
		}
		if expand["members"] {
			members, err := h.deps.Store.ListOrganizationMembers(c.Request.Context(), octx.Organization.PublicID)
			if err != nil {
				contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationNotFound, runtimehttp.MsgOrganizationNotFound)
				c.Abort()
				return
			}
			memberPage, ok := h.toOrganizationMemberDTOs(c, members)
			if !ok {
				return
			}
			if len(memberPage) > 0 {
				dto.Members = memberPage
			}
		}
		if expand["workspaces"] {
			workspaces, err := h.deps.Store.ListOrganizationWorkspaces(c.Request.Context(), octx.Organization.PublicID)
			if err != nil {
				if errors.Is(err, tenancy.ErrNotFound) {
					contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationNotFound, runtimehttp.MsgOrganizationNotFound)
				} else {
					contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable53)
				}
				c.Abort()
				return
			}
			workspacePage := make([]v1WorkspaceDTO, 0, len(workspaces))
			for _, w := range workspaces {
				workspacePage = append(workspacePage, toWorkspaceDTO(w))
			}
			if len(workspacePage) > 0 {
				dto.Workspaces = workspacePage
			}
		}
		contract.RenderSuccess(c, dto)
	})
}

type v1UpdateOrganizationBody struct {
	Data struct {
		Name string `json:"name"`
	} `json:"data"`
}

func (h *contractV1Handler) updateOrganization(c *gin.Context) {
	h.withOrganization(c, v1OrganizationOpts{Allow: func(role string, _ bool) bool {
		return role == tenancy.RoleOwner || role == tenancy.RoleAdmin
	}}, func(octx v1OrganizationCtx) {
		var body v1UpdateOrganizationBody
		if bindErr := contract.BindLenientJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
			return
		}
		name := strings.TrimSpace(body.Data.Name)
		if validateErr := tenancy.ValidateName(name); validateErr != nil {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				runtimehttp.MsgCreateOrganization, contract.FieldErrors{Fields: map[string]string{"name": "required"}})
			c.Abort()
			return
		}
		updated, err := h.deps.Store.UpdateOrganization(c.Request.Context(), octx.Organization.PublicID, name)
		if err != nil {
			if errors.Is(err, tenancy.ErrOrganizationClosed) {
				renderOrganizationClosureError(c, err)
				return
			}
			contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationNotFound, runtimehttp.MsgOrganizationNotFound)
			c.Abort()
			return
		}
		contract.RenderSuccess(c, toOrganizationDTO(updated))
	})
}

func (h *contractV1Handler) listOrganizationWorkspaces(c *gin.Context) {
	h.withOrganization(c, v1OrganizationOpts{}, func(octx v1OrganizationCtx) {
		workspaces, err := h.deps.Store.ListOrganizationWorkspaces(c.Request.Context(), octx.Organization.PublicID)
		if err != nil {
			if errors.Is(err, tenancy.ErrNotFound) {
				contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationNotFound, runtimehttp.MsgOrganizationNotFound)
			} else {
				contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable53)
			}
			c.Abort()
			return
		}
		page := make([]v1WorkspaceDTO, 0, len(workspaces))
		for _, w := range workspaces {
			page = append(page, toWorkspaceDTO(w))
		}
		contract.RenderSuccess(c, page)
	})
}

type v1CreateOrganizationWorkspaceBody struct {
	Data struct {
		Name string `json:"name"`
	} `json:"data"`
}

func (h *contractV1Handler) createOrganizationWorkspace(c *gin.Context) {
	h.withOrganization(c, v1OrganizationOpts{Allow: func(role string, _ bool) bool {
		return role == tenancy.RoleOwner || role == tenancy.RoleAdmin
	}}, func(octx v1OrganizationCtx) {
		identityPrincipal := octx.Principal
		organization := octx.Organization
		key, err := contract.IdempotencyKeyFromHeader(c)
		if err != nil {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, runtimehttp.MsgIdempotencyKeyRequired2)
			c.Abort()
			return
		}
		var body v1CreateOrganizationWorkspaceBody
		if bindErr := contract.BindLenientJSON(c, &body); bindErr != nil {
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
		ctx := c.Request.Context()
		orgID := organization.PublicID
		externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
		scope := contract.IdempotencyScope(externalUser, "workspace:create")
		hash := contract.HashIdempotencyRequest("workspace:create", name, orgID)
		result, _, doErr := h.deps.Idempotency.Do(ctx, scope, identityPrincipal.UserID(), key, hash, func() (any, error) {
			return h.deps.Store.CreateOrganizationWorkspace(ctx, orgID, identityPrincipal.UserID(), name)
		})
		if doErr != nil {
			if contract.IsIdempotencyConflict(doErr) {
				contract.RenderError(c, http.StatusConflict, contract.CodeWorkspaceIdempotencyConflict, runtimehttp.MsgWorkspaceIdempotencyConflict)
				c.Abort()
				return
			}
			if errors.Is(doErr, tenancy.ErrOrganizationClosed) {
				renderOrganizationClosureError(c, doErr)
				return
			}
			if errors.Is(doErr, tenancy.ErrNotFound) {
				contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationNotFound, runtimehttp.MsgOrganizationNotFound)
			} else {
				contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable38)
			}
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
	})
}
