package httpapi

// Control platform-settings family (issue #808): runtime settings reads
// and updates behind control:platform.
//
// Handlers delegate to the shared *settings.RuntimeSettingsService seam
// (the same instance serving legacy admin settings); updates carry the
// operator as actor where the seam supports it (outbound policy) and
// always carry the guard-enforced operator reason in the audit record.
// Legacy /api/admin/settings/* routes are untouched.

import (
	"net/http"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/platform/validation"

	"github.com/gin-gonic/gin"
)

func (d *ContractControlDeps) requireControlSettings(c *gin.Context) (*settings.RuntimeSettingsService, bool) {
	if d == nil || d.Settings == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable21)
		return nil, false
	}
	return d.Settings, true
}

func (d *ContractControlDeps) getControlRegistrationSetting(c *gin.Context, _ controlContext) {
	runtimeSettings, ok := d.requireControlSettings(c)
	if !ok {
		return
	}
	setting, err := runtimeSettings.GetRegistration(c.Request.Context())
	if err != nil {
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable22)
		return
	}
	contract.RenderSuccess(c, setting)
}

func (d *ContractControlDeps) updateControlRegistrationSetting(c *gin.Context, _ controlContext) {
	runtimeSettings, ok := d.requireControlSettings(c)
	if !ok {
		return
	}
	var req settings.RegistrationSetting
	if err := contract.BindStrictJSON(c, &req); err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest4)
		return
	}
	updated, err := runtimeSettings.UpdateRegistration(c.Request.Context(), req)
	if err != nil {
		writeControlSettingsError(c, err)
		return
	}
	contract.RenderSuccess(c, updated)
}

func (d *ContractControlDeps) getControlRateLimitSetting(c *gin.Context, _ controlContext) {
	runtimeSettings, ok := d.requireControlSettings(c)
	if !ok {
		return
	}
	setting, err := runtimeSettings.GetRateLimit(c.Request.Context())
	if err != nil {
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable22)
		return
	}
	contract.RenderSuccess(c, setting)
}

func (d *ContractControlDeps) updateControlRateLimitSetting(c *gin.Context, _ controlContext) {
	runtimeSettings, ok := d.requireControlSettings(c)
	if !ok {
		return
	}
	var req settings.RateLimitSetting
	if err := contract.BindStrictJSON(c, &req); err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest4)
		return
	}
	updated, err := runtimeSettings.UpdateRateLimit(c.Request.Context(), req)
	if err != nil {
		writeControlSettingsError(c, err)
		return
	}
	contract.RenderSuccess(c, updated)
}

func (d *ContractControlDeps) getControlBrandingSetting(c *gin.Context, _ controlContext) {
	runtimeSettings, ok := d.requireControlSettings(c)
	if !ok {
		return
	}
	setting, err := runtimeSettings.GetBranding(c.Request.Context())
	if err != nil {
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable22)
		return
	}
	contract.RenderSuccess(c, setting)
}

func (d *ContractControlDeps) updateControlBrandingSetting(c *gin.Context, _ controlContext) {
	runtimeSettings, ok := d.requireControlSettings(c)
	if !ok {
		return
	}
	var req settings.BrandingSetting
	if err := contract.BindStrictJSON(c, &req); err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest4)
		return
	}
	updated, err := runtimeSettings.UpdateBranding(c.Request.Context(), req)
	if err != nil {
		writeControlSettingsError(c, err)
		return
	}
	contract.RenderSuccess(c, updated)
}

func (d *ContractControlDeps) getControlMediaUploadSetting(c *gin.Context, _ controlContext) {
	runtimeSettings, ok := d.requireControlSettings(c)
	if !ok {
		return
	}
	setting, err := runtimeSettings.GetMediaUpload(c.Request.Context())
	if err != nil {
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable22)
		return
	}
	contract.RenderSuccess(c, setting)
}

func (d *ContractControlDeps) updateControlMediaUploadSetting(c *gin.Context, _ controlContext) {
	runtimeSettings, ok := d.requireControlSettings(c)
	if !ok {
		return
	}
	var req settings.MediaUploadSetting
	if err := contract.BindStrictJSON(c, &req); err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest4)
		return
	}
	updated, err := runtimeSettings.UpdateMediaUpload(c.Request.Context(), req)
	if err != nil {
		writeControlSettingsError(c, err)
		return
	}
	contract.RenderSuccess(c, updated)
}

func (d *ContractControlDeps) getControlOutboundPolicy(c *gin.Context, _ controlContext) {
	runtimeSettings, ok := d.requireControlSettings(c)
	if !ok {
		return
	}
	setting, err := runtimeSettings.GetOutboundPolicy(c.Request.Context())
	if err != nil {
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable22)
		return
	}
	contract.RenderSuccess(c, setting)
}

func (d *ContractControlDeps) updateControlOutboundPolicy(c *gin.Context, ctx controlContext) {
	runtimeSettings, ok := d.requireControlSettings(c)
	if !ok {
		return
	}
	var req settings.OutboundPolicySetting
	if err := contract.BindStrictJSON(c, &req); err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest4)
		return
	}
	updated, err := runtimeSettings.UpdateOutboundPolicy(c.Request.Context(), req, ctx.Operator.UserID())
	if err != nil {
		writeControlSettingsError(c, err)
		return
	}
	contract.RenderSuccess(c, updated)
}

func writeControlSettingsError(c *gin.Context, err error) {
	if validation.Is(err) {
		contract.RenderError(c, http.StatusUnprocessableEntity, contract.CodeValidationFailed, msgBadRequest4)
		return
	}
	contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable23)
}
