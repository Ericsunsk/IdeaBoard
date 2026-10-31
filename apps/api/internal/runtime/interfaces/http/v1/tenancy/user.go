package tenancy

import (
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/principal"
	identitydomain "ideaboard/api/internal/identity/users/domain"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/runtime/interfaces/http/v1/shared"
	domaintenancy "ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

type UserDTO struct {
	ID                  string  `json:"id"`
	DisplayName         string  `json:"display_name"`
	Role                string  `json:"role"`
	PersonalWorkspaceID *string `json:"personal_workspace_id,omitempty"`
	Avatar              string  `json:"avatar,omitempty"`
	StorageNamespace    string  `json:"storage_namespace,omitempty"`
	CreatedAt           string  `json:"created_at,omitempty"`
	UpdatedAt           string  `json:"updated_at,omitempty"`
	Status              int16   `json:"status"`
}

type v1UserDTO = UserDTO

type v1UpdateUserBody struct {
	Data struct {
		DisplayName string `json:"display_name"`
	} `json:"data"`
}

func (h *Handler) getUser(c *gin.Context) {
	identityPrincipal, ok := shared.RequestPrincipal(c)
	if !ok {
		return
	}
	h.renderUser(c, identityPrincipal, identityPrincipal.DisplayName())
}

// updateUser changes only the authenticated user's display name.
func (h *Handler) updateUser(c *gin.Context) {
	identityPrincipal, ok := shared.RequestPrincipal(c)
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
		shared.WriteInputError(c)
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

func (h *Handler) renderUser(c *gin.Context, identityPrincipal principal.Principal, displayName string) {
	dto := v1UserDTO{
		ID:          domaintenancy.FormatUserPublicID(identityPrincipal.PublicID()),
		DisplayName: displayName,
		Role:        identityPrincipal.Role().String(),
		Status:      identitydomain.StatusActive,
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
		dto.CreatedAt = shared.FormatTime(profile.CreatedAt)
		dto.UpdatedAt = shared.FormatTime(profile.UpdatedAt)
		dto.Status = profile.Status
	}
	if h.deps.Store != nil {
		if def, err := h.deps.Store.EnsureDefaultWorkspace(c.Request.Context(), identityPrincipal.UserID()); err == nil && def != nil {
			id := def.PublicID
			dto.PersonalWorkspaceID = &id
		}
	}
	contract.RenderSuccess(c, dto)
}
