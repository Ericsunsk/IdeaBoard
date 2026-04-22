package service

import (
	"backend/internal/model"
	"testing"
)

func TestCanonicalizeStorageProfileMigratesLegacyAliyunProvider(t *testing.T) {
	provider, migrated, changed := canonicalizeStorageProfile(model.StorageProvider("aliyun-oss"), map[string]interface{}{
		"access_key_id":     "test-ak",
		"access_key_secret": "test-sk",
		"bucket_name":       "feng-assets",
		"endpoint":          "oss-cn-shanghai.aliyuncs.com",
		"region_id":         "cn-shanghai",
		"access_domain":     "https://feng-assets.oss-cn-shanghai.aliyuncs.com",
	})

	if provider != model.StorageProviderS3Compatible {
		t.Fatalf("provider = %q, want %q", provider, model.StorageProviderS3Compatible)
	}
	if !changed {
		t.Fatalf("expected legacy aliyun provider migration to report changed=true")
	}
	if got := configString(migrated, "secret_access_key"); got != "test-sk" {
		t.Fatalf("secret_access_key = %q, want %q", got, "test-sk")
	}
	if got := configString(migrated, "access_key_secret"); got != "" {
		t.Fatalf("access_key_secret should be removed after migration, got %q", got)
	}
	if configBool(migrated, "force_path_style") {
		t.Fatalf("force_path_style should default to false")
	}
}

func TestCanonicalizeStorageProfileKeepsCurrentS3CompatibleConfig(t *testing.T) {
	original := map[string]interface{}{
		"access_key_id":      "test-ak",
		"secret_access_key":  "test-sk",
		"bucket_name":        "feng-assets",
		"endpoint":           "s3.us-east-1.amazonaws.com",
		"access_domain":      "https://feng-assets.s3.us-east-1.amazonaws.com",
		"force_path_style":   true,
	}

	provider, migrated, changed := canonicalizeStorageProfile(model.StorageProviderS3Compatible, original)
	if provider != model.StorageProviderS3Compatible {
		t.Fatalf("provider = %q, want %q", provider, model.StorageProviderS3Compatible)
	}
	if changed {
		t.Fatalf("expected canonical s3-compatible profile to remain unchanged")
	}
	if got := configString(migrated, "secret_access_key"); got != "test-sk" {
		t.Fatalf("secret_access_key = %q, want %q", got, "test-sk")
	}
	if !configBool(migrated, "force_path_style") {
		t.Fatalf("expected force_path_style to stay true")
	}
}
