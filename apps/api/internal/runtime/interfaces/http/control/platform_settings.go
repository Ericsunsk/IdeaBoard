package control

// Control runtime settings delegate to settings.RuntimeSettingsService.
// Updates carry the acting operator where supported and emit guarded audit records.

import (
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"net/http"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/errorrender"
	"ideaboard/api/internal/platform/settings"

	"github.com/gin-gonic/gin"
)

func (d *ContractControlDeps) requireControlSettings(c *gin.Context) (*settings.RuntimeSettingsService, bool) {
	if d == nil || d.Settings == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable21)
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
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable22)
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
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest4)
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
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable22)
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
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest4)
		return
	}
	updated, err := runtimeSettings.UpdateRateLimit(c.Request.Context(), req)
	if err != nil {
		writeControlSettingsError(c, err)
		return
	}
	contract.RenderSuccess(c, updated)
}

func (d *ContractControlDeps) getControlLoginLockoutSetting(c *gin.Context, _ controlContext) {
	runtimeSettings, ok := d.requireControlSettings(c)
	if !ok {
		return
	}
	setting, err := runtimeSettings.GetLoginLockout(c.Request.Context())
	if err != nil {
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable22)
		return
	}
	contract.RenderSuccess(c, setting)
}

func (d *ContractControlDeps) updateControlLoginLockoutSetting(c *gin.Context, _ controlContext) {
	runtimeSettings, ok := d.requireControlSettings(c)
	if !ok {
		return
	}
	var req settings.LoginLockoutSetting
	if err := contract.BindStrictJSON(c, &req); err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest4)
		return
	}
	updated, err := runtimeSettings.UpdateLoginLockout(c.Request.Context(), req)
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
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable22)
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
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest4)
		return
	}
	updated, err := runtimeSettings.UpdateBranding(c.Request.Context(), req)
	if err != nil {
		writeControlSettingsError(c, err)
		return
	}
	contract.RenderSuccess(c, updated)
}

func (d *ContractControlDeps) getControlSupportChannels(c *gin.Context, _ controlContext) {
	runtimeSettings, ok := d.requireControlSettings(c)
	if !ok {
		return
	}
	setting, err := runtimeSettings.GetSupportChannels(c.Request.Context())
	if err != nil {
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable22)
		return
	}
	contract.RenderSuccess(c, setting)
}

// controlSupportChannelInput keeps request presence semantics (enabled and
// sort_order are required fields) at the HTTP boundary while the normalized
// settings.SupportChannel remains the single domain shape.
type controlSupportChannelInput struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	URL       string `json:"url"`
	Enabled   *bool  `json:"enabled"`
	SortOrder *int   `json:"sort_order"`
}

type controlSupportChannelsRequest struct {
	Channels *[]controlSupportChannelInput `json:"channels"`
}

func (d *ContractControlDeps) updateControlSupportChannels(c *gin.Context, _ controlContext) {
	runtimeSettings, ok := d.requireControlSettings(c)
	if !ok {
		return
	}
	var req controlSupportChannelsRequest
	if err := contract.BindStrictJSON(c, &req); err != nil || req.Channels == nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest4)
		return
	}
	channels := make([]settings.SupportChannel, 0, len(*req.Channels))
	for _, channel := range *req.Channels {
		if channel.Enabled == nil || channel.SortOrder == nil {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest4)
			return
		}
		channels = append(channels, settings.SupportChannel{
			ID: channel.ID, Name: channel.Name, Kind: channel.Kind, URL: channel.URL,
			Enabled: *channel.Enabled, SortOrder: *channel.SortOrder,
		})
	}
	updated, err := runtimeSettings.UpdateSupportChannels(c.Request.Context(), settings.SupportChannelsSetting{Channels: channels})
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
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable22)
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
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest4)
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
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable22)
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
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest4)
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
	if errorrender.IsValidationError(err) {
		contract.RenderError(c, http.StatusUnprocessableEntity, contract.CodeValidationFailed, runtimehttp.MsgBadRequest4)
		return
	}
	contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable23)
}
