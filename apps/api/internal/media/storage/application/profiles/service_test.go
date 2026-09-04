package profiles

import (
	"context"
	"testing"

	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/ports"
)

type stubRepo struct {
	profiles []domain.StorageProfile
}

func (r *stubRepo) ListProfiles(_ context.Context, _ bool) ([]domain.StorageProfile, error) {
	return r.profiles, nil
}

func (r *stubRepo) ListProfilesWithSecrets(_ context.Context, _ bool) ([]domain.StorageProfile, error) {
	return r.profiles, nil
}

func (r *stubRepo) SaveProfile(_ context.Context, p domain.StorageProfile) error {
	r.profiles = append(r.profiles, p)
	return nil
}

func (r *stubRepo) SaveAndApplyProfile(_ context.Context, p domain.StorageProfile) error {
	for i := range r.profiles {
		r.profiles[i].IsCurrent = false
	}
	p.IsCurrent = true
	r.profiles = append(r.profiles, p)
	return nil
}

func (r *stubRepo) DeleteProfile(_ context.Context, id string) error {
	return nil
}

func (r *stubRepo) ApplyProfile(_ context.Context, id string) error {
	for i := range r.profiles {
		r.profiles[i].IsCurrent = (r.profiles[i].ID == id)
	}
	return nil
}

type stubDriverResolver struct{}

func (d stubDriverResolver) GetDriver(provider domain.StorageProvider) (ports.StorageDriver, error) {
	return nil, nil
}

func TestProfilesServiceDefaultLocal(t *testing.T) {
	repo := &stubRepo{}
	svc := NewService(repo, stubDriverResolver{})

	profiles, err := svc.ListProfiles(context.Background())
	if err != nil {
		t.Fatalf("ListProfiles failed: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 default profile, got %d", len(profiles))
	}
	if profiles[0].ID != "builtin-local" {
		t.Fatalf("profile id = %q, want builtin-local", profiles[0].ID)
	}
}

func TestProfilesServiceMasksSecret(t *testing.T) {
	repo := &stubRepo{
		profiles: []domain.StorageProfile{
			{
				ID:       "s3-test",
				Provider: domain.StorageProviderS3Compatible,
				Name:     "S3 Test",
				Enabled:  true,
				Config: map[string]interface{}{
					"secret_access_key": "my-secret-key-value",
				},
			},
		},
	}
	svc := NewService(repo, stubDriverResolver{})

	profiles, err := svc.ListProfiles(context.Background())
	if err != nil {
		t.Fatalf("ListProfiles failed: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}
	masked := profiles[0].Config["secret_access_key"].(string)
	if masked == "my-secret-key-value" || len(masked) == 0 {
		t.Fatalf("expected masked secret, got %q", masked)
	}
}
