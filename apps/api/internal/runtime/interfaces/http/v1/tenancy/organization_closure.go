package tenancy

import (
	"errors"
	"net/http"
	"strings"

	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/runtime/interfaces/http/v1/shared"
	domaintenancy "ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

const maxOrganizationCloseReason = 280

func renderOrganizationClosureError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domaintenancy.ErrOrganizationClosed):
		contract.RenderError(c, contract.StatusOf(contract.CodeOrganizationClosed),
			contract.CodeOrganizationClosed, runtimehttp.MsgOrganizationClosed)
	case errors.Is(err, domaintenancy.ErrOrganizationClosureBlocked):
		contract.RenderError(c, contract.StatusOf(contract.CodeOrganizationClosureBlocked),
			contract.CodeOrganizationClosureBlocked, runtimehttp.MsgOrganizationClosureBlocked)
	case errors.Is(err, domaintenancy.ErrForbidden):
		contract.RenderError(c, http.StatusForbidden, contract.CodeOrganizationForbidden, runtimehttp.MsgForbidden)
	case errors.Is(err, domaintenancy.ErrNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeOrganizationNotFound, runtimehttp.MsgOrganizationNotFound)
	default:
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable53)
	}
	c.Abort()
}

type v1CloseOrganizationBody struct {
	Data struct {
		Reason string `json:"reason"`
	} `json:"data"`
}

func (h *Handler) closeOrganization(c *gin.Context) {
	h.withOrganization(c, shared.OrganizationOpts{Allow: func(role string, _ bool) bool {
		return role == domaintenancy.RoleOwner
	}}, func(octx shared.OrganizationCtx) {
		identityPrincipal := octx.Principal
		organization := octx.Organization
		key, ok := shared.RequireIdempotencyKey(c)
		if !ok {
			return
		}
		var body v1CloseOrganizationBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			shared.WriteInputError(c)
			return
		}
		reason := strings.TrimSpace(body.Data.Reason)
		if len([]rune(reason)) > maxOrganizationCloseReason {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				runtimehttp.MsgCreateOrganization, contract.FieldErrors{Fields: map[string]string{"reason": "invalid"}})
			c.Abort()
			return
		}
		ctx := c.Request.Context()
		externalUser := domaintenancy.FormatUserPublicID(identityPrincipal.PublicID())
		scope := contract.IdempotencyScope(externalUser, "organization:close")
		hash := contract.HashIdempotencyRequest("organization:close", organization.PublicID, reason)
		result, _, doErr := h.deps.Idempotency.Do(ctx, scope, identityPrincipal.UserID(), key, hash, func() (any, error) {
			return h.deps.Store.CloseOrganization(ctx, organization.PublicID, identityPrincipal.UserID(), reason)
		})
		if doErr != nil {
			if contract.IsIdempotencyConflict(doErr) {
				contract.RenderError(c, http.StatusConflict, contract.CodeOrganizationIdempotencyConflict, runtimehttp.MsgWorkspaceIdempotencyConflict)
				c.Abort()
				return
			}
			renderOrganizationClosureError(c, doErr)
			return
		}
		closed, ok := result.(*domaintenancy.Organization)
		if !ok || closed == nil {
			renderOrganizationClosureError(c, errors.New("tenancy: organization close type mismatch"))
			return
		}
		contract.RenderSuccess(c, toOrganizationDTO(closed))
	})
}
