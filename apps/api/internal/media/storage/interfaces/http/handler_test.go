package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"ideaboard/api/internal/media/storage/domain"

	"github.com/gin-gonic/gin"
)

type handlerTestService struct {
	providers []domain.StorageProviderMeta
	profiles  []domain.StorageProfile
	listErr   error
}

func (s *handlerTestService) ListProviderMeta() []domain.StorageProviderMeta { return s.providers }
func (s *handlerTestService) ListProfiles(context.Context) ([]domain.StorageProfile, error) {
	return s.profiles, s.listErr
}
func (s *handlerTestService) SaveProfile(context.Context, *domain.StorageProfile) (*domain.StorageProfile, error) {
	return nil, nil
}
func (s *handlerTestService) SaveAndApplyProfile(context.Context, *domain.StorageProfile) (*domain.StorageProfile, error) {
	return nil, nil
}
func (s *handlerTestService) DeleteProfile(context.Context, string) error { return nil }
func (s *handlerTestService) ApplyProfile(context.Context, string) (*domain.StorageProfile, error) {
	return nil, nil
}
func (s *handlerTestService) TestProfile(context.Context, string) error { return nil }
func (s *handlerTestService) GetBucketInfo(context.Context, string) (map[string]interface{}, error) {
	return nil, nil
}

func performHandlerRequest(handler gin.HandlerFunc) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/storage", handler)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/storage", nil))
	return recorder
}

func TestHandlerListProvidersReturnsProviderPayload(t *testing.T) {
	service := &handlerTestService{providers: []domain.StorageProviderMeta{{Provider: domain.StorageProviderLocal, Label: "Local"}}}
	recorder := performHandlerRequest(NewHandler(service).ListProviders)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Data storageProvidersResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data.Providers) != 1 || body.Data.Providers[0].Provider != domain.StorageProviderLocal {
		t.Fatalf("providers = %#v, want local provider", body.Data.Providers)
	}
}

func TestHandlerListProvidersFailsClosedWhenServiceMissing(t *testing.T) {
	recorder := performHandlerRequest(NewHandler(nil).ListProviders)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerListProfilesReturnsProfiles(t *testing.T) {
	service := &handlerTestService{profiles: []domain.StorageProfile{{ID: "profile_1", Name: "Primary"}}}
	recorder := performHandlerRequest(NewHandler(service).ListProfiles)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Data storageProfilesResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data.Profiles) != 1 || body.Data.Profiles[0].ID != "profile_1" {
		t.Fatalf("profiles = %#v, want profile_1", body.Data.Profiles)
	}
}

func TestHandlerListProfilesReturnsUnavailableOnServiceError(t *testing.T) {
	recorder := performHandlerRequest(NewHandler(&handlerTestService{listErr: errors.New("database unavailable")}).ListProfiles)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", recorder.Code, recorder.Body.String())
	}
}
