package adminbranding

import (
	"testing"

	"ideaboard/api/internal/archtest"
)

// platform must stay independent of identity: admin authority enters through
// the injected RequireAdminFunc port, never a direct identity import.
func TestPlatformAdminBrandingDoesNotImportIdentity(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, ".", []string{
		"ideaboard/api/internal/identity",
	})
}
