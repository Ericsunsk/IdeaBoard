package bootstrap

import (
	"strings"
	"testing"

	"ideaboard/api/internal/platform/config"
)

func TestNewAppContainerRejectsMissingPublicOriginsBeforeOpeningResources(t *testing.T) {
	_, err := newAppContainer(&config.Config{})
	if err == nil || !strings.Contains(err.Error(), "PUBLIC_ORIGIN is required") {
		t.Fatalf("newAppContainer() error = %v, want missing public origin", err)
	}
}
