package v1

import (
	"net/http"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/settings"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	searchhttp "ideaboard/api/internal/searchcatalog/interfaces/http"

	"github.com/gin-gonic/gin"
)

func publicBrandingSettingsV1Handler(provider settings.BrandingSettingProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		if provider == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgPublicBrandingSettingsHandler)
			return
		}
		branding, err := provider.GetBranding(c.Request.Context())
		if err != nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable22)
			return
		}
		contract.RenderSuccess(c, branding)
	}
}

func publicSearchV1Handler(handler *searchhttp.SearchHandler) gin.HandlerFunc {
	return func(c *gin.Context) {
		if handler == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeSearchUnavailable, runtimehttp.MsgSearchProviderUnavailable)
			return
		}
		handler.Search(c)
	}
}
