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

// maxOrganizationCloseReason bounds the optional closure reason (mirrors
// the organization_closures reason CHECK).
const maxOrganizationCloseReason = 280

func renderOrganizationClosureError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, tenancy.ErrOrganizationClosed):
		contract.RenderError(c, contract.StatusOf(contract.CodeOrganizationClosed),
			contract.CodeOrganizationClosed, runtimehttp.MsgOrganizationClosed)
	case errors.Is(err, tenancy.ErrOrganizationClosureBlocked):
		contract.RenderError(c, contract.StatusOf(contract.CodeOrganizationClosureBlocked),
			contract.CodeOrganizationClosureBlocked, runtimehttp.MsgOrganizationClosureBlocked)
	case errors.Is(err, tenancy.ErrForbidden):
		contract.RenderError(c, http.StatusForbidden, contract.CodeOrganizationForbidden, runtimehttp.MsgForbidden)
	case errors.Is(err, tenancy.ErrNotFound):
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

// closeOrganization permanently closes a live Organization (issue #955). Only the
// Owner initiates; closure is refused while in-flight Tasks, payment
// orders, refunds, disputes, or a pending handover invite settle.
// Requires Idempotency-Key.
func (h *contractV1Handler) closeOrganization(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) {
		return
	}
	organization, role, visible := h.v1OrganizationContext(c, identityPrincipal)
	if organization == nil {
		return
	}
	if !runtimehttp.RequireOrganizationCapability(c, visible, role == tenancy.RoleOwner) {
		return
	}
	key, err := contract.IdempotencyKeyFromHeader(c)
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, runtimehttp.MsgIdempotencyKeyRequired2)
		c.Abort()
		return
	}
	var body v1CloseOrganizationBody
	if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
		writeV1InputError(c)
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
	externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
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
	closed, ok := result.(*tenancy.Organization)
	if !ok || closed == nil {
		renderOrganizationClosureError(c, errors.New("tenancy: organization close type mismatch"))
		return
	}
	contract.RenderSuccess(c, toOrganizationDTO(closed))
}
