package storage

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"api/internal/platform/settings"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newStorageServiceTest(t *testing.T) (*StorageService, *settings.SystemConfigRepository) {
	t.Helper()

	dbName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:"+dbName+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db failed: %v", err)
	}
	if err := db.AutoMigrate(&settings.SystemConfig{}); err != nil {
		t.Fatalf("auto migrate system configs failed: %v", err)
	}

	repo := settings.NewSystemConfigRepository(db)
	return NewStorageService(repo), repo
}

func writeStorageProfilesConfig(t *testing.T, repo *settings.SystemConfigRepository, profiles []StorageProfile) {
	t.Helper()

	raw, err := json.Marshal(StorageProfilesConfig{Profiles: profiles})
	if err != nil {
		t.Fatalf("marshal profiles failed: %v", err)
	}
	if err := repo.Set(context.Background(), StorageProfilesConfigKey, string(raw)); err != nil {
		t.Fatalf("write profiles config failed: %v", err)
	}
}

func TestListProfilesStartsWithLocalProfile(t *testing.T) {
	service, _ := newStorageServiceTest(t)

	profiles, err := service.ListProfiles(context.Background())
	if err != nil {
		t.Fatalf("ListProfiles returned error: %v", err)
	}

	if len(profiles) != 1 {
		t.Fatalf("profile count = %d, want 1", len(profiles))
	}
	if profiles[0].ID != builtinLocalProfileID() {
		t.Fatalf("profile id = %q, want %q", profiles[0].ID, builtinLocalProfileID())
	}
}

func TestListProfilesDropsUnsupportedCurrentProvider(t *testing.T) {
	service, repo := newStorageServiceTest(t)
	writeStorageProfilesConfig(t, repo, []StorageProfile{
		{
			ID:        builtinLocalProfileID(),
			Name:      "本地存储",
			Provider:  StorageProviderLocal,
			Enabled:   true,
			IsCurrent: false,
			Builtin:   true,
			Config:    map[string]interface{}{},
		},
		{
			ID:        "removed-provider-current",
			Name:      "Removed Provider",
			Provider:  StorageProvider("removed-provider"),
			Enabled:   true,
			IsCurrent: true,
			Builtin:   false,
			Config:    map[string]interface{}{},
		},
	})

	profiles, err := service.ListProfiles(context.Background())
	if err != nil {
		t.Fatalf("ListProfiles returned error: %v", err)
	}

	for _, profile := range profiles {
		if profile.ID == "removed-provider-current" {
			t.Fatalf("unsupported provider profile should be dropped")
		}
		if profile.ID == builtinLocalProfileID() && !profile.IsCurrent {
			t.Fatalf("local profile should become current after unsupported provider is removed")
		}
	}
}

func TestListProfilesDropsUnsupportedNonCurrentProvider(t *testing.T) {
	service, repo := newStorageServiceTest(t)
	writeStorageProfilesConfig(t, repo, []StorageProfile{
		{
			ID:        builtinLocalProfileID(),
			Name:      "本地存储",
			Provider:  StorageProviderLocal,
			Enabled:   true,
			IsCurrent: true,
			Builtin:   true,
			Config:    map[string]interface{}{},
		},
		{
			ID:       "removed-provider-draft",
			Name:     "Removed Provider Draft",
			Provider: StorageProvider("removed-provider"),
			Enabled:  false,
			Builtin:  false,
			Config:   map[string]interface{}{},
		},
	})

	profiles, err := service.ListProfiles(context.Background())
	if err != nil {
		t.Fatalf("ListProfiles returned error: %v", err)
	}

	for _, profile := range profiles {
		if profile.ID == "removed-provider-draft" {
			t.Fatalf("unsupported provider profile should be dropped")
		}
	}
}

func TestCreatedStorageProfileCanBecomeRuntimeActiveProfile(t *testing.T) {
	service, _ := newStorageServiceTest(t)

	saved, err := service.SaveProfile(context.Background(), &StorageProfile{
		Name:     "OSS 存储",
		Provider: StorageProviderS3Compatible,
		Enabled:  true,
		Config: map[string]interface{}{
			"access_key_id":     "ak",
			"secret_access_key": "sk",
			"bucket_name":       "bucket",
			"endpoint":          "oss-cn-shanghai.aliyuncs.com",
			"region_id":         "cn-shanghai",
			"access_domain":     "https://bucket.oss-cn-shanghai.aliyuncs.com",
			"force_path_style":  false,
		},
	})
	if err != nil {
		t.Fatalf("SaveProfile returned error: %v", err)
	}

	if _, err := service.ApplyProfile(context.Background(), saved.ID); err != nil {
		t.Fatalf("ApplyProfile returned error: %v", err)
	}

	active, err := service.resolveActiveProfile(context.Background())
	if err != nil {
		t.Fatalf("resolveActiveProfile returned error: %v", err)
	}
	if active.ID != saved.ID {
		t.Fatalf("active profile id = %q, want %q", active.ID, saved.ID)
	}
	if !service.UsesObjectStorage(context.Background()) {
		t.Fatalf("active object storage profile should affect runtime storage selection")
	}
}

func TestSaveProfileAllowsMultipleProfilesForSameProvider(t *testing.T) {
	service, _ := newStorageServiceTest(t)

	first, err := service.SaveProfile(context.Background(), &StorageProfile{
		ID:       "s3-account-a",
		Name:     "OSS 主账号",
		Provider: StorageProviderS3Compatible,
		Enabled:  true,
		Config: map[string]interface{}{
			"access_key_id":     "ak-1",
			"secret_access_key": "sk-1",
			"bucket_name":       "bucket-a",
			"endpoint":          "oss-cn-shanghai.aliyuncs.com",
			"region_id":         "cn-shanghai",
			"access_domain":     "https://bucket-a.oss-cn-shanghai.aliyuncs.com",
			"force_path_style":  false,
		},
	})
	if err != nil {
		t.Fatalf("SaveProfile first returned error: %v", err)
	}

	second, err := service.SaveProfile(context.Background(), &StorageProfile{
		ID:       "s3-account-b",
		Name:     "OSS 备用账号",
		Provider: StorageProviderS3Compatible,
		Enabled:  true,
		Config: map[string]interface{}{
			"access_key_id":     "ak-2",
			"secret_access_key": "sk-2",
			"bucket_name":       "bucket-b",
			"endpoint":          "oss-cn-beijing.aliyuncs.com",
			"region_id":         "cn-beijing",
			"access_domain":     "https://bucket-b.oss-cn-beijing.aliyuncs.com",
			"force_path_style":  false,
		},
	})
	if err != nil {
		t.Fatalf("SaveProfile second returned error: %v", err)
	}
	if first.ID == second.ID {
		t.Fatalf("profiles for the same provider should have distinct ids")
	}

	profiles, err := service.ListProfiles(context.Background())
	if err != nil {
		t.Fatalf("ListProfiles returned error: %v", err)
	}
	count := 0
	for _, profile := range profiles {
		if profile.Provider == StorageProviderS3Compatible {
			count += 1
		}
	}
	if count != 2 {
		t.Fatalf("s3-compatible profile count = %d, want 2", count)
	}
}
