package worker

import (
	"ideaboard/api/internal/archtest"
	"ideaboard/api/internal/runtime/interfaces/worker/providers"
	runtimeports "ideaboard/api/internal/runtime/ports"
	"testing"
)

func TestProviderExecutorImplementsRuntimePort(t *testing.T) {
	var _ runtimeports.GatewayCaller = (*providers.Executor)(nil)
}

// The worker implementation collaborates with Tasking exclusively through
// task, execution and media contracts. River retry/timeout policy comes
// from the Platform-owned riverpolicy module; no worker may import the
// Tasking concrete queue implementation just to read policy (issue #911).
// Persistence, facades and sibling application packages stay forbidden;
// those seams belong to Tasking and Runtime wiring.
var forbiddenWorkerTaskingImports = []string{
	"ideaboard/api/internal/tasking/infrastructure/postgres",
	"ideaboard/api/internal/tasking/application/task",
	"ideaboard/api/internal/tasking/application/submission",
	"ideaboard/api/internal/tasking/application/billing",
	"ideaboard/api/internal/tasking/application/statistics",
	"ideaboard/api/internal/tasking/application/retention",
	"ideaboard/api/internal/tasking/infrastructure/queue",
}

func TestWorkerCollaboratesThroughTaskingPortsOnly(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, ".", forbiddenWorkerTaskingImports)
}

func TestWorkerSubpackagesIsolation(t *testing.T) {
	archtest.AssertNoForbiddenImports(t, "providers", []string{
		"ideaboard/api/internal/runtime/interfaces/worker/river",
		"ideaboard/api/internal/runtime/interfaces/worker/billing",
	})
	archtest.AssertNoForbiddenImports(t, "billing", []string{
		"ideaboard/api/internal/runtime/interfaces/worker/providers",
		"ideaboard/api/internal/runtime/interfaces/worker/river",
		"ideaboard/api/internal/tasking/infrastructure/queue",
	})
	archtest.AssertNoForbiddenImports(t, "visionbilling", []string{
		"ideaboard/api/internal/tasking/infrastructure/queue",
	})
	archtest.AssertNoForbiddenImports(t, "outbox", []string{
		"ideaboard/api/internal/tasking/infrastructure/queue",
	})
	archtest.AssertNoForbiddenImports(t, "river", []string{
		"ideaboard/api/internal/tasking/infrastructure/queue",
	})
}
