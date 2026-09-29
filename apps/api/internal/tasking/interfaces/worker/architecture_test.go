package worker

import (
	"testing"

	"ideaboard/api/internal/archtest"
)

// The workspace worker settles ONLY through the WorkspaceTaskWorkerSettler
// seam (billing Capture/Release stays compensating and centralized) and
// reads routing ONLY from the pinned snapshot. It must never import the
// concrete Billing/Credits packages or the runtime composition root that
// wires it. River client types and platform telemetry are the worker's own
// adapter vocabulary and stay allowed.
var forbiddenWorkerImports = []string{
	"ideaboard/api/internal/billing",
	"ideaboard/api/internal/credits",
	"ideaboard/api/internal/runtime",
}

func TestWorkerDoesNotImportConcreteBillingOrWiring(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, ".", forbiddenWorkerImports)
}
