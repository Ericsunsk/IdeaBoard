package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/platform/settings"

	"github.com/gin-gonic/gin"
)

type supportChannelsTestProvider struct {
	setting settings.SupportChannelsSetting
}

func (p supportChannelsTestProvider) GetSupportChannels(context.Context) (settings.SupportChannelsSetting, error) {
	return p.setting, nil
}

func TestPublicSupportChannelsRouteIsAnonymousAndFiltersDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterContractV1Routes(engine.Group(ContractV1GroupPath), AuthDeps{}, ContractV1Deps{SupportChannels: supportChannelsTestProvider{
		setting: settings.SupportChannelsSetting{Channels: []settings.SupportChannel{
			{ID: "live", Name: "Discord", Kind: "discord", URL: "https://discord.gg/example", Enabled: true},
			{ID: "hidden", Name: "Telegram", Kind: "telegram", URL: "https://t.me/example", Enabled: false},
		}},
	}})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, ContractV1GroupPath+"/support-channels", nil)
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"live"`) || strings.Contains(response.Body.String(), `"id":"hidden"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
