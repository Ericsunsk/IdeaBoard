package storage

import (
	"context"
	"testing"

	"ideaboard/api/internal/testdb"
)

func newStorageServiceTest(t *testing.T) (*StorageService, *StorageProfileRepository) {
	t.Helper()

	db := testdb.Open(t)
	repo := NewStorageProfileRepository(db)
	service := NewStorageService(repo)
	if err := service.EnsureDefaultProfiles(context.Background()); err != nil {
		t.Fatalf("EnsureDefaultProfiles returned error: %v", err)
	}
	return service, repo
}

func newStorageServiceTestWithoutDefault(t *testing.T) (*StorageService, *StorageProfileRepository) {
	t.Helper()

	db := testdb.Open(t)
	repo := NewStorageProfileRepository(db)
	return NewStorageService(repo), repo
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

func TestListProfilesUsesSeededDefaultProfile(t *testing.T) {
	service, repo := newStorageServiceTestWithoutDefault(t)
	ctx := context.Background()

	profiles, err := service.ListProfiles(ctx)
	if err != nil {
		t.Fatalf("ListProfiles returned error: %v", err)
	}
	if len(profiles) != 1 || profiles[0].ID != builtinLocalProfileID() {
		t.Fatalf("profiles = %#v, want seeded builtin local profile", profiles)
	}

	stored, err := repo.ListProfiles(ctx, false)
	if err != nil {
		t.Fatalf("repo ListProfiles returned error: %v", err)
	}
	if len(stored) != 1 || stored[0].ID != builtinLocalProfileID() {
		t.Fatalf("stored profiles = %#v, want seeded builtin local profile", stored)
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

func TestSaveProfileRejectsUnsupportedProvider(t *testing.T) {
	service, _ := newStorageServiceTest(t)

	_, err := service.SaveProfile(context.Background(), &StorageProfile{
		ID:       "unsupported-storage",
		Name:     "Unsupported Storage",
		Provider: StorageProvider("removed-provider"),
		Enabled:  true,
		Config:   map[string]interface{}{},
	})
	if err == nil {
		t.Fatalf("SaveProfile should reject unsupported provider")
	}
}

func TestSaveAndApplyProfileCreatesRuntimeActiveProfile(t *testing.T) {
	service, _ := newStorageServiceTest(t)

	saved, err := service.SaveAndApplyProfile(context.Background(), &StorageProfile{
		Name:     "OSS 当前存储",
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
		t.Fatalf("SaveAndApplyProfile returned error: %v", err)
	}
	if !saved.IsCurrent {
		t.Fatalf("saved profile should be current")
	}

	active, err := service.resolveActiveProfile(context.Background())
	if err != nil {
		t.Fatalf("resolveActiveProfile returned error: %v", err)
	}
	if active.ID != saved.ID {
		t.Fatalf("active profile id = %q, want %q", active.ID, saved.ID)
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

func TestSaveProfileUpdatesOneProfileWithoutDeletingSiblings(t *testing.T) {
	service, _ := newStorageServiceTest(t)

	for _, profile := range []StorageProfile{
		{
			ID:       "s3-main",
			Name:     "主账号",
			Provider: StorageProviderS3Compatible,
			Enabled:  true,
			Config: map[string]interface{}{
				"access_key_id":     "ak-main",
				"secret_access_key": "sk-main",
				"bucket_name":       "bucket-main",
				"endpoint":          "s3.example.com",
				"access_domain":     "https://bucket-main.s3.example.com",
			},
		},
		{
			ID:       "s3-backup",
			Name:     "备用账号",
			Provider: StorageProviderS3Compatible,
			Enabled:  true,
			Config: map[string]interface{}{
				"access_key_id":     "ak-backup",
				"secret_access_key": "sk-backup",
				"bucket_name":       "bucket-backup",
				"endpoint":          "s3.example.com",
				"access_domain":     "https://bucket-backup.s3.example.com",
			},
		},
	} {
		if _, err := service.SaveProfile(context.Background(), &profile); err != nil {
			t.Fatalf("seed profile %q failed: %v", profile.ID, err)
		}
	}

	_, err := service.SaveProfile(context.Background(), &StorageProfile{
		ID:       "s3-main",
		Name:     "主账号更新",
		Provider: StorageProviderS3Compatible,
		Enabled:  true,
		Config: map[string]interface{}{
			"access_key_id":     "ak-main",
			"secret_access_key": "********",
			"bucket_name":       "bucket-main",
			"endpoint":          "s3.example.com",
			"access_domain":     "https://bucket-main.s3.example.com",
		},
	})
	if err != nil {
		t.Fatalf("update profile returned error: %v", err)
	}

	profiles, err := service.ListProfiles(context.Background())
	if err != nil {
		t.Fatalf("ListProfiles returned error: %v", err)
	}
	seen := map[string]string{}
	for _, profile := range profiles {
		if profile.Provider == StorageProviderS3Compatible {
			seen[profile.ID] = profile.Name
		}
	}
	if len(seen) != 2 || seen["s3-main"] != "主账号更新" || seen["s3-backup"] != "备用账号" {
		t.Fatalf("profiles after update = %#v, want both profiles preserved", seen)
	}
}

func TestListProfilesDoesNotExposeSecretByDefault(t *testing.T) {
	_, repo := newStorageServiceTest(t)
	ctx := context.Background()
	const rawSecret = "sk-sensitive-value"

	if err := repo.SaveProfile(ctx, StorageProfile{
		ID:       "s3-secret-boundary",
		Name:     "Secret Boundary",
		Provider: StorageProviderS3Compatible,
		Enabled:  true,
		Config: map[string]interface{}{
			"access_key_id":            "ak-secret",
			storageSecretAccessKeyName: rawSecret,
			"bucket_name":              "bucket-secret",
			"endpoint":                 "s3.example.com",
			"access_domain":            "https://bucket-secret.s3.example.com",
		},
	}); err != nil {
		t.Fatalf("SaveProfile returned error: %v", err)
	}

	profiles, err := repo.ListProfiles(ctx, false)
	if err != nil {
		t.Fatalf("ListProfiles returned error: %v", err)
	}
	profile := mustFindProfileForTest(t, profiles, "s3-secret-boundary")
	if !profile.SecretConfigured {
		t.Fatalf("SecretConfigured = false, want true")
	}
	if got := configString(profile.Config, storageSecretAccessKeyName); got != "" {
		t.Fatalf("default ListProfiles exposed secret = %q, want empty", got)
	}

	runtimeProfiles, err := repo.ListProfilesWithSecrets(ctx, false)
	if err != nil {
		t.Fatalf("ListProfilesWithSecrets returned error: %v", err)
	}
	runtimeProfile := mustFindProfileForTest(t, runtimeProfiles, "s3-secret-boundary")
	if got := configString(runtimeProfile.Config, storageSecretAccessKeyName); got != rawSecret {
		t.Fatalf("ListProfilesWithSecrets secret = %q, want %q", got, rawSecret)
	}
}

func mustFindProfileForTest(t *testing.T, profiles []StorageProfile, id string) StorageProfile {
	t.Helper()
	for _, profile := range profiles {
		if profile.ID == id {
			return profile
		}
	}
	t.Fatalf("profile %q not found", id)
	return StorageProfile{}
}
