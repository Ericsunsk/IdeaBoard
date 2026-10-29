package v1

import (
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/principal"
	identitydomain "ideaboard/api/internal/identity/users/domain"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

type v1UpdateUserBody struct {
	Data struct {
		DisplayName string `json:"display_name"`
	} `json:"data"`
}

// updateUser changes only the authenticated user's display name.
func (h *contractV1Handler) updateUser(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok {
		return
	}
	if h.deps.UserProfile == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable30)
		c.Abort()
		return
	}
	var body v1UpdateUserBody
	if bindErr := contract.BindLenientJSON(c, &body); bindErr != nil {
		writeV1InputError(c)
		return
	}
	displayName := strings.TrimSpace(body.Data.DisplayName)
	if err := identitydomain.ValidateDisplayName(displayName); err != nil {
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
			runtimehttp.MsgUpdateUserDisplayName, contract.FieldErrors{Fields: map[string]string{"display_name": "required"}})
		c.Abort()
		return
	}
	if err := h.deps.UserProfile.UpdateDisplayNameByID(c.Request.Context(), identityPrincipal.UserID(), displayName); err != nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable30)
		c.Abort()
		return
	}
	h.renderUser(c, identityPrincipal, displayName)
}

// renderUser renders the current-principal envelope with an explicit
// display name. GET passes the session name; PATCH passes the freshly
// persisted row value because the session principal predates the write.
// Every other field follows the GET /user projection exactly.
func (h *contractV1Handler) renderUser(c *gin.Context, identityPrincipal principal.Principal, displayName string) {
	dto := v1UserDTO{
		ID:          tenancy.FormatUserPublicID(identityPrincipal.PublicID()),
		DisplayName: displayName,
		Role:        identityPrincipal.Role().String(),
		// The principal only exists for active sessions, so the
		// profile-less fallback (UserProfile nil in tests) reports
		// active rather than the banned zero value.
		Status: identitydomain.StatusActive,
	}
	if h.deps.UserProfile != nil {
		profile, err := h.deps.UserProfile.GetUserByID(c.Request.Context(), identityPrincipal.UserID())
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
	// Default workspace auto-provision hook: idempotent and
	// concurrent-safe; a provision failure never fails the response itself.
	if h.deps.Store != nil {
		if def, err := h.deps.Store.EnsureDefaultWorkspace(c.Request.Context(), identityPrincipal.UserID()); err == nil && def != nil {
			id := def.PublicID
			dto.PersonalWorkspaceID = &id
		}
	}
	contract.RenderSuccess(c, dto)
}
