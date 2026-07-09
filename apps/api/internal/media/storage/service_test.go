package storage

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestListProviderMetaUsesEmptyFieldSlices(t *testing.T) {
	service := NewStorageService(nil, nil)

	providers := service.ListProviderMeta()
	for _, provider := range providers {
		if provider.Fields == nil {
			t.Fatalf("provider %q fields = nil, want empty slice", provider.Provider)
		}
	}

	payload, err := json.Marshal(providers)
	if err != nil {
		t.Fatalf("marshal provider metadata failed: %v", err)
	}
	if strings.Contains(string(payload), `"fields":null`) {
		t.Fatalf("provider metadata JSON contains null fields: %s", payload)
	}
}

func TestCanonicalizeStorageProfileKeepsCurrentS3CompatibleConfig(t *testing.T) {
	original := map[string]interface{}{
		"access_key_id":     "test-ak",
		"secret_access_key": "test-sk",
		"bucket_name":       "feng-assets",
		"endpoint":          "s3.us-east-1.amazonaws.com",
		"access_domain":     "https://feng-assets.s3.us-east-1.amazonaws.com",
		"force_path_style":  true,
	}

	provider, migrated, changed := canonicalizeStorageProfile(StorageProviderS3Compatible, original)
	if provider != StorageProviderS3Compatible {
		t.Fatalf("provider = %q, want %q", provider, StorageProviderS3Compatible)
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
