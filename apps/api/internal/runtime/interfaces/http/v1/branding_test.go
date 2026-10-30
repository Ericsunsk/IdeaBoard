package v1

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/platform/settings"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"

	"github.com/gin-gonic/gin"
)

type brandingTestProvider struct {
	setting settings.BrandingSetting
	err     error
}

func (p brandingTestProvider) GetBranding(context.Context) (settings.BrandingSetting, error) {
	return p.setting, p.err
}

func TestPublicBrandingSettingsV1RouteIsAnonymousAndReturnsBranding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterContractV1Routes(engine.Group(runtimehttp.ContractV1GroupPath), runtimehttp.AuthDeps{}, ContractV1Deps{
		Branding: brandingTestProvider{
			setting: settings.BrandingSetting{
				ProjectName: "Custom Brand",
				LogoURL:     "uploads/site/assets/logo_123.webp",
			},
		},
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, runtimehttp.ContractV1GroupPath+"/settings/branding", nil)
	engine.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"project_name":"Custom Brand"`) || !strings.Contains(body, `"logo_url":"uploads/site/assets/logo_123.webp"`) {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestPublicBrandingSettingsV1RouteFailsClosedWhenProviderFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterContractV1Routes(engine.Group(runtimehttp.ContractV1GroupPath), runtimehttp.AuthDeps{}, ContractV1Deps{
		Branding: brandingTestProvider{
			err: errors.New("db error"),
		},
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, runtimehttp.ContractV1GroupPath+"/settings/branding", nil)
	engine.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestPublicSearchV1RouteFailsClosedWithoutHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterContractV1Routes(engine.Group(runtimehttp.ContractV1GroupPath), runtimehttp.AuthDeps{}, ContractV1Deps{
		PublicSearchHandler: nil,
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, runtimehttp.ContractV1GroupPath+"/search", nil)
	engine.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
