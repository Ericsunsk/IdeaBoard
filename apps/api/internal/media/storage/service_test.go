package storage

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestListProviderMetaUsesEmptyFieldSlices(t *testing.T) {
	service := NewStorageService(nil)

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

func TestStorageProfileConfigMapKeepsCurrentS3CompatibleConfig(t *testing.T) {
	original := map[string]interface{}{
		"access_key_id":     "test-ak",
		"secret_access_key": "test-sk",
		"bucket_name":       "feng-assets",
		"endpoint":          "s3.us-east-1.amazonaws.com",
		"access_domain":     "https://feng-assets.s3.us-east-1.amazonaws.com",
		"force_path_style":  true,
	}

	migrated := storageProfileConfigMap(StorageProviderS3Compatible, original)
	if !reflect.DeepEqual(migrated, original) {
		t.Fatalf("expected canonical s3-compatible profile to remain unchanged")
	}
	if got := configString(migrated, "secret_access_key"); got != "test-sk" {
		t.Fatalf("secret_access_key = %q, want %q", got, "test-sk")
	}
	if !configBool(migrated, "force_path_style") {
		t.Fatalf("expected force_path_style to stay true")
	}
}
