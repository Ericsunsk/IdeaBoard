package httpapi

// Control OAuth-provider family: social login channel reads and updates
// behind control:users.
//
// Handlers delegate to the Auth-owned social-provider seam
// (ControlOAuthService, the same *auth.ServiceClient instance serving the
// legacy admin OAuth settings); the control guard adds capability auth,
// the separate rate-limit bucket, header-only idempotency, the operator
// reason, and the mandatory audit record. Client secrets never cross this
// boundary (only client_secret_configured); provider allowlisting stays
// inside Auth. Legacy /api/admin/users/oauth-providers/* routes are
// untouched.

import (
	"context"
	"errors"
	"net/http"

	"ideaboard/api/internal/identity/auth"
	"ideaboard/api/internal/platform/contract"

	"github.com/gin-gonic/gin"
)

// ControlOAuthService is the Auth-owned social-provider seam behind the
// control OAuth handlers. *auth.ServiceClient satisfies it; tests
// substitute a fake.
type ControlOAuthService interface {
	GetSocialProviderSettings(ctx context.Context, provider string) (*auth.SocialProviderSettingsSummary, error)
	UpdateSocialProviderSettings(ctx context.Context, provider string, input auth.SocialProviderSettingsUpdate) (*auth.SocialProviderSettingsSummary, error)
	GetSocialProviderCapabilities(ctx context.Context) (*auth.SocialProviderCapabilities, error)
}

func (d *ContractControlDeps) requireControlOAuth(c *gin.Context) (ControlOAuthService, bool) {
	if d == nil || d.OAuth == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable12)
		return nil, false
	}
	return d.OAuth, true
}

// parseControlOAuthProvider resolves a {provider} path param to the
// allowlisted provider slug. Unknown or malformed slugs fail closed with
// 404 (invisible, never 403), mirroring the legacy admin handler.
func parseControlOAuthProvider(c *gin.Context, svc ControlOAuthService) (string, bool) {
	provider, ok := parseControlProviderSlug(c)
	if !ok {
		return "", false
	}
	capabilities, err := svc.GetSocialProviderCapabilities(c.Request.Context())
	if err != nil {
		writeControlOAuthError(c, err)
		return "", false
	}
	if capabilities != nil {
		if len(capabilities.Providers) > 0 {
			for _, candidate := range capabilities.Providers {
				if candidate.Provider == provider {
					return provider, true
				}
			}
		} else if _, exists := capabilities.Social[provider]; exists {
			return provider, true
		}
	}
	contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, msgControlNotFound6)
	return "", false
}

func (d *ContractControlDeps) getControlOAuthProvider(c *gin.Context, _ controlContext) {
	svc, ok := d.requireControlOAuth(c)
	if !ok {
		return
	}
	provider, ok := parseControlOAuthProvider(c, svc)
	if !ok {
		return
	}
	summary, err := svc.GetSocialProviderSettings(c.Request.Context(), provider)
	if err != nil {
		writeControlOAuthError(c, err)
		return
	}
	contract.RenderSuccess(c, summary)
}

func (d *ContractControlDeps) updateControlOAuthProvider(c *gin.Context, _ controlContext) {
	svc, ok := d.requireControlOAuth(c)
	if !ok {
		return
	}
	provider, ok := parseControlOAuthProvider(c, svc)
	if !ok {
		return
	}
	// reason/operator_note are tolerated so a body-carried operator note
	// survives strict binding; the guard already enforced its presence.
	var req struct {
		Enabled      bool   `json:"enabled"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		Reason       string `json:"reason"`
		OperatorNote string `json:"operator_note"`
	}
	if err := contract.BindStrictJSON(c, &req); err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest4)
		return
	}
	summary, err := svc.UpdateSocialProviderSettings(c.Request.Context(), provider, auth.SocialProviderSettingsUpdate{
		Enabled: req.Enabled, ClientID: req.ClientID, ClientSecret: req.ClientSecret,
	})
	if err != nil {
		writeControlOAuthError(c, err)
		return
	}
	contract.RenderSuccess(c, summary)
}

// writeControlOAuthError maps the Auth social-provider seam failures onto
// the control envelope. Auth validation (400), unknown provider (404),
// and setup conflicts (409) keep their status; everything else fails
// closed as auth unavailable (503) without leaking provider details.
func writeControlOAuthError(c *gin.Context, err error) {
	var providerErr *auth.SocialProviderSettingsError
	if errors.As(err, &providerErr) {
		message := nonEmptyControlMessage(providerErr.Message, msgWriteControlOAuthError)
		switch providerErr.Status {
		case http.StatusBadRequest:
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, message)
			return
		case http.StatusNotFound:
			contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, msgControlNotFound6)
			return
		case http.StatusConflict:
			contract.RenderError(c, http.StatusConflict, contract.CodeConflict, message)
			return
		}
	}
	contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable11)
}
