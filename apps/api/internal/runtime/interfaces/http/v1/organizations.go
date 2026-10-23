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
	pageSize, ok := v1PageSize(c)
	if !ok {
		return
	}
	offset := 0
	if raw := strings.TrimSpace(c.Query(contract.CursorParam)); raw != "" {
		cursor, decodeErr := contract.DecodeCursor[v1ListCursor](raw)
		if decodeErr != nil || cursor == nil || cursor.Offset < 0 {
			contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, runtimehttp.MsgPaginationCursorInvalid3)
			c.Abort()
			return
		}
		offset = cursor.Offset
	}
	organizations, err := h.deps.Store.ListOrganizationsForUser(c.Request.Context(), identityPrincipal.UserID())
	if err != nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable53)
		c.Abort()
		return
	}
	if offset > len(organizations) {
		contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, runtimehttp.MsgPaginationCursorInvalid3)
		c.Abort()
		return
	}
	end := offset + pageSize
	if end > len(organizations) {
		end = len(organizations)
	}
	page := make([]v1OrganizationDTO, 0, end-offset)
	for _, o := range organizations[offset:end] {
		page = append(page, toOrganizationDTO(o))
	}
	meta := contract.NewPageMeta("", false)
	if end < len(organizations) {
		next, encodeErr := contract.EncodeCursor(v1ListCursor{Offset: end})
		if encodeErr != nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable53)
			c.Abort()
			return
		}
		meta = contract.NewPageMeta(next, true)
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
	if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
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
		contract.RenderSuccess(c, toOrganizationDTO(octx.Organization))
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
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
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
