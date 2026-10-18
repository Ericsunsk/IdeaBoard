package v1

import (
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"net/http"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/settings"

	"github.com/gin-gonic/gin"
)

func publicSupportChannelsV1Handler(provider settings.SupportChannelsSettingProvider) gin.HandlerFunc {
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
