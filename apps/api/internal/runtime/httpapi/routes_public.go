package httpapi

import (
	"errors"
	"net/http"

	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/searchcatalog"

	"github.com/gin-gonic/gin"
)

type healthResponse struct {
	Status string `json:"status"`
}

func registerPublicRoutes(r *gin.Engine, branding settings.BrandingSettingProvider, searchHandler *searchcatalog.SearchHandler) {
	r.GET(RouteHealth, func(c *gin.Context) {
		response.SuccessDTO(c, healthResponse{Status: "ok"})
	})
	r.GET(RoutePublicSettingsBranding, publicBrandingSettingsHandler(branding))
	if searchHandler != nil {
		r.GET(RoutePublicSearch, searchHandler.Search)
	}
}

func publicBrandingSettingsHandler(provider settings.BrandingSettingProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		if provider == nil {
			response.InternalError(c, http.StatusInternalServerError, "系统设置服务未初始化", errors.New("branding settings provider is not initialized"))
			return
		}
		branding, err := provider.GetBranding(c.Request.Context())
		if err != nil {
			response.InternalError(c, http.StatusInternalServerError, "读取系统设置失败", err)
			return
		}
		response.SuccessDTO(c, branding)
	}
}
