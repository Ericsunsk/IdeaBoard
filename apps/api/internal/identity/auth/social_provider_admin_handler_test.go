package auth

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/identity/principal"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type fakeSocialProviderService struct {
	getCalls          int
	updateCalls       int
	capabilitiesCalls int
	provider          string
	update            SocialProviderSettingsUpdate
	err               error
}

func (f *fakeSocialProviderService) GetSocialProviderSettings(_ context.Context, provider string) (*SocialProviderSettingsSummary, error) {
	f.getCalls++
	f.provider = provider
	if f.err != nil {
		return nil, f.err
	}
	return socialProviderHandlerSummary(), nil
}

func (f *fakeSocialProviderService) UpdateSocialProviderSettings(_ context.Context, provider string, input SocialProviderSettingsUpdate) (*SocialProviderSettingsSummary, error) {
	f.updateCalls++
	f.provider = provider
	f.update = input
	if f.err != nil {
		return nil, f.err
	}
	return socialProviderHandlerSummary(), nil
}

func (f *fakeSocialProviderService) GetSocialProviderCapabilities(context.Context) (*SocialProviderCapabilities, error) {
	f.capabilitiesCalls++
	if f.err != nil {
		return nil, f.err
	}
	return &SocialProviderCapabilities{Social: map[string]bool{"google": true, "discord": true}}, nil
}

func TestSocialProviderAdminHandlerRequiresAdminForEveryOperation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	operations := []struct {
		name   string
		method string
		path   string
		run    func(*SocialProviderAdminHandler, *gin.Context)
	}{
		{name: "get", method: http.MethodGet, path: "/google", run: func(h *SocialProviderAdminHandler, c *gin.Context) { h.Get(c) }},
		{name: "update", method: http.MethodPut, path: "/google", run: func(h *SocialProviderAdminHandler, c *gin.Context) { h.Update(c) }},
		{name: "capabilities", method: http.MethodGet, path: "/capabilities", run: func(h *SocialProviderAdminHandler, c *gin.Context) { h.GetCapabilities(c) }},
	}
	for _, operation := range operations {
		t.Run(operation.name+" unauthenticated", func(t *testing.T) {
			service := &fakeSocialProviderService{}
			status := serveSocialProviderHandler(t, operation.method, operation.path, "", service, operation.run)
			if status != http.StatusUnauthorized || service.getCalls+service.updateCalls+service.capabilitiesCalls != 0 {
				t.Fatalf("status=%d calls=%d", status, service.getCalls+service.updateCalls+service.capabilitiesCalls)
			}
		})
		t.Run(operation.name+" non-admin", func(t *testing.T) {
			service := &fakeSocialProviderService{}
			status := serveSocialProviderHandler(t, operation.method, operation.path, "user", service, operation.run)
			if status != http.StatusForbidden || service.getCalls+service.updateCalls+service.capabilitiesCalls != 0 {
				t.Fatalf("status=%d calls=%d", status, service.getCalls+service.updateCalls+service.capabilitiesCalls)
			}
		})
	}
}

func TestSocialProviderAdminHandlerSuccessAndInputMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeSocialProviderService{}
	getStatus := serveSocialProviderHandler(t, http.MethodGet, "/google", "admin", service, func(h *SocialProviderAdminHandler, c *gin.Context) { h.Get(c) })
	updateStatus := serveSocialProviderHandler(t, http.MethodPut, "/google", "admin", service, func(h *SocialProviderAdminHandler, c *gin.Context) { h.Update(c) })
	capabilitiesStatus := serveSocialProviderHandler(t, http.MethodGet, "/capabilities", "admin", service, func(h *SocialProviderAdminHandler, c *gin.Context) { h.GetCapabilities(c) })
	if getStatus != http.StatusOK || updateStatus != http.StatusOK || capabilitiesStatus != http.StatusOK {
		t.Fatalf("statuses = %d, %d, %d", getStatus, updateStatus, capabilitiesStatus)
	}
	if service.provider != "google" || !service.update.Enabled || service.update.ClientID != "google-client.apps.googleusercontent.com" || service.update.ClientSecret != "secret" {
		t.Fatalf("mapped update = %+v provider=%q", service.update, service.provider)
	}
}

func TestSocialProviderAdminHandlerAcceptsDiscord(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeSocialProviderService{}
	status := serveSocialProviderHandler(t, http.MethodGet, "/discord", "admin", service, func(h *SocialProviderAdminHandler, c *gin.Context) { h.Get(c) })
	if status != http.StatusOK || service.provider != "discord" {
		t.Fatalf("status=%d provider=%q", status, service.provider)
	}
}

func TestSocialProviderAdminHandlerMapsStableUpstreamErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, status := range []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			service := &fakeSocialProviderService{err: &SocialProviderSettingsError{Status: status, Message: "provider error"}}
			got := serveSocialProviderHandler(t, http.MethodGet, "/google", "admin", service, func(h *SocialProviderAdminHandler, c *gin.Context) { h.Get(c) })
			if got != status {
				t.Fatalf("status=%d, want %d", got, status)
			}
		})
	}
	for _, upstreamErr := range []error{
		&SocialProviderSettingsError{Status: http.StatusServiceUnavailable, Message: "unavailable"},
		errors.New("connection refused"),
	} {
		service := &fakeSocialProviderService{err: upstreamErr}
		got := serveSocialProviderHandler(t, http.MethodGet, "/google", "admin", service, func(h *SocialProviderAdminHandler, c *gin.Context) { h.Get(c) })
		if got != http.StatusBadGateway {
			t.Fatalf("status=%d, want 502", got)
		}
	}
}

func TestSocialProviderAdminHandlerRejectsUnsupportedProviderAndMissingService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeSocialProviderService{}
	unsupported := serveSocialProviderHandler(t, http.MethodGet, "/github", "admin", service, func(h *SocialProviderAdminHandler, c *gin.Context) { h.Get(c) })
	if unsupported != http.StatusNotFound || service.getCalls != 0 {
		t.Fatalf("unsupported status=%d calls=%d", unsupported, service.getCalls)
	}
	missing := serveSocialProviderHandler(t, http.MethodGet, "/google", "admin", nil, func(h *SocialProviderAdminHandler, c *gin.Context) { h.Get(c) })
	if missing != http.StatusInternalServerError {
		t.Fatalf("missing service status=%d", missing)
	}
}

func serveSocialProviderHandler(
	t *testing.T,
	method, path, role string,
	service SocialProviderAdminService,
	run func(*SocialProviderAdminHandler, *gin.Context),
) int {
	t.Helper()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "provider", Value: strings.TrimPrefix(path, "/")}}
	body := bytes.NewBufferString(`{"enabled":true,"client_id":"google-client.apps.googleusercontent.com","client_secret":"secret"}`)
	context.Request = httptest.NewRequest(method, path, body)
	context.Request.Header.Set("Content-Type", "application/json")
	if role != "" {
		identityPrincipal, err := principal.New(1, uuid.New(), "auth-admin", "Test Admin", role)
		if err != nil {
			t.Fatal(err)
		}
		if err := principal.Set(context, identityPrincipal); err != nil {
			t.Fatal(err)
		}
	}
	run(NewSocialProviderAdminHandler(service), context)
	return recorder.Code
}

func socialProviderHandlerSummary() *SocialProviderSettingsSummary {
	return &SocialProviderSettingsSummary{
		Provider: "google", Enabled: true, Configured: true, Available: true,
		ClientID: "google-client.apps.googleusercontent.com", ClientSecretConfigured: true,
		Scopes: []string{"openid", "email", "profile"}, ConfigVersion: 1,
		CallbackURL: "https://example.com/api/auth/callback/google",
	}
}
