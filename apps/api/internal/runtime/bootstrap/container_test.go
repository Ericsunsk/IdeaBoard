package bootstrap

import (
	"strings"
	"testing"

	"ideaboard/api/internal/platform/config"
)

func TestNewAppContainerRejectsMissingPublicOriginsBeforeOpeningResources(t *testing.T) {
	_, err := newAppContainer(&config.Config{})
	if err == nil || !strings.Contains(err.Error(), "APP_BASE_URL is required") {
		t.Fatalf("newAppContainer() error = %v, want missing APP_BASE_URL", err)
	}

	_, err = newAppContainer(&config.Config{
		PublicOrigins: config.PublicOriginsConfig{Web: "https://app.example.com"},
	})
	if err == nil || !strings.Contains(err.Error(), "API_BASE_URL is required") {
		t.Fatalf("newAppContainer() error = %v, want missing API_BASE_URL", err)
	}
}
