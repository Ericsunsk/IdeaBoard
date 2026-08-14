package storage

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/identity/users"
	"ideaboard/api/internal/platform/secrets"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"
)

func TestStorageProfileLocatorIsImmutableAfterMediaAssetRegistration(t *testing.T) {
	service, repo := newStorageServiceTest(t)
	ctx := context.Background()
	const profileID = "s3-immutable-locator"
	seed := StorageProfile{
		ID:       profileID,
		Name:     "Immutable Locator",
		Provider: StorageProviderS3Compatible,
		Enabled:  true,
		Config: map[string]interface{}{
			"access_key_id":            "ak",
			storageSecretAccessKeyName: "sk",
			"bucket_name":              "bucket-a",
			"endpoint":                 "s3.example.com",
			"region_id":                "region-a",
			"access_domain":            "https://cdn-a.example.com",
			"force_path_style":         false,
		},
	}
	if _, err := service.SaveProfile(ctx, &seed); err != nil {
		t.Fatalf("seed storage profile: %v", err)
	}
	owner := &users.User{DisplayName: "profile-locator-owner", Status: 1, Role: "user"}
	if err := users.NewUserRepository(repo.db).Create(ctx, owner); err != nil {
		t.Fatalf("create media owner: %v", err)
	}
	if _, err := dbsqlc.New(repo.db).UpsertMediaAsset(ctx, dbsqlc.UpsertMediaAssetParams{
		AssetID:          "asset-profile-locator",
		OwnerUserID:      owner.ID,
		StorageProfileID: profileID,
		StorageKey:       "uploads/canvas/owner/board/content/image.png",
		Url:              "https://cdn-a.example.com/image.png",
		MediaKind:        "image",
		MimeType:         "image/png",
		Status:           "ready",
		Metadata:         []byte(`{}`),
	}); err != nil {
		t.Fatalf("seed media asset: %v", err)
	}

	allowed := seed
	allowed.Name = "Renamed"
	allowed.Config = map[string]interface{}{
		"access_key_id":            "ak-rotated",
		storageSecretAccessKeyName: "sk-rotated",
		"bucket_name":              "bucket-a",
		"endpoint":                 "s3.example.com",
		"region_id":                "region-a",
		"access_domain":            "https://cdn-b.example.com",
		"force_path_style":         false,
	}
	if _, err := service.SaveProfile(ctx, &allowed); err != nil {
		t.Fatalf("safe profile update rejected: %v", err)
	}

	for name, mutate := range map[string]func(*StorageProfile){
		"bucket":     func(profile *StorageProfile) { profile.Config["bucket_name"] = "bucket-b" },
		"endpoint":   func(profile *StorageProfile) { profile.Config["endpoint"] = "other.example.com" },
		"region":     func(profile *StorageProfile) { profile.Config["region_id"] = "region-b" },
		"path style": func(profile *StorageProfile) { profile.Config["force_path_style"] = true },
	} {
		t.Run(name, func(t *testing.T) {
			next := allowed
			next.Config = maps.Clone(allowed.Config)
			mutate(&next)
			if _, err := service.SaveProfile(ctx, &next); !errors.Is(err, ErrStorageProfileLocatorImmutable) {
				t.Fatalf("locator update error = %v, want ErrStorageProfileLocatorImmutable", err)
			}
		})
	}
}

func TestReclamationLoadsSoftDeletedStorageProfileWithSecret(t *testing.T) {
	service, _ := newStorageServiceTest(t)
	ctx := context.Background()
	const profileID = "s3-deleted-reclamation"
	if _, err := service.SaveProfile(ctx, &StorageProfile{
		ID:       profileID,
		Name:     "Deleted Reclamation",
		Provider: StorageProviderS3Compatible,
		Enabled:  true,
		Config: map[string]interface{}{
			"access_key_id":            "ak",
			storageSecretAccessKeyName: "sk-reclaim",
			"bucket_name":              "bucket",
			"endpoint":                 "s3.example.com",
			"access_domain":            "https://cdn.example.com",
			"force_path_style":         false,
		},
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	if err := service.DeleteProfile(ctx, profileID); err != nil {
		t.Fatalf("soft-delete profile: %v", err)
	}

	profile, err := service.loadProfileForReclamation(ctx, profileID)
	if err != nil {
		t.Fatalf("load deleted profile for reclamation: %v", err)
	}
	if profile.ID != profileID || profile.Enabled {
		t.Fatalf("reclamation profile = %#v", profile)
	}
	if got := configString(profile.Config, storageSecretAccessKeyName); got != "sk-reclaim" {
		t.Fatalf("reclamation secret = %q, want stored plaintext secret", got)
	}
}

func newStorageServiceTest(t *testing.T) (*StorageService, *StorageProfileRepository) {
	t.Helper()
	secretstest.EnableEncryption(t)

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
	secretstest.EnableEncryption(t)

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

func TestSaveAndApplyProfileClearsPreviousCurrentProfile(t *testing.T) {
	service, _ := newStorageServiceTest(t)
	ctx := context.Background()

	first, err := service.SaveAndApplyProfile(ctx, &StorageProfile{
		ID:       "s3-current-a",
		Name:     "OSS 当前 A",
		Provider: StorageProviderS3Compatible,
		Enabled:  true,
		Config: map[string]interface{}{
			"access_key_id":     "ak-a",
			"secret_access_key": "sk-a",
			"bucket_name":       "bucket-a",
			"endpoint":          "s3.example.com",
			"access_domain":     "https://bucket-a.s3.example.com",
		},
	})
	if err != nil {
		t.Fatalf("SaveAndApplyProfile first returned error: %v", err)
	}
	second, err := service.SaveAndApplyProfile(ctx, &StorageProfile{
		ID:       "s3-current-b",
		Name:     "OSS 当前 B",
		Provider: StorageProviderS3Compatible,
		Enabled:  true,
		Config: map[string]interface{}{
			"access_key_id":     "ak-b",
			"secret_access_key": "sk-b",
			"bucket_name":       "bucket-b",
			"endpoint":          "s3.example.com",
			"access_domain":     "https://bucket-b.s3.example.com",
		},
	})
	if err != nil {
		t.Fatalf("SaveAndApplyProfile second returned error: %v", err)
	}

	profiles, err := service.ListProfiles(ctx)
	if err != nil {
		t.Fatalf("ListProfiles returned error: %v", err)
	}
	currentIDs := make([]string, 0, 1)
	for _, profile := range profiles {
		if profile.IsCurrent {
			currentIDs = append(currentIDs, profile.ID)
		}
	}
	if len(currentIDs) != 1 || currentIDs[0] != second.ID || first.ID == second.ID {
		t.Fatalf("current profile ids = %#v, want only %q", currentIDs, second.ID)
	}
}

func TestApplyProfileRejectsDisabledProfile(t *testing.T) {
	service, _ := newStorageServiceTest(t)
	ctx := context.Background()

	saved, err := service.SaveProfile(ctx, &StorageProfile{
		ID:       "s3-disabled",
		Name:     "OSS 停用",
		Provider: StorageProviderS3Compatible,
		Enabled:  false,
		Config: map[string]interface{}{
			"access_key_id":     "ak-disabled",
			"secret_access_key": "sk-disabled",
			"bucket_name":       "bucket-disabled",
			"endpoint":          "s3.example.com",
			"access_domain":     "https://bucket-disabled.s3.example.com",
		},
	})
	if err != nil {
		t.Fatalf("SaveProfile returned error: %v", err)
	}

	if _, err := service.ApplyProfile(ctx, saved.ID); err == nil {
		t.Fatal("ApplyProfile should reject disabled profile")
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
	var storedMaterialKind, storedPayload string
	if err := repo.db.QueryRowContext(ctx, `
SELECT secret_material_kind, secret_payload
FROM storage_profile_secret_versions
WHERE profile_id = $1 AND secret_name = $2 AND status = 'active'`,
		"s3-secret-boundary", storageSecretAccessKeyName,
	).Scan(&storedMaterialKind, &storedPayload); err != nil {
		t.Fatalf("load stored secret failed: %v", err)
	}
	if storedMaterialKind != secrets.MaterialKindEncrypted {
		t.Fatalf("stored material kind = %q, want encrypted", storedMaterialKind)
	}
	if storedPayload == rawSecret {
		t.Fatalf("stored secret payload should not equal plaintext")
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

func TestStorageSecretRotationAppendsVersionsAndRetiresPreviousActive(t *testing.T) {
	_, repo := newStorageServiceTest(t)
	ctx := context.Background()

	profile := StorageProfile{
		ID:       "s3-rotation",
		Name:     "Rotation",
		Provider: StorageProviderS3Compatible,
		Enabled:  true,
		Config: map[string]interface{}{
			"access_key_id":            "ak-rotation",
			storageSecretAccessKeyName: "sk-v1",
			"bucket_name":              "bucket-rotation",
			"endpoint":                 "s3.example.com",
			"access_domain":            "https://bucket-rotation.s3.example.com",
		},
	}
	if err := repo.SaveProfile(ctx, profile); err != nil {
		t.Fatalf("SaveProfile v1 returned error: %v", err)
	}
	profile.Config[storageSecretAccessKeyName] = "sk-v2"
	if err := repo.SaveProfile(ctx, profile); err != nil {
		t.Fatalf("SaveProfile v2 returned error: %v", err)
	}

	rows, err := repo.db.QueryContext(ctx, `
SELECT version_number, status
FROM storage_profile_secret_versions
WHERE profile_id = $1 AND secret_name = $2
ORDER BY version_number ASC`,
		profile.ID, storageSecretAccessKeyName)
	if err != nil {
		t.Fatalf("list secret versions failed: %v", err)
	}
	defer rows.Close()

	type versionState struct {
		version int
		status  string
	}
	var states []versionState
	for rows.Next() {
		var state versionState
		if err := rows.Scan(&state.version, &state.status); err != nil {
			t.Fatalf("scan secret version failed: %v", err)
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate secret versions failed: %v", err)
	}
	if len(states) != 2 ||
		states[0] != (versionState{version: 1, status: StorageSecretVersionStatusRetired}) ||
		states[1] != (versionState{version: 2, status: StorageSecretVersionStatusActive}) {
		t.Fatalf("secret version states = %#v, want v1 retired and v2 active", states)
	}

	runtimeProfiles, err := repo.ListProfilesWithSecrets(ctx, false)
	if err != nil {
		t.Fatalf("ListProfilesWithSecrets returned error: %v", err)
	}
	runtimeProfile := mustFindProfileForTest(t, runtimeProfiles, profile.ID)
	if got := configString(runtimeProfile.Config, storageSecretAccessKeyName); got != "sk-v2" {
		t.Fatalf("active runtime secret = %q, want sk-v2", got)
	}
	if len(runtimeProfile.SecretVersions) != 2 {
		t.Fatalf("secret version metadata count = %d, want 2", len(runtimeProfile.SecretVersions))
	}
}

func TestStorageServiceSkipsSecretRotationWhenSecretWasNotExplicitlyProvided(t *testing.T) {
	service, repo := newStorageServiceTest(t)
	ctx := context.Background()
	const profileID = "s3-metadata-only-save"

	if _, err := service.SaveProfile(ctx, &StorageProfile{
		ID:       profileID,
		Name:     "Metadata Only",
		Provider: StorageProviderS3Compatible,
		Enabled:  true,
		Config: map[string]interface{}{
			"access_key_id":            "ak-metadata",
			storageSecretAccessKeyName: "sk-initial",
			"bucket_name":              "bucket-metadata",
			"endpoint":                 "s3.example.com",
			"access_domain":            "https://bucket-metadata.s3.example.com",
		},
	}); err != nil {
		t.Fatalf("SaveProfile seed returned error: %v", err)
	}

	cases := []struct {
		name   string
		config map[string]interface{}
	}{
		{
			name: "omitted secret",
			config: map[string]interface{}{
				"access_key_id": "ak-metadata",
				"bucket_name":   "bucket-metadata-renamed",
				"endpoint":      "s3.example.com",
				"access_domain": "https://bucket-metadata.s3.example.com",
			},
		},
		{
			name: "empty secret",
			config: map[string]interface{}{
				"access_key_id":            "ak-metadata",
				storageSecretAccessKeyName: "",
				"bucket_name":              "bucket-metadata-renamed-again",
				"endpoint":                 "s3.example.com",
				"access_domain":            "https://bucket-metadata.s3.example.com",
			},
		},
		{
			name: "masked secret",
			config: map[string]interface{}{
				"access_key_id":            "ak-metadata",
				storageSecretAccessKeyName: "********",
				"bucket_name":              "bucket-metadata-final",
				"endpoint":                 "s3.example.com",
				"access_domain":            "https://bucket-metadata.s3.example.com",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := service.SaveProfile(ctx, &StorageProfile{
				ID:       profileID,
				Name:     "Metadata Only Updated",
				Provider: StorageProviderS3Compatible,
				Enabled:  true,
				Config:   tc.config,
			}); err != nil {
				t.Fatalf("SaveProfile metadata update returned error: %v", err)
			}
			assertSerialStorageSecretVersionsForTest(t, loadStorageSecretVersionStatesForTest(t, repo, profileID), 1)
		})
	}

	runtimeProfiles, err := repo.ListProfilesWithSecrets(ctx, false)
	if err != nil {
		t.Fatalf("ListProfilesWithSecrets returned error: %v", err)
	}
	runtimeProfile := mustFindProfileForTest(t, runtimeProfiles, profileID)
	if got := configString(runtimeProfile.Config, storageSecretAccessKeyName); got != "sk-initial" {
		t.Fatalf("runtime secret after metadata saves = %q, want sk-initial", got)
	}
}

func TestStorageSecretRotationSkipsEquivalentSecretValue(t *testing.T) {
	_, repo := newStorageServiceTest(t)
	ctx := context.Background()
	const profileID = "s3-equivalent-secret"

	profile := storageSecretRotationProfileForTest(profileID, "sk-same")
	if err := repo.SaveProfile(ctx, profile); err != nil {
		t.Fatalf("SaveProfile seed returned error: %v", err)
	}
	profile.Name = "Equivalent Secret Updated"
	if err := repo.SaveProfile(ctx, profile); err != nil {
		t.Fatalf("SaveProfile equivalent secret returned error: %v", err)
	}

	assertSerialStorageSecretVersionsForTest(t, loadStorageSecretVersionStatesForTest(t, repo, profileID), 1)
}

func TestStorageSecretRotationIntentPreventsStaleMetadataRollback(t *testing.T) {
	_, repo := newStorageServiceTest(t)
	ctx := context.Background()
	const profileID = "s3-stale-metadata"

	if err := repo.SaveProfile(ctx, storageSecretRotationProfileForTest(profileID, "sk-initial")); err != nil {
		t.Fatalf("SaveProfile seed returned error: %v", err)
	}
	if err := repo.SaveProfile(ctx, storageSecretRotationProfileForTest(profileID, "sk-rotated")); err != nil {
		t.Fatalf("SaveProfile rotated returned error: %v", err)
	}
	staleMetadata := storageSecretRotationProfileForTest(profileID, "sk-initial")
	staleMetadata.Name = "Stale Metadata"
	if err := repo.saveProfile(ctx, staleMetadata, storageProfileSaveOptions{rotateSecret: false}); err != nil {
		t.Fatalf("metadata save returned error: %v", err)
	}

	assertSerialStorageSecretVersionsForTest(t, loadStorageSecretVersionStatesForTest(t, repo, profileID), 2)
	runtimeProfiles, err := repo.ListProfilesWithSecrets(ctx, false)
	if err != nil {
		t.Fatalf("ListProfilesWithSecrets returned error: %v", err)
	}
	runtimeProfile := mustFindProfileForTest(t, runtimeProfiles, profileID)
	if got := configString(runtimeProfile.Config, storageSecretAccessKeyName); got != "sk-rotated" {
		t.Fatalf("runtime secret after stale metadata save = %q, want sk-rotated", got)
	}
}

func TestStorageSecretRotationSerializesConcurrentRepositorySavesAndSkipsMatchingFingerprint(t *testing.T) {
	_, repo := newStorageServiceTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	const profileID = "s3-repo-concurrent-rotation"

	if err := repo.SaveProfile(ctx, storageSecretRotationProfileForTest(profileID, "sk-initial")); err != nil {
		t.Fatalf("SaveProfile seed returned error: %v", err)
	}

	const rotations = 12
	start := make(chan struct{})
	errs := make(chan error, rotations)
	var wg sync.WaitGroup
	for i := 0; i < rotations; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			profile := storageSecretRotationProfileForTest(profileID, fmt.Sprintf("sk-repo-%02d", i))
			profile.Name = fmt.Sprintf("Repo Rotation %02d", i)
			errs <- repo.SaveProfile(ctx, profile)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent repository SaveProfile returned error: %v", err)
		}
	}

	assertSerialStorageSecretVersionsForTest(t, loadStorageSecretVersionStatesForTest(t, repo, profileID), rotations+1)

	runtimeProfiles, err := repo.ListProfilesWithSecrets(ctx, false)
	if err != nil {
		t.Fatalf("ListProfilesWithSecrets returned error: %v", err)
	}
	activeSecret := configString(mustFindProfileForTest(t, runtimeProfiles, profileID).Config, storageSecretAccessKeyName)
	if activeSecret == "" {
		t.Fatal("active secret should be available after repository rotations")
	}

	const duplicateSaves = 8
	duplicatesStart := make(chan struct{})
	duplicateErrs := make(chan error, duplicateSaves)
	for i := 0; i < duplicateSaves; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-duplicatesStart
			profile := storageSecretRotationProfileForTest(profileID, activeSecret)
			profile.Name = fmt.Sprintf("Duplicate Fingerprint %02d", i)
			duplicateErrs <- repo.SaveProfile(ctx, profile)
		}()
	}
	close(duplicatesStart)
	wg.Wait()
	close(duplicateErrs)

	for err := range duplicateErrs {
		if err != nil {
			t.Fatalf("concurrent duplicate repository SaveProfile returned error: %v", err)
		}
	}

	assertSerialStorageSecretVersionsForTest(t, loadStorageSecretVersionStatesForTest(t, repo, profileID), rotations+1)
}

func TestStorageServiceRotatesExplicitSecret(t *testing.T) {
	service, repo := newStorageServiceTest(t)
	ctx := context.Background()
	const profileID = "s3-service-rotation-smoke"

	if _, err := service.SaveProfile(ctx, &StorageProfile{
		ID:       profileID,
		Name:     "Service Rotation",
		Provider: StorageProviderS3Compatible,
		Enabled:  true,
		Config: map[string]interface{}{
			"access_key_id":            "ak-service-rotation",
			storageSecretAccessKeyName: "sk-initial",
			"bucket_name":              "bucket-service-rotation",
			"endpoint":                 "s3.example.com",
			"access_domain":            "https://bucket-service-rotation.s3.example.com",
		},
	}); err != nil {
		t.Fatalf("SaveProfile seed returned error: %v", err)
	}

	rotated, err := service.SaveProfile(ctx, &StorageProfile{
		ID:       profileID,
		Name:     "Service Rotation Updated",
		Provider: StorageProviderS3Compatible,
		Enabled:  true,
		Config: map[string]interface{}{
			"access_key_id":            "ak-service-rotation",
			storageSecretAccessKeyName: "sk-service-rotated",
			"bucket_name":              "bucket-service-rotation",
			"endpoint":                 "s3.example.com",
			"access_domain":            "https://bucket-service-rotation.s3.example.com",
		},
	})
	if err != nil {
		t.Fatalf("SaveProfile rotated returned error: %v", err)
	}
	if !rotated.SecretConfigured {
		t.Fatal("rotated service profile should report configured active secret")
	}

	assertSerialStorageSecretVersionsForTest(t, loadStorageSecretVersionStatesForTest(t, repo, profileID), 2)

	profiles, err := service.ListProfiles(ctx)
	if err != nil {
		t.Fatalf("ListProfiles returned error: %v", err)
	}
	profile := mustFindProfileForTest(t, profiles, profileID)
	activeCount := 0
	for _, version := range profile.SecretVersions {
		if version.Status == StorageSecretVersionStatusActive {
			activeCount += 1
		}
	}
	if activeCount != 1 {
		t.Fatalf("service active secret version count = %d, want 1", activeCount)
	}
}

func TestStorageSecretVersionsAllowOnlyOneActivePerProfile(t *testing.T) {
	_, repo := newStorageServiceTest(t)
	ctx := context.Background()
	const profileID = "s3-active-unique"

	if err := repo.SaveProfile(ctx, StorageProfile{
		ID:       profileID,
		Name:     "Active Unique",
		Provider: StorageProviderS3Compatible,
		Enabled:  true,
		Config: map[string]interface{}{
			"access_key_id":            "ak-unique",
			storageSecretAccessKeyName: "sk-unique",
			"bucket_name":              "bucket-unique",
			"endpoint":                 "s3.example.com",
			"access_domain":            "https://bucket-unique.s3.example.com",
		},
	}); err != nil {
		t.Fatalf("SaveProfile returned error: %v", err)
	}

	_, err := repo.db.ExecContext(ctx, `
INSERT INTO storage_profile_secret_versions (
  profile_id, secret_name, version_number, secret_material_kind, secret_payload, fingerprint, status
) VALUES ($1, $2, 99, 'encrypted', 'duplicate-active', 'duplicate-fingerprint', 'active')`,
		profileID, storageSecretAccessKeyName)
	if err == nil {
		t.Fatal("inserting a second active secret version should fail")
	}
	if !strings.Contains(err.Error(), "uq_storage_profile_secret_versions_active") {
		t.Fatalf("duplicate active error = %v, want active unique constraint", err)
	}
}

func TestBuiltinLocalProfileNeverExposesSecretConfig(t *testing.T) {
	service, repo := newStorageServiceTest(t)
	ctx := context.Background()

	if err := repo.SaveProfile(ctx, StorageProfile{
		ID:       builtinLocalProfileID(),
		Name:     "本地存储",
		Provider: StorageProviderLocal,
		Enabled:  true,
		Builtin:  true,
		Config: map[string]interface{}{
			storageSecretAccessKeyName: "should-not-persist",
		},
	}); err != nil {
		t.Fatalf("SaveProfile local returned error: %v", err)
	}

	profiles, err := service.ListProfiles(ctx)
	if err != nil {
		t.Fatalf("ListProfiles returned error: %v", err)
	}
	local := mustFindProfileForTest(t, profiles, builtinLocalProfileID())
	if local.Provider != StorageProviderLocal || !local.Builtin || !local.Enabled {
		t.Fatalf("local profile = %+v, want enabled builtin local", local)
	}
	if local.SecretConfigured {
		t.Fatal("local profile should not report configured secret")
	}
	if got := configString(local.Config, storageSecretAccessKeyName); got != "" {
		t.Fatalf("local profile exposed secret config = %q, want empty", got)
	}
}

func TestListProfilesWithSecretsFailsOnUnsupportedExternalSecretMaterial(t *testing.T) {
	_, repo := newStorageServiceTest(t)
	ctx := context.Background()

	if err := repo.SaveProfile(ctx, StorageProfile{
		ID:       "s3-legacy-secret",
		Name:     "Legacy Secret",
		Provider: StorageProviderS3Compatible,
		Enabled:  true,
		Config: map[string]interface{}{
			"access_key_id":     "ak",
			"secret_access_key": "sk-legacy",
			"bucket_name":       "bucket",
			"endpoint":          "s3.example.com",
			"access_domain":     "https://bucket.s3.example.com",
		},
	}); err != nil {
		t.Fatalf("SaveProfile returned error: %v", err)
	}

	if _, err := repo.db.ExecContext(ctx, `
UPDATE storage_profile_secret_versions
SET secret_material_kind = 'external_ref',
    secret_payload = '',
    secret_ref = 'vault://storage/sk-legacy'
WHERE profile_id = $1 AND secret_name = $2 AND status = 'active'`,
		"s3-legacy-secret", storageSecretAccessKeyName); err != nil {
		t.Fatalf("switch secret kind failed: %v", err)
	}

	if _, err := repo.ListProfilesWithSecrets(ctx, false); err == nil {
		t.Fatal("ListProfilesWithSecrets should fail on unsupported external secret material")
	}
}

func TestListProfilesWithSecretsFailsOnWrongEncryptionKey(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	repo := NewStorageProfileRepository(db)
	ctx := context.Background()

	if err := repo.SaveProfile(ctx, StorageProfile{
		ID:       "s3-wrong-key",
		Name:     "Wrong Key",
		Provider: StorageProviderS3Compatible,
		Enabled:  true,
		Config: map[string]interface{}{
			"access_key_id":     "ak",
			"secret_access_key": "sk-wrong-key",
			"bucket_name":       "bucket",
			"endpoint":          "s3.example.com",
			"access_domain":     "https://bucket.s3.example.com",
		},
	}); err != nil {
		t.Fatalf("SaveProfile returned error: %v", err)
	}

	t.Setenv(secrets.EncryptionKeyEnv, base64.StdEncoding.EncodeToString([]byte(strings.Repeat("2", 32))))
	if err := secrets.LoadEncryptionKey(); err != nil {
		t.Fatalf("LoadEncryptionKey with alternate valid key failed: %v", err)
	}
	if _, err := repo.ListProfilesWithSecrets(ctx, false); err == nil {
		t.Fatal("ListProfilesWithSecrets should fail when ciphertext was encrypted with another key")
	}
}

func TestReadUploadResourceByURLLocalInfersContentType(t *testing.T) {
	service, _ := newStorageServiceTest(t)
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWd)
	})

	uploadPath := filepath.Join("uploads", "canvas", "OWNR1A", "board-1", "content", "image.png")
	if err := os.MkdirAll(filepath.Dir(uploadPath), 0o755); err != nil {
		t.Fatalf("mkdir upload dir: %v", err)
	}
	if err := os.WriteFile(uploadPath, []byte{0x89, 0x50, 0x4E, 0x47}, 0o600); err != nil {
		t.Fatalf("write upload: %v", err)
	}

	obj, err := service.ReadUploadResourceByURL(context.Background(), "/uploads/canvas/OWNR1A/board-1/content/image.png", 1024)
	if err != nil {
		t.Fatalf("ReadUploadResourceByURL returned error: %v", err)
	}
	if obj.ContentType != "image/png" {
		t.Fatalf("ContentType = %q, want image/png", obj.ContentType)
	}
}

func TestStoreMediaRefCopiesInternalUpload(t *testing.T) {
	service, _ := newStorageServiceTest(t)
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWd)
	})

	sourceRef := "/uploads/canvas/OWNR1A/board-1/content/image.png"
	sourcePath := filepath.FromSlash(strings.TrimPrefix(sourceRef, "/"))
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir upload dir: %v", err)
	}
	pngData := tinyPNGBytes(t)
	if err := os.WriteFile(sourcePath, pngData, 0o600); err != nil {
		t.Fatalf("write upload: %v", err)
	}

	stored, err := service.StoreMediaRef(context.Background(), sourceRef, StorageStoreOptions{
		DirPrefix: "tasks/1/prepare-action",
	})
	if err != nil {
		t.Fatalf("StoreMediaRef returned error: %v", err)
	}
	if stored.URL == sourceRef || !strings.HasPrefix(stored.URL, "/uploads/tasks/1/prepare-action/") {
		t.Fatalf("stored URL = %q, want prepared task upload", stored.URL)
	}
	if stored.MIMEType != "image/png" || stored.Width != 1 || stored.Height != 1 {
		t.Fatalf("stored metadata = %#v, want 1x1 image/png", stored)
	}

	copied, err := service.ReadUploadResourceByURL(context.Background(), stored.URL, int64(len(pngData)))
	if err != nil {
		t.Fatalf("read copied upload: %v", err)
	}
	if !bytes.Equal(copied.Data, pngData) {
		t.Fatal("copied upload data differs from source")
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

type storageSecretVersionStateForTest struct {
	version int
	status  string
}

func storageSecretRotationProfileForTest(profileID, secret string) StorageProfile {
	return StorageProfile{
		ID:       profileID,
		Name:     "Rotation",
		Provider: StorageProviderS3Compatible,
		Enabled:  true,
		Config: map[string]interface{}{
			"access_key_id":            "ak-rotation",
			storageSecretAccessKeyName: secret,
			"bucket_name":              "bucket-rotation",
			"endpoint":                 "s3.example.com",
			"access_domain":            "https://bucket-rotation.s3.example.com",
		},
	}
}

func loadStorageSecretVersionStatesForTest(t *testing.T, repo *StorageProfileRepository, profileID string) []storageSecretVersionStateForTest {
	t.Helper()
	rows, err := repo.db.QueryContext(context.Background(), `
SELECT version_number, status
FROM storage_profile_secret_versions
WHERE profile_id = $1 AND secret_name = $2
ORDER BY version_number ASC`,
		profileID, storageSecretAccessKeyName)
	if err != nil {
		t.Fatalf("list secret versions failed: %v", err)
	}
	defer rows.Close()

	var states []storageSecretVersionStateForTest
	for rows.Next() {
		var state storageSecretVersionStateForTest
		if err := rows.Scan(&state.version, &state.status); err != nil {
			t.Fatalf("scan secret version failed: %v", err)
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate secret versions failed: %v", err)
	}
	return states
}

func assertSerialStorageSecretVersionsForTest(t *testing.T, states []storageSecretVersionStateForTest, wantCount int) {
	t.Helper()
	if len(states) != wantCount {
		t.Fatalf("secret version count = %d, want %d; states=%#v", len(states), wantCount, states)
	}
	for i, state := range states {
		wantVersion := i + 1
		if state.version != wantVersion {
			t.Fatalf("secret version at index %d = %d, want %d; states=%#v", i, state.version, wantVersion, states)
		}
		wantStatus := StorageSecretVersionStatusRetired
		if i == len(states)-1 {
			wantStatus = StorageSecretVersionStatusActive
		}
		if state.status != wantStatus {
			t.Fatalf("secret version %d status = %q, want %q; states=%#v", state.version, state.status, wantStatus, states)
		}
	}
}
