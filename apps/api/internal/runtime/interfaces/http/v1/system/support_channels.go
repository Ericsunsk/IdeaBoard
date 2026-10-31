package system

import (
	"net/http"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/settings"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"

	"github.com/gin-gonic/gin"
)

func PublicSupportChannelsV1Handler(provider settings.SupportChannelsSettingProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		if provider == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable21)
			return
		}
		setting, err := provider.GetSupportChannels(c.Request.Context())
		if err != nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable22)
			return
		}
		contract.RenderSuccess(c, settings.EnabledSupportChannels(setting))
	}
}

func publicSupportChannelsV1Handler(provider settings.SupportChannelsSettingProvider) gin.HandlerFunc {
	return PublicSupportChannelsV1Handler(provider)
}
