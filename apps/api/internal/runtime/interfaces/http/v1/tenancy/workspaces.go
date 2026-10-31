package tenancy

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/applog"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/runtime/interfaces/http/v1/shared"
	domaintenancy "ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

type v1WorkspaceDTO = shared.WorkspaceDTO

var toWorkspaceDTO = shared.ToWorkspaceDTO
var withWorkspaceBillingAccount = shared.WithWorkspaceBillingAccount

func (h *Handler) listWorkspaces(c *gin.Context) {
	identityPrincipal, ok := shared.RequestPrincipal(c)
	if !ok || !h.requireStore(c) {
		return
	}
	ctx := c.Request.Context()
	workspaces, err := h.deps.Store.ListWorkspacesForUser(ctx, identityPrincipal.UserID())
	if err != nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable38)
		c.Abort()
		return
	}
	// Default auto-provision hook when the user holds no Default yet.
	hasDefault := false
	for _, w := range workspaces {
		if w.IsDefaultWorkspace() {
			hasDefault = true
			break
		}
	}
	if !hasDefault {
		if def, provisionErr := h.deps.Store.EnsureDefaultWorkspace(ctx, identityPrincipal.UserID()); provisionErr == nil && def != nil {
			workspaces, err = h.deps.Store.ListWorkspacesForUser(ctx, identityPrincipal.UserID())
			if err != nil {
				contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable38)
				c.Abort()
				return
			}
		}
	}
	slice, meta, ok := shared.OffsetPage(c, workspaces, runtimehttp.MsgServiceUnavailable38)
	if !ok {
		return
	}
	page := make([]v1WorkspaceDTO, 0, len(slice))
	for _, w := range slice {
		page = append(page, toWorkspaceDTO(w))
	}
	contract.RenderPaginated(c, page, meta)
}

type v1CreateWorkspaceBody struct {
	Data struct {
		Name           string  `json:"name"`
		OrganizationID *string `json:"organization_id"`
	} `json:"data"`
}

func (h *Handler) createWorkspace(c *gin.Context) {
	identityPrincipal, ok := shared.RequestPrincipal(c)
	if !ok || !h.requireStore(c) {
		return
	}
	key, ok := shared.RequireIdempotencyKey(c)
	if !ok {
		return
	}
	var body v1CreateWorkspaceBody
	if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
		shared.WriteInputError(c)
		return
	}
	name := strings.TrimSpace(body.Data.Name)
	if validateErr := domaintenancy.ValidateName(name); validateErr != nil {
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
	if orgID != nil {
		_, role, orgErr := h.orgRoleForCreate(ctx, *orgID, identityPrincipal.UserID())
		if orgErr != nil {
			contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationNotFound, runtimehttp.MsgOrganizationNotFound)
			c.Abort()
			return
		}
		if role != domaintenancy.RoleOwner && role != domaintenancy.RoleAdmin {
			if !runtimehttp.RequireOrganizationCapability(c, true, false) {
				return
			}
		}
	}
	externalUser := domaintenancy.FormatUserPublicID(identityPrincipal.PublicID())
	orgPart := ""
	if orgID != nil {
		orgPart = *orgID
	}
	scope := contract.IdempotencyScope(externalUser, "workspace:create")
	hash := contract.HashIdempotencyRequest("workspace:create", name, orgPart)
	result, _, doErr := h.deps.Idempotency.Do(ctx, scope, identityPrincipal.UserID(), key, hash, func() (any, error) {
		params := domaintenancy.CreateWorkspaceParams{Name: name, OrganizationID: orgID, OwnerUserID: identityPrincipal.UserID()}
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
		if errors.Is(doErr, domaintenancy.ErrDefaultWorkspaceConflict) {
			contract.RenderError(c, contract.StatusOf(contract.CodeWorkspaceDefaultConflict), contract.CodeWorkspaceDefaultConflict, runtimehttp.MsgWorkspaceDefaultConflict)
			c.Abort()
			return
		}
		if errors.Is(doErr, domaintenancy.ErrNotFound) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationNotFound, runtimehttp.MsgOrganizationNotFound)
			c.Abort()
			return
		}
		if errors.Is(doErr, domaintenancy.ErrBillingForbidden) {
			contract.RenderError(c, http.StatusForbidden, contract.CodeBillingAccountForbidden, runtimehttp.MsgBillingAccountForbidden)
			c.Abort()
			return
		}
		if errors.Is(doErr, domaintenancy.ErrBillingNotFound) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeBillingAccountNotFound, runtimehttp.MsgBillingAccountNotFound)
			c.Abort()
			return
		}
		if errors.Is(doErr, domaintenancy.ErrBillingConflict) || errors.Is(doErr, domaintenancy.ErrBillingAmbiguous) {
			contract.RenderError(c, http.StatusConflict, contract.CodeBillingAccountConflict, runtimehttp.MsgBillingAccountConflict)
			c.Abort()
			return
		}
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable38)
		c.Abort()
		return
	}
	created, ok := result.(*domaintenancy.Workspace)
	if !ok || created == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable38)
		c.Abort()
		return
	}
	dto := toWorkspaceDTO(created)
	if h.deps.BillingAccounts != nil {
		if identity, resErr := h.deps.BillingAccounts.ResolveFunds(ctx, created.PublicID); resErr == nil && identity.BA() != "" {
			dto = withWorkspaceBillingAccount(dto, identity.BA())
		}
	}
	contract.RenderCreated(c, dto)
}

func (h *Handler) orgRoleForCreate(ctx context.Context, orgID string, userID int64) (*domaintenancy.Organization, string, error) {
	organization, err := h.deps.Store.GetOrganization(ctx, orgID)
	if err != nil {
		return nil, "", err
	}
	role, member, err := h.deps.Store.GetOrganizationRole(ctx, orgID, userID)
	if err != nil {
		return nil, "", err
	}
	if !member {
		return organization, "", domaintenancy.ErrForbidden
	}
	return organization, role, nil
}

func (h *Handler) getWorkspace(c *gin.Context) {
	h.withWorkspace(c, shared.WorkspaceOpts{}, func(wctx shared.WorkspaceCtx) {
		contract.RenderSuccess(c, toWorkspaceDTO(wctx.Workspace))
	})
}

type v1UpdateWorkspaceBody struct {
	Data struct {
		Name string `json:"name"`
	} `json:"data"`
}

func (h *Handler) updateWorkspace(c *gin.Context) {
	shared.WithWorkspaceBody(c, h.requireStore, func(c *gin.Context, p principal.Principal) (*domaintenancy.Workspace, string, bool) {
		return shared.ResolveWorkspaceContext(c, h.deps.Store, p)
	}, shared.AllowOwnerOrAdmin, func(wctx shared.WorkspaceCtx, body v1UpdateWorkspaceBody) {
		name := strings.TrimSpace(body.Data.Name)
		if validateErr := domaintenancy.ValidateName(name); validateErr != nil {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				runtimehttp.MsgCreateWorkspace, contract.FieldErrors{Fields: map[string]string{"name": "required"}})
			c.Abort()
			return
		}
		updated, err := h.deps.Store.UpdateWorkspace(c.Request.Context(), wctx.Workspace.PublicID, name)
		if err != nil {
			if errors.Is(err, domaintenancy.ErrDefaultWorkspaceProtected) {
				contract.RenderError(c, contract.StatusOf(contract.CodeWorkspaceDefaultProtected), contract.CodeWorkspaceDefaultProtected, runtimehttp.MsgWorkspaceDefaultProtected)
				c.Abort()
				return
			}
			contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, runtimehttp.MsgListControlBillingAccounts)
			c.Abort()
			return
		}
		contract.RenderSuccess(c, toWorkspaceDTO(updated))
	})
}

func (h *Handler) deleteWorkspace(c *gin.Context) {
	identityPrincipal, ok := shared.RequestPrincipal(c)
	if !ok || !h.requireStore(c) {
		return
	}
	workspaceID := strings.TrimSpace(c.Param("workspace_id"))
	if !contract.IsWorkspaceID(workspaceID) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, runtimehttp.MsgListControlBillingAccounts)
		c.Abort()
		return
	}
	key, ok := shared.RequireIdempotencyKey(c)
	if !ok {
		return
	}
	externalUser := domaintenancy.FormatUserPublicID(identityPrincipal.PublicID())
	hash := contract.HashIdempotencyRequest("workspace:delete", workspaceID)
	result, fnErr, ok := h.workspaceDo(c, externalUser, identityPrincipal.UserID(), "delete", key, hash, func() (any, error) {
		workspace, role, visible := shared.ResolveWorkspaceContext(c, h.deps.Store, identityPrincipal)
		if workspace == nil || !visible {
			return nil, domaintenancy.ErrNotFound
		}
		if !shared.AllowOwnerOrAdmin(role, visible) {
			return nil, domaintenancy.ErrForbidden
		}
		if err := h.deps.Store.DeleteWorkspace(c.Request.Context(), workspaceID); err != nil {
			return nil, err
		}
		return &shared.DeletedDTO{Deleted: true, ID: workspaceID}, nil
	})
	if !ok || c.IsAborted() {
		return
	}
	if fnErr != nil {
		if errors.Is(fnErr, domaintenancy.ErrForbidden) {
			runtimehttp.RequireWorkspaceCapability(c, true, false)
			return
		}
		if errors.Is(fnErr, domaintenancy.ErrWorkspaceActiveTasks) {
			contract.RenderError(c, contract.StatusOf(contract.CodeWorkspaceActiveTasks), contract.CodeWorkspaceActiveTasks, runtimehttp.MsgWorkspaceActiveTasks)
			c.Abort()
			return
		}
		if errors.Is(fnErr, domaintenancy.ErrBillingConflict) || errors.Is(fnErr, domaintenancy.ErrBillingAmbiguous) {
			contract.RenderError(c, contract.StatusOf(contract.CodeBillingAccountConflict), contract.CodeBillingAccountConflict, runtimehttp.MsgBillingAccountConflict)
			c.Abort()
			return
		}
		if errors.Is(fnErr, domaintenancy.ErrDefaultWorkspaceProtected) {
			contract.RenderError(c, contract.StatusOf(contract.CodeWorkspaceDefaultProtected), contract.CodeWorkspaceDefaultProtected, runtimehttp.MsgWorkspaceDefaultProtected)
			c.Abort()
			return
		}
		if errors.Is(fnErr, domaintenancy.ErrNotFound) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceNotFound, runtimehttp.MsgListControlBillingAccounts)
		} else {
			applog.FromContext(c.Request.Context()).Error("workspace deletion failed", "err", fnErr)
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable38)
		}
		c.Abort()
		return
	}
	deleted, ok := result.(*shared.DeletedDTO)
	if !ok || deleted == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable38)
		c.Abort()
		return
	}
	contract.RenderSuccess(c, *deleted)
}
