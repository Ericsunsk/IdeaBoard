package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"ideaboard/api/internal/media/storage/domain"

	"github.com/gin-gonic/gin"
)

type stubStorageProfilesService struct {
	meta     []domain.StorageProviderMeta
	profiles []domain.StorageProfile
}

func (s *stubStorageProfilesService) ListProviderMeta() []domain.StorageProviderMeta {
	return s.meta
}

func (s *stubStorageProfilesService) ListProfiles(ctx context.Context) ([]domain.StorageProfile, error) {
	return s.profiles, nil
}

func (s *stubStorageProfilesService) SaveProfile(ctx context.Context, profile *domain.StorageProfile) (*domain.StorageProfile, error) {
	return profile, nil
}

func (s *stubStorageProfilesService) SaveAndApplyProfile(ctx context.Context, profile *domain.StorageProfile) (*domain.StorageProfile, error) {
	return profile, nil
}

func (s *stubStorageProfilesService) DeleteProfile(ctx context.Context, id string) error {
	return nil
}

func (s *stubStorageProfilesService) ApplyProfile(ctx context.Context, id string) (*domain.StorageProfile, error) {
	return &domain.StorageProfile{ID: id, IsCurrent: true}, nil
}

func (s *stubStorageProfilesService) TestProfile(ctx context.Context, id string) error {
	return nil
}

func (s *stubStorageProfilesService) GetBucketInfo(ctx context.Context, id string) (map[string]interface{}, error) {
	return map[string]interface{}{"status": "ok"}, nil
}

func TestStorageHandlerListProviders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, _ := http.NewRequest(http.MethodGet, "/api/admin/storage/providers", nil)
	c.Request = req

	svc := &stubStorageProfilesService{
		meta: []domain.StorageProviderMeta{{Provider: domain.StorageProviderLocal, Label: "本地存储"}},
	}
	h := NewHandler(svc)
	h.ListProviders(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", w.Code)
	}
}

func TestStorageHandlerListProfiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, _ := http.NewRequest(http.MethodGet, "/api/admin/storage/profiles", nil)
	c.Request = req

	svc := &stubStorageProfilesService{
		profiles: []domain.StorageProfile{{ID: "p1", Name: "Profile 1"}},
	}
	h := NewHandler(svc)
	h.ListProfiles(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", w.Code)
	}
}
