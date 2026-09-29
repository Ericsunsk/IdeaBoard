package adapters

import (
	"testing"

	"ideaboard/api/internal/archtest"
)

// The adapters own the concrete Billing/Credits/River/Telemetry bridges.
// They may depend on platform vocabulary, the tasking domain/ports, and
// the billing/credits contexts they adapt — but never on the application
// services, the worker interfaces, or the runtime composition root that
// wires them.
var forbiddenAdapterImports = []string{
	"ideaboard/api/internal/tasking/application",
	"ideaboard/api/internal/tasking/interfaces",
	"ideaboard/api/internal/runtime",
}

func TestAdaptersDoNotDependOnApplicationOrWiring(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, ".", forbiddenAdapterImports)
}
