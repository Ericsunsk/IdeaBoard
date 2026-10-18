package wiring

import (
	"testing"
)

// Base pairing test for imgproxy.go.
func TestImgproxyConfigFromAppNilIsEmpty(t *testing.T) {
	cfg := imgproxyConfigFromApp(nil)
	if cfg.BaseURL != "" || cfg.KeyHex != "" || cfg.SaltHex != "" {
		t.Fatalf("imgproxyConfigFromApp(nil) = %+v, want empty config", cfg)
	}
}

func TestValidateImgproxyOrFatalRequiresConfig(t *testing.T) {
	if err := ValidateImgproxyOrFatal(nil); err == nil {
		t.Fatal("ValidateImgproxyOrFatal(nil) succeeded, want config-required error")
	}
}
